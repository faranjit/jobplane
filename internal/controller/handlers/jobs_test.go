package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jobplane/internal/controller/middleware"
	"jobplane/internal/store"
	"jobplane/pkg/api"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Helper to setup OTel for testing
func setupMetricsForTest() *metric.ManualReader {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	otel.SetMeterProvider(provider)
	return reader
}

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

	// scheduled time for checking
	scheduleTime := time.Now().Add(1 * time.Second).UTC()

	// Past time for error checking
	pastTime := time.Now().UTC().Add(-1 * time.Hour)

	tests := []struct {
		name           string
		jobIDParam     string
		body           api.RunJobRequest
		mockSetup      func(*mockStore)
		expectedStatus int
		verify         func(*testing.T, *mockStore)
	}{
		{
			name:       "Success",
			jobIDParam: jobID.String(),
			mockSetup: func(m *mockStore) {
				m.getJobByIDResp = validJob
			},
			expectedStatus: http.StatusOK,
			verify: func(t *testing.T, m *mockStore) {
				// Should be roughly now
				if time.Since(m.capturedVisibleAfter) > 5*time.Second {
					t.Error("Expected immediate execution (visibleAfter ~ Now)")
				}
			},
		},
		{
			name:       "Success - Schuled",
			jobIDParam: jobID.String(),
			body:       api.RunJobRequest{ScheduledAt: &scheduleTime},
			mockSetup: func(m *mockStore) {
				m.getJobByIDResp = validJob
			},
			expectedStatus: http.StatusOK,
			verify: func(t *testing.T, m *mockStore) {
				// Should match the scheduled time exactly (ignoring monotonic clock diffs)
				diff := m.capturedVisibleAfter.Sub(scheduleTime)
				if diff < -time.Second || diff > time.Second {
					t.Errorf("Expected visibleAfter %v, got %v", scheduleTime, m.capturedVisibleAfter)
				}
			},
		},
		{
			name:       "Failure - Past Schedule",
			jobIDParam: jobID.String(),
			body:       api.RunJobRequest{ScheduledAt: &pastTime},
			mockSetup: func(m *mockStore) {
				m.getJobByIDResp = validJob
			},
			expectedStatus: http.StatusBadRequest,
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

			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/jobs/"+tt.jobIDParam+"/run", bytes.NewReader(bodyBytes))

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

			if tt.verify != nil {
				tt.verify(t, mock)
			}
		})
	}
}

func TestCreateJob_Metrics(t *testing.T) {
	reader := setupMetricsForTest()

	tenantID := uuid.New()
	mock := &mockStore{
		createJobErr: nil,
	}

	h := New(mock)
	reqBody := api.CreateJobRequest{Name: "metric-test-job", Image: "alpine"}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(bodyBytes))
	req = req.WithContext(middleware.NewContextWithTenantID(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	http.HandlerFunc(h.CreateJob).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Handler failed: %d", rr.Code)
	}

	// Verify Metric
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	found := false
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "jobplane.jobs.created_total" {
				// Check the value
				if data, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, pt := range data.DataPoints {
						if pt.Value == 1 {
							found = true
						}
					}
				}
			}
		}
	}

	if !found {
		t.Error("Expected metric 'jobplane.jobs.created_total' to be incremented to 1")
	}
}
