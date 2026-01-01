// Package worker contains the worker-specific logic for job execution.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jobplane/internal/store"
	"jobplane/internal/worker/runtime"

	"github.com/google/uuid"
)

// MockQueue implements store.Queue for testing.
type MockQueue struct {
	mu sync.Mutex

	// DequeueFunc allows customizing Dequeue behavior per test.
	DequeueFunc func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error)

	// Track method calls
	CompleteCalls []CompleteCall
	FailCalls     []FailCall
}

type CompleteCall struct {
	ExecutionID uuid.UUID
	ExitCode    int
}

type FailCall struct {
	ExecutionID uuid.UUID
	ErrMsg      string
}

func (m *MockQueue) Enqueue(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, payload json.RawMessage) (int64, error) {
	return 0, nil
}

func (m *MockQueue) Dequeue(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
	if m.DequeueFunc != nil {
		return m.DequeueFunc(ctx, tenantIDs)
	}
	return uuid.Nil, nil, errors.New("no job available")
}

func (m *MockQueue) Complete(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, exitCode int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CompleteCalls = append(m.CompleteCalls, CompleteCall{ExecutionID: executionID, ExitCode: exitCode})
	return nil
}

func (m *MockQueue) Fail(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FailCalls = append(m.FailCalls, FailCall{ExecutionID: executionID, ErrMsg: errMsg})
	return nil
}

func (m *MockQueue) SetVisibleAfter(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, visibleAfter time.Time) error {
	return nil
}

// MockRuntime implements runtime.Runtime for testing.
type MockRuntime struct {
	StartFunc func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error)
}

func (m *MockRuntime) Start(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
	if m.StartFunc != nil {
		return m.StartFunc(ctx, opts)
	}
	return &MockHandle{}, nil
}

// MockHandle implements runtime.Handle for testing.
type MockHandle struct {
	WaitFunc func(ctx context.Context) (runtime.ExitResult, error)
	StopFunc func(ctx context.Context) error
}

func (m *MockHandle) Wait(ctx context.Context) (runtime.ExitResult, error) {
	if m.WaitFunc != nil {
		return m.WaitFunc(ctx)
	}
	return runtime.ExitResult{ExitCode: 0}, nil
}

func (m *MockHandle) Stop(ctx context.Context) error {
	if m.StopFunc != nil {
		return m.StopFunc(ctx)
	}
	return nil
}

func (m *MockHandle) StreamLogs(ctx context.Context) (io.ReadCloser, error) {
	return nil, nil
}

// Test: New() Function
func TestNew_DefaultConcurrency(t *testing.T) {
	agent := New(&MockQueue{}, &MockRuntime{}, AgentConfig{Concurrency: 0}, nil)

	if agent.config.Concurrency != 1 {
		t.Errorf("expected default concurrency=1, got %d", agent.config.Concurrency)
	}
}

func TestNew_DefaultConcurrency_Negative(t *testing.T) {
	agent := New(&MockQueue{}, &MockRuntime{}, AgentConfig{Concurrency: -5}, nil)

	if agent.config.Concurrency != 1 {
		t.Errorf("expected default concurrency=1, got %d", agent.config.Concurrency)
	}
}

func TestNew_DefaultPollInterval(t *testing.T) {
	agent := New(&MockQueue{}, &MockRuntime{}, AgentConfig{PollInterval: 0}, nil)

	if agent.config.PollInterval != 1*time.Second {
		t.Errorf("expected default poll interval=1s, got %v", agent.config.PollInterval)
	}
}

func TestNew_DefaultPollInterval_Negative(t *testing.T) {
	agent := New(&MockQueue{}, &MockRuntime{}, AgentConfig{PollInterval: -5 * time.Second}, nil)

	if agent.config.PollInterval != 1*time.Second {
		t.Errorf("expected default poll interval=1s, got %v", agent.config.PollInterval)
	}
}

func TestNew_CustomConfig(t *testing.T) {
	tenantID := uuid.New()
	config := AgentConfig{
		ID:           "test-agent",
		Concurrency:  5,
		PollInterval: 500 * time.Millisecond,
	}

	agent := New(&MockQueue{}, &MockRuntime{}, config, []uuid.UUID{tenantID})

	if agent.config.ID != "test-agent" {
		t.Errorf("expected ID='test-agent', got '%s'", agent.config.ID)
	}
	if agent.config.Concurrency != 5 {
		t.Errorf("expected concurrency=5, got %d", agent.config.Concurrency)
	}
	if agent.config.PollInterval != 500*time.Millisecond {
		t.Errorf("expected poll interval=500ms, got %v", agent.config.PollInterval)
	}
	if len(agent.tenantIDs) != 1 || agent.tenantIDs[0] != tenantID {
		t.Errorf("expected tenantIDs to be set correctly")
	}
}

func TestNew_DoneChannelInitialized(t *testing.T) {
	agent := New(&MockQueue{}, &MockRuntime{}, AgentConfig{}, nil)

	if agent.done == nil {
		t.Error("expected done channel to be initialized")
	}

	// Verify it's not closed
	select {
	case <-agent.done:
		t.Error("done channel should not be closed initially")
	default:
		// Expected
	}
}

// Test: Run() Loop Behavior
func TestRun_GracefulShutdown(t *testing.T) {
	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return uuid.Nil, nil, errors.New("no job")
		},
	}

	agent := New(queue, &MockRuntime{}, AgentConfig{PollInterval: 10 * time.Millisecond}, nil)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.Run(ctx)
	}()

	// Let it run for a bit
	time.Sleep(50 * time.Millisecond)

	// Cancel and wait for graceful shutdown
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Run() did not exit in time")
	}
}

func TestRun_DoneChannelClosed(t *testing.T) {
	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return uuid.Nil, nil, errors.New("no job")
		},
	}

	agent := New(queue, &MockRuntime{}, AgentConfig{PollInterval: 10 * time.Millisecond}, nil)

	ctx, cancel := context.WithCancel(context.Background())

	go agent.Run(ctx)

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-agent.Done():
		// Success - channel was closed
	case <-time.After(1 * time.Second):
		t.Error("Done() channel was not closed after shutdown")
	}
}

func TestRun_ConcurrencyLimit(t *testing.T) {
	var runningJobs int32
	var maxConcurrent int32
	var mu sync.Mutex

	jobID := uuid.New()
	execID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"echo"}})

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, payload, nil
		},
	}

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			current := atomic.AddInt32(&runningJobs, 1)
			mu.Lock()
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					// Simulate work
					time.Sleep(100 * time.Millisecond)
					atomic.AddInt32(&runningJobs, -1)
					return runtime.ExitResult{ExitCode: 0}, nil
				},
			}, nil
		},
	}

	concurrencyLimit := 3
	agent := New(queue, mockRuntime, AgentConfig{
		Concurrency:  concurrencyLimit,
		PollInterval: 10 * time.Millisecond,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())

	go agent.Run(ctx)

	// Let jobs accumulate
	time.Sleep(300 * time.Millisecond)
	cancel()

	// Wait for shutdown
	select {
	case <-agent.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown timeout")
	}

	if int(maxConcurrent) > concurrencyLimit {
		t.Errorf("max concurrent jobs=%d exceeded limit=%d", maxConcurrent, concurrencyLimit)
	}
}

func TestRun_GracefulDrainInFlight(t *testing.T) {
	var jobCompleted int32

	jobID := uuid.New()
	execID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"sleep"}})

	dequeueCount := 0
	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			dequeueCount++
			if dequeueCount == 1 {
				return execID, payload, nil
			}
			return uuid.Nil, nil, errors.New("no more jobs")
		},
	}

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					// Long-running job
					time.Sleep(200 * time.Millisecond)
					atomic.StoreInt32(&jobCompleted, 1)
					return runtime.ExitResult{ExitCode: 0}, nil
				},
			}, nil
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{PollInterval: 10 * time.Millisecond}, nil)

	ctx, cancel := context.WithCancel(context.Background())

	go agent.Run(ctx)

	// Wait for job to start
	time.Sleep(50 * time.Millisecond)

	// Cancel while job is running
	cancel()

	// Run should wait for job to complete before returning
	select {
	case <-agent.Done():
		if atomic.LoadInt32(&jobCompleted) != 1 {
			t.Error("Run() returned before in-flight job completed")
		}
	case <-time.After(1 * time.Second):
		t.Error("shutdown timeout")
	}
}

// Test: processOne() - Job Processing
func TestProcessOne_NoJob(t *testing.T) {
	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return uuid.Nil, nil, errors.New("no job available")
		},
	}

	agent := New(queue, &MockRuntime{}, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if len(queue.CompleteCalls) != 0 || len(queue.FailCalls) != 0 {
		t.Error("expected no calls to Complete or Fail when no job available")
	}
}

func TestProcessOne_InvalidPayload(t *testing.T) {
	execID := uuid.New()
	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, json.RawMessage(`{invalid json`), nil
		},
	}

	agent := New(queue, &MockRuntime{}, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
	if queue.FailCalls[0].ExecutionID != execID {
		t.Error("Fail called with wrong execution ID")
	}
}

func TestProcessOne_RuntimeStartError(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"echo"}})

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, payload, nil
		},
	}

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return nil, errors.New("failed to pull image")
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
	if queue.FailCalls[0].ExecutionID != execID {
		t.Error("Fail called with wrong execution ID")
	}
}

func TestProcessOne_WaitError(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"echo"}})

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, payload, nil
		},
	}

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{ExitCode: -1}, errors.New("container crashed")
				},
			}, nil
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
}

func TestProcessOne_Success(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"echo", "hello"}})

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, payload, nil
		},
	}

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			// Verify options are passed correctly
			if opts.Image != "test:latest" {
				t.Errorf("expected image='test:latest', got '%s'", opts.Image)
			}
			if len(opts.Command) != 2 || opts.Command[0] != "echo" {
				t.Errorf("expected command=['echo', 'hello'], got %v", opts.Command)
			}
			if opts.Env["JOBPLANE_EXECUTION_ID"] != execID.String() {
				t.Error("JOBPLANE_EXECUTION_ID not set correctly")
			}
			if opts.Env["JOBPLANE_JOB_ID"] != jobID.String() {
				t.Error("JOBPLANE_JOB_ID not set correctly")
			}

			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{ExitCode: 0}, nil
				},
			}, nil
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if len(queue.CompleteCalls) != 1 {
		t.Fatalf("expected 1 Complete call, got %d", len(queue.CompleteCalls))
	}
	if queue.CompleteCalls[0].ExecutionID != execID {
		t.Error("Complete called with wrong execution ID")
	}
	if queue.CompleteCalls[0].ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", queue.CompleteCalls[0].ExitCode)
	}
}

func TestProcessOne_FailedExitCode(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"exit", "1"}})

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, payload, nil
		},
	}

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{ExitCode: 1}, nil
				},
			}, nil
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
	if queue.FailCalls[0].ErrMsg != "Exit code 1" {
		t.Errorf("expected 'Exit code 1', got '%s'", queue.FailCalls[0].ErrMsg)
	}
}

func TestProcessOne_FailedWithError(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"crash"}})

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, payload, nil
		},
	}

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{
						ExitCode: 137,
						Error:    errors.New("OOMKilled"),
					}, nil
				},
			}, nil
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
	if queue.FailCalls[0].ErrMsg != "OOMKilled" {
		t.Errorf("expected 'OOMKilled', got '%s'", queue.FailCalls[0].ErrMsg)
	}
}

// Test: Edge Cases
func TestProcessOne_EmptyTenantIDs(t *testing.T) {
	var capturedTenantIDs []uuid.UUID

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			capturedTenantIDs = tenantIDs
			return uuid.Nil, nil, errors.New("no job")
		},
	}

	agent := New(queue, &MockRuntime{}, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if capturedTenantIDs != nil {
		t.Error("expected nil tenantIDs to be passed through")
	}
}

func TestProcessOne_SpecificTenantIDs(t *testing.T) {
	var capturedTenantIDs []uuid.UUID
	tenantID1 := uuid.New()
	tenantID2 := uuid.New()

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			capturedTenantIDs = tenantIDs
			return uuid.Nil, nil, errors.New("no job")
		},
	}

	agent := New(queue, &MockRuntime{}, AgentConfig{}, []uuid.UUID{tenantID1, tenantID2})
	agent.processOne(context.Background())

	if len(capturedTenantIDs) != 2 {
		t.Fatalf("expected 2 tenant IDs, got %d", len(capturedTenantIDs))
	}
	if capturedTenantIDs[0] != tenantID1 || capturedTenantIDs[1] != tenantID2 {
		t.Error("tenant IDs not passed correctly")
	}
}

// Test: Timeout Enforcement
func TestProcessOne_Timeout(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	// Job with 1 second timeout
	payload, _ := json.Marshal(store.Job{
		ID:             jobID,
		Image:          "test:latest",
		Command:        []string{"sleep", "infinity"},
		DefaultTimeout: 1, // 1 second timeout
	})

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, payload, nil
		},
	}

	var stopCalled int32

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					// Block until context times out
					<-ctx.Done()
					return runtime.ExitResult{ExitCode: -1, Error: ctx.Err()}, ctx.Err()
				},
				StopFunc: func(ctx context.Context) error {
					atomic.AddInt32(&stopCalled, 1)
					return nil
				},
			}, nil
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{}, nil)

	start := time.Now()
	agent.processOne(context.Background())
	elapsed := time.Since(start)

	// Should complete around 1 second (the timeout)
	if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("expected ~1s elapsed for timeout, got %v", elapsed)
	}

	// Fail should be called with timeout message
	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
	if queue.FailCalls[0].ExecutionID != execID {
		t.Error("Fail called with wrong execution ID")
	}
	if !strings.Contains(queue.FailCalls[0].ErrMsg, "timed out") {
		t.Errorf("expected timeout error message, got '%s'", queue.FailCalls[0].ErrMsg)
	}

	// Stop should be called to terminate the container
	if atomic.LoadInt32(&stopCalled) != 1 {
		t.Error("expected Stop to be called on timeout")
	}
}

func TestProcessOne_TimeoutUsesJobDefault(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	customTimeout := 2 // 2 seconds
	payload, _ := json.Marshal(store.Job{
		ID:             jobID,
		Image:          "test:latest",
		Command:        []string{"echo"},
		DefaultTimeout: customTimeout,
	})

	var capturedTimeout int

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, payload, nil
		},
	}

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			capturedTimeout = opts.Timeout
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{ExitCode: 0}, nil
				},
			}, nil
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if capturedTimeout != customTimeout {
		t.Errorf("expected timeout=%d passed to runtime, got %d", customTimeout, capturedTimeout)
	}
}

func TestProcessOne_DefaultTimeoutWhenNotSet(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	// Job with NO timeout set (DefaultTimeout = 0)
	payload, _ := json.Marshal(store.Job{
		ID:      jobID,
		Image:   "test:latest",
		Command: []string{"echo"},
	})

	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			return execID, payload, nil
		},
	}

	var contextDeadline time.Time
	var hasDeadline bool

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			contextDeadline, hasDeadline = ctx.Deadline()
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{ExitCode: 0}, nil
				},
			}, nil
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{}, nil)
	agent.processOne(context.Background())

	if !hasDeadline {
		t.Error("expected context to have a deadline (default timeout)")
	}

	// Default is 30 minutes, so deadline should be ~30 mins from now
	expectedDeadline := time.Now().Add(30 * time.Minute)
	if contextDeadline.Before(expectedDeadline.Add(-1*time.Minute)) || contextDeadline.After(expectedDeadline.Add(1*time.Minute)) {
		t.Errorf("expected deadline around 30 minutes, got %v", contextDeadline)
	}
}

func TestRun_MultipleConcurrentJobs(t *testing.T) {
	var processedJobs int32
	jobsToProcess := 10

	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"work"}})

	var dequeueCount int32
	queue := &MockQueue{
		DequeueFunc: func(ctx context.Context, tenantIDs []uuid.UUID) (uuid.UUID, json.RawMessage, error) {
			count := atomic.AddInt32(&dequeueCount, 1)
			if int(count) <= jobsToProcess {
				return uuid.New(), payload, nil
			}
			return uuid.Nil, nil, errors.New("no more jobs")
		},
	}

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					time.Sleep(20 * time.Millisecond)
					atomic.AddInt32(&processedJobs, 1)
					return runtime.ExitResult{ExitCode: 0}, nil
				},
			}, nil
		},
	}

	agent := New(queue, mockRuntime, AgentConfig{
		Concurrency:  5,
		PollInterval: 5 * time.Millisecond,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())

	go agent.Run(ctx)

	// Wait for all jobs to be processed
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&processedJobs) >= int32(jobsToProcess) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-agent.Done()

	if processed := atomic.LoadInt32(&processedJobs); int(processed) < jobsToProcess {
		t.Errorf("expected at least %d jobs processed, got %d", jobsToProcess, processed)
	}
}
