package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jobplane/internal/controller/middleware"
	"jobplane/internal/store"
	"jobplane/pkg/api"

	"github.com/google/uuid"
)

func TestCreateJob(t *testing.T) {
	// Common test data
	tenantID := uuid.New()
	validReq := api.CreateJobRequest{
		Name:    "test-job",
		Image:   "alpine:latest",
		Command: []string{"echo", "hello"},
	}
	validBody, _ := json.Marshal(validReq)

	tests := []struct {
		name           string
		body           []byte
		mockSetup      func(*mockStore)
		expectedStatus int
		expectedInBody string
	}{
		{
			name:           "Success",
			body:           validBody,
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusOK,
			expectedInBody: "job_id",
		},
		{
			name:           "Invalid JSON",
			body:           []byte(`{invalid-json}`),
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
			expectedInBody: "Invalid request body",
		},
		{
			name:           "Missing Required Fields",
			body:           []byte(`{"name": "", "image": ""}`),
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
			expectedInBody: "Name and Image are required",
		},
		{
			name: "Database Transaction Error",
			body: validBody,
			mockSetup: func(m *mockStore) {
				m.beginTxErr = errors.New("db connection failed")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedInBody: "Internal database error",
		},
		{
			name: "Create Job Failure",
			body: validBody,
			mockSetup: func(m *mockStore) {
				m.createJobErr = errors.New("insert failed")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedInBody: "Failed to create job",
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
			req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(tt.body))

			// Inject Tenant Context using the helper
			ctx := middleware.NewContextWithTenantID(req.Context(), tenantID)
			req = req.WithContext(ctx)

			// Execute
			rr := httptest.NewRecorder()
			h.CreateJob(rr, req)

			// Assertions
			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					rr.Code, tt.expectedStatus)
			}

			if tt.expectedInBody != "" && !strings.Contains(rr.Body.String(), tt.expectedInBody) {
				t.Errorf("handler returned unexpected body: got %v want substring %v",
					rr.Body.String(), tt.expectedInBody)
			}
		})
	}
}

func TestRunJob(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()

	// Valid Job that belongs to the tenant
	validJob := &store.Job{
		ID:       jobID,
		TenantID: tenantID, // Matches request tenant
		Name:     "test-job",
		Image:    "alpine",
	}

	tests := []struct {
		name           string
		jobIDParam     string
		mockSetup      func(*mockStore)
		expectedStatus int
	}{
		{
			name:       "Success",
			jobIDParam: jobID.String(),
			mockSetup: func(m *mockStore) {
				m.getJobByIDResp = validJob
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid UUID Format",
			jobIDParam:     "not-a-uuid",
			mockSetup:      func(m *mockStore) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:       "Job Not Found",
			jobIDParam: uuid.New().String(),
			mockSetup: func(m *mockStore) {
				m.getJobByIDErr = errors.New("not found")
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:       "Job Belongs to Different Tenant",
			jobIDParam: jobID.String(),
			mockSetup: func(m *mockStore) {
				// Job exists but has different tenant
				m.getJobByIDResp = &store.Job{
					ID:       jobID,
					TenantID: uuid.New(), // Different ID
				}
			},
			expectedStatus: http.StatusNotFound, // Should mask as 404
		},
		{
			name:       "Enqueue Failure",
			jobIDParam: jobID.String(),
			mockSetup: func(m *mockStore) {
				m.getJobByIDResp = validJob
				m.enqueueErr = errors.New("queue full")
			},
			expectedStatus: http.StatusInternalServerError,
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

			// Setup Router & Request
			mux := http.NewServeMux()
			mux.HandleFunc("POST /jobs/{id}/run", h.RunJob)

			req := httptest.NewRequest(http.MethodPost, "/jobs/"+tt.jobIDParam+"/run", nil)

			// Inject Tenant Context
			ctx := middleware.NewContextWithTenantID(req.Context(), tenantID)
			req = req.WithContext(ctx)

			// Execute via Mux
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			// Assertions
			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v body: %v",
					rr.Code, tt.expectedStatus, rr.Body.String())
			}
		})
	}
}
