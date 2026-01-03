package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"jobplane/internal/store"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGetExecutionByID_Success(t *testing.T) {
	store_, mock := newMockStore(t)
	defer store_.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	jobID := uuid.New()
	tenantID := uuid.New()
	exitCode := 0
	errMsg := ""
	startedAt := time.Now().Add(-5 * time.Minute)
	completedAt := time.Now().Add(-4 * time.Minute)

	mock.ExpectQuery(`SELECT \* FROM executions WHERE id = \$1`).
		WithArgs(executionID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "job_id", "tenant_id", "status", "attempt",
			"exit_code", "error_message", "created_at", "started_at", "finished_at",
		}).AddRow(
			executionID, jobID, tenantID, "SUCCEEDED", 1,
			exitCode, errMsg, time.Now(), startedAt, completedAt,
		))

	execution, err := store_.GetExecutionByID(ctx, executionID)
	if err != nil {
		t.Fatalf("GetExecutionByID failed: %v", err)
	}

	if execution.ID != executionID {
		t.Errorf("got ID %v, want %v", execution.ID, executionID)
	}
	if execution.JobID != jobID {
		t.Errorf("got JobID %v, want %v", execution.JobID, jobID)
	}
	if execution.TenantID != tenantID {
		t.Errorf("got TenantID %v, want %v", execution.TenantID, tenantID)
	}
	if execution.Status != store.ExecutionStatusCompleted {
		t.Errorf("got Status %v, want %v", execution.Status, store.ExecutionStatusCompleted)
	}
	if execution.Attempt != 1 {
		t.Errorf("got Attempt %d, want 1", execution.Attempt)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetExecutionByID_NotFound(t *testing.T) {
	store_, mock := newMockStore(t)
	defer store_.db.Close()

	ctx := context.Background()
	executionID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM executions WHERE id = \$1`).
		WithArgs(executionID).
		WillReturnError(sql.ErrNoRows)

	_, err := store_.GetExecutionByID(ctx, executionID)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestGetExecutionByID_DatabaseError(t *testing.T) {
	store_, mock := newMockStore(t)
	defer store_.db.Close()

	ctx := context.Background()
	executionID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM executions WHERE id = \$1`).
		WithArgs(executionID).
		WillReturnError(sql.ErrConnDone)

	_, err := store_.GetExecutionByID(ctx, executionID)
	if err == nil {
		t.Error("expected error, got nil")
	}
}
