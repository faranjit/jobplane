// Package worker contains the worker-specific logic for job execution.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jobplane/internal/store"
	"jobplane/internal/worker/runtime"
	"jobplane/pkg/api"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// MockQueue implements store.Queue for testing.
type MockQueue struct {
	mu sync.Mutex

	// DequeueBatchFunc allows customizing DequeueBatch behavior per test.
	DequeueBatchFunc func(ctx context.Context, tenantIDs []uuid.UUID, limit int) ([]store.QueueItem, error)

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
	ExitCode    *int
	ErrMsg      string
}

func (m *MockQueue) Enqueue(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, payload json.RawMessage, visibleAfter time.Time) (int64, error) {
	return 0, nil
}

func (m *MockQueue) DequeueBatch(ctx context.Context, tenantIDs []uuid.UUID, limit int) ([]store.QueueItem, error) {
	return nil, nil
}

func (m *MockQueue) Complete(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, exitCode int) error {
	return nil
}

func (m *MockQueue) Fail(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, exitCode *int, errMsg string) error {
	return nil
}

func (m *MockQueue) SetVisibleAfter(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, visibleAfter time.Time) error {
	return nil
}

func (m *MockQueue) Count(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *MockQueue) CountRunningExecutions(ctx context.Context, tx store.DBTransaction, tenantID uuid.UUID) (int64, error) {
	return 0, nil
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
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *MockHandle) ResultDir() string {
	return "/"
}

func (m *MockHandle) Cleanup() error {
	return nil
}

// setupTestAgent creates a mocked controller server and an Agent configured to talk to it.
func setupTestAgent(t *testing.T, rt runtime.Runtime) *Agent {
	t.Helper()

	// mock api
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Cleanup(func() {
		server.Close()
	})

	agent := New(rt, AgentConfig{
		ControllerURL: server.URL,
	}, nil)
	return agent
}

// Test: New() Function
func TestNew_DefaultConcurrency(t *testing.T) {
	agent := New(&MockRuntime{}, AgentConfig{Concurrency: 0}, nil)

	if agent.config.Concurrency != 1 {
		t.Errorf("expected default concurrency=1, got %d", agent.config.Concurrency)
	}
}

func TestNew_DefaultConcurrency_Negative(t *testing.T) {
	agent := New(&MockRuntime{}, AgentConfig{Concurrency: -5}, nil)

	if agent.config.Concurrency != 1 {
		t.Errorf("expected default concurrency=1, got %d", agent.config.Concurrency)
	}
}

func TestNew_DefaultPollInterval(t *testing.T) {
	agent := New(&MockRuntime{}, AgentConfig{PollInterval: 0}, nil)

	if agent.config.PollInterval != 1*time.Second {
		t.Errorf("expected default poll interval=1s, got %v", agent.config.PollInterval)
	}
}

func TestNew_DefaultPollInterval_Negative(t *testing.T) {
	agent := New(&MockRuntime{}, AgentConfig{PollInterval: -5 * time.Second}, nil)

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

	agent := New(&MockRuntime{}, config, []uuid.UUID{tenantID})

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
	agent := New(&MockRuntime{}, AgentConfig{}, nil)

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
	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/executions/dequeue") {
			var req api.DequeueRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			resp := api.DequeueResponse{
				Executions: make([]api.DequeuedExecution, 0),
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	agent := New(&MockRuntime{}, AgentConfig{
		PollInterval:  10 * time.Millisecond,
		ControllerURL: server.URL,
	}, nil)

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
	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/executions/dequeue") {
			resp := api.DequeueResponse{
				Executions: make([]api.DequeuedExecution, 0),
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	agent := New(&MockRuntime{}, AgentConfig{
		PollInterval:  10 * time.Millisecond,
		ControllerURL: server.URL,
	}, nil)

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
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"echo"}})

	queue := &MockQueue{
		DequeueBatchFunc: func(ctx context.Context, tenantIDs []uuid.UUID, limit int) ([]store.QueueItem, error) {
			// Return 'limit' jobs
			items := make([]store.QueueItem, limit)
			for i := range limit {
				items[i] = store.QueueItem{ExecutionID: uuid.New(), Payload: payload}
			}
			return items, nil
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

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/executions/dequeue") {
			var req api.DequeueRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			queueItems, _ := queue.DequeueBatchFunc(r.Context(), req.TenantIDs, req.Limit)

			resp := api.DequeueResponse{
				Executions: make([]api.DequeuedExecution, 0, len(queueItems)),
			}

			for _, item := range queueItems {
				resp.Executions = append(resp.Executions, api.DequeuedExecution{
					ExecutionID: item.ExecutionID,
					Payload:     item.Payload,
				})
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	concurrencyLimit := 3
	agent := New(mockRuntime, AgentConfig{
		Concurrency:   concurrencyLimit,
		PollInterval:  10 * time.Millisecond,
		ControllerURL: server.URL,
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

	mu.Lock()
	finalMaxConcurrent := maxConcurrent
	mu.Unlock()

	if int(finalMaxConcurrent) > concurrencyLimit {
		t.Errorf("max concurrent jobs=%d exceeded limit=%d", finalMaxConcurrent, concurrencyLimit)
	}
}

func TestRun_GracefulDrainInFlight(t *testing.T) {
	var jobCompleted int32

	jobID := uuid.New()
	execID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"sleep"}})

	dequeueCount := 0
	queue := &MockQueue{
		DequeueBatchFunc: func(ctx context.Context, tenantIDs []uuid.UUID, limit int) ([]store.QueueItem, error) {
			dequeueCount++
			if dequeueCount == 1 {
				return []store.QueueItem{{ExecutionID: execID, Payload: payload}}, nil
			}
			return nil, nil // No more jobs
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

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/executions/dequeue") {
			var req api.DequeueRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			queueItems, _ := queue.DequeueBatchFunc(r.Context(), req.TenantIDs, req.Limit)

			resp := api.DequeueResponse{
				Executions: make([]api.DequeuedExecution, 0, len(queueItems)),
			}

			for _, item := range queueItems {
				resp.Executions = append(resp.Executions, api.DequeuedExecution{
					ExecutionID: item.ExecutionID,
					Payload:     item.Payload,
				})
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	agent := New(mockRuntime, AgentConfig{
		PollInterval:  10 * time.Millisecond,
		ControllerURL: server.URL,
	}, nil)

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

// Test: processItem() - Job Processing
func TestProcessItem_InvalidPayload(t *testing.T) {
	execID := uuid.New()
	invalidPayload := json.RawMessage(`{invalid json`)

	queue := &MockQueue{}

	var capturedExecID uuid.UUID

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			// Extract execution ID from path
			parts := strings.Split(r.URL.Path, "/")
			for i, p := range parts {
				if p == "executions" && i+1 < len(parts) {
					capturedExecID, _ = uuid.Parse(parts[i+1])
				}
			}

			var req api.ExecutionResultRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			queue.FailCalls = append(queue.FailCalls,
				FailCall{ExecutionID: capturedExecID, ExitCode: req.ExitCode, ErrMsg: req.Error},
			)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	agent := New(&MockRuntime{}, AgentConfig{
		ControllerURL: server.URL,
	}, nil)
	agent.processItem(context.Background(), execID, invalidPayload)

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
	if queue.FailCalls[0].ExecutionID != execID {
		t.Error("Fail called with wrong execution ID")
	}
}

func TestProcessItem_RuntimeStartError(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"echo"}})

	queue := &MockQueue{}

	var capturedExecID uuid.UUID

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			// Extract execution ID from path
			parts := strings.Split(r.URL.Path, "/")
			for i, p := range parts {
				if p == "executions" && i+1 < len(parts) {
					capturedExecID, _ = uuid.Parse(parts[i+1])
				}
			}

			var req api.ExecutionResultRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			queue.FailCalls = append(queue.FailCalls,
				FailCall{ExecutionID: capturedExecID, ExitCode: req.ExitCode, ErrMsg: req.Error},
			)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return nil, errors.New("failed to pull image")
		},
	}

	agent := New(mockRuntime, AgentConfig{
		ControllerURL: server.URL,
	}, nil)
	agent.processItem(context.Background(), execID, payload)

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
	if queue.FailCalls[0].ExecutionID != execID {
		t.Error("Fail called with wrong execution ID")
	}
}

func TestProcessItem_WaitError(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"echo"}})

	queue := &MockQueue{}

	var capturedExecID uuid.UUID

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			// Extract execution ID from path
			parts := strings.Split(r.URL.Path, "/")
			for i, p := range parts {
				if p == "executions" && i+1 < len(parts) {
					capturedExecID, _ = uuid.Parse(parts[i+1])
				}
			}

			var req api.ExecutionResultRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			queue.FailCalls = append(queue.FailCalls,
				FailCall{ExecutionID: capturedExecID, ExitCode: req.ExitCode, ErrMsg: req.Error},
			)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{ExitCode: -1}, errors.New("container crashed")
				},
			}, nil
		},
	}

	agent := New(mockRuntime, AgentConfig{
		ControllerURL: server.URL,
	}, nil)
	agent.processItem(context.Background(), execID, payload)

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
}

func TestProcessItem_Success(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"echo", "hello"}})

	queue := &MockQueue{}

	var capturedExecID uuid.UUID

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			// Extract execution ID from path
			parts := strings.Split(r.URL.Path, "/")
			for i, p := range parts {
				if p == "executions" && i+1 < len(parts) {
					capturedExecID, _ = uuid.Parse(parts[i+1])
				}
			}

			var req api.ExecutionResultRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			exitCode := -1
			if req.ExitCode != nil && *req.ExitCode == 0 {
				exitCode = 0
			}
			queue.CompleteCalls = append(queue.CompleteCalls,
				CompleteCall{ExecutionID: capturedExecID, ExitCode: exitCode},
			)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

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

	agent := New(mockRuntime, AgentConfig{
		ControllerURL: server.URL,
	}, nil)
	agent.processItem(context.Background(), execID, payload)

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

func TestProcessItem_FailedExitCode(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"exit", "1"}})

	queue := &MockQueue{}

	var capturedExecID uuid.UUID

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			// Extract execution ID from path
			parts := strings.Split(r.URL.Path, "/")
			for i, p := range parts {
				if p == "executions" && i+1 < len(parts) {
					capturedExecID, _ = uuid.Parse(parts[i+1])
				}
			}

			var req api.ExecutionResultRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			queue.FailCalls = append(queue.FailCalls,
				FailCall{ExecutionID: capturedExecID, ExitCode: req.ExitCode, ErrMsg: req.Error},
			)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{ExitCode: 1}, nil
				},
			}, nil
		},
	}

	agent := New(mockRuntime, AgentConfig{
		ControllerURL: server.URL,
	}, nil)
	agent.processItem(context.Background(), execID, payload)

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
	if queue.FailCalls[0].ErrMsg != "Exit code 1" {
		t.Errorf("expected 'Exit code 1', got '%s'", queue.FailCalls[0].ErrMsg)
	}
}

func TestProcessItem_FailedWithError(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"crash"}})

	var capturedExecID uuid.UUID

	queue := &MockQueue{}

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			// Extract execution ID from path
			parts := strings.Split(r.URL.Path, "/")
			for i, p := range parts {
				if p == "executions" && i+1 < len(parts) {
					capturedExecID, _ = uuid.Parse(parts[i+1])
				}
			}

			var req api.ExecutionResultRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			queue.FailCalls = append(queue.FailCalls,
				FailCall{ExecutionID: capturedExecID, ExitCode: req.ExitCode, ErrMsg: req.Error},
			)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

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

	agent := New(mockRuntime, AgentConfig{
		ControllerURL: server.URL,
	}, nil)
	agent.processItem(context.Background(), execID, payload)

	if len(queue.FailCalls) != 1 {
		t.Fatalf("expected 1 Fail call, got %d", len(queue.FailCalls))
	}
	if queue.FailCalls[0].ErrMsg != "OOMKilled" {
		t.Errorf("expected 'OOMKilled', got '%s'", queue.FailCalls[0].ErrMsg)
	}
	expectedExitCode := 137
	if queue.FailCalls[0].ExitCode == nil {
		t.Errorf("expected exitCode to be %d, but got nil", expectedExitCode)
	} else {
		gotCode := *queue.FailCalls[0].ExitCode
		if gotCode != expectedExitCode {
			t.Errorf("expected exitCode %d, got %d", expectedExitCode, gotCode)
		}
	}
}

// Test: Timeout Enforcement
func TestProcessItem_Timeout(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	// Job with 1 second timeout
	payload, _ := json.Marshal(store.Job{
		ID:             jobID,
		Image:          "test:latest",
		Command:        []string{"sleep", "infinity"},
		DefaultTimeout: 1, // 1 second timeout
	})

	queue := &MockQueue{}

	var capturedExecID uuid.UUID

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			// Extract execution ID from path
			parts := strings.Split(r.URL.Path, "/")
			for i, p := range parts {
				if p == "executions" && i+1 < len(parts) {
					capturedExecID, _ = uuid.Parse(parts[i+1])
				}
			}

			var req api.ExecutionResultRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			queue.FailCalls = append(queue.FailCalls,
				FailCall{ExecutionID: capturedExecID, ExitCode: req.ExitCode, ErrMsg: req.Error},
			)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

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

	agent := New(mockRuntime, AgentConfig{
		ControllerURL: server.URL,
	}, nil)

	start := time.Now()
	agent.processItem(context.Background(), execID, payload)
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

func TestProcessItem_TimeoutUsesJobDefault(t *testing.T) {
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

	agent := setupTestAgent(t, mockRuntime)
	agent.processItem(context.Background(), execID, payload)

	if capturedTimeout != customTimeout {
		t.Errorf("expected timeout=%d passed to runtime, got %d", customTimeout, capturedTimeout)
	}
}

func TestProcessItem_DefaultTimeoutWhenNotSet(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	// Job with NO timeout set (DefaultTimeout = 0)
	payload, _ := json.Marshal(store.Job{
		ID:      jobID,
		Image:   "test:latest",
		Command: []string{"echo"},
	})

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

	agent := setupTestAgent(t, mockRuntime)
	agent.processItem(context.Background(), execID, payload)

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

	var totalDequeued int32
	queue := &MockQueue{
		DequeueBatchFunc: func(ctx context.Context, tenantIDs []uuid.UUID, limit int) ([]store.QueueItem, error) {
			remaining := jobsToProcess - int(atomic.LoadInt32(&totalDequeued))
			if remaining <= 0 {
				return nil, nil
			}
			toDequeue := limit
			if toDequeue > remaining {
				toDequeue = remaining
			}
			atomic.AddInt32(&totalDequeued, int32(toDequeue))
			items := make([]store.QueueItem, toDequeue)
			for i := 0; i < toDequeue; i++ {
				items[i] = store.QueueItem{ExecutionID: uuid.New(), Payload: payload}
			}
			return items, nil
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

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/executions/dequeue") {
			var req api.DequeueRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			queueItems, _ := queue.DequeueBatchFunc(r.Context(), req.TenantIDs, req.Limit)

			resp := api.DequeueResponse{
				Executions: make([]api.DequeuedExecution, 0, len(queueItems)),
			}

			for _, item := range queueItems {
				resp.Executions = append(resp.Executions, api.DequeuedExecution{
					ExecutionID: item.ExecutionID,
					Payload:     item.Payload,
				})
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	agent := New(mockRuntime, AgentConfig{
		Concurrency:   5,
		PollInterval:  5 * time.Millisecond,
		ControllerURL: server.URL,
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

// Test: Heartbeat During Execution
func TestHeartbeat_RefreshesVisibilityDuringLongJob(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"sleep"}})

	var heartbeatCount int32
	var capturedExecID string
	var mu sync.Mutex

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/heartbeat") {
			mu.Lock()
			atomic.AddInt32(&heartbeatCount, 1)
			mu.Unlock()

			// Extract execution ID from path
			parts := strings.Split(r.URL.Path, "/")
			for i, p := range parts {
				if p == "executions" && i+1 < len(parts) {
					capturedExecID = parts[i+1]
				}
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		// Handle log shipping too (streamLogs calls sendLogs)
		if strings.Contains(r.URL.Path, "/logs") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					// Simulate a job that takes 150ms (longer than heartbeat interval of 50ms)
					time.Sleep(150 * time.Millisecond)
					return runtime.ExitResult{ExitCode: 0}, nil
				},
			}, nil
		},
	}

	agent := New(mockRuntime, AgentConfig{
		HeartbeatInterval: 50 * time.Millisecond, // Short interval for testing
		ControllerURL:     server.URL,
	}, nil)

	agent.processItem(context.Background(), execID, payload)

	// Heartbeat should have been called at least once (150ms job / 50ms interval = ~2 calls)
	count := atomic.LoadInt32(&heartbeatCount)
	if count < 1 {
		t.Errorf("expected at least 1 heartbeat call during long job, got %d", count)
	}

	if capturedExecID != execID.String() {
		t.Errorf("heartbeat sent for wrong execution: got %s, want %s", capturedExecID, execID.String())
	}
}

func TestHeartbeat_StopsWhenJobCompletes(t *testing.T) {
	execID := uuid.New()
	jobID := uuid.New()
	payload, _ := json.Marshal(store.Job{ID: jobID, Image: "test:latest", Command: []string{"echo"}})

	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					// Fast job - completes before heartbeat fires
					return runtime.ExitResult{ExitCode: 0}, nil
				},
			}, nil
		},
	}

	var heartbeatCount int32

	// Mock controller HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/heartbeat") {
			atomic.AddInt32(&heartbeatCount, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		// Handle log shipping too (streamLogs calls sendLogs)
		if strings.Contains(r.URL.Path, "/logs") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	agent := New(mockRuntime, AgentConfig{
		HeartbeatInterval: 5 * time.Second, // Long interval - job will complete before it fires
		ControllerURL:     server.URL,
	}, nil)

	agent.processItem(context.Background(), execID, payload)

	// Wait a bit to ensure no delayed heartbeat
	time.Sleep(50 * time.Millisecond)

	// No heartbeat should have been called for a fast job
	if heartbeatCount != 0 {
		t.Errorf("expected 0 heartbeat calls for fast job, got %d", heartbeatCount)
	}
}

func TestProcessItem_Metrics(t *testing.T) {
	// Setup Metrics
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	otel.SetMeterProvider(provider)

	// Mock Runtime that "runs" instantly
	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{ExitCode: 0}, nil
				},
			}, nil
		},
	}

	agent := setupTestAgent(t, mockRuntime)

	// Create Payload with tenant ID to verify attributes
	tenantID := uuid.New()
	job := store.Job{ID: uuid.New(), Name: "test-job", Image: "img", TenantID: tenantID}
	payloadBytes, _ := json.Marshal(job)

	// Run ProcessItem directly (bypassing the loop)
	agent.processItem(context.Background(), uuid.New(), payloadBytes)

	// Collect Metrics
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	foundHistogram := false
	foundCounter := false

	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "jobplane.worker.execution_duration_seconds":
				if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
					foundHistogram = true
					// Verify there's at least one data point
					if len(hist.DataPoints) == 0 {
						t.Error("Expected histogram to have data points")
					}
				}
			case "jobplane.executions.completed_total":
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					foundCounter = true
					// Verify counter has data points with correct attributes
					if len(sum.DataPoints) == 0 {
						t.Error("Expected counter to have data points")
					} else {
						// Check for status attribute
						dp := sum.DataPoints[0]
						hasStatus := false
						hasTenantID := false
						hasJobName := false
						for _, attr := range dp.Attributes.ToSlice() {
							switch attr.Key {
							case "status":
								hasStatus = true
								if attr.Value.AsString() != "success" {
									t.Errorf("Expected status='success', got '%s'", attr.Value.AsString())
								}
							case "tenant_id":
								hasTenantID = true
								if attr.Value.AsString() != tenantID.String() {
									t.Errorf("Expected tenant_id='%s', got '%s'", tenantID.String(), attr.Value.AsString())
								}
							case "job_name":
								hasJobName = true
								if attr.Value.AsString() != "test-job" {
									t.Errorf("Expected job_name='test-job', got '%s'", attr.Value.AsString())
								}
							}
						}
						if !hasStatus {
							t.Error("Expected counter to have 'status' attribute")
						}
						if !hasTenantID {
							t.Error("Expected counter to have 'tenant_id' attribute")
						}
						if !hasJobName {
							t.Error("Expected counter to have 'job_name' attribute")
						}
					}
				}
			}
		}
	}

	if !foundHistogram {
		t.Error("Expected metric 'jobplane.worker.execution_duration_seconds' to be recorded")
	}
	if !foundCounter {
		t.Error("Expected metric 'jobplane.executions.completed_total' to be recorded")
	}
}

func TestProcessItem_Metrics_Failure(t *testing.T) {
	// Setup Metrics
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	otel.SetMeterProvider(provider)

	// Mock Runtime that fails
	mockRuntime := &MockRuntime{
		StartFunc: func(ctx context.Context, opts runtime.StartOptions) (runtime.Handle, error) {
			return &MockHandle{
				WaitFunc: func(ctx context.Context) (runtime.ExitResult, error) {
					return runtime.ExitResult{ExitCode: 1}, nil // Non-zero exit code
				},
			}, nil
		},
	}

	agent := setupTestAgent(t, mockRuntime)

	job := store.Job{ID: uuid.New(), Name: "failing-job", Image: "img", TenantID: uuid.New()}
	payloadBytes, _ := json.Marshal(job)

	agent.processItem(context.Background(), uuid.New(), payloadBytes)

	// Collect Metrics
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "jobplane.executions.completed_total" {
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					if len(sum.DataPoints) > 0 {
						for _, attr := range sum.DataPoints[0].Attributes.ToSlice() {
							if attr.Key == "status" && attr.Value.AsString() != "failure" {
								t.Errorf("Expected status='failure' for failed execution, got '%s'", attr.Value.AsString())
							}
						}
					}
				}
			}
		}
	}
}

func TestAgent_doWithRetry(t *testing.T) {
	tests := []struct {
		name          string
		responses     []int
		expectError   bool
		expectedCalls int
		timeout       time.Duration
	}{
		{
			name:          "Success on First Try",
			responses:     []int{200},
			expectError:   false,
			expectedCalls: 1,
		},
		{
			name:          "Retry Once then Success (500 -> 200)",
			responses:     []int{500, 200},
			expectError:   false,
			expectedCalls: 2, // Should retry once
		},
		{
			name:          "Fail Fast on 400 (Bad Request)",
			responses:     []int{400},
			expectError:   true,
			expectedCalls: 1, // Should NOT retry
		},
		{
			name:          "Fail Fast on 401 (Unauthorized)",
			responses:     []int{401},
			expectError:   true,
			expectedCalls: 1, // Should NOT retry
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0

			// a mock server that returns different codes in sequence
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := callCount
				callCount++

				if count < len(tt.responses) {
					w.WriteHeader(tt.responses[count])
				} else {
					w.WriteHeader(500)
				}
			}))
			defer server.Close()

			agent := New(nil, AgentConfig{
				ControllerURL: server.URL,
			}, nil)

			// create context
			ctx := context.Background()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			// Run doWithRetry
			resp, err := agent.doWithRetry(ctx, http.MethodGet, server.URL, nil)

			// assertions
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp.Body != nil {
					resp.Body.Close()
				}
			}

			if callCount != tt.expectedCalls {
				t.Errorf("expected %d HTTP calls, got %d", tt.expectedCalls, callCount)
			}
		})
	}
}
