// Package store contains the database layer for jobplane.
package store

import (
	"time"

	"github.com/google/uuid"
)

// Tenant represents a customer or team using the platform.
// All operations must be scoped by TenantID.
type Tenant struct {
	ID                      uuid.UUID
	Name                    string
	CreatedAt               time.Time
	RateLimit               int
	RateLimitBurst          int
	MaxConcurrentExecutions int // 0 = unlimited
}

// Job represents a job definition submitted by a tenant.
type Job struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	Name           string
	Image          string // Container image to run
	Command        []string
	DefaultTimeout int
	Priority       int
	CreatedAt      time.Time
}

// Execution represents a single execution attempt of a job.
// It tracks the lifecycle state (Pending -> Running -> Succeeded/Failed/Cancelled).
type Execution struct {
	ID           uuid.UUID
	JobID        uuid.UUID
	TenantID     uuid.UUID
	Status       ExecutionStatus
	Priority     int
	Attempt      int
	ExitCode     *int
	ErrorMessage *string
	RetriedFrom  *uuid.UUID
	ScheduledAt  *time.Time
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

type LogEntry struct {
	ID          int64
	ExecutionID uuid.UUID
	Content     string
	CreatedAt   time.Time
}

type DLQEntry struct {
	ID           int64
	ExecutionID  uuid.UUID
	TenantID     uuid.UUID
	JobID        string
	JobName      string
	Priority     int
	Payload      []byte
	ErrorMessage *string
	Attempts     int
	FailedAt     *time.Time
}

// ExecutionStatus represents the state of an execution.
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "PENDING"
	ExecutionStatusScheduled ExecutionStatus = "SCHEDULED"
	ExecutionStatusRunning   ExecutionStatus = "RUNNING"
	ExecutionStatusCompleted ExecutionStatus = "SUCCEEDED"
	ExecutionStatusFailed    ExecutionStatus = "FAILED"
	ExecutionStatusCancelled ExecutionStatus = "CANCELLED"
)
