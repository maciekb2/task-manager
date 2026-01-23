package main

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
)

type Broadcaster interface {
	Broadcast(event Event)
}

type Consumer struct {
	broadcaster Broadcaster
}

func NewConsumer(broadcaster Broadcaster) *Consumer {
	return &Consumer{broadcaster: broadcaster}
}

func (c *Consumer) HandleMessage(msg *nats.Msg) {
	var payload interface{}
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		payload = string(msg.Data)
	}

	event := Event{
		Subject:   msg.Subject,
		Data:      payload,
		Timestamp: time.Now().UnixMilli(),
	}
	c.broadcaster.Broadcast(event)
}
