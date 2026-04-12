package postgres

import (
	"context"
	"jobplane/internal/store"
	"time"

	"github.com/google/uuid"
)

// CreateArtifact inserts a new artifact record with status "pending".
func (s *Store) CreateArtifact(ctx context.Context, artifact *store.ExecutionArtifact) error {
	return nil
}

// ConfirmArtifact transitions an artifact from "pending" to "confirmed"
// and records the backend-specific storage path where the file was written.
func (s *Store) ConfirmArtifact(ctx context.Context, artifactID uuid.UUID, storagePath string) error {
	return nil
}

// GetArtifact retrieves a single artifact by its ID.
func (s *Store) GetArtifact(ctx context.Context, artifactID uuid.UUID) (*store.ExecutionArtifact, error) {
	return nil, nil
}

// ListArtifactsByExecution returns all confirmed artifacts for an execution.
func (s *Store) ListArtifactsByExecution(ctx context.Context, executionID uuid.UUID) ([]store.ExecutionArtifact, error) {
	return nil, nil
}

// GetPendingArtifacts returns artifacts still in "pending" status older than the given time.
func (s *Store) GetPendingArtifacts(ctx context.Context, olderThan time.Time) ([]store.ExecutionArtifact, error) {
	return nil, nil
}

// DeleteArtifact removes an artifact metadata record.
func (s *Store) DeleteArtifact(ctx context.Context, artifactID uuid.UUID) error {
	return nil
}

// GetTenantStorageUsage returns the total size in bytes of all artifacts
// (both pending and confirmed) for a tenant. Used for quota enforcement.
func (s *Store) GetTenantStorageUsage(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	return 0, nil
}
