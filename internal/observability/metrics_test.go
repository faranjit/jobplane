package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func TestInitMetrics(t *testing.T) {
	handler, shutdown, err := InitMetrics()
	if err != nil {
		t.Fatalf("InitMetrics failed: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx)
	}()

	if handler == nil {
		t.Fatal("expected handler to be non-nil")
	}
	if shutdown == nil {
		t.Fatal("expected shutdown function to be non-nil")
	}

	// Smoke test: verify handler returns 200 and non-empty body
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}
	if rr.Body.Len() == 0 {
		t.Error("handler returned empty body")
	}
}

func TestInitMetrics_CustomMetricAppearsInOutput(t *testing.T) {
	ctx := context.Background()

	handler, shutdown, err := InitMetrics()
	if err != nil {
		t.Fatalf("InitMetrics failed: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx)
	}()

	// Create a custom counter using the global MeterProvider
	meter := otel.Meter("test-meter")
	counter, err := meter.Int64Counter("test_custom_counter")
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// Increment the counter
	counter.Add(ctx, 42)

	// Scrape metrics and verify our custom metric appears
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()

	// Verify the custom metric appears in Prometheus format
	if !strings.Contains(body, "test_custom_counter") {
		t.Errorf("expected custom metric 'test_custom_counter' in output, got:\n%s", body)
	}

	// Verify the value is present (Prometheus format: metric_name{labels} value)
	if !strings.Contains(body, "42") {
		t.Errorf("expected value '42' in output, got:\n%s", body)
	}
}
