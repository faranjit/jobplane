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
	"strings"
	"testing"

	"github.com/google/uuid"
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

func TestUpdateTenant(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		mockSetup      func(*mockStore)
		expectedStatus int
		expectedInBody string
		callback       func(expected uuid.UUID, actual uuid.UUID)
	}{
		{
			name:           "Success",
			body:           `{"name": "Acme corp", "rate_limit": 10, "rate_limit_burst": 20, "max_concurrent_executions": 5}`,
			mockSetup:      func(ms *mockStore) {},
			expectedStatus: http.StatusNoContent,
			callback: func(expected uuid.UUID, actual uuid.UUID) {
				if expected != actual {
					t.Errorf("handler returned unexpected tenant id: got %s but want %s", actual, expected)
				}
			},
		},
		{
			name:           "Invalid Request Body",
			body:           `{invalid}`,
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
			expectedInBody: "Invalid request body",
			callback: func(expected uuid.UUID, actual uuid.UUID) {
				t.Errorf("Unexpected callback")
			},
		},
		{
			name: "Database Error",
			body: `{"name": "Crash Corp", "rate_limit": 10, "rate_limit_burst": 20, "max_concurrent_executions": 5}`,
			mockSetup: func(m *mockStore) {
				m.updateTenantErr = errors.New("db down")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedInBody: "Failed to update tenant",
			callback: func(expected uuid.UUID, actual uuid.UUID) {
				t.Errorf("Unexpected callback")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mock := &mockStore{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}

			tenantID := uuid.New()
			h := New(mock).WithCallbacks(Callbacks{
				OnTenantUpdated: func(u uuid.UUID) {
					tt.callback(tenantID, u)
				},
			})

			// Request
			req := httptest.NewRequest(http.MethodPatch, "/tenants/"+tenantID.String(), bytes.NewBufferString(tt.body))
			ctx := middleware.NewContextWithTenant(req.Context(), &store.Tenant{ID: tenantID, MaxConcurrentExecutions: 100})
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()

			// Execute
			h.UpdateTenant(rr, req)

			// Assertions
			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %d but want %d", rr.Code, tt.expectedStatus)
			}

			if tt.expectedInBody != "" && !strings.Contains(rr.Body.String(), tt.expectedInBody) {
				t.Errorf("handler returned unexpected body: got %s want substring %s", rr.Body.String(), tt.expectedInBody)
			}
		})
	}
}

// curl -X PATCH http://localhost:6161/tenants/10ac19bd-0875-4440-9eea-a340611e506d \
//   -H "Authorization: Bearer jp_88738e9cfad109dc3a646422c579a0e2823653483233c1fd0578d51078012310" \
//   -H "Content-Type: application/json" \
//   -d '{"rate_limit": 10, "rate_limit_burst": 20}'
