package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	storeTypes "jobplane/internal/store"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

// --- CreateJob Tests ---

func TestCreateJob_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	callbackURL := "https://hooks.example.com/jobs"
	callbackHeaders := json.RawMessage(`{"X-Api-Key":"secret"}`)

	job := &storeTypes.Job{
		ID:              uuid.New(),
		TenantID:        uuid.New(),
		Name:            "test-job",
		Image:           "python:3.9-slim",
		Command:         []string{"python", "main.py"},
		DefaultTimeout:  300,
		Priority:        50,
		CallbackURL:     &callbackURL,
		CallbackHeaders: callbackHeaders,
		CreatedAt:       time.Now().Truncate(time.Second),
	}

	cmdJSON, _ := json.Marshal(job.Command)

	mock.ExpectBegin()
	tx, _ := store.db.Begin()

	mock.ExpectExec(`INSERT INTO jobs`).
		WithArgs(job.ID, job.TenantID, job.Name, job.Image, cmdJSON, job.DefaultTimeout, job.Priority, job.CallbackURL, job.CallbackHeaders, job.CreatedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.CreateJob(ctx, tx, job)
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreateJob_WithoutCallback(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()

	job := &storeTypes.Job{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		Name:           "simple-job",
		Image:          "alpine:latest",
		Command:        []string{"echo", "hello"},
		DefaultTimeout: 60,
		Priority:       0,
		CreatedAt:      time.Now().Truncate(time.Second),
	}

	cmdJSON, _ := json.Marshal(job.Command)

	mock.ExpectBegin()
	tx, _ := store.db.Begin()

	mock.ExpectExec(`INSERT INTO jobs`).
		WithArgs(job.ID, job.TenantID, job.Name, job.Image, cmdJSON, job.DefaultTimeout, job.Priority, nil, []byte(nil), job.CreatedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.CreateJob(ctx, tx, job)
	if err != nil {
		t.Fatalf("CreateJob without callback failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreateJob_DatabaseError(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()

	job := &storeTypes.Job{
		ID:       uuid.New(),
		TenantID: uuid.New(),
		Name:     "fail-job",
		Image:    "alpine:latest",
		Command:  []string{"echo"},
	}

	mock.ExpectBegin()
	tx, _ := store.db.Begin()

	mock.ExpectExec(`INSERT INTO jobs`).
		WillReturnError(sql.ErrConnDone)

	err := store.CreateJob(ctx, tx, job)
	if err != sql.ErrConnDone {
		t.Errorf("expected sql.ErrConnDone, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// --- GetJobByID Tests ---
func TestGetJobByID_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	jobID := uuid.New()
	tenantID := uuid.New()
	callbackURL := "https://hooks.example.com/jobs"
	callbackHeaders := json.RawMessage(`{"Authorization":"Bearer token"}`)
	createdAt := time.Now().Truncate(time.Second)
	cmd := []string{"python", "train.py", "--epochs=10"}
	cmdJSON, _ := json.Marshal(cmd)

	mock.ExpectQuery(`SELECT \* FROM jobs WHERE id = \$1`).
		WithArgs(jobID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "image", "default_command",
			"default_timeout", "priority", "created_at", "callback_url", "callback_headers",
		}).AddRow(jobID, tenantID, "train-model", "ml:latest", cmdJSON, 600, 75, createdAt, callbackURL, callbackHeaders))

	job, err := store.GetJobByID(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJobByID failed: %v", err)
	}

	if job.ID != jobID {
		t.Errorf("got ID %v, want %v", job.ID, jobID)
	}
	if job.TenantID != tenantID {
		t.Errorf("got TenantID %v, want %v", job.TenantID, tenantID)
	}
	if job.Name != "train-model" {
		t.Errorf("got Name %s, want train-model", job.Name)
	}
	if job.Image != "ml:latest" {
		t.Errorf("got Image %s, want ml:latest", job.Image)
	}
	if len(job.Command) != 3 || job.Command[0] != "python" {
		t.Errorf("got Command %v, want %v", job.Command, cmd)
	}
	if job.DefaultTimeout != 600 {
		t.Errorf("got DefaultTimeout %d, want 600", job.DefaultTimeout)
	}
	if job.Priority != 75 {
		t.Errorf("got Priority %d, want 75", job.Priority)
	}
	if *job.CallbackURL != callbackURL {
		t.Errorf("got CallbackURL %v, want %v", *job.CallbackURL, callbackURL)
	}
	if string(job.CallbackHeaders) != string(callbackHeaders) {
		t.Errorf("got CallbackHeaders %s, want %s", job.CallbackHeaders, callbackHeaders)
	}
	if !job.CreatedAt.Equal(createdAt) {
		t.Errorf("got CreatedAt %v, want %v", job.CreatedAt, createdAt)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetJobByID_WithoutCallback(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	jobID := uuid.New()
	tenantID := uuid.New()
	createdAt := time.Now().Truncate(time.Second)

	mock.ExpectQuery(`SELECT \* FROM jobs WHERE id = \$1`).
		WithArgs(jobID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "image", "default_command",
			"default_timeout", "priority", "created_at", "callback_url", "callback_headers",
		}).AddRow(jobID, tenantID, "simple-job", "alpine:latest", []byte(`["echo","hello"]`), 60, 0, createdAt, nil, []byte(nil)))

	job, err := store.GetJobByID(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJobByID failed: %v", err)
	}

	if job.CallbackURL != nil {
		t.Errorf("got CallbackURL %v, want nil", *job.CallbackURL)
	}
	if job.CallbackHeaders != nil {
		t.Errorf("got CallbackHeaders %s, want nil", job.CallbackHeaders)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetJobByID_NotFound(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	jobID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM jobs WHERE id = \$1`).
		WithArgs(jobID).
		WillReturnError(sql.ErrNoRows)

	job, err := store.GetJobByID(ctx, jobID)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
	if job != nil {
		t.Error("expected nil job")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetJobByID_EmptyCommand(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	jobID := uuid.New()
	tenantID := uuid.New()
	createdAt := time.Now().Truncate(time.Second)

	mock.ExpectQuery(`SELECT \* FROM jobs WHERE id = \$1`).
		WithArgs(jobID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "image", "default_command",
			"default_timeout", "priority", "created_at", "callback_url", "callback_headers",
		}).AddRow(jobID, tenantID, "no-cmd-job", "alpine:latest", []byte(nil), 60, 0, createdAt, nil, []byte(nil)))

	job, err := store.GetJobByID(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJobByID failed: %v", err)
	}

	if len(job.Command) != 0 {
		t.Errorf("expected empty command, got %v", job.Command)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// --- CreateExecution Tests ---

func TestCreateExecution_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	callbackURL := "https://hooks.example.com/executions"
	callbackHeaders := json.RawMessage(`{"X-Webhook-Secret":"s3cret"}`)
	callbackStatus := storeTypes.CallbackStatusPending

	execution := &storeTypes.Execution{
		ID:              uuid.New(),
		JobID:           uuid.New(),
		TenantID:        uuid.New(),
		Status:          storeTypes.ExecutionStatusPending,
		Priority:        75,
		CallbackURL:     &callbackURL,
		CallbackHeaders: callbackHeaders,
		CallbackStatus:  &callbackStatus,
		CreatedAt:       now,
		ScheduledAt:     &now,
	}

	mock.ExpectBegin()
	tx, _ := store.db.Begin()

	mock.ExpectExec(`INSERT INTO executions`).
		WithArgs(
			execution.ID, execution.JobID, execution.TenantID,
			execution.Status, execution.Priority,
			execution.CallbackURL, execution.CallbackHeaders, execution.CallbackStatus,
			execution.CreatedAt, execution.ScheduledAt,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.CreateExecution(ctx, tx, execution)
	if err != nil {
		t.Fatalf("CreateExecution failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreateExecution_WithoutCallback(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	execution := &storeTypes.Execution{
		ID:          uuid.New(),
		JobID:       uuid.New(),
		TenantID:    uuid.New(),
		Status:      storeTypes.ExecutionStatusPending,
		Priority:    0,
		CreatedAt:   now,
		ScheduledAt: &now,
	}

	mock.ExpectBegin()
	tx, _ := store.db.Begin()

	mock.ExpectExec(`INSERT INTO executions`).
		WithArgs(
			execution.ID, execution.JobID, execution.TenantID,
			execution.Status, execution.Priority,
			nil, []byte(nil), nil,
			execution.CreatedAt, execution.ScheduledAt,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.CreateExecution(ctx, tx, execution)
	if err != nil {
		t.Fatalf("CreateExecution without callback failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreateExecution_DatabaseError(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	execution := &storeTypes.Execution{
		ID:        uuid.New(),
		JobID:     uuid.New(),
		TenantID:  uuid.New(),
		Status:    storeTypes.ExecutionStatusPending,
		CreatedAt: now,
	}

	mock.ExpectBegin()
	tx, _ := store.db.Begin()

	mock.ExpectExec(`INSERT INTO executions`).
		WillReturnError(sql.ErrConnDone)

	err := store.CreateExecution(ctx, tx, execution)
	if err != sql.ErrConnDone {
		t.Errorf("expected sql.ErrConnDone, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
