package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	return &Store{db: db}, mock
}

func TestEnqueue_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	payload := json.RawMessage(`{"key": "value"}`)
	expectedQueueID := int64(42)

	mock.ExpectQuery(`INSERT INTO execution_queue`).
		WithArgs(executionID, payload).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(expectedQueueID))

	id, err := store.Enqueue(ctx, nil, executionID, payload)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if id != expectedQueueID {
		t.Errorf("got id %d, want %d", id, expectedQueueID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestEnqueue_ExecutionNotFound(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	payload := json.RawMessage(`{}`)

	mock.ExpectQuery(`INSERT INTO execution_queue`).
		WithArgs(executionID, payload).
		WillReturnError(sql.ErrNoRows)

	_, err := store.Enqueue(ctx, nil, executionID, payload)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestDequeue_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	queueID := int64(1)
	payload := json.RawMessage(`{"task": "test"}`)

	mock.ExpectBegin()

	// SELECT ... FOR UPDATE SKIP LOCKED
	mock.ExpectQuery(`SELECT id, execution_id, payload FROM execution_queue`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "execution_id", "payload"}).
			AddRow(queueID, executionID, payload))

	// UPDATE visibility timeout
	mock.ExpectExec(`UPDATE execution_queue`).
		WithArgs(VisibilityTimeout.Seconds(), queueID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// UPDATE executions status
	mock.ExpectExec(`UPDATE executions`).
		WithArgs("RUNNING", executionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	resultID, resultPayload, err := store.Dequeue(ctx, nil)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if resultID != executionID {
		t.Errorf("got executionID %v, want %v", resultID, executionID)
	}
	if string(resultPayload) != string(payload) {
		t.Errorf("got payload %s, want %s", resultPayload, payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestDequeue_EmptyQueue(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, execution_id, payload FROM execution_queue`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, _, err := store.Dequeue(ctx, nil)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestDequeue_WithTenantFilter(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	tenantIDs := []uuid.UUID{uuid.New(), uuid.New()}
	executionID := uuid.New()
	queueID := int64(5)
	payload := json.RawMessage(`{}`)

	mock.ExpectBegin()

	// Should include tenant filter
	mock.ExpectQuery(`SELECT id, execution_id, payload FROM execution_queue`).
		WithArgs(pq.Array(tenantIDs)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "execution_id", "payload"}).
			AddRow(queueID, executionID, payload))

	mock.ExpectExec(`UPDATE execution_queue`).
		WithArgs(VisibilityTimeout.Seconds(), queueID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(`UPDATE executions`).
		WithArgs("RUNNING", executionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	_, _, err := store.Dequeue(ctx, tenantIDs)
	if err != nil {
		t.Fatalf("Dequeue with tenant filter failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestComplete_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	exitCode := 0

	mock.ExpectExec(`DELETE FROM execution_queue`).
		WithArgs(executionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(`UPDATE executions`).
		WithArgs("SUCCEEDED", exitCode, executionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Complete(ctx, nil, executionID, exitCode)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestFail_WithRetry(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	currentAttempt := 2 // Less than MaxRetries (5)

	// Query current attempt
	mock.ExpectQuery(`SELECT attempt FROM execution_queue`).
		WithArgs(executionID).
		WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(currentAttempt))

	// Expect retry with exponential backoff: 10 * 2^2 = 40 seconds
	expectedBackoff := time.Duration(10*(1<<currentAttempt)) * time.Second
	mock.ExpectExec(`UPDATE execution_queue`).
		WithArgs(expectedBackoff.Seconds(), executionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Fail(ctx, nil, executionID, nil, "temporary error")
	if err != nil {
		t.Fatalf("Fail with retry failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestFail_PermanentFailure(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	errMsg := "max retries exceeded"

	// Query current attempt - exceeds MaxRetries
	mock.ExpectQuery(`SELECT attempt FROM execution_queue`).
		WithArgs(executionID).
		WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(MaxRetries + 1))

	// Expect permanent failure path
	mock.ExpectExec(`DELETE FROM execution_queue`).
		WithArgs(executionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	exitCode := -1
	mock.ExpectExec(`UPDATE executions`).
		WithArgs("FAILED", exitCode, errMsg, executionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Fail(ctx, nil, executionID, &exitCode, errMsg)
	if err != nil {
		t.Fatalf("Fail permanent failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestFail_JobNotInQueue(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	errMsg := "job vanished"

	// Job not found in queue
	mock.ExpectQuery(`SELECT attempt FROM execution_queue`).
		WithArgs(executionID).
		WillReturnError(sql.ErrNoRows)

	// Treat as permanent failure
	mock.ExpectExec(`DELETE FROM execution_queue`).
		WithArgs(executionID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	exitCode := 61
	mock.ExpectExec(`UPDATE executions`).
		WithArgs("FAILED", exitCode, errMsg, executionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Fail(ctx, nil, executionID, &exitCode, errMsg)
	if err != nil {
		t.Fatalf("Fail job not in queue failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSetVisibleAfter_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	visibleAfter := time.Now().Add(10 * time.Minute)

	mock.ExpectExec(`UPDATE execution_queue`).
		WithArgs(visibleAfter, executionID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.SetVisibleAfter(ctx, nil, executionID, visibleAfter)
	if err != nil {
		t.Fatalf("SetVisibleAfter failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
