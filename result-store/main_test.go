package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
)

type MockBusClient struct {
	PublishedJSON []struct {
		Subject string
		Payload any
	}
	PublishError error
}

func (m *MockBusClient) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	if m.PublishError != nil {
		return nil, m.PublishError
	}
	m.PublishedJSON = append(m.PublishedJSON, struct {
		Subject string
		Payload any
	}{Subject: subject, Payload: payload})
	return &nats.PubAck{Sequence: 1}, nil
}

func TestProcessMessage_Success(t *testing.T) {
	// Setup Redis
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})

	// Setup Mock Bus
	mockBus := &MockBusClient{}

	// Setup Tracer and Metrics
	tracer := otel.Tracer("test")
	metrics, _ := initMetrics()

	// Prepare Task
	taskID := "task-123"
	resultEnv := flow.ResultEnvelope{
		Task: flow.TaskEnvelope{
			TaskID:      taskID,
			CreatedAt:   time.Now().Format(time.RFC3339Nano),
			TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		},
		Result:      42,
		LatencyMs:   100,
		ProcessedAt: time.Now().Format(time.RFC3339Nano),
		WorkerID:    1,
	}
	data, _ := json.Marshal(resultEnv)
	msg := &nats.Msg{Data: data}

	// Run
	err := ProcessMessage(context.Background(), msg, rdb, mockBus, tracer, metrics)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// Verify Redis
	key := "task_result:" + taskID
	val, err := rdb.HGetAll(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("Redis read failed: %v", err)
	}
	if val["result"] != "42" {
		t.Errorf("Expected result 42, got %s", val["result"])
	}

	// Verify Audit Event
	foundAudit := false
	for _, p := range mockBus.PublishedJSON {
		if p.Subject == bus.SubjectEventAudit {
			audit, ok := p.Payload.(flow.AuditEvent)
			if ok && audit.TaskID == taskID {
				foundAudit = true
				break
			}
		}
	}
	if !foundAudit {
		t.Error("Audit event not published")
	}
}

func TestProcessMessage_InvalidJSON(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	mockBus := &MockBusClient{}
	tracer := otel.Tracer("test")
	metrics, _ := initMetrics()

	msg := &nats.Msg{Data: []byte("invalid json")}

	err := ProcessMessage(context.Background(), msg, rdb, mockBus, tracer, metrics)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// Verify Deadletter
	foundDLQ := false
	for _, p := range mockBus.PublishedJSON {
		if p.Subject == bus.SubjectEventDeadLetter {
			foundDLQ = true
			break
		}
	}
	if !foundDLQ {
		t.Error("Deadletter not published for invalid JSON")
	}
}

func TestProcessMessage_RedisError(t *testing.T) {
	// Mock DeliveryAttempt to force DLQ
	oldGetDeliveryAttempt := getDeliveryAttempt
	getDeliveryAttempt = func(msg *nats.Msg) int { return 100 }
	defer func() { getDeliveryAttempt = oldGetDeliveryAttempt }()

	s := miniredis.RunT(t)
	s.SetError("redis failure") // Simulate redis error
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})

	mockBus := &MockBusClient{}
	tracer := otel.Tracer("test")
	metrics, _ := initMetrics()

	resultEnv := flow.ResultEnvelope{
		Task: flow.TaskEnvelope{
			TaskID: "task-error",
		},
	}
	data, _ := json.Marshal(resultEnv)
	msg := &nats.Msg{Data: data}

	err := ProcessMessage(context.Background(), msg, rdb, mockBus, tracer, metrics)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// Verify Deadletter
	foundDLQ := false
	for _, p := range mockBus.PublishedJSON {
		if p.Subject == bus.SubjectEventDeadLetter {
			dlq, ok := p.Payload.(flow.DeadLetter)
			if ok && dlq.Task.TaskID == "task-error" && dlq.Reason == "persist failed" {
				foundDLQ = true
				break
			}
		}
	}
	if !foundDLQ {
		t.Error("Deadletter not published for Redis error")
	}
}
