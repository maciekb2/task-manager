package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
)

// MockPublisher implements Publisher interface for testing
type MockPublisher struct {
	mu        sync.Mutex
	Published []PublishedMsg
}

type PublishedMsg struct {
	Subject string
	Payload any
}

func (m *MockPublisher) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Published = append(m.Published, PublishedMsg{Subject: subject, Payload: payload})
	return &nats.PubAck{Sequence: 1}, nil
}

func TestPerformHttpCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	statusCode, latency, err := performHttpCheck(context.Background(), server.URL, "GET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusCode != 200 {
		t.Errorf("expected status 200, got %d", statusCode)
	}
	if latency < 0 {
		t.Errorf("expected non-negative latency, got %d", latency)
	}
}

func TestProcessLoop(t *testing.T) {
	publisher := &MockPublisher{}
	jobs := make(chan *nats.Msg, 1)

	// Create a dummy task
	task := flow.TaskEnvelope{
		TaskID:   "test-task-1",
		Priority: 2,
		URL:      "http://example.com",
		Method:   "GET",
	}
	data, _ := json.Marshal(task)

	msg := &nats.Msg{
		Subject: "tasks.worker.high",
		Data:    data,
		Header:  nats.Header{},
	}

	jobs <- msg
	close(jobs) // Close channel so processLoop exits after processing

	// We need a mock check function that returns success immediately
	mockCheck := func(ctx context.Context, url, method string) (int, int64, error) {
		return 200, 10, nil
	}

	// Run processLoop
	processLoop(context.Background(), publisher, jobs, 1, 0.0, mockCheck)

	// Verify published messages
	publisher.mu.Lock()
	defer publisher.mu.Unlock()

	foundResult := false
	foundStatus := false

	for _, p := range publisher.Published {
		if p.Subject == bus.SubjectTaskResults {
			foundResult = true
			res, ok := p.Payload.(flow.ResultEnvelope)
			if !ok {
				t.Errorf("expected ResultEnvelope payload, got %T", p.Payload)
			} else {
				if res.Task.TaskID != task.TaskID {
					t.Errorf("expected task ID %s, got %s", task.TaskID, res.Task.TaskID)
				}
				if res.Result != 200 {
					t.Errorf("expected result 200, got %d", res.Result)
				}
			}
		}
		if p.Subject == bus.SubjectEventStatus {
			foundStatus = true
		}
	}

	if !foundResult {
		t.Error("expected result to be published")
	}
	if !foundStatus {
		t.Error("expected status updates to be published")
	}
}

func TestProcessLoop_Failure(t *testing.T) {
	publisher := &MockPublisher{}
	jobs := make(chan *nats.Msg, 1)

	task := flow.TaskEnvelope{
		TaskID:   "test-task-fail",
		Priority: 1,
		URL:      "http://fail.com",
	}
	data, _ := json.Marshal(task)
	msg := &nats.Msg{
		Subject: "tasks.worker.medium",
		Data:    data,
	}
	jobs <- msg
	close(jobs)

	// Mock check
	mockCheck := func(ctx context.Context, url, method string) (int, int64, error) {
		return 500, 5, nil
	}

	// Set failRate = 1.0 to force simulated failure path
	processLoop(context.Background(), publisher, jobs, 1, 1.0, mockCheck)

	publisher.mu.Lock()
	defer publisher.mu.Unlock()

	foundDeadLetter := false
	for _, p := range publisher.Published {
		if p.Subject == bus.SubjectEventDeadLetter {
			foundDeadLetter = true
			dl, ok := p.Payload.(flow.DeadLetter)
			if ok && dl.Reason == "processing failed" {
				// Success
			}
		}
	}

	if !foundDeadLetter {
		t.Error("expected deadletter due to simulated failure")
	}
}
