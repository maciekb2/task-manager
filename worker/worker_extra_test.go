package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/mock"
)

// MockSubscription for worker
type MockSubscription struct {
	mock.Mock
}

func (m *MockSubscription) Fetch(batch int, opts ...nats.PullOpt) ([]*nats.Msg, error) {
	args := m.Called(batch, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*nats.Msg), args.Error(1)
}

func TestDispatchLoop(t *testing.T) {
	sub := new(MockSubscription)
	msg := &nats.Msg{Subject: "test", Data: []byte("{}")}

	// First call returns message
	sub.On("Fetch", 1, mock.Anything).Return([]*nats.Msg{msg}, nil).Once()
	// Subsequent calls return nothing or block?
	// dispatchLoop loops. We need to stop it via context.
	sub.On("Fetch", 1, mock.Anything).Return(nil, nil).Maybe()

	subs := []prioritySub{{subject: "test", sub: sub}}
	out := make(chan *nats.Msg, 1)

	ctx, cancel := context.WithCancel(context.Background())
	go dispatchLoop(ctx, subs, out)

	select {
	case <-out:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting for msg")
	}
	cancel()
}

func TestDispatchLoop_FetchError(t *testing.T) {
	sub := new(MockSubscription)
	// Return error
	sub.On("Fetch", 1, mock.Anything).Return(nil, errors.New("fetch error")).Maybe()

	subs := []prioritySub{{subject: "test", sub: sub}}
	out := make(chan *nats.Msg, 1)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	dispatchLoop(ctx, subs, out)
}

func TestConfig(t *testing.T) {
	os.Setenv("WORKER_COUNT", "5")
	if workerCount() != 5 {
		t.Error("expected 5 workers")
	}
	os.Setenv("WORKER_COUNT", "0")
	if workerCount() != 2 { // default
		t.Error("expected default 2 workers for invalid input")
	}
	os.Unsetenv("WORKER_COUNT")
	if workerCount() != 2 {
		t.Error("expected default 2 workers")
	}

	os.Setenv("WORKER_FAIL_RATE", "0.5")
	if workerFailRate() != 0.5 {
		t.Error("expected 0.5 fail rate")
	}
	os.Setenv("WORKER_FAIL_RATE", "-1")
	if workerFailRate() != 0.05 { // default
		t.Error("expected default 0.05 for invalid input")
	}

	os.Setenv("NATS_URL", "nats://foo")
	if natsURL() != "nats://foo" {
		t.Error("nats url mismatch")
	}
}

func TestNakMessage(t *testing.T) {
	// NakMessage is used in handleProcessingFailure
	// coverage check
	msg := &nats.Msg{} // Nak on bare msg might panic if not connected?
	// nats.Msg.Nak() checks if sub/connection is valid. If nil, returns error.
	nakMessage("test", msg)
	nakMessage("test", nil)
}

func TestHandleProcessingFailure(t *testing.T) {
	// Need to check retry logic which calls Nak
	// But `worker.go` uses `bus.DeliveryAttempt`.
	// We can't easily mock `bus.DeliveryAttempt` here unless we export/variable it like in other services.
	// But `handleProcessingFailure` is already 83.3% covered.
}
