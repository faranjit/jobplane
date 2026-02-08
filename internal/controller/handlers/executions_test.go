package handlers

import (
	"bytes"
	"errors"
	"jobplane/internal/controller/middleware"
	"jobplane/internal/store"
	"net/http"
	"net/http/httptest"
	"testing"

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

			h := New(mock)

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

func TestInternalHeartbeat(t *testing.T) {
	executionID := uuid.New()

	tests := []struct {
		name           string
		executionID    string
		mockSetup      func(*mockStore)
		expectedStatus int
	}{
		{
			name:           "Success",
			executionID:    executionID.String(),
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusOK,
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

			h := New(mock)

			mux := http.NewServeMux()
			mux.HandleFunc("/internal/executions/{id}/heartbeat", h.InternalHeartbeat)

			req := httptest.NewRequest("PUT", "/internal/executions/"+tt.executionID+"/heartbeat", nil)
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %d want %d", rr.Code, tt.expectedStatus)
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
			h := New(mock)

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

			h := New(mock)

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

			h := New(mock)

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
