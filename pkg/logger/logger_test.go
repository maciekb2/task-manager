package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestWithContext(t *testing.T) {
	// Setup capture
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	slog.SetDefault(slog.New(handler))

	// Case 1: No Trace
	ctx := context.Background()
	logger := WithContext(ctx)
	logger.Info("test message")

	var logMap map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logMap)
	assert.NoError(t, err)
	assert.Equal(t, "test message", logMap["msg"])
	_, hasTrace := logMap["trace_id"]
	assert.False(t, hasTrace, "should not have trace_id")

	// Reset buffer
	buf.Reset()

	// Case 2: With Trace
	// Create a valid trace ID
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctxWithTrace := trace.ContextWithSpanContext(context.Background(), spanContext)

	logger = WithContext(ctxWithTrace)
	logger.Info("traced message")

	err = json.Unmarshal(buf.Bytes(), &logMap)
	assert.NoError(t, err)
	assert.Equal(t, "traced message", logMap["msg"])
	val, ok := logMap["trace_id"]
	assert.True(t, ok, "should have trace_id")
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", val)
}

func TestError(t *testing.T) {
	// Setup capture
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	slog.SetDefault(slog.New(handler))

	testErr := errors.New("something went wrong")
	Error("operation failed", testErr, "key", "value")

	var logMap map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logMap)
	assert.NoError(t, err)

	assert.Equal(t, "operation failed", logMap["msg"])
	assert.Equal(t, "ERROR", logMap["level"])
	assert.Equal(t, "something went wrong", logMap["error"])
	assert.Equal(t, "value", logMap["key"])
}

func TestSetup(t *testing.T) {
	// Setup modifies global state (slog.Default), so we should be careful.
	// We can test that it doesn't panic.
	// Since tests run in parallel/random order, modifying global logger might affect other tests
	// if we are not careful. However, we already modified it in previous tests.
	// We'll just run it and verifying it sets a default logger.

	assert.NotPanics(t, func() {
		Setup("test-service")
	})

	// Verify it writes JSON
	// We can't easily capture os.Stdout here without piping, which is complex.
	// But we verified Setup runs.
}
