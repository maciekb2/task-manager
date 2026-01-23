package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockBusPublisher
type MockBusPublisher struct {
	mock.Mock
}

func (m *MockBusPublisher) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	args := m.Called(ctx, subject, payload, headers, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*nats.PubAck), args.Error(1)
}

// MockMessage
type MockMessage struct {
	mock.Mock
}

func (m *MockMessage) GetData() []byte {
	args := m.Called()
	return args.Get(0).([]byte)
}

func (m *MockMessage) GetHeader() nats.Header {
	args := m.Called()
	return args.Get(0).(nats.Header)
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

func TestProcessTask_HappyPath(t *testing.T) {
	publisher := new(MockBusPublisher)
	svc := NewService(publisher)
	msg := new(MockMessage)

	task := flow.TaskEnvelope{
		TaskID:      "123",
		Priority:    1,
		URL:         "http://example.com",
		Method:      "GET",
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}
	data, _ := json.Marshal(task)

	msg.On("GetData").Return(data)
	msg.On("Ack").Return(nil)

	// Worker Queue
	publisher.On("PublishJSON", mock.Anything, bus.SubjectTaskWorkerMedium, mock.MatchedBy(func(t flow.TaskEnvelope) bool {
		return t.TaskID == "123"
	}), mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	// Status Update
	publisher.On("PublishJSON", mock.Anything, bus.SubjectEventStatus, mock.MatchedBy(func(s flow.StatusUpdate) bool {
		return s.TaskID == "123" && s.Status == "SCHEDULED"
	}), mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	// Audit
	publisher.On("PublishJSON", mock.Anything, bus.SubjectEventAudit, mock.MatchedBy(func(a flow.AuditEvent) bool {
		return a.TaskID == "123" && a.Event == "task.scheduled"
	}), mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	err := svc.ProcessTask(context.Background(), msg)
	require.NoError(t, err)

	publisher.AssertExpectations(t)
	msg.AssertExpectations(t)
}

func TestProcessTask_BadPayload(t *testing.T) {
	publisher := new(MockBusPublisher)
	svc := NewService(publisher)
	msg := new(MockMessage)

	msg.On("GetData").Return([]byte("invalid json"))
	msg.On("DeliveryAttempt").Return(1)
	msg.On("Ack").Return(nil)

	// Expect DeadLetter
	publisher.On("PublishJSON", mock.Anything, bus.SubjectEventDeadLetter, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	err := svc.ProcessTask(context.Background(), msg)
	require.NoError(t, err)

	publisher.AssertExpectations(t)
	msg.AssertExpectations(t)
}

func TestProcessTask_PublishError_Retry(t *testing.T) {
	publisher := new(MockBusPublisher)
	svc := NewService(publisher)
	msg := new(MockMessage)

	task := flow.TaskEnvelope{TaskID: "123", Priority: 0}
	data, _ := json.Marshal(task)

	msg.On("GetData").Return(data)
	msg.On("DeliveryAttempt").Return(1)
	msg.On("Nak").Return(nil)

	publisher.On("PublishJSON", mock.Anything, bus.SubjectTaskWorkerLow, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("nats error")).Once()

	err := svc.ProcessTask(context.Background(), msg)
	require.NoError(t, err)

	publisher.AssertExpectations(t)
	msg.AssertExpectations(t)
}

func TestProcessTask_PublishError_MaxRetriesExceeded(t *testing.T) {
	publisher := new(MockBusPublisher)
	svc := NewService(publisher)
	msg := new(MockMessage)

	task := flow.TaskEnvelope{TaskID: "123", Priority: 0}
	data, _ := json.Marshal(task)

	msg.On("GetData").Return(data)
	// MaxDeliver is likely 6. So if attempt is 6, it should deadletter.
	msg.On("DeliveryAttempt").Return(6)
	msg.On("Ack").Return(nil)

	publisher.On("PublishJSON", mock.Anything, bus.SubjectTaskWorkerLow, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("nats error")).Once()

	// Expect DeadLetter
	publisher.On("PublishJSON", mock.Anything, bus.SubjectEventDeadLetter, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	err := svc.ProcessTask(context.Background(), msg)
	require.NoError(t, err)

	publisher.AssertExpectations(t)
	msg.AssertExpectations(t)
}

func TestProcessTask_PriorityRouting(t *testing.T) {
	tests := []struct {
		name     string
		priority int32
		expected string
	}{
		{"High Priority", 2, bus.SubjectTaskWorkerHigh},
		{"Medium Priority", 1, bus.SubjectTaskWorkerMedium},
		{"Low Priority", 0, bus.SubjectTaskWorkerLow},
		{"Default (Unknown) Priority", 99, bus.SubjectTaskWorkerLow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := new(MockBusPublisher)
			svc := NewService(publisher)
			msg := new(MockMessage)

			task := flow.TaskEnvelope{
				TaskID:   "123",
				Priority: tt.priority,
				URL:      "http://example.com",
			}
			data, _ := json.Marshal(task)

			msg.On("GetData").Return(data)
			msg.On("Ack").Return(nil)

			// Expect publish to the CORRECT queue
			publisher.On("PublishJSON", mock.Anything, tt.expected, mock.MatchedBy(func(t flow.TaskEnvelope) bool {
				return t.TaskID == "123"
			}), mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

			// Expect status/audit (we don't strictly care about arguments here for this test, but they are called)
			publisher.On("PublishJSON", mock.Anything, bus.SubjectEventStatus, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)
			publisher.On("PublishJSON", mock.Anything, bus.SubjectEventAudit, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

			err := svc.ProcessTask(context.Background(), msg)
			require.NoError(t, err)

			publisher.AssertExpectations(t)
		})
	}
}
