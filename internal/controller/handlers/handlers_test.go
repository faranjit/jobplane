package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"jobplane/internal/store"
	"time"

	"github.com/google/uuid"
)

// Mock transaction
type mockTx struct{}

func (m *mockTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, nil
}
func (m *mockTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, nil
}
func (m *mockTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return nil
}

func (m *mockTx) Commit() error { return nil }

func (m *mockTx) Rollback() error { return nil }

// Mock Store
type mockStore struct {
	// Job Hooks
	beginTxErr     error
	createJobErr   error
	getJobByIDErr  error
	getJobByIDResp *store.Job
	createExecErr  error
	enqueueErr     error
	pingErr        error

	// Tenant Hooks
	createTenantErr error
	updateTenantErr error

	// Execution Hooks
	getExecutionResp           *store.Execution
	getExecutionErr            error
	setVisibleErr              error
	completeErr                error
	failErr                    error
	countRunningExecutionsResp int64
	countRunningExecutionsErr  error

	// Log Hooks
	addLogEntryErr       error
	getExecutionLogsResp []store.LogEntry
	getExecutionLogsErr  error

	// Spies (to verify arguments passed by handlers)
	capturedAfterID      int64
	capturedLimit        int
	capturedVisibleAfter time.Time
	capturedPriority     int

	dequeueBatchResp []store.QueueItem
	dequeueBatchErr  error
}

func (m *mockStore) BeginTx(ctx context.Context) (store.Tx, error) {
	if m.beginTxErr != nil {
		return nil, m.beginTxErr
	}
	return &mockTx{}, nil
}

func (m *mockStore) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockStore) CreateJob(ctx context.Context, tx store.DBTransaction, job *store.Job) error {
	return m.createJobErr
}

func (m *mockStore) GetJobByID(ctx context.Context, id uuid.UUID) (*store.Job, error) {
	return m.getJobByIDResp, m.getJobByIDErr
}

func (m *mockStore) CreateExecution(ctx context.Context, tx store.DBTransaction, execution *store.Execution) error {
	m.capturedPriority = execution.Priority
	return m.createExecErr
}

func (m *mockStore) Enqueue(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, payload json.RawMessage, visibleAfter time.Time) (int64, error) {
	m.capturedVisibleAfter = visibleAfter
	return 1, m.enqueueErr
}

func (m *mockStore) CreateTenant(ctx context.Context, tenant *store.Tenant, hashedKey string) error {
	// Simulate DB error if configured
	return m.createTenantErr
}

func (m *mockStore) GetTenantByID(ctx context.Context, id uuid.UUID) (*store.Tenant, error) {
	return nil, nil // Not used in handlers yet
}

func (m *mockStore) GetTenantByAPIKeyHash(ctx context.Context, hash string) (*store.Tenant, error) {
	return nil, nil // Handled by Auth Middleware, not Handlers
}

func (m *mockStore) UpdateTenant(ctx context.Context, tenant *store.Tenant) error {
	return m.updateTenantErr
}

func (m *mockStore) DequeueBatch(ctx context.Context, tenantIDs []uuid.UUID, limit int) ([]store.QueueItem, error) {
	return m.dequeueBatchResp, m.dequeueBatchErr
}

func (m *mockStore) GetExecutionByID(ctx context.Context, id uuid.UUID) (*store.Execution, error) {
	return m.getExecutionResp, m.getExecutionErr
}

func (m *mockStore) AddLogEntry(ctx context.Context, executionID uuid.UUID, content string) error {
	return m.addLogEntryErr
}

func (m *mockStore) GetExecutionLogs(ctx context.Context, executionID uuid.UUID, afterID int64, limit int) ([]store.LogEntry, error) {
	m.capturedAfterID = afterID
	m.capturedLimit = limit
	return m.getExecutionLogsResp, m.getExecutionLogsErr
}

func (m *mockStore) SetVisibleAfter(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, visibleAfter time.Time) error {
	m.capturedVisibleAfter = visibleAfter
	return m.setVisibleErr
}

func (m *mockStore) Complete(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, exitCode int, result json.RawMessage) error {
	return m.completeErr
}

func (m *mockStore) Fail(ctx context.Context, tx store.DBTransaction, executionID uuid.UUID, exitCode *int, errMsg string) error {
	return m.failErr
}

func (m *mockStore) Count(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockStore) CountRunningExecutions(ctx context.Context, tx store.DBTransaction, tenantID uuid.UUID) (int64, error) {
	return m.countRunningExecutionsResp, m.countRunningExecutionsErr
}

// DLQ Hooks
var (
	listDLQResp      []store.DLQEntry
	listDLQErr       error
	retryFromDLQResp uuid.UUID
	retryFromDLQErr  error
)

func (m *mockStore) ListDLQ(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]store.DLQEntry, error) {
	return listDLQResp, listDLQErr
}

func (m *mockStore) RetryFromDLQ(ctx context.Context, executionID uuid.UUID) (uuid.UUID, error) {
	return retryFromDLQResp, retryFromDLQErr
}
