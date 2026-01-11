package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	ctx := context.Background()
	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "audit"})
	if err != nil {
		log.Fatalf("nats connect failed: %v", err)
	}
	defer busClient.Close()
	if _, err := busClient.EnsureStream(bus.EventsStreamConfig()); err != nil {
		log.Fatalf("nats stream setup failed: %v", err)
	}
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamEvents, "audit"), bus.SubjectEventAudit)
	if _, err := busClient.EnsureConsumer(bus.StreamEvents, consumerCfg); err != nil {
		log.Fatalf("nats consumer setup failed: %v", err)
	}
	sub, err := busClient.PullSubscribe(bus.SubjectEventAudit, consumerCfg.Durable)
	if err != nil {
		log.Fatalf("nats subscribe failed: %v", err)
	}
	tracer := otel.Tracer("audit")

	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 10, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		var event flow.AuditEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("audit: bad payload: %v", err)
			handleEventFailure(msgCtx, busClient, msg, flow.TaskEnvelope{}, "bad payload", false)
			return nil
		}

		if event.TraceParent == "" {
			event.TraceParent = traceparentFromContext(msgCtx)
		}

		parentCtx := msgCtx
		if !trace.SpanContextFromContext(parentCtx).IsValid() && event.TraceParent != "" {
			parentCtx = contextFromTraceParent(event.TraceParent)
		}
		ctxSpan, span := tracer.Start(parentCtx, "audit.persist")
		bus.AnnotateSpan(span, msg)
		span.SetAttributes(
			attribute.String("task.id", event.TaskID),
			attribute.String("audit.event", event.Event),
			attribute.String("audit.source", event.Source),
			attribute.String("queue.name", bus.SubjectEventAudit),
		)

		payload, err := json.Marshal(event)
		if err != nil {
			log.Printf("audit: marshal failed: %v", err)
			span.End()
			return nil
		}

		if err := rdb.RPush(ctxSpan, "audit_events", payload).Err(); err != nil {
			log.Printf("audit: persist failed: %v", err)
			span.RecordError(err)
			span.End()
			handleEventFailure(msgCtx, busClient, msg, flow.TaskEnvelope{TaskID: event.TaskID, TraceParent: event.TraceParent}, "audit persist failed", true)
			return nil
		}

		log.Printf("audit: %s %s", event.Event, event.TaskID)
		span.End()
		ackMessage("audit", msg)
		return nil
	}); err != nil {
		log.Fatalf("audit consume failed: %v", err)
	}
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "redis-service:6379"
}

func natsURL() string {
	if addr := os.Getenv("NATS_URL"); addr != "" {
		return addr
	}
	return bus.DefaultURL
}

func contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}

func traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}

func handleEventFailure(ctx context.Context, busClient *bus.Client, msg *nats.Msg, task flow.TaskEnvelope, reason string, retry bool) {
	attempts := bus.DeliveryAttempt(msg)
	if !retry || attempts >= bus.MaxDeliver() {
		enqueueDeadLetter(ctx, busClient, task, reason, attempts)
		ackMessage("audit", msg)
		return
	}
	nakMessage("audit", msg)
}

func enqueueDeadLetter(ctx context.Context, busClient *bus.Client, task flow.TaskEnvelope, reason string, attempts int) {
	if attempts <= 0 {
		attempts = 1
	}
	entry := flow.DeadLetter{
		Task:      task,
		Reason:    reason,
		Attempts:  attempts,
		Source:    "audit",
		Timestamp: flow.Now(),
	}
	if _, err := busClient.PublishJSON(ctx, bus.SubjectEventDeadLetter, entry, nil); err != nil {
		log.Printf("audit: deadletter publish failed: %v", err)
	}
}

func ackMessage(service string, msg *nats.Msg) {
	if msg == nil {
		return
	}
	if err := msg.Ack(); err != nil {
		log.Printf("%s: ack failed: %v", service, err)
	}
}

func nakMessage(service string, msg *nats.Msg) {
	if msg == nil {
		return
	}
	if err := msg.Nak(); err != nil {
		log.Printf("%s: nak failed: %v", service, err)
	}
}
