package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewExecRuntime_DefaultWorkDir(t *testing.T) {
	rt := NewExecRuntime("")

	expectedPrefix := filepath.Join(os.TempDir(), "jobplane", "runner")
	if rt.WorkDir != expectedPrefix {
		t.Errorf("expected WorkDir to be %s, got %s", expectedPrefix, rt.WorkDir)
	}
}

func TestNewExecRuntime_CustomWorkDir(t *testing.T) {
	customDir := "/custom/path"
	rt := NewExecRuntime(customDir)

	if rt.WorkDir != customDir {
		t.Errorf("expected WorkDir to be %s, got %s", customDir, rt.WorkDir)
	}
}

func TestStart_Success(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	ctx := context.Background()
	handle, err := rt.Start(ctx, StartOptions{
		Command: []string{"echo", "hello"},
		Env:     map[string]string{"JOBPLANE_EXECUTION_ID": "test-exec-123"},
	})

	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if handle == nil {
		t.Fatal("expected handle to be non-nil")
	}

	// Clean up
	result, _ := handle.Wait(ctx)
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestStart_EmptyCommand(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	ctx := context.Background()
	_, err := rt.Start(ctx, StartOptions{
		Command: []string{},
	})

	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if !strings.Contains(err.Error(), "command is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_CommandNotFound(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	ctx := context.Background()
	_, err := rt.Start(ctx, StartOptions{
		Command: []string{"nonexistent-binary-xyz"},
	})

	if err == nil {
		t.Fatal("expected error for non-existent command")
	}
}

func TestStart_CreatesWorkDir(t *testing.T) {
	baseDir := t.TempDir()
	rt := NewExecRuntime(baseDir)
	execID := "test-workdir-creation"

	ctx := context.Background()
	handle, err := rt.Start(ctx, StartOptions{
		Command: []string{"echo", "test"},
		Env:     map[string]string{"JOBPLANE_EXECUTION_ID": execID},
	})

	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify work directory was created
	expectedDir := filepath.Join(baseDir, execID)
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("work directory was not created: %s", expectedDir)
	}

	handle.Wait(ctx)
}

func TestWait_ExitCodeZero(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	ctx := context.Background()
	handle, err := rt.Start(ctx, StartOptions{
		Command: []string{"true"}, // exits with 0
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	result, err := handle.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}
}

func TestWait_ExitCodeNonZero(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	ctx := context.Background()
	handle, err := rt.Start(ctx, StartOptions{
		Command: []string{"false"}, // exits with 1
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	result, err := handle.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}

func TestWait_ContextCancellation(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	// Create a context that will time out quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	handle, err := rt.Start(ctx, StartOptions{
		Command: []string{"sleep", "10"}, // Long-running command
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	result, err := handle.Wait(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	if result.ExitCode != -1 {
		t.Errorf("expected exit code -1 on timeout, got %d", result.ExitCode)
	}
}

func TestStop_GracefulTermination(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	ctx := context.Background()
	handle, err := rt.Start(ctx, StartOptions{
		Command: []string{"sleep", "30"},
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give the process a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop should terminate the process gracefully
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err = handle.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestStreamLogs_CapturesOutput(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	ctx := context.Background()
	handle, err := rt.Start(ctx, StartOptions{
		Command: []string{"echo", "hello world"},
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	reader, err := handle.StreamLogs(ctx)
	if err != nil {
		t.Fatalf("StreamLogs failed: %v", err)
	}

	// Read all output
	buf := make([]byte, 1024)
	n, _ := reader.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "hello world") {
		t.Errorf("expected output to contain 'hello world', got: %s", output)
	}

	handle.Wait(ctx)
}

func TestStart_PassesEnvironment(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	ctx := context.Background()
	handle, err := rt.Start(ctx, StartOptions{
		Command: []string{"sh", "-c", "echo $JOBPLANE_TEST_VAR"},
		Env: map[string]string{
			"JOBPLANE_EXECUTION_ID": "env-test",
			"JOBPLANE_TEST_VAR":     "custom-value",
		},
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	reader, err := handle.StreamLogs(ctx)
	if err != nil {
		t.Fatalf("StreamLogs failed: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := reader.Read(buf)
	output := strings.TrimSpace(string(buf[:n]))

	if output != "custom-value" {
		t.Errorf("expected 'custom-value', got: '%s'", output)
	}

	handle.Wait(ctx)
}

func TestStart_ImageFieldIgnored(t *testing.T) {
	rt := NewExecRuntime(t.TempDir())

	ctx := context.Background()
	// Image field should be ignored without error
	handle, err := rt.Start(ctx, StartOptions{
		Image:   "python:3.9-slim", // This should be logged but ignored
		Command: []string{"echo", "works"},
	})

	if err != nil {
		t.Fatalf("Start failed with image field: %v", err)
	}

	result, _ := handle.Wait(ctx)
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}
