// Package logger provides structured logging setup using slog.
package logger

import (
	"context"
	"log/slog"
	"os"
)

// requestIDKey is the context key for request/correlation IDs.
type requestIDKey struct{}

// New creates a new structured JSON logger.
func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// WithRequestID returns a new context with the given request ID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if v := ctx.Value(requestIDKey{}); v != nil {
		return v.(string)
	}
	return ""
}

// FromContext returns a logger with context fields (request ID, etc.) attached.
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	if reqID := RequestIDFromContext(ctx); reqID != "" {
		return base.With("request_id", reqID)
	}
	return base
}
