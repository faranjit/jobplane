package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"jobplane/internal/store"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGetExecutionByID_Success(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	jobID := uuid.New()
	tenantID := uuid.New()
	exitCode := 0
	errMsg := ""
	startedAt := time.Now().Add(-5 * time.Minute)
	completedAt := time.Now().Add(-4 * time.Minute)
	now := time.Now()

	mock.ExpectQuery(`SELECT id, job_id, tenant_id, status, priority, attempt, exit_code, error_message, retried_from, created_at, scheduled_at, started_at, finished_at, result FROM executions WHERE id = \$1`).
		WithArgs(executionID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "job_id", "tenant_id", "status", "priority", "attempt",
			"exit_code", "error_message", "retried_from",
			"scheduled_at", "created_at", "started_at", "finished_at", "result",
		}).AddRow(
			executionID, jobID, tenantID, "SUCCEEDED", 61, 1,
			exitCode, errMsg, nil, now, now, startedAt, completedAt, []byte(nil),
		))

	execution, err := s.GetExecutionByID(ctx, executionID)
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
	if execution.ScheduledAt.IsZero() {
		t.Errorf("want ScheduledAt %s", now)
	}
	if execution.CreatedAt.IsZero() {
		t.Errorf("want CreatedAt %s", now)
	}
	if execution.Priority != 61 {
		t.Errorf("got Priority %d, want 61", execution.Priority)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetExecutionByID_NotFound(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	executionID := uuid.New()

	mock.ExpectQuery(`SELECT id, job_id, tenant_id, status, priority, attempt, exit_code, error_message, retried_from, created_at, scheduled_at, started_at, finished_at, result FROM executions WHERE id = \$1`).
		WithArgs(executionID).
		WillReturnError(sql.ErrNoRows)

	_, err := s.GetExecutionByID(ctx, executionID)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestGetExecutionByID_DatabaseError(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	executionID := uuid.New()

	mock.ExpectQuery(`SELECT id, job_id, tenant_id, status, priority, attempt, exit_code, error_message, retried_from, created_at, scheduled_at, started_at, finished_at, result FROM executions WHERE id = \$1`).
		WithArgs(executionID).
		WillReturnError(sql.ErrConnDone)

	_, err := s.GetExecutionByID(ctx, executionID)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestCountRunningExecutions_Success(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()
	count := int64(3)

	// Expect advisory lock first
	tenantLockKey := int32(tenantID[0])<<24 | int32(tenantID[1])<<16 | int32(tenantID[2])<<8 | int32(tenantID[3])
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(1, $1)`)).
		WithArgs(tenantLockKey).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Then expect count query
	mock.ExpectQuery(
		regexp.QuoteMeta(`SELECT COUNT(*) FROM executions WHERE tenant_id = $1 AND status = $2`),
	).WithArgs(tenantID, store.ExecutionStatusRunning).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).
			AddRow(count))

	actualCount, err := s.CountRunningExecutions(ctx, nil, tenantID)
	if err != nil {
		t.Fatalf("CountRunningExecutions failed: %v", err)
	}

	if actualCount != count {
		t.Errorf("got count %d, want %d", actualCount, count)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCountRunningExecutions_NotFound(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()

	// Expect advisory lock first
	tenantLockKey := int32(tenantID[0])<<24 | int32(tenantID[1])<<16 | int32(tenantID[2])<<8 | int32(tenantID[3])
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(1, $1)`)).
		WithArgs(tenantLockKey).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Then expect count query
	mock.ExpectQuery(
		regexp.QuoteMeta(`SELECT COUNT(*) FROM executions WHERE tenant_id = $1 AND status = $2`),
	).WithArgs(tenantID, store.ExecutionStatusRunning).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).
			AddRow(int64(0)))

	actualCount, err := s.CountRunningExecutions(ctx, nil, tenantID)
	if err != nil {
		t.Fatalf("CountRunningExecutions failed: %v", err)
	}

	if actualCount != 0 {
		t.Errorf("got count %d, want 0", actualCount)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCountRunningExecutions_DatabaseError(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()

	// Advisory lock fails
	tenantLockKey := int32(tenantID[0])<<24 | int32(tenantID[1])<<16 | int32(tenantID[2])<<8 | int32(tenantID[3])
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(1, $1)`)).
		WithArgs(tenantLockKey).
		WillReturnError(sql.ErrConnDone)

	_, err := s.CountRunningExecutions(ctx, nil, tenantID)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestListDLQ_Success(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()
	executionID := uuid.New()
	jobName := "test-job"
	errMsg := "max retries exceeded"
	failedAt := time.Now()

	mock.ExpectQuery(`SELECT dlq\.\*, e\.job_id, j\.name as job_name, e\.priority FROM execution_dlq dlq`).
		WithArgs(tenantID, 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "execution_id", "tenant_id", "payload", "error_message", "attempts", "failed_at",
			"job_id", "job_name", "priority",
		}).AddRow(
			1, executionID, tenantID, []byte(`{"key":"value"}`), errMsg, 5, failedAt,
			uuid.New(), jobName, 50,
		))

	entries, err := s.ListDLQ(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListDLQ failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.ExecutionID != executionID {
		t.Errorf("got ExecutionID %v, want %v", entry.ExecutionID, executionID)
	}
	if entry.JobName != jobName {
		t.Errorf("got JobName %s, want %s", entry.JobName, jobName)
	}
	if entry.Attempts != 5 {
		t.Errorf("got Attempts %d, want 5", entry.Attempts)
	}
	if entry.Priority != 50 {
		t.Errorf("got Priority %d, want 50", entry.Priority)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestListDLQ_Empty(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()

	mock.ExpectQuery(`SELECT dlq\.\*, e\.job_id, j\.name as job_name, e\.priority FROM execution_dlq dlq`).
		WithArgs(tenantID, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "execution_id", "tenant_id", "payload", "error_message", "attempts", "failed_at",
			"job_id", "job_name", "priority",
		}))

	entries, err := s.ListDLQ(ctx, tenantID, 20, 0)
	if err != nil {
		t.Fatalf("ListDLQ failed: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRetryFromDLQ_Success(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	jobID := uuid.New()
	tenantID := uuid.New()
	payload := []byte(`{"test": true}`)

	mock.ExpectBegin()

	// SELECT from DLQ with JOIN
	mock.ExpectQuery(`SELECT dlq\.id, dlq\.payload, e\.id, e\.job_id, e\.tenant_id, e\.priority FROM execution_dlq dlq`).
		WithArgs(executionID).
		WillReturnRows(sqlmock.NewRows([]string{
			"dlq_id", "payload", "exec_id", "job_id", "tenant_id", "priority",
		}).AddRow(
			1, payload, executionID, jobID, tenantID, 75,
		))

	// INSERT new execution
	mock.ExpectExec(`INSERT INTO executions`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Enqueue new execution
	mock.ExpectQuery(`INSERT INTO execution_queue`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	// DELETE from DLQ
	mock.ExpectExec(`DELETE FROM execution_dlq`).
		WithArgs(executionID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	newExecID, err := s.RetryFromDLQ(ctx, executionID)
	if err != nil {
		t.Fatalf("RetryFromDLQ failed: %v", err)
	}

	if newExecID == uuid.Nil {
		t.Error("expected non-nil execution ID")
	}

	if newExecID == executionID {
		t.Error("new execution ID should be different from original")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRetryFromDLQ_NotFound(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	executionID := uuid.New()

	mock.ExpectBegin()

	mock.ExpectQuery(`SELECT dlq\.id, dlq\.payload, e\.id, e\.job_id, e\.tenant_id, e\.priority FROM execution_dlq dlq`).
		WithArgs(executionID).
		WillReturnError(sql.ErrNoRows)

	mock.ExpectRollback()

	_, err := s.RetryFromDLQ(ctx, executionID)
	if err == nil {
		t.Error("expected error for non-existent DLQ entry")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
