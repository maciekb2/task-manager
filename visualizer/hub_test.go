package main

import (
	"reflect"
	"testing"
	"time"
)

type mockClient struct {
	messages chan interface{}
	closed   chan bool
}

func newMockClient() *mockClient {
	return &mockClient{
		messages: make(chan interface{}, 10),
		closed:   make(chan bool, 1),
	}
}

func (m *mockClient) WriteJSON(v interface{}) error {
	m.messages <- v
	return nil
}

func (m *mockClient) Close() error {
	m.closed <- true
	return nil
}

func TestNewHub(t *testing.T) {
	hub := NewHub()
	if hub == nil {
		t.Fatal("NewHub returned nil")
	}
	if hub.clients == nil {
		t.Error("Hub clients map is nil")
	}
}

func TestHub_Run(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := newMockClient()

	// Test Register
	hub.register <- client
	// We can't easily verify registration happened without side effects or peeking internal state.
	// But subsequent broadcast relies on it.

	// Test Broadcast
	msg := Event{Subject: "test", Data: "data", Timestamp: 123}
	hub.broadcast <- msg

	select {
	case received := <-client.messages:
		if !reflect.DeepEqual(received, msg) {
			t.Errorf("expected message %v, got %v", msg, received)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for broadcast message")
	}

	// Test Unregister
	hub.unregister <- client

	select {
	case <-client.closed:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for client close")
	}

	// Verify no more messages sent to unregistered client
	hub.broadcast <- msg

	select {
	case <-client.messages:
		t.Error("client received message after unregister")
	case <-time.After(50 * time.Millisecond):
		// success (short wait to ensure nothing comes through)
	}
}
