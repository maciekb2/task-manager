package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestConsumer_Consume(t *testing.T) {
	hub := NewHub()
	// We don't start hub.Run() because we want to inspect channels directly.

	consumer := NewConsumer(hub)

	// Case 1: Valid JSON
	data := map[string]interface{}{"foo": "bar"}
	bytes, _ := json.Marshal(data)
	msg := &nats.Msg{
		Subject: "tasks.test",
		Data:    bytes,
	}

	// Consume sends to channel, so we must be ready to receive or run it in goroutine
	go consumer.Consume(msg)

	select {
	case event := <-hub.broadcast:
		if event.Subject != "tasks.test" {
			t.Errorf("expected subject tasks.test, got %s", event.Subject)
		}
		// Data should be map map[string]interface{}
		// json.Unmarshal into interface{} results in map[string]interface{} for objects
		val, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Errorf("expected data to be map, got %T", event.Data)
		} else {
			if val["foo"] != "bar" {
				t.Errorf("expected foo=bar, got %v", val["foo"])
			}
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for broadcast")
	}

	// Case 2: Invalid JSON (String)
	msgStr := &nats.Msg{
		Subject: "events.log",
		Data:    []byte("plain text"),
	}

	go consumer.Consume(msgStr)

	select {
	case event := <-hub.broadcast:
		if event.Subject != "events.log" {
			t.Errorf("expected subject events.log, got %s", event.Subject)
		}
		val, ok := event.Data.(string)
		if !ok {
			t.Errorf("expected data to be string, got %T", event.Data)
		} else {
			if val != "plain text" {
				t.Errorf("expected plain text, got %v", val)
			}
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}
