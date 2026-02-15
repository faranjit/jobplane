// Package runtime provides the Runtime interface for job execution backends.
package runtime

import (
	"context"
	"io"
)

// Runtime defines the interface for executing jobs.
// Implementations include Docker and raw process execution.
type Runtime interface {
	// Start begins execution of a job and returns a handle.
	Start(ctx context.Context, opts StartOptions) (Handle, error)
}

// StartOptions contains the parameters for starting a job.
type StartOptions struct {
	Image   string
	Command []string
	Payload []byte
	Env     map[string]string
	Timeout int // seconds
}

// ExitResult contains the result of a job execution.
type ExitResult struct {
	ExitCode int
	Error    error
}

// Handle represents a SPECIFIC running job execution.
type Handle interface {
	// Wait blocks until this specific job completes.
	Wait(ctx context.Context) (ExitResult, error)

	// Stop forcefully terminates this specific job.
	Stop(ctx context.Context) error

	// StreamLogs returns a reader for this specific job's logs.
	StreamLogs(ctx context.Context) (io.ReadCloser, error)

	// ResultDir returns the directory where the result of this job is stored.
	ResultDir() string

	// Cleanup removes the temporary directory where the result of this job is stored.
	// This is called by the worker after the job is completed.
	Cleanup() error
}
