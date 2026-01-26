package main

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
)

type Consumer struct {
	hub *Hub
}

func NewConsumer(hub *Hub) *Consumer {
	return &Consumer{hub: hub}
}

func (c *Consumer) Consume(msg *nats.Msg) {
	var payload interface{}
	// Try to unmarshal, if it fails, send as string
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		payload = string(msg.Data)
	}

	event := Event{
		Subject:   msg.Subject,
		Data:      payload,
		Timestamp: time.Now().UnixMilli(),
	}
	c.hub.broadcast <- event
}
