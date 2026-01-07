// Package api contains shared JSON request/response structs.
// This package is shared between the CLI and Controller.
package api

import "time"

// CreateTenantRequest is the request body for creating a new tenant.
type CreateTenantRequest struct {
	Name string `json:"name"`
}

// CreateTenantResponse is the response body after creating a tenant.
type CreateTenantResponse struct {
	ID     string `json:"tenant_id"`
	Name   string `json:"name"`
	ApiKey string `json:"api_key"`
}

// CreateJobRequest is the request body for creating a new job.
type CreateJobRequest struct {
	Name           string   `json:"name"`
	Image          string   `json:"image"`
	Command        []string `json:"command,omitempty"`
	DefaultTimeout int      `json:"default_timeout,omitempty"`
	// Priority must be between 0 and 100
	Priority int `json:"priority,omitempty"`
}

// CreateJobResponse is the response body after submitting a job.
type CreateJobResponse struct {
	JobID string `json:"job_id"`
}

// RunJobRequest is the request body for submitting a new execution of a job.
type RunJobRequest struct {
	ScheduledAt *time.Time `json:"scheduled_at"`
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
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Priority    int        `json:"priority"`
	Attempt     int        `json:"attempt"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	Error       *string    `json:"error,omitempty"`
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

// Priority levels for job execution
const (
	PriorityLow      = 0
	PriorityNormal   = 50
	PriorityHigh     = 75
	PriorityCritical = 100

	PriorityMin = 0
	PriorityMax = 100
)
