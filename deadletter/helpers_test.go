package main

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
)

func TestHandleSinkFailure(t *testing.T) {
	msg := &nats.Msg{}
	handleSinkFailure(msg, "test")
	// Should ack since default attempts is 1 which is < MaxDeliver?
	// deliveryCount defaults to 1. MaxDeliver is 5.
	// So it should Nak.
}

func TestAckNak(t *testing.T) {
	ackMessage("test", nil)
	nakMessage("test", nil)

	msg := &nats.Msg{}
	ackMessage("test", msg)
	nakMessage("test", msg)
}

func TestTelemetry(t *testing.T) {
	// mock listenAndServe? It's hardcoded in deadletter/telemetry.go as http.ListenAndServe
	// I didn't refactor deadletter/telemetry.go yet to expose it.
	// But initTelemetry returns a shutdown function.
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	_ = contextFromTraceParent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	_ = traceparentFromContext(ctx)
}
