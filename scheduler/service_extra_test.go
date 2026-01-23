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
)

func TestProcessTask_StatusPublishFailure(t *testing.T) {
	publisher := new(MockBusPublisher)
	svc := NewService(publisher)
	msg := new(MockMessage)

	task := flow.TaskEnvelope{TaskID: "123", Priority: 0}
	data, _ := json.Marshal(task)

	msg.On("GetData").Return(data)
	msg.On("Ack").Return(nil)

	// Worker publish success
	publisher.On("PublishJSON", mock.Anything, bus.SubjectTaskWorkerLow, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	// Status publish fail
	publisher.On("PublishJSON", mock.Anything, bus.SubjectEventStatus, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("status fail"))

	// Audit success
	publisher.On("PublishJSON", mock.Anything, bus.SubjectEventAudit, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	svc.ProcessTask(context.Background(), msg)

	publisher.AssertExpectations(t)
	msg.AssertExpectations(t)
}

func TestProcessTask_AuditPublishFailure(t *testing.T) {
	publisher := new(MockBusPublisher)
	svc := NewService(publisher)
	msg := new(MockMessage)

	task := flow.TaskEnvelope{TaskID: "123", Priority: 0}
	data, _ := json.Marshal(task)

	msg.On("GetData").Return(data)
	msg.On("Ack").Return(nil)

	publisher.On("PublishJSON", mock.Anything, bus.SubjectTaskWorkerLow, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)
	publisher.On("PublishJSON", mock.Anything, bus.SubjectEventStatus, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)
	// Audit fail
	publisher.On("PublishJSON", mock.Anything, bus.SubjectEventAudit, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("audit fail"))

	svc.ProcessTask(context.Background(), msg)

	publisher.AssertExpectations(t)
	msg.AssertExpectations(t)
}

func TestProcessTask_DeadLetterPublishFailure(t *testing.T) {
	publisher := new(MockBusPublisher)
	svc := NewService(publisher)
	msg := new(MockMessage)

	// Bad payload
	msg.On("GetData").Return([]byte("invalid"))
	msg.On("DeliveryAttempt").Return(1)
	msg.On("Ack").Return(nil)

	// DLQ fail
	publisher.On("PublishJSON", mock.Anything, bus.SubjectEventDeadLetter, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("dlq fail"))

	svc.ProcessTask(context.Background(), msg)

	publisher.AssertExpectations(t)
	msg.AssertExpectations(t)
}

func TestProcessTask_AckNakFailure(t *testing.T) {
	publisher := new(MockBusPublisher)
	svc := NewService(publisher)
	msg := new(MockMessage)

	task := flow.TaskEnvelope{TaskID: "123", Priority: 0}
	data, _ := json.Marshal(task)

	msg.On("GetData").Return(data)
	msg.On("Ack").Return(errors.New("ack fail"))

	publisher.On("PublishJSON", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&nats.PubAck{}, nil)

	svc.ProcessTask(context.Background(), msg)
}

func TestProcessTask_NakFailure(t *testing.T) {
	publisher := new(MockBusPublisher)
	svc := NewService(publisher)
	msg := new(MockMessage)

	task := flow.TaskEnvelope{TaskID: "123", Priority: 0}
	data, _ := json.Marshal(task)

	msg.On("GetData").Return(data)
	msg.On("DeliveryAttempt").Return(1)
	msg.On("Nak").Return(errors.New("nak fail"))

	publisher.On("PublishJSON", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("pub fail"))

	svc.ProcessTask(context.Background(), msg)
}
