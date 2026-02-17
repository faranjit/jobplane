package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type mockCallbackStore struct {
	mu         sync.Mutex
	lastID     uuid.UUID
	lastStatus string
	callCount  int32
	err        error
}

func (m *mockCallbackStore) UpdateCallbackStatus(_ context.Context, id uuid.UUID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastID = id
	m.lastStatus = status
	m.callCount++
	return m.err
}

func (m *mockCallbackStore) CallCount() int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *mockCallbackStore) LastStatus() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastStatus
}

func (m *mockCallbackStore) LastID() uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastID
}

// helper to build a test execution with a callback URL pointing at the test server
func testExecution(url string) *store.Execution {
	exitCode := 0
	startedAt := time.Now().Add(-2 * time.Minute)
	completedAt := time.Now().Add(-1 * time.Minute)
	callbackStatus := store.CallbackStatusPending

	return &store.Execution{
		ID:              uuid.New(),
		JobID:           uuid.New(),
		TenantID:        uuid.New(),
		Status:          store.ExecutionStatusCompleted,
		Priority:        50,
		ExitCode:        &exitCode,
		CallbackURL:     &url,
		CallbackHeaders: json.RawMessage(`{"X-Api-Key":"test-secret"}`),
		CallbackStatus:  &callbackStatus,
		Result:          json.RawMessage(`{"accuracy": 0.95}`),
		StartedAt:       &startedAt,
		CompletedAt:     &completedAt,
		CreatedAt:       time.Now(),
	}
}

// Override backoffs for testing
func init() {
	retryBackoffs = []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
	}
}

func TestDeliver_Success(t *testing.T) {
	var receivedBody []byte
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	execution := testExecution(server.URL)
	dispatcher.Deliver(context.Background(), execution)
	dispatcher.Shutdown(context.Background())

	// Verify store was updated with DELIVERED
	if mockStore.LastStatus() != store.CallbackStatusDelivered {
		t.Errorf("got status %q, want %q", mockStore.LastStatus(), store.CallbackStatusDelivered)
	}
	if mockStore.LastID() != execution.ID {
		t.Errorf("got ID %v, want %v", mockStore.LastID(), execution.ID)
	}

	// Verify payload
	var payload api.WebhookPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload.Event != "execution.completed" {
		t.Errorf("got event %q, want %q", payload.Event, "execution.completed")
	}
	if payload.ExecutionID != execution.ID.String() {
		t.Errorf("got execution_id %q, want %q", payload.ExecutionID, execution.ID.String())
	}
	if payload.JobID != execution.JobID.String() {
		t.Errorf("got job_id %q, want %q", payload.JobID, execution.JobID.String())
	}
	if payload.Status != string(store.ExecutionStatusCompleted) {
		t.Errorf("got status %q, want %q", payload.Status, store.ExecutionStatusCompleted)
	}
	if payload.Duration <= 0 {
		t.Errorf("expected positive duration, got %d", payload.Duration)
	}

	// Verify custom headers were sent
	if receivedHeaders.Get("X-Api-Key") != "test-secret" {
		t.Errorf("got X-Api-Key %q, want %q", receivedHeaders.Get("X-Api-Key"), "test-secret")
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("got Content-Type %q, want %q", receivedHeaders.Get("Content-Type"), "application/json")
	}
	if receivedHeaders.Get("User-Agent") != "jobplane-webhook/1.0" {
		t.Errorf("got User-Agent %q, want %q", receivedHeaders.Get("User-Agent"), "jobplane-webhook/1.0")
	}
}

func TestDeliver_NilCallbackURL(t *testing.T) {
	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	execution := &store.Execution{
		ID:     uuid.New(),
		Status: store.ExecutionStatusCompleted,
	}

	dispatcher.Deliver(context.Background(), execution)
	dispatcher.Shutdown(context.Background())

	if mockStore.CallCount() != 0 {
		t.Error("store should not be called when callback URL is nil")
	}
}

func TestDeliver_EmptyCallbackURL(t *testing.T) {
	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	emptyURL := ""
	execution := &store.Execution{
		ID:          uuid.New(),
		Status:      store.ExecutionStatusCompleted,
		CallbackURL: &emptyURL,
	}

	dispatcher.Deliver(context.Background(), execution)
	dispatcher.Shutdown(context.Background())

	if mockStore.CallCount() != 0 {
		t.Error("store should not be called when callback URL is empty")
	}
}

func TestDeliver_FailedExecution_EventType(t *testing.T) {
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	execution := testExecution(server.URL)
	execution.Status = store.ExecutionStatusFailed

	dispatcher.Deliver(context.Background(), execution)
	dispatcher.Shutdown(context.Background())

	var payload api.WebhookPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload.Event != "execution.failed" {
		t.Errorf("got event %q, want %q", payload.Event, "execution.failed")
	}
}

// --- Retry behavior tests ---
func TestDeliver_RetriesOnServerError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	execution := testExecution(server.URL)
	dispatcher.Deliver(context.Background(), execution)
	dispatcher.Shutdown(context.Background())

	// Should have made 3 attempts (2 failures + 1 success)
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("got %d attempts, want 3", atomic.LoadInt32(&attempts))
	}
	if mockStore.LastStatus() != store.CallbackStatusDelivered {
		t.Errorf("got status %q, want %q", mockStore.LastStatus(), store.CallbackStatusDelivered)
	}
	if mockStore.CallCount() != 1 {
		t.Errorf("got %d calls, want 1", mockStore.CallCount())
	}
}

func TestDeliver_ExhaustsRetries(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	execution := testExecution(server.URL)
	dispatcher.Deliver(context.Background(), execution)
	dispatcher.Shutdown(context.Background())

	// Should have made maxRetries + 1 attempts (initial + 3 retries)
	expectedAttempts := int32(maxRetries + 1)
	if atomic.LoadInt32(&attempts) != expectedAttempts {
		t.Errorf("got %d attempts, want %d", atomic.LoadInt32(&attempts), expectedAttempts)
	}
	if mockStore.LastStatus() != store.CallbackStatusFailed {
		t.Errorf("got status %q, want %q", mockStore.LastStatus(), store.CallbackStatusFailed)
	}
}

func TestDeliver_UnreachableURL(t *testing.T) {
	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	execution := testExecution("http://127.0.0.1:1") // port 1 = unreachable
	dispatcher.Deliver(context.Background(), execution)
	dispatcher.Shutdown(context.Background())

	if mockStore.LastStatus() != store.CallbackStatusFailed {
		t.Errorf("got status %q, want %q", mockStore.LastStatus(), store.CallbackStatusFailed)
	}
}

func TestDeliver_NonRetryableError_StopsImmediately(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest) // 400 = non-retryable
	}))
	defer server.Close()

	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	execution := testExecution(server.URL)
	dispatcher.Deliver(context.Background(), execution)
	dispatcher.Shutdown(context.Background())

	// Should have made only 1 attempt — no retries for 4xx
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("got %d attempts, want 1 (should not retry 4xx)", atomic.LoadInt32(&attempts))
	}
	if mockStore.LastStatus() != store.CallbackStatusFailed {
		t.Errorf("got status %q, want %q", mockStore.LastStatus(), store.CallbackStatusFailed)
	}
}

func TestDoPost_RetryableFlag(t *testing.T) {
	tests := []struct {
		code      int
		retryable bool
	}{
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true}, // Too Many Requests
		{500, true},
		{502, true},
		{503, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
			}))
			defer server.Close()

			dispatcher := NewDispatcher(&mockCallbackStore{})
			err := dispatcher.doPost(server.URL, []byte(`{}`), nil)
			if err == nil {
				t.Fatalf("expected error for status %d", tt.code)
			}

			var de *deliveryError
			if !errors.As(err, &de) {
				t.Fatalf("expected *deliveryError, got %T", err)
			}
			if de.Retryable != tt.retryable {
				t.Errorf("status %d: got retryable=%v, want %v", tt.code, de.Retryable, tt.retryable)
			}
			if de.StatusCode != tt.code {
				t.Errorf("got StatusCode=%d, want %d", de.StatusCode, tt.code)
			}
		})
	}
}

// --- doPost tests ---
func TestDoPost_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dispatcher := NewDispatcher(&mockCallbackStore{})

	err := dispatcher.doPost(server.URL, []byte(`{}`), nil)
	if err != nil {
		t.Errorf("doPost failed: %v", err)
	}
}

func TestDoPost_Non2xx(t *testing.T) {
	codes := []int{400, 401, 403, 404, 500, 502, 503}

	for _, code := range codes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			dispatcher := NewDispatcher(&mockCallbackStore{})
			err := dispatcher.doPost(server.URL, []byte(`{}`), nil)
			if err == nil {
				t.Errorf("expected error for status %d, got nil", code)
			}
		})
	}
}

func TestDoPost_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dispatcher := NewDispatcher(&mockCallbackStore{})
	headers := map[string]string{
		"Authorization": "Bearer my-token",
		"X-Custom":      "custom-value",
	}

	err := dispatcher.doPost(server.URL, []byte(`{}`), headers)
	if err != nil {
		t.Fatalf("doPost failed: %v", err)
	}

	if receivedHeaders.Get("Authorization") != "Bearer my-token" {
		t.Errorf("got Authorization %q, want %q", receivedHeaders.Get("Authorization"), "Bearer my-token")
	}
	if receivedHeaders.Get("X-Custom") != "custom-value" {
		t.Errorf("got X-Custom %q, want %q", receivedHeaders.Get("X-Custom"), "custom-value")
	}
}

// --- buildPayload tests ---
func TestBuildPayload_CompletedExecution(t *testing.T) {
	dispatcher := NewDispatcher(&mockCallbackStore{})
	execution := testExecution("http://example.com")

	data := dispatcher.buildPayload(execution)

	var payload api.WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if payload.Event != "execution.completed" {
		t.Errorf("got event %q, want %q", payload.Event, "execution.completed")
	}
	if *payload.ExitCode != 0 {
		t.Errorf("got exit_code %d, want 0", *payload.ExitCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(payload.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result["accuracy"] != 0.95 {
		t.Errorf("got accuracy %v, want 0.95", result["accuracy"])
	}
}

func TestBuildPayload_FailedExecution(t *testing.T) {
	dispatcher := NewDispatcher(&mockCallbackStore{})
	execution := testExecution("http://example.com")
	execution.Status = store.ExecutionStatusFailed

	data := dispatcher.buildPayload(execution)

	var payload api.WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if payload.Event != "execution.failed" {
		t.Errorf("got event %q, want %q", payload.Event, "execution.failed")
	}
}

func TestBuildPayload_NoDuration(t *testing.T) {
	dispatcher := NewDispatcher(&mockCallbackStore{})
	execution := testExecution("http://example.com")
	execution.StartedAt = nil
	execution.CompletedAt = nil

	data := dispatcher.buildPayload(execution)

	var payload api.WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if payload.Duration != 0 {
		t.Errorf("expected 0 duration when times are nil, got %d", payload.Duration)
	}
}

// --- Concurrency test ---
func TestDeliver_ConcurrentDeliveries(t *testing.T) {
	var activeCount int32
	var maxActive int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&activeCount, 1)
		defer atomic.AddInt32(&activeCount, -1)

		// Track peak concurrency
		for {
			old := atomic.LoadInt32(&maxActive)
			if current <= old || atomic.CompareAndSwapInt32(&maxActive, old, current) {
				break
			}
		}

		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	// Fire 20 deliveries
	for range 20 {
		execution := testExecution(server.URL)
		dispatcher.Deliver(context.Background(), execution)
	}
	dispatcher.Shutdown(context.Background())

	// maxConcurrent = 10, so peak should not exceed it
	if atomic.LoadInt32(&maxActive) > int32(maxConcurrent) {
		t.Errorf("peak concurrency %d exceeded max %d", atomic.LoadInt32(&maxActive), maxConcurrent)
	}
}

// --- Shutdown test ---
func TestShutdown_WaitsForInFlight(t *testing.T) {
	requestDone := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-requestDone // block until test says go
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockStore := &mockCallbackStore{}
	dispatcher := NewDispatcher(mockStore)

	execution := testExecution(server.URL)
	dispatcher.Deliver(context.Background(), execution)

	// Let the request complete
	shutdownDone := make(chan struct{})
	go func() {
		dispatcher.Shutdown(context.Background())
		close(shutdownDone)
	}()

	// Shutdown should be blocked
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned before delivery completed")
	case <-time.After(50 * time.Millisecond):
		// Expected — still blocked
	}

	// Unblock the request
	close(requestDone)

	// Now Shutdown should complete
	select {
	case <-shutdownDone:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return after delivery completed")
	}

	if mockStore.LastStatus() != store.CallbackStatusDelivered {
		t.Errorf("got status %q, want %q", mockStore.LastStatus(), store.CallbackStatusDelivered)
	}
}
