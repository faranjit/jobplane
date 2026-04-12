// Package store contains the database layer for jobplane.
package store

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Tenant represents a customer or team using the platform.
// All operations must be scoped by TenantID.
type Tenant struct {
	ID                      uuid.UUID
	Name                    string
	RateLimit               int
	RateLimitBurst          int
	MaxConcurrentExecutions int // 0 = unlimited
	CallbackURL             *string
	CallbackHeaders         json.RawMessage
	CreatedAt               time.Time
}

// Job represents a job definition submitted by a tenant.
type Job struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	Name            string
	Image           string // Container image to run
	Command         []string
	DefaultTimeout  int
	Priority        int
	CallbackURL     *string
	CallbackHeaders json.RawMessage
	CreatedAt       time.Time
}

// Execution represents a single execution attempt of a job.
// It tracks the lifecycle state (Pending -> Running -> Succeeded/Failed/Cancelled).
type Execution struct {
	ID              uuid.UUID
	JobID           uuid.UUID
	TenantID        uuid.UUID
	Status          ExecutionStatus
	Priority        int
	Attempt         int
	ExitCode        *int
	ErrorMessage    *string
	RetriedFrom     *uuid.UUID
	CallbackURL     *string
	CallbackHeaders json.RawMessage
	CallbackStatus  *string
	Result          json.RawMessage
	ScheduledAt     *time.Time
	CreatedAt       time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
}

// ExecutionArtifact represents a file produced by a job execution.
// Artifacts follow a two-phase lifecycle: they are created with "pending" status
// at authorization time (before upload), then confirmed after the file has been
// successfully uploaded and verified. This prevents quota bypass and enables
// cleanup of orphaned uploads.
type ExecutionArtifact struct {
	ID             uuid.UUID // Unique artifact identifier
	ExecutionID    uuid.UUID // The execution that produced this artifact
	TenantID       uuid.UUID // Tenant that owns the execution (for multi-tenancy isolation)
	Filename       string    // Original filename (unique per execution)
	ContentType    string    // MIME type (e.g., "text/csv", "application/octet-stream")
	SizeBytes      int64     // Declared file size in bytes
	StorageBackend string    // Backend type that stores this artifact ("local", "s3")
	StoragePath    *string   // Backend-specific path to the stored file (nil while pending)
	Status         string    // Lifecycle status: "pending" or "confirmed"
	CreatedAt      time.Time // When the artifact was authorized
	UpdatedAt      time.Time // Last status change timestamp
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

// Artifact lifecycle statuses.
const (
	// ExecutionArtifactStatusPending indicates the artifact has been authorized but not yet uploaded.
	// Pending artifacts count toward tenant storage quotas and are cleaned up if not confirmed
	// within the configured TTL (artifact_pending_ttl).
	ExecutionArtifactStatusPending = "pending"

	// ExecutionArtifactStatusConfirmed indicates the artifact has been successfully uploaded
	// and verified. Only confirmed artifacts are visible in listing endpoints.
	ExecutionArtifactStatusConfirmed = "confirmed"
)

const (
	CallbackStatusPending   = "PENDING"
	CallbackStatusDelivered = "DELIVERED"
	CallbackStatusFailed    = "FAILED"
)
