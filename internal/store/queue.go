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
	Enqueue(ctx context.Context, tx DBTransaction, executionID uuid.UUID, payload json.RawMessage) (int64, error)

	// Dequeue claims the next available execution.
	// It updates the row to 'RUNNING' status and sets a visibility timeout.
	// It does NOT return an AckFunc; the worker must call Complete or Fail.
	Dequeue(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error)

	// Complete marks execution as SUCCEEDED/COMPLETED and saves the exit code.
	Complete(ctx context.Context, tx DBTransaction, executionID uuid.UUID, exitCode int) error

	// Fail marks execution as FAILED.
	// If retries are exhausted, it saves the errMsg.
	Fail(ctx context.Context, tx DBTransaction, executionID uuid.UUID, errMsg string) error

	// SetVisibleAfter extends the visibility timeout (heartbeat).
	SetVisibleAfter(ctx context.Context, tx DBTransaction, executionID uuid.UUID, visibleAfter time.Time) error
}
