package main

import (
	"context"
	"errors"
	"testing"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/mock"
)

// Reuse MockBusClient from ingest if possible, but package separation prevents it easily without a shared mock pkg.
// Re-implementing MockBusClient here.

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

func (m *MockBusClient) PullSubscribe(subject, durable string, opts ...nats.SubOpt) (*nats.Subscription, error) {
	args := m.Called(subject, durable, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*nats.Subscription), args.Error(1)
}

func (m *MockBusClient) Consume(ctx context.Context, sub *nats.Subscription, opts bus.ConsumeOptions, handler bus.Handler) error {
	args := m.Called(ctx, sub, opts, handler)
	return args.Error(0)
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
	mockBus.On("EnsureConsumer", mock.Anything, mock.Anything).Return(&nats.ConsumerInfo{}, nil)
	mockBus.On("PullSubscribe", mock.Anything, mock.Anything, mock.Anything).Return(&nats.Subscription{}, nil)
	mockBus.On("Consume", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
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

	ctx, cancel := context.WithCancel(context.Background())
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

func TestRun_ConsumerFail(t *testing.T) {
	mockBus := new(MockBusClient)
	mockBus.On("EnsureStream", mock.Anything).Return(&nats.StreamInfo{}, nil)
	mockBus.On("EnsureConsumer", mock.Anything, mock.Anything).Return(nil, errors.New("consumer fail"))
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

func TestRun_SubscribeFail(t *testing.T) {
	mockBus := new(MockBusClient)
	mockBus.On("EnsureStream", mock.Anything).Return(&nats.StreamInfo{}, nil)
	mockBus.On("EnsureConsumer", mock.Anything, mock.Anything).Return(&nats.ConsumerInfo{}, nil)
	mockBus.On("PullSubscribe", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("sub fail"))
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

func TestRun_ConsumeFail(t *testing.T) {
	mockBus := new(MockBusClient)
	mockBus.On("EnsureStream", mock.Anything).Return(&nats.StreamInfo{}, nil)
	mockBus.On("EnsureConsumer", mock.Anything, mock.Anything).Return(&nats.ConsumerInfo{}, nil)
	mockBus.On("PullSubscribe", mock.Anything, mock.Anything, mock.Anything).Return(&nats.Subscription{}, nil)
	mockBus.On("Consume", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("consume fail"))
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
