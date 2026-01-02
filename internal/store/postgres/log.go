package postgres

import (
	"context"
	"jobplane/internal/store"

	"github.com/google/uuid"
)

func (s *Store) AddLogEntry(ctx context.Context, executionID uuid.UUID, content string) error {
	query := `INSERT INTO execution_logs (execution_id, content) VALUES ($1, $2)`
	_, err := s.db.ExecContext(ctx, query, executionID, content)
	return err
}

func (s *Store) GetExecutionLogs(ctx context.Context, executionID uuid.UUID, afterID int64, limit int) ([]store.LogEntry, error) {
	query := `
		SELECT id, execution_id, content, created_at 
		FROM execution_logs
		WHERE execution_id = $1 and id > $2
		ORDER BY created_at ASC
		LIMIT $3
	`

	rows, err := s.db.QueryContext(ctx, query, executionID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []store.LogEntry
	for rows.Next() {
		var entry store.LogEntry
		if err := rows.Scan(&entry.ID, &entry.ExecutionID, &entry.Content, &entry.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, entry)
	}

	return logs, nil
}
