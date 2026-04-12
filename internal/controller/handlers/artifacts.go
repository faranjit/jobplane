package handlers

import (
	"encoding/json"
	"net/http"

	"jobplane/internal/artifact"
	"jobplane/internal/store"
	"jobplane/pkg/api"

	"github.com/google/uuid"
)

// InternalAuthorizeArtifact handles POST /internal/executions/{id}/artifacts/authorize.
// Called by the worker before uploading an artifact. It validates the execution exists,
// checks the file size against the configured limit, inserts a pending artifact record
// in the database, and returns a backend-specific upload URL.
//
// The worker then performs a raw HTTP PUT to the returned URL to upload the file bytes.
func (h *Handlers) InternalAuthorizeArtifact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	executionIDStr := r.PathValue("id")
	executionID, err := uuid.Parse(executionIDStr)
	if err != nil {
		h.httpError(w, "Invalid execution id", http.StatusBadRequest)
		return
	}

	var req api.AuthorizeArtifactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SizeBytes > h.config.ArtifactMaxSizeBytes {
		h.httpError(w, "artifact too large", http.StatusBadRequest)
		return
	}

	execution, err := h.store.GetExecutionByID(ctx, executionID)
	if err != nil {
		h.httpError(w, "Execution not found", http.StatusNotFound)
		return
	}
	tenantID := execution.TenantID

	artifactID := uuid.New()
	art := &store.ExecutionArtifact{
		ID:             artifactID,
		ExecutionID:    executionID,
		TenantID:       tenantID,
		Filename:       req.Filename,
		ContentType:    req.ContentType,
		SizeBytes:      req.SizeBytes,
		StorageBackend: h.config.ArtifactStorageBackend,
		Status:         store.ExecutionArtifactStatusPending,
	}

	if err := h.store.CreateArtifact(ctx, art); err != nil {
		h.httpError(w, "failed to create artifact", http.StatusInternalServerError)
		return
	}

	instructions, err := h.artifacts.GenerateUploadURL(ctx, artifact.ArtifactMeta{
		ArtifactID:  artifactID,
		ExecutionID: executionID,
		TenantID:    tenantID,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
	})

	if err != nil {
		h.httpError(w, "failed to generate upload url", http.StatusInternalServerError)
		return
	}

	h.respondJson(w, http.StatusOK, api.AuthorizeArtifactResponse{
		ArtifactID: artifactID.String(),
		UploadURL:  instructions.URL,
		Method:     instructions.Method,
		ExpiresIn:  instructions.ExpiresIn,
	})
}
