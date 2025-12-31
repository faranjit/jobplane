package handlers

import (
	"encoding/json"
	"jobplane/internal/controller/middleware"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// GetExecution handles GET /executions/{id}.
// Returns the current status and result of a job run.
func (h *Handlers) GetExecution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	executionIDStr := r.PathValue("id")
	executionID, err := uuid.Parse(executionIDStr)
	if err != nil {
		h.httpError(w, "Invalid execution id", http.StatusBadRequest)
		return
	}

	tenantID, ok := middleware.TenantIDFromContext(ctx)
	if !ok {
		h.httpError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	execution, err := h.store.GetExecutionByID(ctx, executionID)
	if err != nil || execution.TenantID != tenantID {
		h.httpError(w, "Execution not found", http.StatusNotFound)
		return
	}

	h.respondJson(w, http.StatusOK, execution)
}

// ---------------------------------------------------------
// Internal Worker Endpoints
// These should NOT use the Tenant Middleware.
// ---------------------------------------------------------

// InternalHeartbeat handles PUT /internal/executions/{id}/heartbeat.
// The worker calls this to say "I'm still working on it, don't give it to anyone else."
func (h *Handlers) InternalHeartbeat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	executionIDStr := r.PathValue("id")
	executionID, err := uuid.Parse(executionIDStr)
	if err != nil {
		h.httpError(w, "Invalid execution id", http.StatusBadRequest)
		return
	}

	// Extend visibility by 5 minutes from now
	newVisibility := time.Now().Add(5 * time.Minute)

	if err := h.store.SetVisibleAfter(ctx, nil, executionID, newVisibility); err != nil {
		h.httpError(w, "Failed to update heartbeat", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// InternalUpdateResult handles PUT /internal/executions/{id}/result.
// The worker calls this when the job finishes or crashes.
func (h *Handlers) InternalUpdateResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	executionIDStr := r.PathValue("id")
	executionID, err := uuid.Parse(executionIDStr)
	if err != nil {
		h.httpError(w, "Invalid execution id", http.StatusBadRequest)
		return
	}

	var req struct {
		ExitCode int    `json:"exit_code"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpError(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.ExitCode == 0 && req.Error == "" {
		err := h.store.Complete(ctx, nil, executionID, req.ExitCode)
		if err != nil {
			h.httpError(w, "Failed to mark complete", http.StatusInternalServerError)
			return
		}
	} else {
		err := h.store.Fail(ctx, nil, executionID, req.Error)
		if err != nil {
			h.httpError(w, "Failed to mark failed", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}
