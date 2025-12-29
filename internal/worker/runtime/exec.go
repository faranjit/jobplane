// Package runtime provides the Runtime interface for job execution backends.
package runtime

import (
	"context"
)

// ExecRuntime implements the Runtime interface using raw OS processes.
// This is optional and primarily used for development/testing.
type ExecRuntime struct{}

// NewExecRuntime creates a new process-based runtime.
func NewExecRuntime() *ExecRuntime {
	return &ExecRuntime{}
}

// Start implements Runtime.Start using os/exec.
func (e *ExecRuntime) Start(ctx context.Context, opts StartOptions) (Handle, error) {
	// TODO: Start process using os/exec
	return nil, nil
}
