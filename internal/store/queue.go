// Package store contains the database layer for jobplane.
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Queue defines the interface for job queue operations.
// Implementations must use SELECT ... FOR UPDATE SKIP LOCKED semantics.
type Queue interface {
	// Enqueue adds a new execution to the queue.
	Enqueue(ctx context.Context, tx DBTransaction, executionID uuid.UUID, payload json.RawMessage, visibleAfter time.Time) (int64, error)

	// DequeueBatch claims up to 'limit' available executions atomically.
	// Returns nil slice if queue is empty.
	DequeueBatch(ctx context.Context, tenantIDs []uuid.UUID, limit int) ([]QueueItem, error)

	// Complete marks execution as SUCCEEDED/COMPLETED and saves the exit code.
	Complete(ctx context.Context, tx DBTransaction, executionID uuid.UUID, exitCode int) error

	// Fail marks execution as FAILED.
	// If retries are exhausted, it saves the errMsg.
	Fail(ctx context.Context, tx DBTransaction, executionID uuid.UUID, exitCode *int, errMsg string) error

	// SetVisibleAfter extends the visibility timeout (heartbeat).
	SetVisibleAfter(ctx context.Context, tx DBTransaction, executionID uuid.UUID, visibleAfter time.Time) error
}

// QueueItem represents a dequeued execution from the queue.
type QueueItem struct {
	ExecutionID uuid.UUID
	Payload     json.RawMessage
}
