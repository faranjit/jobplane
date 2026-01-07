package postgres

import (
	"context"
	"jobplane/internal/store"

	"github.com/google/uuid"
)

func (s *Store) GetExecutionByID(ctx context.Context, id uuid.UUID) (*store.Execution, error) {
	query := "SELECT * FROM executions WHERE id = $1"

	var execution store.Execution

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&execution.ID, &execution.JobID, &execution.TenantID, &execution.Status,
		&execution.Priority, &execution.Attempt, &execution.ExitCode, &execution.ErrorMessage,
		&execution.ScheduledAt, &execution.CreatedAt, &execution.StartedAt, &execution.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	return &execution, nil
}
