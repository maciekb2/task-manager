package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestSetup(t *testing.T) {
	// Just ensure it doesn't panic
	assert.NotPanics(t, func() {
		Setup("test-service")
	})
}

func TestWithContext(t *testing.T) {
	// Case 1: No span
	ctx := context.Background()
	log := WithContext(ctx)
	assert.NotNil(t, log)

	// Case 2: With valid span
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx = trace.ContextWithRemoteSpanContext(context.Background(), sc)

	log = WithContext(ctx)
	assert.NotNil(t, log)
}
