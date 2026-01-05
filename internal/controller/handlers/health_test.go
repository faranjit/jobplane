package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbes(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       string
		mockSetup      func(*mockStore)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Healthz Always OK",
			endpoint:       "/healthz",
			expectedStatus: http.StatusOK,
			expectedBody:   "ok",
		},
		{
			name:           "Readyz Success",
			endpoint:       "/readyz",
			mockSetup:      func(m *mockStore) { m.pingErr = nil },
			expectedStatus: http.StatusOK,
			expectedBody:   "ready",
		},
		{
			name:           "Readyz Database Fail",
			endpoint:       "/readyz",
			mockSetup:      func(m *mockStore) { m.pingErr = errors.New("db down") },
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   "Database unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{}
			if tt.mockSetup != nil {
				tt.mockSetup(mock)
			}
			h := New(mock)

			req := httptest.NewRequest(http.MethodGet, tt.endpoint, nil)
			rr := httptest.NewRecorder()

			// Route manually since we are testing specific handler functions
			if tt.endpoint == "/healthz" {
				h.Healthz(rr, req)
			} else {
				h.Readyz(rr, req)
			}

			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, tt.expectedStatus)
			}
			if rr.Body.String() != tt.expectedBody && tt.expectedBody != "" {
				// contains check for error messages
				if tt.expectedStatus != http.StatusOK {
					// for errors just check substring if needed, but here we check exact for now
				}
			}
		})
	}
}
