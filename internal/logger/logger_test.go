package logger

import (
	"context"
	"testing"
)

func TestWithRequestID_And_RequestIDFromContext(t *testing.T) {
	ctx := context.Background()
	requestID := "req-12345"

	// Initially empty
	if got := RequestIDFromContext(ctx); got != "" {
		t.Errorf("RequestIDFromContext() on empty ctx = %v, want empty", got)
	}

	// After setting
	ctx = WithRequestID(ctx, requestID)
	if got := RequestIDFromContext(ctx); got != requestID {
		t.Errorf("RequestIDFromContext() = %v, want %v", got, requestID)
	}
}

func TestFromContext_WithRequestID(t *testing.T) {
	base := New()
	ctx := context.Background()
	requestID := "req-67890"

	// Without request ID - should return base logger (not nil)
	logger := FromContext(ctx, base)
	if logger == nil {
		t.Error("FromContext() returned nil")
	}

	// With request ID - should return logger with request_id attached
	ctx = WithRequestID(ctx, requestID)
	loggerWithID := FromContext(ctx, base)
	if loggerWithID == nil {
		t.Error("FromContext() with request ID returned nil")
	}
}

func TestNew_ReturnsLogger(t *testing.T) {
	logger := New()
	if logger == nil {
		t.Error("New() returned nil")
	}
}
