package postgres

import (
	"context"
	"encoding/json"
	"jobplane/internal/store"

	"github.com/google/uuid"
)

// CreateJob inserts a new job row.
// It converts the command slice into a JSON array for storage.
func (s *Store) CreateJob(ctx context.Context, tx store.DBTransaction, job *store.Job) error {
	query := `
		INSERT INTO jobs (id, tenant_id, name, image, default_command, default_timeout, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	cmdJson, err := json.Marshal(job.Command)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, query,
		job.ID,
		job.TenantID,
		job.Name,
		job.Image,
		cmdJson,
		job.DefaultTimeout,
		job.CreatedAt,
	)
	return err
}

func (s *Store) GetJobByID(ctx context.Context, id uuid.UUID) (*store.Job, error) {
	query := "SELECT * FROM jobs WHERE id = $1"

	var job store.Job

	err := s.db.QueryRowContext(ctx, query, id).Scan(&job.ID, &job.TenantID, &job.Name, &job.Image, &job.Command, &job.DefaultTimeout, &job.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &job, nil
}

// CreateExecution inserts a new execution row.
func (s *Store) CreateExecution(ctx context.Context, tx store.DBTransaction, execution *store.Execution) error {
	query := `
		INSERT INTO executions (id, job_id, tenant_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := tx.ExecContext(ctx, query,
		execution.ID,
		execution.JobID,
		execution.TenantID,
		execution.Status,
		execution.CreatedAt,
	)
	return err
}
