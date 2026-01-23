package main

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	pb "github.com/maciekb2/task-manager/proto"
	"github.com/nats-io/nats.go"
)

// MockEventBus mocks the EventBus interface
type MockEventBus struct {
	PublishedMessages []PublishedMessage
}

type PublishedMessage struct {
	Subject string
	Payload any
}

func (m *MockEventBus) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	m.PublishedMessages = append(m.PublishedMessages, PublishedMessage{
		Subject: subject,
		Payload: payload,
	})
	return &nats.PubAck{Sequence: 1}, nil
}

func TestSubmitTask(t *testing.T) {
	// Setup miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("could not start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// Setup MockEventBus
	mockBus := &MockEventBus{
		PublishedMessages: make([]PublishedMessage, 0),
	}

	// Create server
	srv := newServer(rdb, mockBus)

	// Create request
	req := &pb.TaskRequest{
		TaskDescription: "Test Task",
		Priority:        1,
		Url:             "http://example.com",
		Method:          "POST",
	}

	// Call SubmitTask
	ctx := context.Background()
	resp, err := srv.SubmitTask(ctx, req)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// Verify response
	if resp.TaskId == "" {
		t.Error("expected TaskId, got empty string")
	}

	// Verify Redis
	key := "task:" + resp.TaskId
	exists := mr.Exists(key)
	if !exists {
		t.Errorf("expected key %s to exist in redis", key)
	}

	val, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		t.Fatalf("could not get hash from redis: %v", err)
	}
	if val["description"] != req.TaskDescription {
		t.Errorf("expected description %s, got %s", req.TaskDescription, val["description"])
	}

	// Verify Bus
	if len(mockBus.PublishedMessages) != 3 {
		t.Errorf("expected 3 messages published, got %d", len(mockBus.PublishedMessages))
	}

	// Check subjects
	subjects := make(map[string]bool)
	for _, msg := range mockBus.PublishedMessages {
		subjects[msg.Subject] = true
	}

	if !subjects[bus.SubjectTaskIngest] {
		t.Errorf("expected message on %s", bus.SubjectTaskIngest)
	}
	if !subjects[bus.SubjectEventStatus] {
		t.Errorf("expected message on %s", bus.SubjectEventStatus)
	}
	if !subjects[bus.SubjectEventAudit] {
		t.Errorf("expected message on %s", bus.SubjectEventAudit)
	}
}

func TestSubmitTask_Idempotency(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("could not start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mockBus := &MockEventBus{}
	srv := newServer(rdb, mockBus)

	req := &pb.TaskRequest{
		TaskDescription: "Idempotent Task",
		Priority:        1,
		Url:             "http://example.com",
		IdempotencyKey:  "unique-key-123",
	}

	// First call
	resp1, err := srv.SubmitTask(context.Background(), req)
	if err != nil {
		t.Fatalf("First SubmitTask failed: %v", err)
	}

	// Second call
	resp2, err := srv.SubmitTask(context.Background(), req)
	if err != nil {
		t.Fatalf("Second SubmitTask failed: %v", err)
	}

	if resp1.TaskId != resp2.TaskId {
		t.Errorf("expected same TaskId for idempotent request, got %s and %s", resp1.TaskId, resp2.TaskId)
	}

	// Should only have published once set of messages (3 messages total from first call)
	// Because the second call returns early.
	// Wait, does SubmitTask return early?
	// Yes: "if !created { ... return &pb.TaskResponse{TaskId: existingID}, nil }"
	if len(mockBus.PublishedMessages) != 3 {
		t.Errorf("expected 3 messages total (idempotency hit should not publish), got %d", len(mockBus.PublishedMessages))
	}
}
