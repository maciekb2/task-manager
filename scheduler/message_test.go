package main

import (
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
)

func TestNatsMessage_Getters(t *testing.T) {
	data := []byte(`{"task_id":"123"}`)
	header := nats.Header{"Traceparent": []string{"00-123-456-01"}}

	msg := &nats.Msg{
		Data:    data,
		Header:  header,
		Subject: "tasks.worker.high",
	}

	wrapper := &NatsMessage{Msg: msg}

	assert.Equal(t, data, wrapper.GetData())
	assert.Equal(t, header, wrapper.GetHeader())
	// DeliveryAttempt defaults to 1 when metadata is missing
	assert.Equal(t, 1, wrapper.DeliveryAttempt())
}
