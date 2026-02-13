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

	"github.com/google/uuid"
)

func TestInternalAddLogs(t *testing.T) {
	executionID := uuid.New()

	tests := []struct {
		name           string
		executionID    string
		body           string
		mockSetup      func(*mockStore)
		expectedStatus int
	}{
		{
			name:           "Success",
			executionID:    executionID.String(),
			body:           `{"content": "something happened"}`,
			mockSetup:      func(ms *mockStore) {},
			expectedStatus: http.StatusAccepted,
		},
		{
			name:           "Invalid UUID",
			executionID:    "bad-uuid",
			body:           `{"content": "..."}`,
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
			name:        "Store Error",
			executionID: executionID.String(),
			body:        `{"content": "..."}`,
			mockSetup: func(m *mockStore) {
				m.addLogEntryErr = errors.New("db failed")
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
			mux.HandleFunc("POST /internal/executions/{id}/logs", h.InternalAddLogs)

			req := httptest.NewRequest(http.MethodPost, "/internal/executions/"+tt.executionID+"/logs", bytes.NewBufferString(tt.body))
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.expectedStatus)
			}
		})
	}
}

func TestGetLogs(t *testing.T) {
	tenantID := uuid.New()
	executionID := uuid.New()

	validExec := &store.Execution{
		ID:       executionID,
		TenantID: tenantID,
	}

	tests := []struct {
		name           string
		url            string
		mockSetup      func(*mockStore)
		expectedStatus int
		verifySpy      func(*testing.T, *mockStore) // Check if args were passed correctly
	}{
		{
			name: "Success - Default Params",
			url:  "/executions/" + executionID.String() + "/logs",
			mockSetup: func(m *mockStore) {
				m.getExecutionResp = validExec
				m.getExecutionLogsResp = []store.LogEntry{{ID: 1, Content: "log1"}}
			},
			expectedStatus: http.StatusOK,
			verifySpy: func(t *testing.T, m *mockStore) {
				if m.capturedLimit != 1000 { // Default limit
					t.Errorf("expected default limit 1000, got %d", m.capturedLimit)
				}
				if m.capturedAfterID != 0 {
					t.Errorf("expected default afterID 0, got %d", m.capturedAfterID)
				}
			},
		},
		{
			name: "Success - Custom Pagination",
			url:  "/executions/" + executionID.String() + "/logs?after_id=50&limit=10",
			mockSetup: func(m *mockStore) {
				m.getExecutionResp = validExec
				m.getExecutionLogsResp = []store.LogEntry{}
			},
			expectedStatus: http.StatusOK,
			verifySpy: func(t *testing.T, m *mockStore) {
				if m.capturedLimit != 10 {
					t.Errorf("expected limit 10, got %d", m.capturedLimit)
				}
				if m.capturedAfterID != 50 {
					t.Errorf("expected afterID 50, got %d", m.capturedAfterID)
				}
			},
		},
		{
			name: "Execution Not Found",
			url:  "/executions/" + executionID.String() + "/logs",
			mockSetup: func(m *mockStore) {
				m.getExecutionErr = errors.New("not found")
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "Wrong Tenant (Security Check)",
			url:  "/executions/" + executionID.String() + "/logs",
			mockSetup: func(m *mockStore) {
				m.getExecutionResp = &store.Execution{
					ID:       executionID,
					TenantID: uuid.New(), // Mismatch
				}
			},
			expectedStatus: http.StatusNotFound,
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
			mux.HandleFunc("GET /executions/{id}/logs", h.GetExecutionLogs)

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			ctx := middleware.NewContextWithTenant(req.Context(), &store.Tenant{ID: tenantID})
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				var resp api.GetLogsResponse
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
			}

			if tt.verifySpy != nil {
				tt.verifySpy(t, mock)
			}
		})
	}
}
