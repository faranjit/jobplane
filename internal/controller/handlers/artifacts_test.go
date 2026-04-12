package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"jobplane/internal/artifact"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// mockArtifactStore implements store.ArtifactStore locally to avoid bloating the main mockStore
type mockArtifactStore struct {
	store.ArtifactStore
	createArtifactErr error
}

func (m *mockArtifactStore) CreateArtifact(ctx context.Context, artifact *store.ExecutionArtifact) error {
	return m.createArtifactErr
}

// mockStorageBackend implements artifact.StorageBackend
type mockStorageBackend struct {
	generateUploadURLErr  error
	generateUploadURLResp artifact.UploadInstructions
}

func (m *mockStorageBackend) GenerateUploadURL(ctx context.Context, meta artifact.ArtifactMeta) (artifact.UploadInstructions, error) {
	return m.generateUploadURLResp, m.generateUploadURLErr
}

func (m *mockStorageBackend) GenerateDownloadURL(ctx context.Context, storagePath string) (string, error) {
	return "", nil
}

func TestInternalAuthorizeArtifact(t *testing.T) {
	executionID := uuid.New()
	tenantID := uuid.New()

	validBody := api.AuthorizeArtifactRequest{
		Filename:    "report.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1024,
	}
	validBodyBytes, _ := json.Marshal(validBody)

	tests := []struct {
		name           string
		executionID    string
		body           []byte
		mockStoreSetup func(*mockStore)
		mockArtifact   *mockArtifactStore
		mockStorage    *mockStorageBackend
		config         HandlerConfig
		expectedStatus int
	}{
		{
			name:        "Success",
			executionID: executionID.String(),
			body:        validBodyBytes,
			mockStoreSetup: func(m *mockStore) {
				m.getExecutionResp = &store.Execution{
					ID:       executionID,
					TenantID: tenantID,
				}
			},
			mockArtifact: &mockArtifactStore{},
			mockStorage: &mockStorageBackend{
				generateUploadURLResp: artifact.UploadInstructions{
					URL:       "http://example.local/upload",
					Method:    "PUT",
					ExpiresIn: 300,
				},
			},
			config:         HandlerConfig{ArtifactMaxSizeBytes: 2048},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid Execution ID Format",
			executionID:    "not-a-uuid",
			body:           validBodyBytes,
			mockStoreSetup: func(m *mockStore) {},
			mockArtifact:   nil,
			mockStorage:    nil,
			config:         HandlerConfig{ArtifactMaxSizeBytes: 2048},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid Request Body",
			executionID:    executionID.String(),
			body:           []byte("bad json"),
			mockStoreSetup: func(m *mockStore) {},
			mockArtifact:   nil,
			mockStorage:    nil,
			config:         HandlerConfig{ArtifactMaxSizeBytes: 2048},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Artifact Too Large",
			executionID:    executionID.String(),
			body:           validBodyBytes, // 1024 bytes
			mockStoreSetup: func(m *mockStore) {},
			mockArtifact:   nil,
			mockStorage:    nil,
			config:         HandlerConfig{ArtifactMaxSizeBytes: 500}, // Too small limit
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Execution Not Found",
			executionID: executionID.String(),
			body:        validBodyBytes,
			mockStoreSetup: func(m *mockStore) {
				m.getExecutionErr = errors.New("db find failed")
			},
			mockArtifact:   nil,
			mockStorage:    nil,
			config:         HandlerConfig{ArtifactMaxSizeBytes: 2048},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "Store Error On Create",
			executionID: executionID.String(),
			body:        validBodyBytes,
			mockStoreSetup: func(m *mockStore) {
				m.getExecutionResp = &store.Execution{
					ID:       executionID,
					TenantID: tenantID,
				}
			},
			mockArtifact: &mockArtifactStore{
				createArtifactErr: errors.New("db insert failed"),
			},
			mockStorage:    nil,
			config:         HandlerConfig{ArtifactMaxSizeBytes: 2048},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Storage Backend Error",
			executionID: executionID.String(),
			body:        validBodyBytes,
			mockStoreSetup: func(m *mockStore) {
				m.getExecutionResp = &store.Execution{
					ID:       executionID,
					TenantID: tenantID,
				}
			},
			mockArtifact: &mockArtifactStore{},
			mockStorage: &mockStorageBackend{
				generateUploadURLErr: errors.New("backend timeout"),
			},
			config:         HandlerConfig{ArtifactMaxSizeBytes: 2048},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock store
			ms := &mockStore{}
			if tt.mockStoreSetup != nil {
				tt.mockStoreSetup(ms)
			}
			
			// Inject artifact store mock if provided
			if tt.mockArtifact != nil {
				ms.ArtifactStore = tt.mockArtifact
			}

			// Setup Handlers
			h := New(ms, tt.config)
			
			// Inject storage backend mock if provided
			if tt.mockStorage != nil {
				h.WithArtifacts(tt.mockStorage)
			}

			// Serve HTTP
			mux := http.NewServeMux()
			mux.HandleFunc("POST /internal/executions/{id}/artifacts/authorize", h.InternalAuthorizeArtifact)

			req := httptest.NewRequest(http.MethodPost, "/internal/executions/"+tt.executionID+"/artifacts/authorize", bytes.NewBuffer(tt.body))
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			// Assert
			if rr.Code != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %d want %d", rr.Code, tt.expectedStatus)
			}

			// If success, verify JSON payload has the required fields
			if rr.Code == http.StatusOK {
				var resp api.AuthorizeArtifactResponse
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp.UploadURL == "" {
					t.Error("expected UploadURL to be populated")
				}
				if resp.ArtifactID == "" {
					t.Error("expected ArtifactID to be populated")
				}
			}
		})
	}
}
