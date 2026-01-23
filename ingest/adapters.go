package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
)

// NatsMessageAdapter wraps *nats.Msg to satisfy the Message interface.
type NatsMessageAdapter struct {
	Msg *nats.Msg
}

func NewNatsMessageAdapter(msg *nats.Msg) *NatsMessageAdapter {
	return &NatsMessageAdapter{Msg: msg}
}

func (m *NatsMessageAdapter) GetData() []byte {
	return m.Msg.Data
}

func (m *NatsMessageAdapter) GetHeaders() nats.Header {
	return m.Msg.Header
}

func (m *NatsMessageAdapter) Ack() error {
	return m.Msg.Ack()
}

func (m *NatsMessageAdapter) Nak() error {
	return m.Msg.Nak()
}

func (m *NatsMessageAdapter) Metadata() (*nats.MsgMetadata, error) {
	return m.Msg.Metadata()
}

func (m *NatsMessageAdapter) GetSubject() string {
	return m.Msg.Subject
}

// HttpEnricher implements Enricher using an HTTP client.
type HttpEnricher struct {
	client *http.Client
	url    string
}

func NewHttpEnricher(client *http.Client, url string) *HttpEnricher {
	return &HttpEnricher{
		client: client,
		url:    url,
	}
}

func (e *HttpEnricher) Enrich(ctx context.Context, task flow.TaskEnvelope) (flow.TaskEnvelope, error) {
	payload, err := json.Marshal(task)
	if err != nil {
		return task, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(payload))
	if err != nil {
		return task, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return task, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return task, errInvalidTask("enricher rejected payload")
	}

	var result enrichResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return task, err
	}

	task.Category = result.Category
	task.Score = result.Score
	return task, nil
}

type enrichResponse struct {
	Category string `json:"category"`
	Score    int32  `json:"score"`
}
