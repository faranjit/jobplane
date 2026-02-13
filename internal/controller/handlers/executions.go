package handlers

import (
	"encoding/json"
	"jobplane/internal/controller/middleware"
	"jobplane/pkg/api"
	"log"
	"net/http"
	"strconv"
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

	executionResponse := &api.ExecutionResponse{
		ID:          execution.ID.String(),
		Status:      string(execution.Status),
		Priority:    execution.Priority,
		Attempt:     execution.Attempt,
		ScheduledAt: execution.ScheduledAt,
		StartedAt:   execution.StartedAt,
		CompletedAt: execution.CompletedAt,
		ExitCode:    execution.ExitCode,
		Error:       execution.ErrorMessage,
	}

	h.respondJson(w, http.StatusOK, executionResponse)
}

// GetDQLExecutions handles GET /executions/dlq.
// Returns a list of executions that failed to be processed.
func (h *Handlers) GetDQLExecutions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenantID, ok := middleware.TenantIDFromContext(ctx)
	if !ok {
		h.httpError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query()
	limit := 20 // default limit
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 10000 {
			limit = parsed
		}
	}

	offset := 0
	if o := query.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil {
			offset = parsed
		}
	}

	executions, err := h.store.ListDLQ(ctx, tenantID, limit, offset)
	if err != nil {
		h.httpError(w, "Failed to list DLQ executions", http.StatusInternalServerError)
		return
	}

	var executionResponses []api.DLQExecutionResponse
	for _, execution := range executions {
		executionResponses = append(executionResponses, api.DLQExecutionResponse{
			ID:           execution.ID,
			ExecutionID:  execution.ExecutionID.String(),
			JobID:        execution.JobID,
			JobName:      execution.JobName,
			Priority:     execution.Priority,
			ErrorMessage: execution.ErrorMessage,
			Attempts:     execution.Attempts,
			FailedAt:     execution.FailedAt,
		})
	}

	h.respondJson(w, http.StatusOK, executionResponses)
}

// RetryDQLExecution handles POST /executions/dlq/{id}/retry.
func (h *Handlers) RetryDQLExecution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenantID, ok := middleware.TenantIDFromContext(ctx)
	if !ok {
		h.httpError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	executionIDStr := r.PathValue("id")
	executionID, err := uuid.Parse(executionIDStr)
	if err != nil {
		h.httpError(w, "Invalid execution id", http.StatusBadRequest)
		return
	}

	execution, err := h.store.GetExecutionByID(ctx, executionID)
	if err != nil || execution.TenantID != tenantID {
		h.httpError(w, "Execution not found", http.StatusNotFound)
		return
	}

	newExecutionID, err := h.store.RetryFromDLQ(ctx, executionID)
	if err != nil {
		h.httpError(w, "Failed to retry DLQ execution", http.StatusInternalServerError)
		return
	}

	h.respondJson(w, http.StatusOK, api.RetryDQLExecutionResponse{
		NewExecutionID: newExecutionID.String(),
	})
}

// ---------------------------------------------------------
// Internal Worker Endpoints
// These should NOT use the Tenant Middleware.
// ---------------------------------------------------------

// InternalDequeue handles POST /internal/executions/dequeue.
// Workers call this endpoint to claim pending jobs.
func (h *Handlers) InternalDequeue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req api.DequeueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Sanity check the concurrency limit to prevent resource exhaustion or abuse
	if req.Limit <= 0 {
		req.Limit = 1
	}
	if req.Limit > 100 { // Max batch size
		req.Limit = 100
	}

	items, err := h.store.DequeueBatch(ctx, req.TenantIDs, req.Limit)
	if err != nil {
		// Log the actual error internally, but return a generic 500 to the worker
		log.Printf("Failed to dequeue executions: %v", err)
		h.httpError(w, "Failed to dequeue executions", http.StatusInternalServerError)
		return
	}

	resp := api.DequeueResponse{
		Executions: make([]api.DequeuedExecution, 0, len(items)),
	}

	for _, item := range items {
		resp.Executions = append(resp.Executions, api.DequeuedExecution{
			ExecutionID: item.ExecutionID,
			Payload:     item.Payload,
		})
	}

	h.respondJson(w, http.StatusOK, resp)
}

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

	// Extend visibility by some time from now
	newVisibility := time.Now().Add(h.config.VisibilityExtension)

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

	var req api.ExecutionResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpError(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.ExitCode != nil && *req.ExitCode == 0 && req.Error == "" {
		err := h.store.Complete(ctx, nil, executionID, 0)
		if err != nil {
			h.httpError(w, "Failed to mark complete", http.StatusInternalServerError)
			return
		}
	} else {
		err := h.store.Fail(ctx, nil, executionID, req.ExitCode, req.Error)
		if err != nil {
			h.httpError(w, "Failed to mark failed", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}
