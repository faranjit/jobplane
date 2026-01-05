package handlers

import (
	"encoding/json"
	"io"
	"jobplane/internal/controller/middleware"
	"jobplane/internal/store"
	"jobplane/pkg/api"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// CreateJob handles POST /jobs.
// It saves a reusable Job Definition (blueprint) to the database.
func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req api.CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Image == "" {
		h.httpError(w, "Name and Image are required", http.StatusBadRequest)
		return
	}

	tenantID, ok := middleware.TenantIDFromContext(ctx)
	if !ok {
		h.httpError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	jobID := uuid.New()

	job := &store.Job{
		ID:             jobID,
		TenantID:       tenantID,
		Name:           req.Name,
		Image:          req.Image,
		Command:        req.Command,
		DefaultTimeout: req.DefaultTimeout,
		CreatedAt:      time.Now().UTC(),
	}

	tx, err := h.store.BeginTx(ctx)
	if err != nil {
		h.httpError(w, "Internal database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Save job
	if err := h.store.CreateJob(ctx, tx, job); err != nil {
		h.httpError(w, "Failed to create job", http.StatusInternalServerError)
		return
	}

	// Commit
	if err := tx.Commit(); err != nil {
		h.httpError(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	resp := api.CreateJobResponse{
		JobID: jobID.String(),
	}
	h.respondJson(w, http.StatusOK, resp)
}

// RunJob handles POST /jobs/{id}/run.
// Creates an execution history and enqueues it,
// so workers can pull it to run.
func (h *Handlers) RunJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	jobIDStr := r.PathValue("id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		h.httpError(w, "Invalid job id", http.StatusBadRequest)
		return
	}

	tenantID, ok := middleware.TenantIDFromContext(ctx)
	if !ok {
		h.httpError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse optional body for scheduling
	var req api.RunJobRequest
	// Empty body is allowed, so check errors carefully
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if err != io.EOF {
				h.httpError(w, "Invalid request body", http.StatusBadRequest)
				return
			}
		}
	}

	job, err := h.store.GetJobByID(ctx, jobID)
	if err != nil || job.TenantID != tenantID {
		h.httpError(w, "Related job not found", http.StatusNotFound)
		return
	}

	status := store.ExecutionStatusPending
	now := time.Now().UTC()
	scheduledAt := now
	createdAt := now

	if req.ScheduledAt != nil {
		if req.ScheduledAt.Before(now) {
			h.httpError(w, "Scheduled time cannot be in the past", http.StatusBadRequest)
			return
		}
		status = store.ExecutionStatusScheduled
		scheduledAt = *req.ScheduledAt
	}

	execution := &store.Execution{
		ID:          uuid.New(),
		JobID:       jobID,
		TenantID:    tenantID,
		Status:      status,
		ScheduledAt: &scheduledAt,
		CreatedAt:   createdAt,
	}

	tx, err := h.store.BeginTx(ctx)
	if err != nil {
		h.httpError(w, "Internal database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Create execution history
	if err := h.store.CreateExecution(ctx, tx, execution); err != nil {
		h.httpError(w, "Failed to create execution", http.StatusInternalServerError)
		return
	}

	// Enqueue
	// Serialize the job details into the queue payload so the worker
	// doesn't need to query the 'jobs' table
	paylod, _ := json.Marshal(job)
	if _, err := h.store.Enqueue(ctx, tx, execution.ID, paylod, scheduledAt); err != nil {
		h.httpError(w, "Failed to enqueue", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		h.httpError(w, "Failed to commit transaction", http.StatusInternalServerError)
	}

	h.respondJson(w, http.StatusOK, api.RunJobResponse{ExecutionID: execution.ID.String()})
}
