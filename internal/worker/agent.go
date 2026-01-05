// Package worker contains the worker-specific logic for job execution.
package worker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"jobplane/internal/store"
	"jobplane/internal/worker/runtime"
	"jobplane/pkg/api"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// AgentConfig holds configuration for the worker agent.
type AgentConfig struct {
	ID                  string
	Concurrency         int
	PollInterval        time.Duration
	ControllerURL       string
	MaxBackoff          time.Duration // Maximum backoff when queue is empty (default: 30s)
	HeartbeatInterval   time.Duration // Interval between heartbeat calls (default: 2m)
	VisibilityExtension time.Duration // How long to extend visibility on heartbeat (default: 5m)
}

// Agent is the main worker agent that runs the pull-loop for job execution.
type Agent struct {
	queue      store.Queue
	runtime    runtime.Runtime
	config     AgentConfig
	tenantIDs  []uuid.UUID
	httpClient *http.Client
	done       chan struct{}
}

// Internal struct to unwrap payload
type executionPayload struct {
	Job   store.Job              `json:"job"`
	Trace propagation.MapCarrier `json:"trace"`
}

// New creates a new worker agent.
// tenantIDs: Optional. If provided, this worker only pulls jobs for these specific tenants.
func New(q store.Queue, rt runtime.Runtime, config AgentConfig, tenantIDs []uuid.UUID) *Agent {
	if config.Concurrency <= 0 {
		config.Concurrency = 1
	}

	if config.PollInterval <= 0 {
		config.PollInterval = 1 * time.Second
	}

	if config.MaxBackoff <= 0 {
		config.MaxBackoff = 30 * time.Second
	}

	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 2 * time.Minute
	}

	if config.VisibilityExtension <= 0 {
		config.VisibilityExtension = 5 * time.Minute
	}

	// Ensure no trailing slash
	if len(config.ControllerURL) > 0 && config.ControllerURL[len(config.ControllerURL)-1] == '/' {
		config.ControllerURL = config.ControllerURL[:len(config.ControllerURL)-1]
	}

	return &Agent{
		queue:     q,
		runtime:   rt,
		config:    config,
		tenantIDs: tenantIDs,
		done:      make(chan struct{}),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Run starts the main pull-loop. It blocks until the context is cancelled.
// On SIGTERM, it stops dequeuing new work and allows in-flight executions to finish.
func (a *Agent) Run(ctx context.Context) error {
	log.Printf("Agent %s starting with concurrency %d", a.config.ID, a.config.Concurrency)

	// Semaphore to limit concurrency
	sem := make(chan struct{}, a.config.Concurrency)
	var wg sync.WaitGroup

	// Channel to signal when a slot becomes available (adaptive polling)
	pollNow := make(chan struct{}, 1)

	// Current backoff duration (increases on empty queue, resets on work found)
	currentBackoff := a.config.PollInterval

	// Helper to trigger immediate non-blocking re-poll
	triggerPoll := func() {
		select {
		case pollNow <- struct{}{}:
		default:
			// Already a poll pending
		}
	}

	// Initial poll
	triggerPoll()

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, waiting for running jobs to finish...")
			wg.Wait()
			close(a.done)
			return ctx.Err()

		case <-time.After(currentBackoff):
			// Timer-based poll (with backoff)
			triggerPoll()

		case <-pollNow:
			// Count available slots
			availableSlots := a.config.Concurrency - len(sem)
			if availableSlots <= 0 {
				continue
			}

			// Batch dequeue up to available slots
			items, err := a.queue.DequeueBatch(ctx, a.tenantIDs, availableSlots)
			if err != nil {
				log.Printf("DequeueBatch error: %v", err)
				continue
			}

			if len(items) == 0 {
				// Empty queue - increase backoff (exponential, capped at MaxBackoff)
				currentBackoff = currentBackoff * 2
				if currentBackoff > a.config.MaxBackoff {
					currentBackoff = a.config.MaxBackoff
				}
				continue
			}

			// Found work - reset backoff to minimum
			currentBackoff = a.config.PollInterval

			log.Printf("Claimed %d executions", len(items))

			// Dispatch each job to a worker goroutine
			for _, item := range items {
				// Acquire semaphore slot
				sem <- struct{}{}

				wg.Add(1)
				go func(execID uuid.UUID, payload json.RawMessage) {
					defer wg.Done()
					defer func() {
						<-sem
						// Signal that a slot is now available - trigger immediate re-poll
						triggerPoll()
					}()
					a.processItem(ctx, execID, payload)
				}(item.ExecutionID, item.Payload)
			}

			// If we got jobs and there are still slots available, poll again immediately
			if len(items) > 0 && len(items) < availableSlots {
				triggerPoll()
			}
		}
	}
}

// Done returns a channel that is closed when the agent has fully stopped.
func (a *Agent) Done() <-chan struct{} {
	return a.done
}

// processItem processes a single execution that has already been dequeued.
func (a *Agent) processItem(ctx context.Context, execID uuid.UUID, payload json.RawMessage) {
	var wrapper executionPayload
	var traceCtx context.Context

	var jobDef store.Job
	if err := json.Unmarshal(payload, &wrapper); err == nil && wrapper.Job.ID != uuid.Nil {
		jobDef = wrapper.Job

		// Extract Trace Context
		if wrapper.Trace != nil {
			traceCtx = otel.GetTextMapPropagator().Extract(ctx, wrapper.Trace)
		} else {
			traceCtx = ctx
		}
	} else {
		// Fallback: Try unmarshaling directly to Job (old format / backward compatibility)
		if err := json.Unmarshal(payload, &jobDef); err != nil {
			log.Printf("Failed to unmarshal job payload: %v", err)
			a.queue.Fail(ctx, nil, execID, nil, fmt.Sprintf("Invalid payload: %v", err))
			return
		}
		traceCtx = ctx
	}

	// Start Span
	tracer := otel.Tracer("worker-agent")
	spanCtx, span := tracer.Start(traceCtx, "process_job",
		trace.WithAttributes(
			attribute.String("job.id", jobDef.ID.String()),
			attribute.String("execution.id", execID.String()),
			attribute.String("job.name", jobDef.Name),
			attribute.String("job.image", jobDef.Image),
			attribute.String("tenant.id", jobDef.TenantID.String()),
		),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	defer span.End()

	log.Printf("Processing execution %s", execID)

	runtimeOpts := runtime.StartOptions{
		Image:   jobDef.Image,
		Command: jobDef.Command,
		Env: map[string]string{
			"JOBPLANE_EXECUTION_ID": execID.String(),
			"JOBPLANE_JOB_ID":       jobDef.ID.String(),
		},
		Timeout: jobDef.DefaultTimeout,
	}

	// Determine timeout: use job's DefaultTimeout, or fall back to 30 minutes
	timeout := 30 * time.Minute
	if jobDef.DefaultTimeout > 0 {
		timeout = time.Duration(jobDef.DefaultTimeout) * time.Second
	}

	// Create execution context with timeout.
	// This is independent of the worker's poll context - we want the job
	// to complete even if SIGTERM is received (graceful drain).
	execContext, cancel := context.WithTimeout(spanCtx, timeout)
	defer cancel()

	// Start Runtime
	handle, err := a.runtime.Start(execContext, runtimeOpts)
	if err != nil {
		log.Printf("Failed to start runtime for %s: %v", execID, err)
		a.queue.Fail(context.Background(), nil, execID, nil, fmt.Sprintf("Failed to start runtime. %s", err.Error()))
		return
	}

	// Start heartbeat to refresh visibility timeout during execution
	heartbeatCtx, cancelHeartbeat := context.WithCancel(context.Background())
	defer cancelHeartbeat()
	go a.runHeartbeat(heartbeatCtx, execID)

	// Using WaitGroup to track logs
	var wg sync.WaitGroup
	wg.Add(1)

	// streaming logs
	go func() {
		defer wg.Done()
		a.streamLogs(ctx, execID, handle)
	}()

	// Wait for result
	result, err := handle.Wait(execContext)

	// Wait for logs
	wg.Wait()

	if err != nil {
		span.RecordError(err)

		// Check if this was a timeout
		if execContext.Err() == context.DeadlineExceeded {
			log.Printf("Execution %s timed out after %v", execID, timeout)
			// Forcefully stop the container
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer stopCancel()
			handle.Stop(stopCtx)
			a.queue.Fail(context.Background(), nil, execID, nil, fmt.Sprintf("Execution timed out after %v", timeout))
			return
		}

		log.Printf("Runtime waiting error for %s: %v", execID, err)
		a.queue.Fail(context.Background(), nil, execID, nil, fmt.Sprintf("Runtime waiting error: %v", err))
		return
	}

	span.SetAttributes(attribute.Int("exit_code", result.ExitCode))

	// Update Queue
	if result.ExitCode == 0 {
		log.Printf("Execution %s completed successfully", execID)
		a.queue.Complete(context.Background(), nil, execID, 0)
	} else {
		log.Printf("Execution %s failed with code %d", execID, result.ExitCode)
		errorMessage := fmt.Sprintf("Exit code %d", result.ExitCode)
		if result.Error != nil {
			errorMessage = result.Error.Error()
			span.RecordError(result.Error)
		}
		a.queue.Fail(context.Background(), nil, execID, &result.ExitCode, errorMessage)
	}
}

// runHeartbeat refreshes the visibility timeout periodically while a job is executing.
// This prevents long-running jobs from being picked up by another worker.
func (a *Agent) runHeartbeat(ctx context.Context, execID uuid.UUID) {
	ticker := time.NewTicker(a.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Extend visibility timeout
			visibleAfter := time.Now().Add(a.config.VisibilityExtension)
			if err := a.queue.SetVisibleAfter(context.Background(), nil, execID, visibleAfter); err != nil {
				log.Printf("Heartbeat failed for %s: %v", execID, err)
			}
		}
	}
}

func (a *Agent) streamLogs(ctx context.Context, executionID uuid.UUID, handle runtime.Handle) {
	const (
		logBatchSize     = 100         // Max lines per batch
		logFlushInterval = time.Second // Flush at least every second
	)

	rc, err := handle.StreamLogs(ctx)
	if err != nil {
		log.Printf("Failed to get log stream for %s: %v", executionID, err)
		return
	}
	defer rc.Close()

	var batch []string
	flushTicker := time.NewTicker(logFlushInterval)
	defer flushTicker.Stop()

	// Channel for lines from scanner
	lineChan := make(chan string, 100)

	// Scanner goroutine - reads lines and sends to channel
	go func() {
		defer close(lineChan)
		scanner := bufio.NewScanner(rc)
		for scanner.Scan() {
			line := scanner.Text()
			// Sanitize null bytes (Postgres rejects \x00)
			if strings.Contains(line, "\x00") {
				line = strings.ReplaceAll(line, "\x00", "")
			}
			select {
			case lineChan <- line:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Flush helper - sends batch to controller
	flush := func() {
		if len(batch) == 0 {
			return
		}
		content := strings.Join(batch, "\n")
		if err := a.sendLogs(ctx, executionID, content); err != nil {
			log.Printf("Failed to ship logs for %s: %v", executionID, err)
		}
		batch = batch[:0] // Clear batch
	}

	// Main loop: batch lines and flush periodically
	for {
		select {
		case line, ok := <-lineChan:
			if !ok {
				// Scanner finished, flush remaining
				flush()
				return
			}
			batch = append(batch, line)
			if len(batch) >= logBatchSize {
				flush()
			}
		case <-flushTicker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func (a *Agent) sendLogs(ctx context.Context, executionID uuid.UUID, content string) error {
	url := fmt.Sprintf("%s/internal/executions/%s/logs", a.config.ControllerURL, executionID)

	body := api.AddLogRequest{
		Content: content,
	}
	reqBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(string(reqBody)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	return nil
}
