package logger

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// Setup configures the global logger to output JSON with the service name.
func Setup(serviceName string) {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	// Check for debug env var if needed, but for now default to Info.
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		opts.Level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler).With("service", serviceName)
	slog.SetDefault(logger)
}

// WithContext adds trace ID from context to the logger if available.
func WithContext(ctx context.Context) *slog.Logger {
	logger := slog.Default()
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		return logger.With("trace_id", span.SpanContext().TraceID().String())
	}
	return logger
}

// Error logs an error with an "error" attribute.
func Error(msg string, err error, args ...any) {
	if err != nil {
		args = append(args, "error", err.Error())
	}
	slog.Error(msg, args...)
}

// Fatal logs an error and exits.
func Fatal(msg string, err error, args ...any) {
	Error(msg, err, args...)
	os.Exit(1)
}
