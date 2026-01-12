package postgres

import (
	"context"
	"jobplane/internal/store"
	"time"

	"github.com/google/uuid"
)

func (s *Store) GetExecutionByID(ctx context.Context, id uuid.UUID) (*store.Execution, error) {
	query := "SELECT * FROM executions WHERE id = $1"

	var execution store.Execution

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&execution.ID, &execution.JobID, &execution.TenantID,
		&execution.Status, &execution.Priority, &execution.Attempt,
		&execution.ExitCode, &execution.ErrorMessage, &execution.RetriedFrom,
		&execution.CreatedAt, &execution.ScheduledAt, &execution.StartedAt, &execution.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	return &execution, nil
}

func (s *Store) ListDLQ(ctx context.Context, tenantID uuid.UUID, limit int, offset int) ([]store.DLQEntry, error) {
	query := `
	SELECT dlq.*, e.job_id, j.name as job_name, e.priority
	FROM execution_dlq dlq
	JOIN executions e ON dlq.execution_id = e.id
	JOIN jobs j ON e.job_id = j.id
	WHERE dlq.tenant_id = $1
	ORDER BY dlq.failed_at DESC
	LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var executions []store.DLQEntry
	for rows.Next() {
		var execution store.DLQEntry
		if err := rows.Scan(
			&execution.ID, &execution.ExecutionID, &execution.TenantID,
			&execution.Payload, &execution.ErrorMessage, &execution.Attempts,
			&execution.FailedAt,
			&execution.JobID, &execution.JobName, &execution.Priority,
		); err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}

	return executions, nil
}

func (s *Store) RetryFromDLQ(ctx context.Context, executionID uuid.UUID) (uuid.UUID, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()

	query := `
		SELECT dlq.id, dlq.payload, e.id, e.job_id, e.tenant_id, e.priority FROM execution_dlq dlq
		LEFT JOIN executions e ON dlq.execution_id = e.id
		WHERE dlq.execution_id = $1
	`

	var execution store.Execution
	var dlqEntry store.DLQEntry
	err = tx.QueryRowContext(ctx, query, executionID).Scan(
		&dlqEntry.ID, &dlqEntry.Payload, &execution.ID, &execution.JobID, &execution.TenantID, &execution.Priority,
	)
	if err != nil {
		return uuid.Nil, err
	}

	newExecution := store.Execution{
		ID:          uuid.New(),
		JobID:       execution.JobID,
		TenantID:    execution.TenantID,
		Priority:    execution.Priority,
		Status:      store.ExecutionStatusPending,
		RetriedFrom: &execution.ID,
		CreatedAt:   time.Now().UTC(),
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO executions (id, job_id, tenant_id, priority, status, retried_from, created_at) 
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		newExecution.ID, newExecution.JobID, newExecution.TenantID,
		newExecution.Priority, newExecution.Status,
		newExecution.RetriedFrom, newExecution.CreatedAt,
	); err != nil {
		return uuid.Nil, err
	}

	_, err = s.Enqueue(ctx, tx, newExecution.ID, dlqEntry.Payload, time.Now().UTC())
	if err != nil {
		return uuid.Nil, err
	}

	_, err = tx.ExecContext(ctx, "DELETE FROM execution_dlq WHERE execution_id = $1", executionID)
	if err != nil {
		return uuid.Nil, err
	}

	if err := tx.Commit(); err != nil {
		return uuid.Nil, err
	}

	return newExecution.ID, nil
}
