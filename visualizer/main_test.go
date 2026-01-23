package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

type MockSubscriber struct {
	SubscribeCalled bool
}

func (m *MockSubscriber) Subscribe(subj string, cb nats.MsgHandler) (*nats.Subscription, error) {
	m.SubscribeCalled = true
	return nil, nil
}

func (m *MockSubscriber) Close() {}

func TestRun(t *testing.T) {
	mockSub := &MockSubscriber{}
	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error)
	go func() {
		errChan <- run(ctx, mockSub, ":0")
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("run timed out")
	}

	if !mockSub.SubscribeCalled {
		t.Error("Subscribe not called")
	}
}

func TestRun_StartupError(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	mockSub := &MockSubscriber{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = run(ctx, mockSub, ":"+port)
	if err == nil {
		t.Error("expected error due to port conflict, got nil")
	}
}
