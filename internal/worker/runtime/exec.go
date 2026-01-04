// Package runtime provides the Runtime interface for job execution backends.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// ExecRuntime implements the Runtime interface using raw OS processes.
// This runtime is intended for development, testing, and trusted internal workloads.
// WARNING: No sandboxing - jobs run with the same privileges as the Worker process.
type ExecRuntime struct {
	// WorkDir is the base directory where per-execution directories are created.
	// Each execution gets an isolated subdirectory: {WorkDir}/{executionID}/
	WorkDir string
}

// ExecHandle represents a running process execution.
type ExecHandle struct {
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	stderr   io.ReadCloser
	combined *combinedReader
	workDir  string // Per-execution directory path
}

// combinedReader combines stdout and stderr into a single reader.
type combinedReader struct {
	stdout io.ReadCloser
	stderr io.ReadCloser
	reader io.Reader
}

func newCombinedReader(stdout, stderr io.ReadCloser) *combinedReader {
	return &combinedReader{
		stdout: stdout,
		stderr: stderr,
		reader: io.MultiReader(stdout, stderr),
	}
}

func (c *combinedReader) Read(p []byte) (n int, err error) {
	return c.reader.Read(p)
}

func (c *combinedReader) Close() error {
	var errs []error
	if err := c.stdout.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := c.stderr.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// NewExecRuntime creates a new process-based runtime.
// workDir specifies the base directory for per-execution working directories.
// If empty, os.TempDir() is used.
func NewExecRuntime(workDir string) *ExecRuntime {
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "jobplane", "runner")
	}
	return &ExecRuntime{WorkDir: workDir}
}

// Start implements Runtime.Start using os/exec.
// It creates an isolated working directory for the execution and spawns the process.
func (e *ExecRuntime) Start(ctx context.Context, opts StartOptions) (Handle, error) {
	// Log that we're ignoring the image field (as per design decision)
	if opts.Image != "" {
		log.Printf("ExecRuntime: ignoring image field: %s", opts.Image)
	}

	// Validate command
	if len(opts.Command) == 0 {
		return nil, fmt.Errorf("command is required")
	}

	// Generate execution ID for working directory (extract from env if available)
	execID := uuid.New().String()
	if id, ok := opts.Env["JOBPLANE_EXECUTION_ID"]; ok && id != "" {
		execID = id
	}

	// Create isolated working directory for this execution
	workDir := filepath.Join(e.WorkDir, execID)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Create command with context
	cmd := exec.CommandContext(ctx, opts.Command[0], opts.Command[1:]...)
	cmd.Dir = workDir

	// Set up environment: inherit from host and add opts.Env
	cmd.Env = os.Environ()
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set up stdout/stderr pipes for log streaming
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(workDir)
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		os.RemoveAll(workDir)
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		os.RemoveAll(workDir)
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	return &ExecHandle{
		cmd:      cmd,
		stdout:   stdout,
		stderr:   stderr,
		combined: newCombinedReader(stdout, stderr),
		workDir:  workDir,
	}, nil
}

// Wait blocks until the process completes and returns the exit result.
func (h *ExecHandle) Wait(ctx context.Context) (ExitResult, error) {
	// Wait for the process to complete
	err := h.cmd.Wait()

	if err != nil {
		// Check for context cancellation (timeout)
		if ctx.Err() != nil {
			return ExitResult{ExitCode: -1, Error: ctx.Err()}, ctx.Err()
		}

		// Extract exit code from ExitError
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ExitResult{
				ExitCode: exitErr.ExitCode(),
				Error:    nil, // Non-zero exit is not an error, just a result
			}, nil
		}

		// Other error (e.g., process not started)
		return ExitResult{ExitCode: -1, Error: err}, err
	}

	// Success
	return ExitResult{ExitCode: 0}, nil
}

// Stop terminates the process gracefully (SIGTERM), then forcefully (SIGKILL) if needed.
func (h *ExecHandle) Stop(ctx context.Context) error {
	if h.cmd.Process == nil {
		return nil
	}

	// Send SIGTERM for graceful shutdown
	if err := h.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Process might have already exited
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait up to 5 seconds for graceful termination
	gracePeriod := 5 * time.Second
	done := make(chan struct{})

	go func() {
		h.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Process exited gracefully
		return nil
	case <-time.After(gracePeriod):
		// Grace period exceeded, force kill
		if err := h.cmd.Process.Kill(); err != nil {
			if errors.Is(err, os.ErrProcessDone) {
				return nil
			}
			return fmt.Errorf("failed to send SIGKILL: %w", err)
		}
		<-done // Wait for process to be reaped
		return nil
	case <-ctx.Done():
		// Context cancelled during stop, force kill immediately
		h.cmd.Process.Kill()
		return ctx.Err()
	}
}

// StreamLogs returns a reader that combines stdout and stderr.
func (h *ExecHandle) StreamLogs(ctx context.Context) (io.ReadCloser, error) {
	return h.combined, nil
}
