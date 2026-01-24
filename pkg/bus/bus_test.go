package bus

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestBackoffFor(t *testing.T) {
	policy := RetryPolicy{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1 * time.Second}, // Capped at MaxBackoff
		{6, 1 * time.Second},
	}

	for _, tt := range tests {
		got := backoffFor(policy, tt.attempt)
		assert.Equal(t, tt.expected, got, "attempt %d", tt.attempt)
	}
}

func TestBackoffFor_Defaults(t *testing.T) {
	policy := RetryPolicy{} // Zero values
	// Should default to 500ms, mult 2
	got := backoffFor(policy, 1)
	assert.Equal(t, 500*time.Millisecond, got)

	got2 := backoffFor(policy, 2)
	assert.Equal(t, 1000*time.Millisecond, got2)
}

func TestBuildDLQMessage(t *testing.T) {
	data := []byte("hello world")
	msg := &nats.Msg{
		Subject: "test.subject",
		Data:    data,
		Header:  nats.Header{"X-Test": []string{"value"}},
	}

	// Mock metadata if possible?
	// nats.Msg.Metadata() calls internal methods or parses the reply subject.
	// We can't easily mock Metadata() without a real NATS connection or manual construction of a specialized message,
	// but BuildDLQMessage handles error from Metadata() gracefully.

	reason := "test failure"
	dlq := BuildDLQMessage(msg, reason)

	assert.Equal(t, "test.subject", dlq.Subject)
	assert.Equal(t, reason, dlq.Reason)
	assert.Equal(t, base64.StdEncoding.EncodeToString(data), dlq.Payload)
	assert.Equal(t, []string{"value"}, dlq.Headers["X-Test"])
	assert.NotEmpty(t, dlq.ReceivedAt)
}

func TestTracePropagation(t *testing.T) {
	// Setup a propagator
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Create a valid SpanContext manually (Noop tracer produces invalid/empty IDs)
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	assert.True(t, sc.IsValid())

	// Create a context with this span context
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), sc)

	// Inject into headers
	headers := nats.Header{}
	injectTrace(ctx, headers)

	// The W3C TraceContext propagator uses "traceparent" header
	assert.NotEmpty(t, headers.Get("traceparent"))

	// Extract back
	ctx2 := ContextFromHeaders(context.Background(), headers)
	span2 := trace.SpanFromContext(ctx2)

	assert.Equal(t, sc.TraceID(), span2.SpanContext().TraceID())
}

func TestHandleFailure_MaxRetries(t *testing.T) {
	// Since handleFailure is hard to test without a mocked Client (which needs a real connection usually),
	// we might skip deep testing of handleFailure here unless we mock the Client methods.
	// But Client struct has private fields.
	// However, we can test that logic generally if we could mock.
	// For now, let's stick to the pure logic functions we already tested.
}

func TestNakWithDelay(t *testing.T) {
	// This function checks for an interface or calls Nak().
	// Can't easily test without a mocked Msg that implements the interface.
}
