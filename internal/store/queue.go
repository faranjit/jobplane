// Package store contains the database layer for jobplane.
package store

import (
	"context"
	"time"
)

// Queue defines the interface for job queue operations.
// Implementations must use SELECT ... FOR UPDATE SKIP LOCKED semantics.
type Queue interface {
	// Enqueue adds a new execution to the queue.
	Enqueue(ctx context.Context, execution *Execution) error

	// Dequeue claims the next available execution for processing.
	// Returns nil if no work is available.
	Dequeue(ctx context.Context, tenantID string) (*Execution, error)

	// Complete marks an execution as completed with the given exit code.
	Complete(ctx context.Context, executionID string, exitCode int) error

	// Fail marks an execution as failed with the given error message.
	// If retries remain, the execution will be re-queued with backoff.
	Fail(ctx context.Context, executionID string, errMsg string) error

	// SetVisibleAfter sets when the execution becomes visible again (for retry backoff).
	SetVisibleAfter(ctx context.Context, executionID string, visibleAfter time.Time) error
}
