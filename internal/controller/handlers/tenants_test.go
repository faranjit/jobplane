package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"jobplane/pkg/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateTenant(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		mockSetup      func(*mockStore)
		expectedStatus int
		expectedInBody string
	}{
		{
			name:           "Success",
			body:           `{"name": "Acme corp"}`,
			mockSetup:      func(ms *mockStore) {},
			expectedStatus: http.StatusCreated,
			expectedInBody: "api_key",
		},
		{
			name:           "Invalid Request Body",
			body:           `{invalid}`,
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
			expectedInBody: "Invalid request body",
		},
		{
			name: "Database Error",
			body: `{"name": "Crash Corp"}`,
			mockSetup: func(m *mockStore) {
				m.createTenantErr = errors.New("db down")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedInBody: "Failed to create",
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
			req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(tt.body))
			rr := httptest.NewRecorder()

			// Execute
			h.CreateTenant(rr, req)

			// Assertions
			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %d but want %d", rr.Code, tt.expectedStatus)
			}

			if tt.expectedInBody != "" && !strings.Contains(rr.Body.String(), tt.expectedInBody) {
				t.Errorf("handler returned unexpected body: got %s want substring %s", rr.Body.String(), tt.expectedInBody)
			}

			// Verify response
			if tt.expectedStatus == http.StatusCreated {
				var resp api.CreateTenantResponse
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if !strings.HasPrefix(resp.ApiKey, "jp_") {
					t.Errorf("api_key must start with 'jp_', got %s", resp.ApiKey)
				}

				if len(resp.ApiKey) < 30 {
					t.Errorf("api_key looks too short: %s", resp.ApiKey)
				}
			}
		})
	}
}
