package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

const (
	maxRetries    = 3
	maxConcurrent = 10
)

// Backoff durations for each retry attempt: 1s, 5s, 25s
var retryBackoffs = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	25 * time.Second,
}

// CallbackStore is a minimal interface for updating callback delivery status.
type CallbackStore interface {
	UpdateCallbackStatus(ctx context.Context, executionID uuid.UUID, status string) error
}

// Dispatcher sends webhook callbacks asynchronously with bounded concurrency and retries.
type Dispatcher struct {
	client *http.Client
	store  CallbackStore
	sem    chan struct{}
	wg     sync.WaitGroup

	// Metrics
	deliveryAttempts otelmetric.Int64Counter
	deliveryOutcome  otelmetric.Int64Counter
	deliveryDuration otelmetric.Float64Histogram
}

// NewDispatcher creates a new webhook dispatcher.
func NewDispatcher(store CallbackStore) *Dispatcher {
	meter := otel.Meter("jobplane.webhook")

	attempts, _ := meter.Int64Counter("webhook.delivery.attempts",
		otelmetric.WithDescription("Total HTTP delivery attempts"),
	)
	outcome, _ := meter.Int64Counter("webhook.delivery.outcome",
		otelmetric.WithDescription("Final delivery outcomes (delivered/failed)"),
	)
	duration, _ := meter.Float64Histogram("webhook.delivery.duration",
		otelmetric.WithDescription("End-to-end delivery duration including retries"),
		otelmetric.WithUnit("ms"),
	)

	return &Dispatcher{
		client:           &http.Client{Timeout: 10 * time.Second},
		store:            store,
		sem:              make(chan struct{}, maxConcurrent),
		deliveryAttempts: attempts,
		deliveryOutcome:  outcome,
		deliveryDuration: duration,
	}
}

// Deliver fires a webhook callback asynchronously. It is non-blocking —
// the actual HTTP request, retries, and status updates happen in a background goroutine.
func (d *Dispatcher) Deliver(ctx context.Context, execution *store.Execution) {
	if execution.CallbackURL == nil || *execution.CallbackURL == "" {
		return
	}

	d.wg.Go(func() {
		d.sem <- struct{}{}
		defer func() { <-d.sem }()

		start := time.Now()
		payload := d.buildPayload(execution)

		delivered := d.deliverWithRetry(execution, payload)

		status := store.CallbackStatusDelivered
		if !delivered {
			status = store.CallbackStatusFailed
		}

		// Record metrics
		d.deliveryOutcome.Add(context.Background(), 1,
			otelmetric.WithAttributes(attribute.String("status", status)))
		d.deliveryDuration.Record(context.Background(),
			float64(time.Since(start).Milliseconds()),
			otelmetric.WithAttributes(attribute.String("status", status)))

		if err := d.store.UpdateCallbackStatus(context.Background(), execution.ID, status); err != nil {
			log.Printf("[webhook] failed to update callback status for execution %s: %v", execution.ID, err)
		}
	})
}

// Shutdown blocks until all in-flight webhook deliveries complete.
// Call this during graceful server shutdown.
func (d *Dispatcher) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// buildPayload constructs the webhook JSON payload from an execution.
func (d *Dispatcher) buildPayload(execution *store.Execution) []byte {
	event := "execution.completed"
	if execution.Status == store.ExecutionStatusFailed {
		event = "execution.failed"
	}

	var duration int64
	if execution.StartedAt != nil && execution.CompletedAt != nil {
		duration = execution.CompletedAt.Sub(*execution.StartedAt).Milliseconds()
	}

	payload := api.WebhookPayload{
		Event:       event,
		ExecutionID: execution.ID.String(),
		JobID:       execution.JobID.String(),
		Status:      string(execution.Status),
		ExitCode:    execution.ExitCode,
		Result:      execution.Result,
		Duration:    duration,
		Timestamp:   time.Now().UTC(),
	}

	data, _ := json.Marshal(payload)
	return data
}

// deliveryError represents an HTTP delivery error with retry semantics.
type deliveryError struct {
	StatusCode int
	Message    string
	Retryable  bool
}

func (e *deliveryError) Error() string {
	return e.Message
}

// deliverWithRetry attempts to POST the payload to the callback URL,
// retrying up to maxRetries times with exponential backoff.
// Only retries on retryable errors (5xx, network). Stops immediately on 4xx.
// Returns true if delivery succeeded (2xx response).
func (d *Dispatcher) deliverWithRetry(execution *store.Execution, payload []byte) bool {
	var customHeaders map[string]string
	if len(execution.CallbackHeaders) > 0 {
		if err := json.Unmarshal(execution.CallbackHeaders, &customHeaders); err != nil {
			log.Printf("[webhook] failed to parse callback headers for execution %s: %v", execution.ID, err)
		}
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := retryBackoffs[attempt-1]
			log.Printf("[webhook] retry %d/%d for execution %s in %v", attempt, maxRetries, execution.ID, backoff)
			time.Sleep(backoff)
		}

		err := d.doPost(*execution.CallbackURL, payload, customHeaders)

		if err == nil {
			d.deliveryAttempts.Add(context.Background(), 1,
				otelmetric.WithAttributes(attribute.String("status", "success"), attribute.Int("attempt", attempt+1)))
			log.Printf("[webhook] delivered callback for execution %s", execution.ID)
			return true
		}

		d.deliveryAttempts.Add(context.Background(), 1,
			otelmetric.WithAttributes(attribute.String("status", "error"), attribute.Int("attempt", attempt+1)))
		log.Printf("[webhook] attempt %d failed for execution %s: %v", attempt+1, execution.ID, err)

		// If the error is not retryable (e.g. 4xx), stop immediately
		var de *deliveryError
		if errors.As(err, &de) && !de.Retryable {
			log.Printf("[webhook] non-retryable error for execution %s (HTTP %d), giving up", execution.ID, de.StatusCode)
			break
		}
	}

	log.Printf("[webhook] exhausted retries for execution %s, marking as FAILED", execution.ID)
	return false
}

// doPost sends a single HTTP POST and returns nil on 2xx, error otherwise.
// Returns a *deliveryError with Retryable=true for 5xx and network errors,
// and Retryable=false for 4xx client errors.
func (d *Dispatcher) doPost(url string, payload []byte, headers map[string]string) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "jobplane-webhook/1.0")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		// Network errors are always retryable
		return &deliveryError{Message: fmt.Sprintf("sending request: %v", err), Retryable: true}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) // drain body to reuse connection

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	retryable := resp.StatusCode >= 500 || resp.StatusCode == 429
	return &deliveryError{
		StatusCode: resp.StatusCode,
		Message:    fmt.Sprintf("non-2xx response: %d", resp.StatusCode),
		Retryable:  retryable,
	}
}
