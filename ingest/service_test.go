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

// Mocks

type MockMessage struct {
	DataVal    []byte
	HeadersVal nats.Header
	AckCalled  bool
	NakCalled  bool
	MetaVal    *nats.MsgMetadata
	MetaErr    error
	SubjectVal string
	AckErr     error
	NakErr     error
}

func (m *MockMessage) GetData() []byte { return m.DataVal }
func (m *MockMessage) GetHeaders() nats.Header {
	if m.HeadersVal == nil {
		return nats.Header{}
	}
	return m.HeadersVal
}
func (m *MockMessage) Ack() error {
	m.AckCalled = true
	return m.AckErr
}
func (m *MockMessage) Nak() error {
	m.NakCalled = true
	return m.NakErr
}
func (m *MockMessage) Metadata() (*nats.MsgMetadata, error) {
	return m.MetaVal, m.MetaErr
}
func (m *MockMessage) GetSubject() string { return m.SubjectVal }

type PublishedMsg struct {
	Subject string
	Payload any
}

type MockPublisher struct {
	Published []PublishedMsg
	Err       error
	ErrMap    map[string]error
}

func (p *MockPublisher) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error) {
	if p.ErrMap != nil {
		if err, ok := p.ErrMap[subject]; ok {
			return nil, err
		}
	}
	if p.Err != nil && subject == bus.SubjectTaskSchedule {
		return nil, p.Err
	}
	p.Published = append(p.Published, PublishedMsg{Subject: subject, Payload: payload})
	return &nats.PubAck{}, nil
}

type MockEnricher struct {
	EnrichFunc func(context.Context, flow.TaskEnvelope) (flow.TaskEnvelope, error)
}

func (e *MockEnricher) Enrich(ctx context.Context, task flow.TaskEnvelope) (flow.TaskEnvelope, error) {
	if e.EnrichFunc != nil {
		return e.EnrichFunc(ctx, task)
	}
	return task, nil
}

// Tests

func TestProcessMessage_Success(t *testing.T) {
	task := flow.TaskEnvelope{
		TaskID:          "123",
		TaskDescription: "test task",
		Priority:        1,
	}
	data, _ := json.Marshal(task)

	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{NumDelivered: 1},
	}

	pub := &MockPublisher{}
	enricher := &MockEnricher{
		EnrichFunc: func(ctx context.Context, t flow.TaskEnvelope) (flow.TaskEnvelope, error) {
			t.Category = "test-cat"
			return t, nil
		},
	}

	svc := NewIngestService(pub, enricher)
	err := svc.ProcessMessage(context.Background(), msg)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !msg.AckCalled {
		t.Error("expected Ack")
	}

	// Verify publications
	// Should have: ENRICHED status, Schedule, VALIDATED status, Audit
	// Order: Enriched (status), Schedule, Validated (status), Audit.
	expectedSubjects := []string{
		bus.SubjectEventStatus,
		bus.SubjectTaskSchedule,
		bus.SubjectEventStatus,
		bus.SubjectEventAudit,
	}

	if len(pub.Published) != len(expectedSubjects) {
		t.Errorf("expected %d publications, got %d", len(expectedSubjects), len(pub.Published))
	} else {
		for i, sub := range expectedSubjects {
			if pub.Published[i].Subject != sub {
				t.Errorf("msg %d: expected subject %s, got %s", i, sub, pub.Published[i].Subject)
			}
		}
	}

	// Verify enriched task published
	scheduleMsg := pub.Published[1].Payload.(flow.TaskEnvelope)
	if scheduleMsg.Category != "test-cat" {
		t.Errorf("expected category 'test-cat', got '%s'", scheduleMsg.Category)
	}
}

func TestProcessMessage_ValidationFailure(t *testing.T) {
	task := flow.TaskEnvelope{
		TaskID:          "123",
		TaskDescription: "   ", // Empty
	}
	data, _ := json.Marshal(task)

	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{NumDelivered: 1},
	}

	pub := &MockPublisher{}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	if !msg.AckCalled {
		t.Error("expected Ack")
	}

	// Expected: Deadletter, Status(REJECTED)
	foundDeadletter := false
	foundRejected := false

	for _, p := range pub.Published {
		if p.Subject == bus.SubjectEventDeadLetter {
			foundDeadletter = true
		}
		if p.Subject == bus.SubjectEventStatus {
			status := p.Payload.(flow.StatusUpdate)
			if status.Status == "REJECTED" {
				foundRejected = true
			}
		}
	}

	if !foundDeadletter {
		t.Error("expected DeadLetter")
	}
	if !foundRejected {
		t.Error("expected REJECTED status")
	}
}

func TestProcessMessage_BadPayload(t *testing.T) {
	msg := &MockMessage{
		DataVal: []byte("invalid-json"),
		MetaVal: &nats.MsgMetadata{NumDelivered: 1},
	}

	pub := &MockPublisher{}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	if !msg.AckCalled {
		t.Error("expected Ack")
	}

	// Expected: Deadletter
	if len(pub.Published) == 0 {
		t.Error("expected DeadLetter publication")
	} else if pub.Published[0].Subject != bus.SubjectEventDeadLetter {
		t.Errorf("expected first publish to be DeadLetter, got %s", pub.Published[0].Subject)
	}
}

func TestProcessMessage_EnrichFailure(t *testing.T) {
	task := flow.TaskEnvelope{
		TaskID:          "123",
		TaskDescription: "test",
	}
	data, _ := json.Marshal(task)

	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{NumDelivered: 1},
	}

	pub := &MockPublisher{}
	enricher := &MockEnricher{
		EnrichFunc: func(ctx context.Context, t flow.TaskEnvelope) (flow.TaskEnvelope, error) {
			return t, errors.New("enrich error")
		},
	}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	// It should proceed without enrichment
	if !msg.AckCalled {
		t.Error("expected Ack")
	}

	// Should publish to Schedule
	foundSchedule := false
	for _, p := range pub.Published {
		if p.Subject == bus.SubjectTaskSchedule {
			foundSchedule = true
		}
	}
	if !foundSchedule {
		t.Error("expected Schedule publication despite enrich failure")
	}
}

func TestProcessMessage_PublishFail_Retry(t *testing.T) {
	task := flow.TaskEnvelope{
		TaskID:          "123",
		TaskDescription: "test",
	}
	data, _ := json.Marshal(task)

	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{NumDelivered: 1}, // Attempt 1
	}

	pub := &MockPublisher{
		Err: errors.New("publish error"),
	}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	// Expect Nak because it's retriable and attempts < MaxDeliver (default 5 usually)
	if !msg.NakCalled {
		t.Error("expected Nak for retry")
	}
	if msg.AckCalled {
		t.Error("did not expect Ack")
	}
}

func TestProcessMessage_PublishFail_MaxRetries(t *testing.T) {
	task := flow.TaskEnvelope{
		TaskID:          "123",
		TaskDescription: "test",
	}
	data, _ := json.Marshal(task)

	// bus.MaxDeliver() defaults to 5. We use a high number to trigger deadletter.
	msg := &MockMessage{
		DataVal: data,
		MetaVal: &nats.MsgMetadata{NumDelivered: 100}, // Definitely > MaxDeliver
	}

	pub := &MockPublisher{
		Err: errors.New("publish error"),
	}
	enricher := &MockEnricher{}

	svc := NewIngestService(pub, enricher)
	_ = svc.ProcessMessage(context.Background(), msg)

	// Expect Ack + Deadletter
	if !msg.AckCalled {
		t.Error("expected Ack (give up)")
	}
	if msg.NakCalled {
		t.Error("did not expect Nak")
	}

	foundDeadletter := false
	for _, p := range pub.Published {
		if p.Subject == bus.SubjectEventDeadLetter {
			foundDeadletter = true
		}
	}
	if !foundDeadletter {
		t.Error("expected DeadLetter")
	}
}
