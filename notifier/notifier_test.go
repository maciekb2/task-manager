package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/otel/trace"
)

// MockBusPublisher
type MockBusPublisher struct {
	mock.Mock
}

func (m *MockBusPublisher) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	// Variadic arguments handling in testify/mock can be tricky.
	// We just pass the slice 'opts' as one argument to Called.
	args := m.Called(ctx, subject, payload, headers, opts)
	var ack *nats.PubAck
	if args.Get(0) != nil {
		ack = args.Get(0).(*nats.PubAck)
	}
	return ack, args.Error(1)
}

// MockMessage
type MockMessage struct {
	mock.Mock
}

func (m *MockMessage) Data() []byte {
	args := m.Called()
	return args.Get(0).([]byte)
}

func (m *MockMessage) Ack() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockMessage) Nak() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockMessage) DeliveryAttempt() int {
	args := m.Called()
	return args.Int(0)
}

func TestNotifier_HandleMessage_Success(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	busMock := new(MockBusPublisher)
	tracer := trace.NewNoopTracerProvider().Tracer("test")

	notifier := NewNotifier(rdb, busMock, tracer)

	update := flow.StatusUpdate{
		TaskID:    "task-123",
		Status:    "completed",
		Source:    "worker",
		Timestamp: flow.Now(),
	}
	data, _ := json.Marshal(update)

	msgMock := new(MockMessage)
	msgMock.On("Data").Return(data)
	msgMock.On("Ack").Return(nil)

	err := notifier.HandleMessage(context.Background(), msgMock)

	assert.NoError(t, err)
	msgMock.AssertExpectations(t)

	// Verify Redis HSet
	statusKey := flow.StatusChannel("task-123")
	val := s.HGet(statusKey, "status")
	assert.Equal(t, "completed", val)
}

func TestNotifier_HandleMessage_BadPayload(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	busMock := new(MockBusPublisher)
	tracer := trace.NewNoopTracerProvider().Tracer("test")

	notifier := NewNotifier(rdb, busMock, tracer)

	msgMock := new(MockMessage)
	msgMock.On("Data").Return([]byte("invalid-json"))
	msgMock.On("DeliveryAttempt").Return(1)
	msgMock.On("Ack").Return(nil)

	// Expect DeadLetter
	// Note: opts is a slice, so we expect a slice (nil or empty)
	busMock.On("PublishJSON", mock.Anything, bus.SubjectEventDeadLetter, mock.AnythingOfType("flow.DeadLetter"), mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	err := notifier.HandleMessage(context.Background(), msgMock)

	assert.NoError(t, err)
	msgMock.AssertExpectations(t)
	busMock.AssertExpectations(t)
}

func TestNotifier_HandleMessage_RedisError(t *testing.T) {
	s := miniredis.RunT(t)
	s.SetError("redis failure") // Simulate redis failure
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})

	busMock := new(MockBusPublisher)
	tracer := trace.NewNoopTracerProvider().Tracer("test")

	notifier := NewNotifier(rdb, busMock, tracer)

	update := flow.StatusUpdate{TaskID: "task-123"}
	data, _ := json.Marshal(update)

	msgMock := new(MockMessage)
	msgMock.On("Data").Return(data)
	msgMock.On("DeliveryAttempt").Return(1)
	// Attempts=1 < MaxDeliver -> Nak
	msgMock.On("Nak").Return(nil)

	err := notifier.HandleMessage(context.Background(), msgMock)

	assert.NoError(t, err)
	msgMock.AssertExpectations(t)
}

func TestNotifier_HandleMessage_MaxRetriesReached(t *testing.T) {
	s := miniredis.RunT(t)
	s.SetError("redis failure")
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})

	busMock := new(MockBusPublisher)
	tracer := trace.NewNoopTracerProvider().Tracer("test")

	notifier := NewNotifier(rdb, busMock, tracer)

	update := flow.StatusUpdate{TaskID: "task-123"}
	data, _ := json.Marshal(update)

	msgMock := new(MockMessage)
	msgMock.On("Data").Return(data)
	msgMock.On("DeliveryAttempt").Return(100) // Exceeds MaxDeliver
	msgMock.On("Ack").Return(nil)

	busMock.On("PublishJSON", mock.Anything, bus.SubjectEventDeadLetter, mock.AnythingOfType("flow.DeadLetter"), mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	err := notifier.HandleMessage(context.Background(), msgMock)

	assert.NoError(t, err)
	msgMock.AssertExpectations(t)
	busMock.AssertExpectations(t)
}
