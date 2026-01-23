package main

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/mock"
)

// MockBusClient for server (similar to ingest/scheduler but re-declared)
type MockBusClient struct {
	mock.Mock
}

func (m *MockBusClient) Close() {
	m.Called()
}

func (m *MockBusClient) EnsureStream(cfg *nats.StreamConfig) (*nats.StreamInfo, error) {
	args := m.Called(cfg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*nats.StreamInfo), args.Error(1)
}

func (m *MockBusClient) EnsureConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	args := m.Called(stream, cfg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*nats.ConsumerInfo), args.Error(1)
}

func (m *MockBusClient) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	args := m.Called(ctx, subject, payload, headers, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*nats.PubAck), args.Error(1)
}

func TestRun_Success(t *testing.T) {
	mockBus := new(MockBusClient)
	mockBus.On("EnsureStream", mock.Anything).Return(&nats.StreamInfo{}, nil)
	mockBus.On("Close").Return()

	origConnect := busConnect
	busConnect = func(cfg bus.Config) (BusClient, error) {
		return mockBus, nil
	}
	defer func() { busConnect = origConnect }()

	origInit := initTelemetryFunc
	initTelemetryFunc = func(ctx context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	defer func() { initTelemetryFunc = origInit }()

	origListen := netListen
	netListen = func(network, address string) (net.Listener, error) {
		return net.Listen(network, ":0") // Random port
	}
	defer func() { netListen = origListen }()

	// We need a real Redis or mock?
	// The run function creates redis client but doesn't ping it immediately.
	// `NewClient` just returns a client handle.
	// But `run` doesn't use redis until serving requests.
	// So we should be fine.

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately to trigger graceful stop
	cancel()

	err := run(ctx)
	if err != nil {
		t.Errorf("run failed: %v", err)
	}
}

func TestRun_TelemetryFail(t *testing.T) {
	origInit := initTelemetryFunc
	initTelemetryFunc = func(ctx context.Context) (func(context.Context) error, error) {
		return nil, errors.New("telemetry fail")
	}
	defer func() { initTelemetryFunc = origInit }()

	err := run(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestRun_ConnectFail(t *testing.T) {
	origInit := initTelemetryFunc
	initTelemetryFunc = func(ctx context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	defer func() { initTelemetryFunc = origInit }()

	origConnect := busConnect
	busConnect = func(cfg bus.Config) (BusClient, error) {
		return nil, errors.New("connect fail")
	}
	defer func() { busConnect = origConnect }()

	err := run(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestRun_StreamFail(t *testing.T) {
	mockBus := new(MockBusClient)
	mockBus.On("EnsureStream", mock.Anything).Return(nil, errors.New("stream fail")).Once()
	mockBus.On("Close").Return()

	origConnect := busConnect
	busConnect = func(cfg bus.Config) (BusClient, error) {
		return mockBus, nil
	}
	defer func() { busConnect = origConnect }()

	origInit := initTelemetryFunc
	initTelemetryFunc = func(ctx context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	defer func() { initTelemetryFunc = origInit }()

	err := run(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestRun_ListenFail(t *testing.T) {
	mockBus := new(MockBusClient)
	mockBus.On("EnsureStream", mock.Anything).Return(&nats.StreamInfo{}, nil)
	mockBus.On("Close").Return()

	origConnect := busConnect
	busConnect = func(cfg bus.Config) (BusClient, error) {
		return mockBus, nil
	}
	defer func() { busConnect = origConnect }()

	origInit := initTelemetryFunc
	initTelemetryFunc = func(ctx context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	defer func() { initTelemetryFunc = origInit }()

	origListen := netListen
	netListen = func(network, address string) (net.Listener, error) {
		return nil, errors.New("listen fail")
	}
	defer func() { netListen = origListen }()

	err := run(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}
