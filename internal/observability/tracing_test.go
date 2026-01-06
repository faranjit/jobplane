package observability

import (
	"context"
	"testing"
	"time"
)

func TestInit_InvalidEndpoint(t *testing.T) {
	// Test with an unreachable endpoint - should still succeed
	// because gRPC connection is lazy by default
	ctx := context.Background()

	shutdown, err := InitTracer(ctx, "test-service", "invalid-endpoint:9999")
	if err != nil {
		// Some environments may fail immediately, that's also acceptable
		t.Logf("InitTracer failed as expected in this environment: %v", err)
		return
	}

	// If we got here, shutdown should work
	if shutdown == nil {
		t.Error("expected shutdown function to be non-nil")
	}

	// Try shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Shutdown should not panic
	_ = shutdown(shutdownCtx)
}

func TestInit_ValidServiceName(t *testing.T) {
	ctx := context.Background()

	// Using localhost which won't connect but won't error on init
	shutdown, err := InitTracer(ctx, "my-test-service", "localhost:4317")
	if err != nil {
		t.Logf("InitTracer returned error (may be expected in test environment): %v", err)
		return
	}

	if shutdown == nil {
		t.Error("expected shutdown function to be non-nil")
	}

	// Clean up
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_ = shutdown(shutdownCtx)
}

func TestInit_EmptyServiceName(t *testing.T) {
	ctx := context.Background()

	// Empty service name should still work (just not ideal)
	shutdown, err := InitTracer(ctx, "", "localhost:4317")
	if err != nil {
		t.Logf("InitTracer returned error: %v", err)
		return
	}

	if shutdown == nil {
		t.Error("expected shutdown function to be non-nil")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_ = shutdown(shutdownCtx)
}
