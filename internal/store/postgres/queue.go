package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"jobplane/internal/store"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Default retry policy
const (
	MaxRetries        = 5
	VisibilityTimeout = 5 * time.Minute
)

// Enqueue adds a job to the execution_queue.
func (s *Store) Enqueue(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, payload json.RawMessage) (int64, error) {
	query := `
		INSERT INTO execution_queue (execution_id, tenant_id, payload)
		SELECT $1, tenant_id, $2
		FROM executions 
		WHERE id = $1
		RETURNING id
	`

	var executor store.DBTransaction = s.db
	if tx != nil {
		executor = tx
	}

	var id int64
	err := executor.QueryRowContext(ctx, query, executionID, payload).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to enqueue execution %s: %w", executionID, err)
	}

	return id, nil
}

// Dequeue claims the next available job using SELECT ... FOR UPDATE SKIP LOCKED.
func (s *Store) Dequeue(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
	// Start a transaction solely for the dequeue operation
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, nil, err
	}
	defer tx.Rollback()

	// Find and Lock the next job
	var queueID int64
	var executionID uuid.UUID
	var payload json.RawMessage

	args := []interface{}{}
	whereClause := "WHERE visible_after <= NOW()"

	// Support filtering by specific tenants if provided
	if len(tenantIDs) > 0 {
		whereClause += " AND tenant_id = ANY($1)"
		args = append(args, pq.Array(tenantIDs))
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, execution_id, payload
		FROM execution_queue
		%s
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, whereClause)

	err = tx.QueryRowContext(ctx, selectQuery, args...).Scan(&queueID, &executionID, &payload)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, nil, err // Empty queue
		}
		return uuid.Nil, nil, fmt.Errorf("dequeue scan failed: %w", err)
	}

	// Bump Visibility Timeout (Heartbeat)
	_, err = tx.ExecContext(ctx, `
		UPDATE execution_queue 
		SET visible_after = NOW() + ($1 * INTERVAL '1 second')
		WHERE id = $2
	`, VisibilityTimeout.Seconds(), queueID)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("failed to update visibility: %w", err)
	}

	// Update History Status
	_, err = tx.ExecContext(ctx, `
		UPDATE executions 
		SET status = $1, started_at = NOW() 
		WHERE id = $2
	`, store.ExecutionStatusRunning, executionID)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("failed to mark running: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return uuid.Nil, nil, err
	}

	return executionID, payload, nil
}

// Complete handles a successful job execution.
func (s *Store) Complete(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, exitCode int) error {
	executor := s.getExecutor(tx)

	// Delete from Queue
	_, err := executor.ExecContext(ctx, "DELETE FROM execution_queue WHERE execution_id = $1", executionID)
	if err != nil {
		return err
	}

	// Update History
	_, err = executor.ExecContext(ctx, `
		UPDATE executions 
		SET status = $1, exit_code = $2, finished_at = NOW() 
		WHERE id = $3
	`, store.ExecutionStatusCompleted, exitCode, executionID)

	return err
}

// Fail handles a failed job execution with retries.
func (s *Store) Fail(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, exitCode *int, errMsg string) error {
	executor := s.getExecutor(tx)

	// Check current attempts
	var attempt int
	err := executor.QueryRowContext(ctx, "SELECT attempt FROM execution_queue WHERE execution_id = $1", executionID).Scan(&attempt)

	isFatal := false
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Job not found in queue -> treat as fatal/already gone
			isFatal = true
		} else {
			// Return actual DB error to avoid accidentally retrying
			return err
		}
	} else if attempt > MaxRetries {
		isFatal = true
	}

	if !isFatal {
		// RETRY: Exponential Backoff (10s * 2^attempt)
		backoff := time.Duration(10*(1<<attempt)) * time.Second
		_, err = executor.ExecContext(ctx, `
			UPDATE execution_queue 
			SET attempt = attempt + 1, visible_after = NOW() + ($1 * INTERVAL '1 second')
			WHERE execution_id = $2
		`, backoff.Seconds(), executionID)
		return err
	}

	// permanent failure
	_, err = executor.ExecContext(ctx, "DELETE FROM execution_queue WHERE execution_id = $1", executionID)
	if err != nil {
		return fmt.Errorf("failed to delete failed job from queue: %w", err)
	}

	_, err = executor.ExecContext(ctx, `
		UPDATE executions 
		SET status = $1, exit_code = $2, error_message = $3, finished_at = NOW() 
		WHERE id = $4
	`, store.ExecutionStatusFailed, exitCode, errMsg, executionID)
	return err
}

// SetVisibleAfter extends the heartbeat.
func (s *Store) SetVisibleAfter(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, visibleAfter time.Time) error {
	executor := s.getExecutor(tx)
	_, err := executor.ExecContext(ctx, `
		UPDATE execution_queue 
		SET visible_after = $1 
		WHERE execution_id = $2
	`, visibleAfter, executionID)
	return err
}

func (s *Store) getExecutor(tx store.DBTransaction) store.DBTransaction {
	if tx != nil {
		return tx
	}
	return s.db
}
