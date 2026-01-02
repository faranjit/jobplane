package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestAddLogEntry(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	executionID := uuid.New()
	content := "New test logs..."

	mock.ExpectExec(`INSERT INTO execution_logs`).
		WithArgs(executionID, content).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := s.AddLogEntry(ctx, executionID, content); err != nil {
		t.Fatalf("AddLogEntry failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetExecutionLogs(t *testing.T) {
	s, mock := newMockStore(t)
	defer s.db.Close()

	ctx := context.Background()
	executinID := uuid.New()
	afterID := int64(100)
	limit := 50

	// Mock rows return
	rows := sqlmock.NewRows([]string{"id", "execution_id", "content", "created_at"}).
		AddRow(101, executinID, "Log 101", time.Now().Add(-2*time.Second)).
		AddRow(102, executinID, "Log 102", time.Now().Add(-1*time.Second))

	mock.ExpectQuery(`SELECT id, execution_id, content, created_at FROM execution_logs`).
		WithArgs(executinID, afterID, limit).
		WillReturnRows(rows)

	logs, err := s.GetExecutionLogs(ctx, executinID, afterID, limit)
	if err != nil {
		t.Fatalf("GetExecutionLogs failed: %v", err)
	}

	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}

	if logs[0].ID != 101 {
		t.Errorf("expected first log ID 101, got %d", logs[0].ID)
	}

	if logs[1].ID != 102 {
		t.Errorf("expected second log ID 102, got %d", logs[0].ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
