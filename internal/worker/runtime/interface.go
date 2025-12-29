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
	Env     map[string]string
	Timeout int // seconds
}

// Handle represents a running job execution.
type Handle interface {
	// Wait blocks until the job completes and returns the exit code.
	Wait(ctx context.Context) (int, error)

	// Stop forcefully terminates the job.
	Stop(ctx context.Context) error

	// Logs returns a reader for the job's stdout/stderr.
	Logs() io.ReadCloser
}
