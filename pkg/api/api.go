// Package api contains shared JSON request/response structs.
// This package is shared between the CLI and Controller.
package api

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CreateTenantRequest is the request body for creating a new tenant.
type CreateTenantRequest struct {
	Name                    string            `json:"name"`
	RateLimit               int               `json:"rate_limit,omitempty"`
	RateLimitBurst          int               `json:"rate_limit_burst,omitempty"`
	MaxConcurrentExecutions int               `json:"max_concurrent_executions,omitempty"`
	CallbackURL             *string           `json:"callback_url,omitempty"`
	CallbackHeaders         map[string]string `json:"callback_headers,omitempty"`
}

// CreateTenantResponse is the response body after creating a tenant.
type CreateTenantResponse struct {
	ID     string `json:"tenant_id"`
	Name   string `json:"name"`
	ApiKey string `json:"api_key"`
}

// UpdateTenantRequest is the request body for updating an existing tenant.
type UpdateTenantRequest struct {
	Name                    string            `json:"name,omitempty"`
	RateLimit               int               `json:"rate_limit,omitempty"`
	RateLimitBurst          int               `json:"rate_limit_burst,omitempty"`
	MaxConcurrentExecutions int               `json:"max_concurrent_executions,omitempty"`
	CallbackURL             *string           `json:"callback_url,omitempty"`
	CallbackHeaders         map[string]string `json:"callback_headers,omitempty"`
}

// CreateJobRequest is the request body for creating a new job.
type CreateJobRequest struct {
	Name           string   `json:"name"`
	Image          string   `json:"image"`
	Command        []string `json:"command,omitempty"`
	DefaultTimeout int      `json:"default_timeout,omitempty"`
	// Priority must be between 0 and 100
	Priority        int               `json:"priority,omitempty"`
	CallbackURL     *string           `json:"callback_url,omitempty"`
	CallbackHeaders map[string]string `json:"callback_headers,omitempty"`
}

// CreateJobResponse is the response body after submitting a job.
type CreateJobResponse struct {
	JobID string `json:"job_id"`
}

// RunJobRequest is the request body for submitting a new execution of a job.
type RunJobRequest struct {
	ScheduledAt     *time.Time        `json:"scheduled_at"`
	CallbackURL     *string           `json:"callback_url,omitempty"`
	CallbackHeaders map[string]string `json:"callback_headers,omitempty"`
}

// RunJobResponse is the response body after running a job.
type RunJobResponse struct {
	ExecutionID string `json:"execution_id"`
}

// JobStatusResponse is the response body for job status queries.
type JobStatusResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Image     string     `json:"image"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// ExecutionResponse represents an execution in API responses.
type ExecutionResponse struct {
	ID             string          `json:"id"`
	Status         string          `json:"status"`
	Priority       int             `json:"priority"`
	Attempt        int             `json:"attempt"`
	ScheduledAt    *time.Time      `json:"scheduled_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	ExitCode       *int            `json:"exit_code,omitempty"`
	Error          *string         `json:"error,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	CallbackURL    *string         `json:"callback_url,omitempty"`
	CallbackStatus string          `json:"callback_status,omitempty"`
}

// ExecutionResultRequest represents the result of an execution
type ExecutionResultRequest struct {
	ExitCode *int            `json:"exit_code"`
	Error    string          `json:"error"`
	Result   json.RawMessage `json:"result"`
}

// DequeueRequest represents the payload sent by a worker to claim new jobs.
type DequeueRequest struct {
	// Limit is the maximum number of executions the worker can currently accept.
	Limit int `json:"limit"`

	// TenantIDs restricts the claimed jobs to specific tenants.
	// If empty, the worker pulls jobs for all tenants.
	TenantIDs []uuid.UUID `json:"tenant_ids,omitempty"`
}

// DequeuedExecution represents a single claimed execution returned to the worker.
type DequeuedExecution struct {
	ExecutionID uuid.UUID       `json:"execution_id"`
	Payload     json.RawMessage `json:"payload"` // Contains the store.Job and Trace context
}

// DequeueResponse is the payload returned by the Controller containing the claimed jobs.
type DequeueResponse struct {
	Executions []DequeuedExecution `json:"executions"`
}

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// AddLogRequest is the payload sent by the Worker.
type AddLogRequest struct {
	Content string `json:"content"`
}

// LogEntry represents a single log line in the response.
type LogEntry struct {
	ID        int64     `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// GetLogsResponse is the response body for fetching logs.
type GetLogsResponse struct {
	Logs []LogEntry `json:"logs"`
}

// DLQExecutionResponse represents a DLQ execution.
type DLQExecutionResponse struct {
	ID           int64      `json:"id"`
	ExecutionID  string     `json:"execution_id"`
	JobID        string     `json:"job_id"`
	JobName      string     `json:"job_name"`
	Priority     int        `json:"priority"`
	ErrorMessage *string    `json:"error_message"`
	Attempts     int        `json:"attempts"`
	FailedAt     *time.Time `json:"failed_at"`
}

// RetryDQLExecutionResponse represents a retry response for a DLQ execution.
type RetryDQLExecutionResponse struct {
	NewExecutionID string `json:"new_execution_id"`
}

// WebhookPayload represents the payload sent to the webhook URL.
type WebhookPayload struct {
	Event       string          `json:"event"` // "execution.completed" | "execution.failed"
	ExecutionID string          `json:"execution_id"`
	JobID       string          `json:"job_id"`
	Status      string          `json:"status"`
	ExitCode    *int            `json:"exit_code,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
	Duration    int64           `json:"duration"`
	Timestamp   time.Time       `json:"timestamp"`
}

// AuthorizeArtifactRequest is sent by the worker to request permission to upload an artifact.
// The controller validates the request, creates a pending artifact record, and returns
// a backend-specific upload URL.
type AuthorizeArtifactRequest struct {
	Filename    string `json:"filename"`     // Name of the artifact file (must be unique per execution)
	ContentType string `json:"content_type"` // MIME type of the file
	SizeBytes   int64  `json:"size_bytes"`   // Declared file size (validated against max limit)
}

// AuthorizeArtifactResponse is returned by the controller after a successful authorization.
// The worker uses the upload URL and method to upload the artifact bytes directly,
// without knowing which storage backend is active.
type AuthorizeArtifactResponse struct {
	ArtifactID string `json:"artifact_id"` // Unique identifier for the artifact record
	UploadURL  string `json:"upload_url"`  // URL to PUT the artifact bytes to
	Method     string `json:"method"`      // HTTP method to use (typically "PUT")
	ExpiresIn  int64  `json:"expires_in"`  // Seconds until the upload URL expires
}

// Priority levels for job execution
const (
	PriorityLow      = 0
	PriorityNormal   = 50
	PriorityHigh     = 75
	PriorityCritical = 100

	PriorityMin = 0
	PriorityMax = 100
)
