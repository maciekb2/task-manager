package main

import (
	"context"

	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
)

// Message abstracts a NATS message to allow mocking.
type Message interface {
	GetData() []byte
	GetHeaders() nats.Header
	Ack() error
	Nak() error
	Metadata() (*nats.MsgMetadata, error)
	GetSubject() string
}

// Publisher abstracts the NATS client publishing capability.
type Publisher interface {
	PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error)
}

// Enricher abstracts the external task enrichment service.
type Enricher interface {
	Enrich(ctx context.Context, task flow.TaskEnvelope) (flow.TaskEnvelope, error)
}
