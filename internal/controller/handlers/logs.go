package handlers

import (
	"encoding/json"
	"fmt"
	"jobplane/internal/controller/middleware"
	"jobplane/pkg/api"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

// InternalAddLogs handles POST /internal/executions/{id}/logs
// Called by the Worker to append a log chunk.
func (h *Handlers) InternalAddLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	executionIDStr := r.PathValue("id")
	executionID, err := uuid.Parse(executionIDStr)
	if err != nil {
		h.httpError(w, "Invalid execution id", http.StatusBadRequest)
		return
	}

	var req api.AddLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// For now, writing directly to Postgres is fine for the prototype.
	if err := h.store.AddLogEntry(ctx, executionID, req.Content); err != nil {
		h.httpError(w, "Failed to persist log", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// GetExecutionLogs handles GET /executions/{id}/logs
// Called by the User (CLI/UI) to view logs.
func (h *Handlers) GetExecutionLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	executionIDStr := r.PathValue("id")
	executionID, err := uuid.Parse(executionIDStr)
	if err != nil {
		h.httpError(w, "Invalid execution id", http.StatusBadRequest)
		return
	}

	tenantID, ok := middleware.TenantIDFromContext(ctx)
	fmt.Printf("tenant id from request: %s \n", tenantID)
	if !ok {
		h.httpError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query()
	limit := 1000 // default limit
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 10000 {
			limit = parsed
		}
	}

	var afterID int64 = 0
	if after := query.Get("after_id"); after != "" {
		if parsed, err := strconv.ParseInt(after, 10, 64); err == nil {
			afterID = parsed
		}
	}

	// Verify ownership
	execution, err := h.store.GetExecutionByID(ctx, executionID)
	if err != nil || execution.TenantID != tenantID {
		h.httpError(w, "Execution not found", http.StatusNotFound)
		return
	}
	fmt.Printf("tenant id from execution: %s \n", execution.TenantID)

	logs, err := h.store.GetExecutionLogs(ctx, executionID, afterID, limit)
	if err != nil {
		h.httpError(w, "Failed to fetch logs", http.StatusInternalServerError)
		return
	}

	apiLogs := make([]api.LogEntry, len(logs))
	for i, log := range logs {
		apiLogs[i] = api.LogEntry{
			ID:        log.ID,
			Content:   log.Content,
			CreatedAt: log.CreatedAt,
		}
	}

	h.respondJson(w, http.StatusOK, api.GetLogsResponse{Logs: apiLogs})
}
