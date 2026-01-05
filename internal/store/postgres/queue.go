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
func (s *Store) Enqueue(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, payload json.RawMessage, visibleAfter time.Time) (int64, error) {
	if visibleAfter.IsZero() {
		visibleAfter = time.Now()
	}

	query := `
		INSERT INTO execution_queue (execution_id, tenant_id, payload, visible_after)
		SELECT $1, tenant_id, $2, $3
		FROM executions 
		WHERE id = $1
		RETURNING id
	`

	var executor store.DBTransaction = s.db
	if tx != nil {
		executor = tx
	}

	var id int64
	err := executor.QueryRowContext(ctx, query, executionID, payload, visibleAfter).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to enqueue execution %s: %w", executionID, err)
	}

	return id, nil
}

// DequeueBatch claims up to 'limit' available jobs atomically using SELECT ... FOR UPDATE SKIP LOCKED.
// Returns nil slice if no jobs are available.
func (s *Store) DequeueBatch(ctx context.Context, tenantIDs []uuid.UUID, limit int) ([]store.QueueItem, error) {
	if limit <= 0 {
		limit = 1
	}

	// Start a transaction for the batch dequeue operation
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Build WHERE clause and args
	args := []interface{}{limit}
	whereClause := "WHERE visible_after <= NOW()"

	if len(tenantIDs) > 0 {
		whereClause += " AND tenant_id = ANY($2)"
		args = append(args, pq.Array(tenantIDs))
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, execution_id, payload
		FROM execution_queue
		%s
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	`, whereClause)

	rows, err := tx.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("batch dequeue query failed: %w", err)
	}
	defer rows.Close()

	var items []store.QueueItem
	var queueIDs []int64
	var execIDs []uuid.UUID

	for rows.Next() {
		var queueID int64
		var item store.QueueItem
		if err := rows.Scan(&queueID, &item.ExecutionID, &item.Payload); err != nil {
			return nil, fmt.Errorf("batch dequeue scan failed: %w", err)
		}
		items = append(items, item)
		queueIDs = append(queueIDs, queueID)
		execIDs = append(execIDs, item.ExecutionID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("batch dequeue rows error: %w", err)
	}

	// Empty queue
	if len(items) == 0 {
		return nil, nil
	}

	// Bulk update visibility timeout for all claimed jobs
	_, err = tx.ExecContext(ctx, `
		UPDATE execution_queue 
		SET visible_after = NOW() + ($1 * INTERVAL '1 second')
		WHERE id = ANY($2)
	`, VisibilityTimeout.Seconds(), pq.Array(queueIDs))
	if err != nil {
		return nil, fmt.Errorf("batch visibility update failed: %w", err)
	}

	// Bulk update execution status to RUNNING
	_, err = tx.ExecContext(ctx, `
		UPDATE executions 
		SET status = $1, started_at = COALESCE(started_at, NOW()), attempt = attempt + 1
		WHERE id = ANY($2)
	`, store.ExecutionStatusRunning, pq.Array(execIDs))
	if err != nil {
		return nil, fmt.Errorf("batch status update failed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return items, nil
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
			SET visible_after = NOW() + ($1 * INTERVAL '1 second')
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
