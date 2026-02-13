package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"jobplane/internal/controller/middleware"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGetExecution(t *testing.T) {
	tenantID := uuid.New()
	executionID := uuid.New()
	otherTenantID := uuid.New()

	validExecution := &store.Execution{
		ID:       executionID,
		TenantID: tenantID,
		Status:   store.ExecutionStatusPending,
	}

	tests := []struct {
		name           string
		executionID    string
		mockSetup      func(*mockStore)
		expectedStatus int
	}{
		{
			name:        "Success",
			executionID: executionID.String(),
			mockSetup: func(m *mockStore) {
				m.getExecutionResp = validExecution
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid UUID",
			executionID:    "not-a-uuid",
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Execution Not Found",
			executionID: executionID.String(),
			mockSetup: func(m *mockStore) {
				m.getExecutionErr = errors.New("db error or not found")
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Access Denied to Different Tenant",
			executionID: executionID.String(),
			mockSetup: func(m *mockStore) {
				m.getExecutionResp = &store.Execution{
					ID:       executionID,
					TenantID: otherTenantID, // Mismatch
				}
			},
			expectedStatus: http.StatusNotFound, // Security: Mask as 404
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mock := &mockStore{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			h := New(mock, HandlerConfig{})

			// Request
			mux := http.NewServeMux()
			mux.HandleFunc("GET /executions/{id}", h.GetExecution)

			req := httptest.NewRequest(http.MethodGet, "/executions/"+tt.executionID, nil)
			rr := httptest.NewRecorder()

			// Inject tenant to context
			ctx := middleware.NewContextWithTenant(req.Context(), &store.Tenant{ID: tenantID})
			req = req.WithContext(ctx)

			// Execute
			mux.ServeHTTP(rr, req)

			// Assertions
			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %d want %d", rr.Code, tt.expectedStatus)
			}
		})
	}
}

func TestInternalDequeue(t *testing.T) {
	tenantID := uuid.New()
	execID := uuid.New()
	payload := json.RawMessage(`{"job":{"id":"` + uuid.New().String() + `"}}`)

	tests := []struct {
		name           string
		reqBody        any
		mockSetup      func(*mockStore)
		expectedStatus int
		expectCount    int
	}{
		{
			name: "Success - Returns Claimed Executions",
			reqBody: api.DequeueRequest{
				Limit:     5,
				TenantIDs: []uuid.UUID{tenantID},
			},
			mockSetup: func(m *mockStore) {
				m.dequeueBatchResp = []store.QueueItem{
					{ExecutionID: execID, Payload: payload},
					{ExecutionID: execID, Payload: payload},
				}
			},
			expectedStatus: http.StatusOK,
			expectCount:    2,
		},
		{
			name: "Success - Empty Queue (Returns 200 with 0 items)",
			reqBody: api.DequeueRequest{
				Limit: 5,
			},
			mockSetup: func(m *mockStore) {
				m.dequeueBatchResp = []store.QueueItem{}
			},
			expectedStatus: http.StatusOK,
			expectCount:    0,
		},
		{
			name:    "Invalid JSON Body",
			reqBody: "this-is-not-json",
			mockSetup: func(m *mockStore) {
				// Should not reach the store
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Store Error - Returns 500",
			reqBody: api.DequeueRequest{
				Limit: 1,
			},
			mockSetup: func(m *mockStore) {
				m.dequeueBatchErr = errors.New("database connection lost")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			h := New(mock, HandlerConfig{})

			var bodyBytes []byte
			if str, ok := tt.reqBody.(string); ok {
				bodyBytes = []byte(str)
			} else {
				bodyBytes, _ = json.Marshal(tt.reqBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/internal/executions/dequeue", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			h.InternalDequeue(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// For successful responses, verify the payload structure
			if rr.Code == http.StatusOK {
				var resp api.DequeueResponse
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if len(resp.Executions) != tt.expectCount {
					t.Errorf("expected %d executions, got %d", tt.expectCount, len(resp.Executions))
				}

				// If we expected items, do a quick sanity check on the ID
				if tt.expectCount > 0 {
					if resp.Executions[0].ExecutionID != execID {
						t.Errorf("expected execution ID %s, got %s", execID, resp.Executions[0].ExecutionID)
					}
				}
			}
		})
	}
}

func TestInternalHeartbeat(t *testing.T) {
	executionID := uuid.New()

	tests := []struct {
		name           string
		executionID    string
		mockSetup      func(*mockStore)
		expectedStatus int
		verify         func(time.Time, int16)
	}{
		{
			name:           "Success",
			executionID:    executionID.String(),
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusOK,
			verify: func(newVisibility time.Time, atLeastDiff int16) {
				diff := time.Until(newVisibility).Minutes()
				if int16(diff) < atLeastDiff-1 { // play safe because of int conversion
					t.Errorf("handler updated the visibility wrong: got %v want %v", newVisibility, newVisibility)
				}
			},
		},
		{
			name:           "Invalid UUID",
			executionID:    "invalid",
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Store Error",
			executionID: executionID.String(),
			mockSetup: func(m *mockStore) {
				m.setVisibleErr = errors.New("db failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(t.Name(), func(t *testing.T) {
			mock := &mockStore{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			h := New(mock, HandlerConfig{
				VisibilityExtension: 10 * time.Minute,
			})

			mux := http.NewServeMux()
			mux.HandleFunc("/internal/executions/{id}/heartbeat", h.InternalHeartbeat)

			req := httptest.NewRequest("PUT", "/internal/executions/"+tt.executionID+"/heartbeat", nil)
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %d want %d", rr.Code, tt.expectedStatus)
			}

			if tt.verify != nil {
				tt.verify(mock.capturedVisibleAfter, 10)
			}
		})
	}
}

func TestInternalUpdateResult(t *testing.T) {
	executionID := uuid.New()

	tests := []struct {
		name           string
		executionID    string
		body           string
		mockSetup      func(*mockStore)
		expectedStatus int
	}{
		{
			name:           "Success - Completed",
			executionID:    executionID.String(),
			body:           `{"exit_code": 0, "error": ""}`,
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - Failed",
			executionID:    executionID.String(),
			body:           `{"exit_code": 1, "error": "process crashed"}`,
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid UUID",
			executionID:    "bad-id",
			body:           `{"exit_code": 0}`,
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid Body",
			executionID:    executionID.String(),
			body:           `{invalid-json}`,
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Store Complete Error",
			executionID: executionID.String(),
			body:        `{"exit_code": 0}`,
			mockSetup: func(m *mockStore) {
				m.completeErr = errors.New("db failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Store Fail Error",
			executionID: executionID.String(),
			body:        `{"exit_code": 1, "error": "oops"}`,
			mockSetup: func(m *mockStore) {
				m.failErr = errors.New("db failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}
			h := New(mock, HandlerConfig{})

			mux := http.NewServeMux()
			mux.HandleFunc("PUT /internal/executions/{id}/result", h.InternalUpdateResult)

			req := httptest.NewRequest(http.MethodPut, "/internal/executions/"+tt.executionID+"/result", bytes.NewBufferString(tt.body))
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					rr.Code, tt.expectedStatus)
			}
		})
	}
}

func TestGetDLQExecutions(t *testing.T) {
	tenantID := uuid.New()
	executionID := uuid.New()

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func()
		expectedStatus int
	}{
		{
			name:        "Success - Default Params",
			queryParams: "",
			mockSetup: func() {
				listDLQResp = []store.DLQEntry{
					{
						ID:          1,
						ExecutionID: executionID,
						TenantID:    tenantID,
						JobID:       uuid.New().String(),
						JobName:     "test-job",
						Priority:    50,
						Attempts:    5,
					},
				}
				listDLQErr = nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Success - Empty DLQ",
			queryParams: "",
			mockSetup: func() {
				listDLQResp = []store.DLQEntry{}
				listDLQErr = nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Store Error",
			queryParams: "",
			mockSetup: func() {
				listDLQResp = nil
				listDLQErr = errors.New("db failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{}
			if tt.mockSetup != nil {
				tt.mockSetup()
			}

			h := New(mock, HandlerConfig{})

			mux := http.NewServeMux()
			mux.HandleFunc("GET /executions/dlq", h.GetDQLExecutions)

			req := httptest.NewRequest(http.MethodGet, "/executions/dlq"+tt.queryParams, nil)
			rr := httptest.NewRecorder()

			ctx := middleware.NewContextWithTenant(req.Context(), &store.Tenant{ID: tenantID})
			req = req.WithContext(ctx)

			mux.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %d want %d", rr.Code, tt.expectedStatus)
			}
		})
	}
}

func TestRetryDLQExecution(t *testing.T) {
	tenantID := uuid.New()
	executionID := uuid.New()
	newExecutionID := uuid.New()

	tests := []struct {
		name           string
		executionID    string
		mockSetup      func(*mockStore)
		expectedStatus int
	}{
		{
			name:        "Success",
			executionID: executionID.String(),
			mockSetup: func(m *mockStore) {
				m.getExecutionResp = &store.Execution{
					ID:       executionID,
					TenantID: tenantID,
					Status:   store.ExecutionStatusFailed,
				}
				retryFromDLQResp = newExecutionID
				retryFromDLQErr = nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid UUID",
			executionID:    "not-a-uuid",
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Execution Not Found",
			executionID: executionID.String(),
			mockSetup: func(m *mockStore) {
				m.getExecutionErr = errors.New("not found")
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Wrong Tenant",
			executionID: executionID.String(),
			mockSetup: func(m *mockStore) {
				m.getExecutionResp = &store.Execution{
					ID:       executionID,
					TenantID: uuid.New(),
					Status:   store.ExecutionStatusFailed,
				}
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Retry Store Error",
			executionID: executionID.String(),
			mockSetup: func(m *mockStore) {
				m.getExecutionResp = &store.Execution{
					ID:       executionID,
					TenantID: tenantID,
					Status:   store.ExecutionStatusFailed,
				}
				retryFromDLQErr = errors.New("db failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			h := New(mock, HandlerConfig{})

			mux := http.NewServeMux()
			mux.HandleFunc("POST /executions/dlq/{id}/retry", h.RetryDQLExecution)

			req := httptest.NewRequest(http.MethodPost, "/executions/dlq/"+tt.executionID+"/retry", nil)
			rr := httptest.NewRecorder()

			ctx := middleware.NewContextWithTenant(req.Context(), &store.Tenant{ID: tenantID})
			req = req.WithContext(ctx)

			mux.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %d want %d, body: %s", rr.Code, tt.expectedStatus, rr.Body.String())
			}
		})
	}
}
