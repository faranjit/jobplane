// Package worker contains the worker-specific logic for job execution.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"jobplane/internal/store"
	"jobplane/internal/worker/runtime"

	"github.com/google/uuid"
)

// AgentConfig holds configuration for the worker agent.
type AgentConfig struct {
	ID           string
	Concurrency  int
	PollInterval time.Duration
}

// Agent is the main worker agent that runs the pull-loop for job execution.
type Agent struct {
	queue     store.Queue
	runtime   runtime.Runtime
	config    AgentConfig
	tenantIDs []uuid.UUID
	done      chan struct{}
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

	return &Agent{
		queue:     q,
		runtime:   rt,
		config:    config,
		tenantIDs: tenantIDs,
		done:      make(chan struct{}),
	}
}

// Run starts the main pull-loop. It blocks until the context is cancelled.
// On SIGTERM, it stops dequeuing new work and allows in-flight executions to finish.
func (a *Agent) Run(ctx context.Context) error {
	log.Printf("Agent %s starting with concurrency %d", a.config.ID, a.config.Concurrency)

	// Semaphore to limit concurrency
	sem := make(chan struct{}, a.config.Concurrency)
	var wg sync.WaitGroup

	ticker := time.NewTicker(a.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, waiting for running jobs to finish...")
			wg.Wait()
			close(a.done)
			return ctx.Err()
		case <-ticker.C:
			// Try to acquire a slot in the semaphore
			select {
			case sem <- struct{}{}:
				// Slot acquired, fetch and run a job in background
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() {
						<-sem
					}()
					a.processOne(ctx)
				}()
			default:
				// No available slots, skip this iteration
			}
		}
	}
}

// Done returns a channel that is closed when the agent has fully stopped.
func (a *Agent) Done() <-chan struct{} {
	return a.done
}

func (a *Agent) processOne(ctx context.Context) {
	execID, payload, err := a.queue.Dequeue(ctx, a.tenantIDs)
	if err != nil {
		return
	}

	log.Printf("Claimed execution %s", execID)

	var jobDef store.Job
	if err := json.Unmarshal(payload, &jobDef); err != nil {
		log.Printf("Failed to unmarshal job payload: %v", err)
		a.queue.Fail(ctx, nil, execID, nil, fmt.Sprintf("Invalid payload: %v", err))
		return
	}

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
	execContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Start Runtime
	handle, err := a.runtime.Start(execContext, runtimeOpts)
	if err != nil {
		log.Printf("Failed to start runtime for %s: %v", execID, err)
		a.queue.Fail(context.Background(), nil, execID, nil, fmt.Sprintf("Failed to start runtime. %s", err.Error()))
		return
	}

	// Wait for result
	result, err := handle.Wait(execContext)
	if err != nil {
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

	// Update Queue
	if result.ExitCode == 0 {
		log.Printf("Execution %s completed successfully", execID)
		a.queue.Complete(context.Background(), nil, execID, 0)
	} else {
		log.Printf("Execution %s failed with code %d", execID, result.ExitCode)
		errorMessage := fmt.Sprintf("Exit code %d", result.ExitCode)
		if result.Error != nil {
			errorMessage = result.Error.Error()
		}
		a.queue.Fail(context.Background(), nil, execID, &result.ExitCode, errorMessage)
	}
}
