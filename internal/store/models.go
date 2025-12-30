// Package store contains the database layer for jobplane.
package store

import (
	"time"

	"github.com/google/uuid"
)

// Tenant represents a tenant in the multi-tenant system.
// All operations must be scoped by TenantID.
type Tenant struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

// Job represents a job definition submitted by a tenant.
type Job struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Name      string
	Image     string // Container image to run
	Command   []string
	CreatedAt time.Time
}

// Execution represents a single execution attempt of a job.
type Execution struct {
	ID           uuid.UUID
	JobID        uuid.UUID
	TenantID     uuid.UUID
	Status       ExecutionStatus
	Attempt      int
	StartedAt    *time.Time
	CompletedAt  *time.Time
	ExitCode     *int
	ErrorMessage *string
	CreatedAt    time.Time
}

// ExecutionStatus represents the state of an execution.
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
)
