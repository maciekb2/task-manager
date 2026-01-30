package logger

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestWithContext(t *testing.T) {
	// Setup trace
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)

	// Capture output
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	original := slog.Default()
	defer slog.SetDefault(original)
	slog.SetDefault(slog.New(handler))

	// Execute
	log := WithContext(ctx)
	log.Info("test message")

	// Verify
	output := buf.String()
	if !strings.Contains(output, "trace_id") {
		t.Error("expected trace_id in log output")
	}
	if !strings.Contains(output, "4bf92f3577b34da6a3ce929d0e0e4736") {
		t.Errorf("expected specific trace ID, got: %s", output)
	}
}

func TestError(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	original := slog.Default()
	defer slog.SetDefault(original)
	slog.SetDefault(slog.New(handler))

	err := errors.New("something went wrong")
	Error("operation failed", err, "key", "value")

	output := buf.String()
	if !strings.Contains(output, "operation failed") {
		t.Error("expected message in output")
	}
	if !strings.Contains(output, "something went wrong") {
		t.Error("expected error message in output")
	}
	if !strings.Contains(output, "\"key\":\"value\"") {
		t.Error("expected extra args in output")
	}
}

func TestSetup(t *testing.T) {
	// Just verify it doesn't panic
	Setup("test-service")
}
