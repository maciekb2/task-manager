package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
)

func TestProcessMessage_StatusPublishFailure(t *testing.T) {
	task := flow.TaskEnvelope{TaskID: "123", TaskDescription: "test"}
	data, _ := json.Marshal(task)
	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{NumDelivered: 1},
	}

	pub := &MockPublisher{
		ErrMap: map[string]error{
			bus.SubjectEventStatus: errors.New("status failed"),
		},
	}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	// Should log error but continue
	if !msg.AckCalled {
		t.Error("expected Ack despite status publish failure")
	}
}

func TestProcessMessage_AuditPublishFailure(t *testing.T) {
	task := flow.TaskEnvelope{TaskID: "123", TaskDescription: "test"}
	data, _ := json.Marshal(task)
	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{NumDelivered: 1},
	}

	pub := &MockPublisher{
		ErrMap: map[string]error{
			bus.SubjectEventAudit: errors.New("audit failed"),
		},
	}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	// Should log error but continue
	if !msg.AckCalled {
		t.Error("expected Ack despite audit publish failure")
	}
}

func TestProcessMessage_DeadLetterPublishFailure(t *testing.T) {
	// Bad payload triggers DeadLetter
	msg := &MockMessage{
		DataVal: []byte("invalid"),
		MetaVal: &nats.MsgMetadata{NumDelivered: 1},
	}

	pub := &MockPublisher{
		ErrMap: map[string]error{
			bus.SubjectEventDeadLetter: errors.New("dlq failed"),
		},
	}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	// Should consume (Ack) anyway
	if !msg.AckCalled {
		t.Error("expected Ack despite dlq publish failure")
	}
}

func TestProcessMessage_AckNakFailure(t *testing.T) {
	task := flow.TaskEnvelope{TaskID: "123", TaskDescription: "test"}
	data, _ := json.Marshal(task)
	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{NumDelivered: 1},
		AckErr:  errors.New("ack failed"),
	}

	pub := &MockPublisher{}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	if !msg.AckCalled {
		t.Error("expected Ack called")
	}
}

func TestProcessMessage_NakFailure(t *testing.T) {
	// Retryable error
	task := flow.TaskEnvelope{TaskID: "123", TaskDescription: "test"}
	data, _ := json.Marshal(task)
	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{NumDelivered: 1},
		NakErr:  errors.New("nak failed"),
	}

	pub := &MockPublisher{Err: errors.New("publish failed")}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	if !msg.NakCalled {
		t.Error("expected Nak called")
	}
}

func TestProcessMessage_MetaError(t *testing.T) {
	task := flow.TaskEnvelope{TaskID: "123", TaskDescription: "test"}
	data, _ := json.Marshal(task)
	msg := &MockMessage{
		DataVal: data,
		MetaErr: errors.New("meta error"),
	}

	pub := &MockPublisher{
		Err: errors.New("publish error"),
	}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	// getDeliveryAttempt returns 1 on error
	// publish fail -> retry
	if !msg.NakCalled {
		t.Error("expected Nak")
	}
}

func TestProcessMessage_TraceParent(t *testing.T) {
	task := flow.TaskEnvelope{TaskID: "123", TaskDescription: "test"}
	data, _ := json.Marshal(task)
	msg := &MockMessage{DataVal: data} // No trace parent in task

	// Case 1: Header has traceparent
	msg.HeadersVal = nats.Header{"traceparent": []string{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}}

	pub := &MockPublisher{}
	enricher := &MockEnricher{}
	svc := NewIngestService(pub, enricher)

	_ = svc.ProcessMessage(context.Background(), msg)
}

func TestProcessMessage_TaskTraceParent(t *testing.T) {
	// Task has TraceParent, Context doesn't
	task := flow.TaskEnvelope{
		TaskID: "123",
		TaskDescription: "test",
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}
	data, _ := json.Marshal(task)
	msg := &MockMessage{DataVal: data}

	pub := &MockPublisher{}
	enricher := &MockEnricher{}
	svc := NewIngestService(pub, enricher)

	_ = svc.ProcessMessage(context.Background(), msg)
}

func TestProcessMessage_AnnotateSpan_Coverage(t *testing.T) {
	// Full metadata coverage
	task := flow.TaskEnvelope{TaskID: "123", TaskDescription: "test"}
	data, _ := json.Marshal(task)
	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{
			Stream:       "stream",
			Consumer:     "consumer",
			NumDelivered: 5,
		},
		SubjectVal: "subj",
	}

	pub := &MockPublisher{}
	enricher := &MockEnricher{}
	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)
}

func TestErrInvalidTask(t *testing.T) {
	err := errInvalidTask("test error")
	if err.Error() != "test error" {
		t.Error("error string mismatch")
	}
}
