package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/mock"
)

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

func TestRun_Success(t *testing.T) {
	s := miniredis.RunT(t)
	// We need to set REDIS_ADDR
	// But main uses redisAddr() which reads env.
	// Mocking redisAddr or env var.
	// Since redisAddr is in main.go, we can check it there.
	// Or we can just set env var.
	t.Setenv("REDIS_ADDR", s.Addr())

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

func TestRun_RedisError(t *testing.T) {
	// redis.NewClient doesn't error on create.
	// But we use it in service.
	// run() creates redis client.
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
