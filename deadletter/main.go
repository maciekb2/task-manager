package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type BusClient interface {
	Close()
	EnsureStream(cfg *nats.StreamConfig) (*nats.StreamInfo, error)
	EnsureConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error)
	PullSubscribe(subject, durable string, opts ...nats.SubOpt) (*nats.Subscription, error)
	Consume(ctx context.Context, sub *nats.Subscription, opts bus.ConsumeOptions, handler bus.Handler) error
}

var (
	busConnect = func(cfg bus.Config) (BusClient, error) {
		return bus.Connect(cfg)
	}
	initTelemetryFunc = initTelemetry
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("deadletter run failed: %v", err)
	}
}

func run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetryFunc(ctx)
	if err != nil {
		return err
	}
	defer shutdown(ctx)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	defer rdb.Close()

	busClient, err := busConnect(bus.Config{URL: natsURL(), Name: "deadletter"})
	if err != nil {
		return err
	}
	defer busClient.Close()

	if _, err := busClient.EnsureStream(bus.EventsStreamConfig()); err != nil {
		return err
	}
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamEvents, "deadletter"), bus.SubjectEventDeadLetter)
	if _, err := busClient.EnsureConsumer(bus.StreamEvents, consumerCfg); err != nil {
		return err
	}
	sub, err := busClient.PullSubscribe(bus.SubjectEventDeadLetter, consumerCfg.Durable)
	if err != nil {
		return err
	}
	tracer := otel.Tracer("deadletter")
	service := NewDeadLetterService(rdb, tracer)

	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 10, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		var entry flow.DeadLetter
		if err := json.Unmarshal(msg.Data, &entry); err != nil {
			log.Printf("deadletter: bad payload: %v", err)
			ackMessage("deadletter", msg)
			return nil
		}

		if entry.Task.TraceParent == "" {
			entry.Task.TraceParent = traceparentFromContext(msgCtx)
		}

		parentCtx := msgCtx
		if !trace.SpanContextFromContext(parentCtx).IsValid() && entry.Task.TraceParent != "" {
			parentCtx = contextFromTraceParent(entry.Task.TraceParent)
		}

		if err := service.Persist(parentCtx, entry, msg); err != nil {
			log.Printf("deadletter: %v", err)
			handleSinkFailure(msg, "deadletter")
			return nil
		}

		log.Printf("deadletter: %s %s", entry.Task.TaskID, entry.Reason)
		ackMessage("deadletter", msg)
		return nil
	}); err != nil && err != context.Canceled {
		return err
	}
	return nil
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

func handleSinkFailure(msg *nats.Msg, service string) {
	attempts := bus.DeliveryAttempt(msg)
	if attempts >= bus.MaxDeliver() {
		ackMessage(service, msg)
		return
	}
	nakMessage(service, msg)
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

// Wrapper for testing
type NatsMessage struct {
	Msg *nats.Msg
}
// We don't use the interface in Persist yet? `service.Persist` signature?
// `service.go`: func (s *DeadLetterService) Persist(ctx context.Context, entry flow.DeadLetter, msg Message) error
// Message interface was used in tests but `main` passed `nil` or `*nats.Msg`?
// In `read_file` output: `service.Persist(parentCtx, entry, msg)` where `msg` is `*nats.Msg`.
// So `DeadLetterService.Persist` expects something compatible.
// I'll check `deadletter/service.go`.
