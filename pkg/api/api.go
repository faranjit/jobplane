// Package api contains shared JSON request/response structs.
// This package is shared between the CLI and Controller.
package api

import "time"

// SubmitJobRequest is the request body for submitting a new job.
type SubmitJobRequest struct {
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	Command []string `json:"command,omitempty"`
}

// SubmitJobResponse is the response body after submitting a job.
type SubmitJobResponse struct {
	JobID       string `json:"job_id"`
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
