package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// BusPublisher defines the interface for publishing to NATS
type BusPublisher interface {
	PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error)
}

// Message defines the interface for NATS messages to allow mocking
type Message interface {
	Data() []byte
	Ack() error
	Nak() error
	DeliveryAttempt() int
}

// NatsMessageAdapter adapts *nats.Msg to the Message interface
type NatsMessageAdapter struct {
	Msg *nats.Msg
}

func (m *NatsMessageAdapter) Data() []byte { return m.Msg.Data }
func (m *NatsMessageAdapter) Ack() error   { return m.Msg.Ack() }
func (m *NatsMessageAdapter) Nak() error   { return m.Msg.Nak() }
func (m *NatsMessageAdapter) DeliveryAttempt() int {
	return bus.DeliveryAttempt(m.Msg)
}

type Notifier struct {
	RDB    *redis.Client
	Bus    BusPublisher
	Tracer trace.Tracer
}

func NewNotifier(rdb *redis.Client, bus BusPublisher, tracer trace.Tracer) *Notifier {
	return &Notifier{
		RDB:    rdb,
		Bus:    bus,
		Tracer: tracer,
	}
}

func (n *Notifier) HandleMessage(msgCtx context.Context, msg Message) error {
	var update flow.StatusUpdate
	if err := json.Unmarshal(msg.Data(), &update); err != nil {
		log.Printf("notifier: bad payload: %v", err)
		n.handleEventFailure(msgCtx, msg, flow.TaskEnvelope{}, "bad payload", false)
		return nil
	}

	if update.TraceParent == "" {
		update.TraceParent = n.traceparentFromContext(msgCtx)
	}

	parentCtx := msgCtx
	if !trace.SpanContextFromContext(parentCtx).IsValid() && update.TraceParent != "" {
		parentCtx = n.contextFromTraceParent(update.TraceParent)
	}
	ctxSpan, span := n.Tracer.Start(parentCtx, "notifier.update")

	// Annotate span if the message is a real NATS message
	if adapter, ok := msg.(*NatsMessageAdapter); ok {
		bus.AnnotateSpan(span, adapter.Msg)
	}

	span.SetAttributes(
		attribute.String("task.id", update.TaskID),
		attribute.String("task.status", update.Status),
		attribute.String("status.source", update.Source),
		attribute.String("queue.name", bus.SubjectEventStatus),
	)

	statusKey := flow.StatusChannel(update.TaskID)
	if err := n.RDB.HSet(ctxSpan, statusKey, map[string]interface{}{
		"status":     update.Status,
		"updated_at": update.Timestamp,
		"source":     update.Source,
	}).Err(); err != nil {
		log.Printf("notifier: status update failed: %v", err)
		span.RecordError(err)
		span.End()
		n.handleEventFailure(msgCtx, msg, flow.TaskEnvelope{TaskID: update.TaskID, TraceParent: update.TraceParent}, "status update failed", true)
		return nil
	}

	if err := n.RDB.Publish(ctxSpan, statusKey, update.Status).Err(); err != nil {
		log.Printf("notifier: publish failed: %v", err)
		span.RecordError(err)
		span.End()
		n.handleEventFailure(msgCtx, msg, flow.TaskEnvelope{TaskID: update.TaskID, TraceParent: update.TraceParent}, "publish status failed", true)
		return nil
	}
	span.End()

	if err := msg.Ack(); err != nil {
		log.Printf("notifier: ack failed: %v", err)
	}
	return nil
}

func (n *Notifier) handleEventFailure(ctx context.Context, msg Message, task flow.TaskEnvelope, reason string, retry bool) {
	attempts := msg.DeliveryAttempt()
	if !retry || attempts >= bus.MaxDeliver() {
		n.enqueueDeadLetter(ctx, task, reason, attempts)
		if err := msg.Ack(); err != nil {
			log.Printf("notifier: ack failed: %v", err)
		}
		return
	}
	if err := msg.Nak(); err != nil {
		log.Printf("notifier: nak failed: %v", err)
	}
}

func (n *Notifier) enqueueDeadLetter(ctx context.Context, task flow.TaskEnvelope, reason string, attempts int) {
	if attempts <= 0 {
		attempts = 1
	}
	entry := flow.DeadLetter{
		Task:      task,
		Reason:    reason,
		Attempts:  attempts,
		Source:    "notifier",
		Timestamp: flow.Now(),
	}
	if _, err := n.Bus.PublishJSON(ctx, bus.SubjectEventDeadLetter, entry, nil); err != nil {
		log.Printf("notifier: deadletter publish failed: %v", err)
	}
}

func (n *Notifier) contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}

func (n *Notifier) traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}
