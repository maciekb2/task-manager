package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/mock"
)

func TestHandleProcessingFailure(t *testing.T) {
	// Mock retry logic
	msg := &nats.Msg{}

	// Let's use a new mock
	mBus := new(MockBusClient)
	mBus.On("PublishJSON", mock.Anything, bus.SubjectEventDeadLetter, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	// Force MaxDeliver
	oldGetDeliveryAttempt := getDeliveryAttempt
	getDeliveryAttempt = func(msg *nats.Msg) int { return 100 }
	defer func() { getDeliveryAttempt = oldGetDeliveryAttempt }()

	handleProcessingFailure(context.Background(), mBus, msg, flow.TaskEnvelope{}, "reason", false)

	mBus.AssertExpectations(t)
}

func TestHandleProcessingFailure_Retry(t *testing.T) {
	oldGetDeliveryAttempt := getDeliveryAttempt
	getDeliveryAttempt = func(msg *nats.Msg) int { return 1 }
	defer func() { getDeliveryAttempt = oldGetDeliveryAttempt }()

	mBus := new(MockBusClient)
	// Expect Nak?
	// nakMessage just calls msg.Nak. Since msg is empty, it returns error usually if not bound.
	// But we just want to cover the path.

	handleProcessingFailure(context.Background(), mBus, &nats.Msg{}, flow.TaskEnvelope{}, "reason", true)
}

func TestAckNak(t *testing.T) {
	ackMessage("test", nil)
	nakMessage("test", nil)

	msg := &nats.Msg{}
	ackMessage("test", msg)
	nakMessage("test", msg)
}

func TestTelemetry(t *testing.T) {
	// reuse logic from other services
	origListen := listenAndServe
	listenAndServe = func(addr string, handler http.Handler) error { return nil }
	defer func() { listenAndServe = origListen }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		t.Errorf("initTelemetry failed: %v", err)
	}
	if shutdown != nil {
		shutdown(ctx)
	}
}
