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
			ctx := middleware.NewContextWithTenantID(req.Context(), tenantID)
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
