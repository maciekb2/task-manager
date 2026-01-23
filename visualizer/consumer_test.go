package main

import (
	"encoding/json"
	"testing"

	"github.com/nats-io/nats.go"
)

type MockBroadcaster struct {
	LastEvent Event
	Called    bool
}

func (m *MockBroadcaster) Broadcast(event Event) {
	m.LastEvent = event
	m.Called = true
}

func TestConsumer_HandleMessage_JSON(t *testing.T) {
	mock := &MockBroadcaster{}
	consumer := NewConsumer(mock)

	data := map[string]interface{}{"foo": "bar"}
	bytes, _ := json.Marshal(data)

	msg := &nats.Msg{
		Subject: "test.subject",
		Data:    bytes,
	}

	consumer.HandleMessage(msg)

	if !mock.Called {
		t.Fatal("Broadcast not called")
	}
	if mock.LastEvent.Subject != "test.subject" {
		t.Errorf("expected subject test.subject, got %s", mock.LastEvent.Subject)
	}

	payload, ok := mock.LastEvent.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", mock.LastEvent.Data)
	}
	if payload["foo"] != "bar" {
		t.Errorf("expected foo=bar, got %v", payload["foo"])
	}
}

func TestConsumer_HandleMessage_String(t *testing.T) {
	mock := &MockBroadcaster{}
	consumer := NewConsumer(mock)

	msg := &nats.Msg{
		Subject: "test.subject",
		Data:    []byte("not json"),
	}

	consumer.HandleMessage(msg)

	if !mock.Called {
		t.Fatal("Broadcast not called")
	}
	if mock.LastEvent.Data != "not json" {
		t.Errorf("expected data 'not json', got %v", mock.LastEvent.Data)
	}
}
