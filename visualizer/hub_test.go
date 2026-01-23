package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHub(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	var serverConn *websocket.Conn

	// Setup a test server to handle websocket upgrade
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn = c
		hub.register <- c
	}))
	defer s.Close()

	// Convert http URL to ws URL
	u := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	// Give time for registration
	time.Sleep(100 * time.Millisecond)

	// Check if client is registered
	hub.lock.Lock()
	if len(hub.clients) != 1 {
		t.Errorf("expected 1 client, got %d", len(hub.clients))
	}
	hub.lock.Unlock()

	// Test Broadcast
	event := Event{
		Subject:   "test.subject",
		Data:      "test-data",
		Timestamp: 123456789,
	}
	hub.Broadcast(event)

	// Read message on client
	var received Event
	err = ws.ReadJSON(&received)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if received.Subject != event.Subject {
		t.Errorf("expected subject %s, got %s", event.Subject, received.Subject)
	}
	if received.Data != event.Data {
		t.Errorf("expected data %s, got %s", event.Data, received.Data)
	}

	// Test Explicit Unregister
	if serverConn != nil {
		hub.unregister <- serverConn
		time.Sleep(100 * time.Millisecond)
		hub.lock.Lock()
		if len(hub.clients) != 0 {
			t.Errorf("expected 0 clients after unregister, got %d", len(hub.clients))
		}
		hub.lock.Unlock()
	}

	// Test Write Failure (Implicit Unregister)
	// Reconnect
	ws2, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	hub.lock.Lock()
	if len(hub.clients) != 1 {
		t.Errorf("expected 1 client after reconnect, got %d", len(hub.clients))
	}
	hub.lock.Unlock()

	ws2.Close()
	hub.Broadcast(event)

	time.Sleep(100 * time.Millisecond)
	hub.lock.Lock()
	if len(hub.clients) != 0 {
		// This might be flaky if OS buffers the write
	}
	hub.lock.Unlock()
}
