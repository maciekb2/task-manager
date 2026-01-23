package main

import (
	"testing"

	"github.com/nats-io/nats.go"
)

func TestNatsMessage(t *testing.T) {
	data := []byte("test")
	header := nats.Header{"foo": []string{"bar"}}
	msg := &nats.Msg{
		Data:   data,
		Header: header,
	}

	nm := &NatsMessage{Msg: msg}

	if string(nm.GetData()) != "test" {
		t.Error("GetData mismatch")
	}
	if nm.GetHeader().Get("foo") != "bar" {
		t.Error("GetHeader mismatch")
	}

	// Ack/Nak
	_ = nm.Ack()
	_ = nm.Nak()

	// DeliveryAttempt defaults to 1 if no metadata
	if nm.DeliveryAttempt() != 1 {
		t.Errorf("expected 1 attempt, got %d", nm.DeliveryAttempt())
	}
}
