// Package observability provides OpenTelemetry instrumentation for tracing and metrics.
package observability

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

// InitMetrics initializes the OpenTelemetry metrics provider with a Prometheus exporter.
// It returns the HTTP handler for the /metrics endpoint and a shutdown function.
// The shutdown function should be called on application exit for graceful cleanup.
func InitMetrics() (http.Handler, func(context.Context) error, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	provider := metric.NewMeterProvider(
		metric.WithReader(exporter),
	)

	otel.SetMeterProvider(provider)

	return promhttp.Handler(), provider.Shutdown, nil
}
