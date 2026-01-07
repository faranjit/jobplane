package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
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
	visibleAfter := time.Now()

	mock.ExpectQuery(`INSERT INTO execution_queue`).
		WithArgs(executionID, payload, visibleAfter).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(expectedQueueID))

	id, err := store.Enqueue(ctx, nil, executionID, payload, visibleAfter)
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

	_, err := store.Enqueue(ctx, nil, executionID, payload, time.Now())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// DequeueBatch Tests
func TestDequeueBatch_Success(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	exec1 := uuid.New()
	exec2 := uuid.New()
	queueID1 := int64(1)
	queueID2 := int64(2)
	payload1 := json.RawMessage(`{"task": "test1"}`)
	payload2 := json.RawMessage(`{"task": "test2"}`)

	mock.ExpectBegin()

	// SELECT ... FOR UPDATE SKIP LOCKED LIMIT 3
	mock.ExpectQuery(`SELECT id, execution_id, payload FROM execution_queue`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "execution_id", "payload"}).
			AddRow(queueID1, exec1, payload1).
			AddRow(queueID2, exec2, payload2))

	// Bulk UPDATE visibility timeout
	mock.ExpectExec(`UPDATE execution_queue`).
		WillReturnResult(sqlmock.NewResult(0, 2))

	// Bulk UPDATE executions status
	mock.ExpectExec(`UPDATE executions`).
		WillReturnResult(sqlmock.NewResult(0, 2))

	mock.ExpectCommit()

	items, err := store.DequeueBatch(ctx, nil, 3)
	if err != nil {
		t.Fatalf("DequeueBatch failed: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if items[0].ExecutionID != exec1 {
		t.Errorf("got executionID %v, want %v", items[0].ExecutionID, exec1)
	}
	if items[1].ExecutionID != exec2 {
		t.Errorf("got executionID %v, want %v", items[1].ExecutionID, exec2)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestDequeueBatch_PriorityQueryStructure(t *testing.T) {
	// We use sqlmock NOT to test sorting, but to test that we generated the correct SQL.
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	tenantID := uuid.New()

	// Verify that the generated SQL explicitly includes "ORDER BY priority DESC"
	// and "created_at ASC". This catches regression if someone deletes the sorting logic.
	mock.ExpectBegin()
	// SELECT id, execution_id, payload FROM execution_queue WHERE visible_after <= NOW() AND tenant_id = ANY($2) ORDER BY priority DESC, created_at ASC FOR UPDATE SKIP LOCKED LIMIT $1
	mock.ExpectQuery(`SELECT id, execution_id, payload FROM execution_queue .* ORDER BY priority DESC, created_at ASC FOR UPDATE SKIP LOCKED .*`).
		WithArgs(1, sqlmock.AnyArg()). // Limit, TenantID array
		WillReturnRows(sqlmock.NewRows([]string{"id", "execution_id", "payload"}).
			AddRow(100, uuid.New(), []byte("{}"))) // Mock returns dummy data

	// Mock the subsequent updates (attempt count sync)
	mock.ExpectExec(`UPDATE execution_queue`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE executions`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	items, err := store.DequeueBatch(ctx, []uuid.UUID{tenantID}, 1)

	if err != nil {
		t.Fatalf("DequeueBatch failed: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestDequeueBatch_EmptyQueue(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, execution_id, payload FROM execution_queue`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "execution_id", "payload"})) // Empty result
	mock.ExpectRollback()

	items, err := store.DequeueBatch(ctx, nil, 5)
	if err != nil {
		t.Errorf("expected no error for empty queue, got %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestDequeueBatch_WithTenantFilter(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	tenantIDs := []uuid.UUID{uuid.New(), uuid.New()}
	execID := uuid.New()
	queueID := int64(5)
	payload := json.RawMessage(`{}`)

	mock.ExpectBegin()

	// Should include tenant filter in query
	mock.ExpectQuery(`SELECT id, execution_id, payload FROM execution_queue`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "execution_id", "payload"}).
			AddRow(queueID, execID, payload))

	mock.ExpectExec(`UPDATE execution_queue`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(`UPDATE executions`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	items, err := store.DequeueBatch(ctx, tenantIDs, 10)
	if err != nil {
		t.Fatalf("DequeueBatch with tenant filter failed: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestDequeueBatch_LimitDefaultsToOne(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, execution_id, payload FROM execution_queue`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "execution_id", "payload"}))
	mock.ExpectRollback()

	// Limit of 0 should default to 1
	_, err := store.DequeueBatch(ctx, nil, 0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDequeueBatch_SingleItem(t *testing.T) {
	store, mock := newMockStore(t)
	defer store.db.Close()

	ctx := context.Background()
	execID := uuid.New()
	queueID := int64(1)
	payload := json.RawMessage(`{"single": true}`)

	mock.ExpectBegin()

	mock.ExpectQuery(`SELECT id, execution_id, payload FROM execution_queue`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "execution_id", "payload"}).
			AddRow(queueID, execID, payload))

	mock.ExpectExec(`UPDATE execution_queue`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(`UPDATE executions`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	items, err := store.DequeueBatch(ctx, nil, 1)
	if err != nil {
		t.Fatalf("DequeueBatch failed: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if string(items[0].Payload) != string(payload) {
		t.Errorf("got payload %s, want %s", items[0].Payload, payload)
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

	// Insert into DLQ first
	mock.ExpectExec(`INSERT INTO execution_dlq`).
		WithArgs(errMsg, executionID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Then delete from queue
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

	// Insert into DLQ first
	mock.ExpectExec(`INSERT INTO execution_dlq`).
		WithArgs(errMsg, executionID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Treat as permanent failure - delete from queue
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
