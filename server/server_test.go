package main

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	pb "github.com/maciekb2/task-manager/proto"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
)

// MockBus implements BusClient for testing
type MockBus struct {
	Published []PublishedMessage
}

type PublishedMessage struct {
	Subject string
	Payload interface{}
}

func (m *MockBus) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	m.Published = append(m.Published, PublishedMessage{Subject: subject, Payload: payload})
	return &nats.PubAck{}, nil
}

func TestSubmitTask(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mockBus := &MockBus{}
	srv := newServer(rdb, mockBus)

	ctx := context.Background()

	// Test Case 1: Valid Task
	req := &pb.TaskRequest{
		TaskDescription: "Test Task",
		Priority:        pb.TaskPriority_HIGH,
		Url:             "http://example.com",
		Method:          "POST",
	}

	resp, err := srv.SubmitTask(ctx, req)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// Verify response
	if resp.TaskId == "" {
		t.Error("expected TaskId, got empty string")
	}

	// Verify Redis
	taskKey := "task:" + resp.TaskId
	if !mr.Exists(taskKey) {
		t.Errorf("task %s not found in redis", taskKey)
	}

	val := mr.HGet(taskKey, "url")
	if val != "http://example.com" {
		t.Errorf("expected url 'http://example.com', got '%s'", val)
	}

	// Verify Bus
	if len(mockBus.Published) < 3 { // TaskIngest, EventStatus, EventAudit
		t.Errorf("expected at least 3 messages published, got %d", len(mockBus.Published))
	}

	// Verify specific messages
	foundIngest := false
	for _, msg := range mockBus.Published {
		if msg.Subject == bus.SubjectTaskIngest {
			foundIngest = true
			taskEnv, ok := msg.Payload.(flow.TaskEnvelope)
			if !ok {
				t.Error("payload is not TaskEnvelope")
			}
			if taskEnv.URL != "http://example.com" {
				t.Errorf("bus payload url mismatch: %s", taskEnv.URL)
			}
		}
	}
	if !foundIngest {
		t.Error("TaskIngest message not published")
	}
}

func TestSubmitTask_Idempotency(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mockBus := &MockBus{}
	srv := newServer(rdb, mockBus)
	ctx := context.Background()

	key := "unique-key-123"
	req := &pb.TaskRequest{
		TaskDescription: "Idempotent Task",
		Url:             "http://example.com",
		IdempotencyKey:  key,
	}

	// First call
	resp1, err := srv.SubmitTask(ctx, req)
	if err != nil {
		t.Fatalf("First SubmitTask failed: %v", err)
	}

	// Second call
	resp2, err := srv.SubmitTask(ctx, req)
	if err != nil {
		t.Fatalf("Second SubmitTask failed: %v", err)
	}

	if resp1.TaskId != resp2.TaskId {
		t.Errorf("expected same TaskId for idempotent request, got %s and %s", resp1.TaskId, resp2.TaskId)
	}

	// Verify only one task published (plus status/audit for the first one)
	// Actually, the second call returns early, so no bus publish.
	// First call: 1 Ingest + 1 Status + 1 Audit = 3 messages.
	if len(mockBus.Published) != 3 {
		t.Errorf("expected 3 messages for first call (second call shouldn't publish), got %d", len(mockBus.Published))
	}
}

func TestSubmitTask_MissingURL(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mockBus := &MockBus{}
	srv := newServer(rdb, mockBus)
	ctx := context.Background()

	req := &pb.TaskRequest{
		TaskDescription: "Invalid Task",
	}

	_, err = srv.SubmitTask(ctx, req)
	if err == nil {
		t.Error("expected error for missing URL, got nil")
	}
}

// MockStream implements pb.TaskManager_StreamTaskStatusServer
type MockStream struct {
	grpc.ServerStream
	Ctx  context.Context
	Sent []*pb.StatusResponse
}

func (m *MockStream) Context() context.Context {
	return m.Ctx
}

func (m *MockStream) Send(resp *pb.StatusResponse) error {
	m.Sent = append(m.Sent, resp)
	return nil
}

func TestStreamTaskStatus_ImmediateReturn(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mockBus := &MockBus{}
	srv := newServer(rdb, mockBus)

	taskID := "12345"
	statusKey := flow.StatusChannel(taskID)
	// Pre-set status in Redis
	mr.HSet(statusKey, "status", "COMPLETED")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := &MockStream{Ctx: ctx}
	req := &pb.StatusRequest{TaskId: taskID}

	err = srv.StreamTaskStatus(req, stream)
	if err != nil {
		t.Fatalf("StreamTaskStatus failed: %v", err)
	}

	if len(stream.Sent) == 0 {
		t.Fatal("expected status to be sent")
	}
	if stream.Sent[0].Status != "COMPLETED" {
		t.Errorf("expected status COMPLETED, got %s", stream.Sent[0].Status)
	}
}

// TestStreamTaskStatus_PubSub tests receiving updates via Redis PubSub.
// Since miniredis supports PubSub, we can test this.
func TestStreamTaskStatus_PubSub(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mockBus := &MockBus{}
	srv := newServer(rdb, mockBus)

	taskID := "wait-for-it"
	statusKey := flow.StatusChannel(taskID)

	// We run StreamTaskStatus in a goroutine because it blocks waiting for PubSub
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream := &MockStream{Ctx: ctx}
	req := &pb.StatusRequest{TaskId: taskID}

	errChan := make(chan error)
	go func() {
		errChan <- srv.StreamTaskStatus(req, stream)
	}()

	// Allow subscriber to connect
	time.Sleep(100 * time.Millisecond)

	// Publish update to Redis
	rdb.Publish(context.Background(), statusKey, "PROCESSING")
	time.Sleep(50 * time.Millisecond)
	rdb.Publish(context.Background(), statusKey, "COMPLETED")

	// Wait for handler to return (it should return on COMPLETED)
	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("StreamTaskStatus returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StreamTaskStatus timed out")
	}

	// Verify sent messages
	if len(stream.Sent) < 2 {
		t.Fatalf("expected at least 2 status updates, got %d", len(stream.Sent))
	}

	lastStatus := stream.Sent[len(stream.Sent)-1].Status
	if lastStatus != "COMPLETED" {
		t.Errorf("expected last status COMPLETED, got %s", lastStatus)
	}
}
