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
}

// CreateJobResponse is the response body after submitting a job.
type CreateJobResponse struct {
	JobID string `json:"job_id"`
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
	Attempt     int        `json:"attempt"`
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
