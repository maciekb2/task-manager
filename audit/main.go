package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
)

func main() {
	logger.Setup("audit")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		logger.Fatal("telemetry init failed", err)
	}
	defer shutdown(ctx)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "audit"})
	if err != nil {
		logger.Fatal("nats connect failed", err)
	}
	defer busClient.Close()
	if _, err := busClient.EnsureStream(bus.EventsStreamConfig()); err != nil {
		logger.Fatal("nats stream setup failed", err)
	}
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamEvents, "audit"), bus.SubjectEventAudit)
	if _, err := busClient.EnsureConsumer(bus.StreamEvents, consumerCfg); err != nil {
		logger.Fatal("nats consumer setup failed", err)
	}
	sub, err := busClient.PullSubscribe(bus.SubjectEventAudit, consumerCfg.Durable)
	if err != nil {
		logger.Fatal("nats subscribe failed", err)
	}
	tracer := otel.Tracer("audit")

	processor := &AuditProcessor{
		RDB:    rdb,
		Tracer: tracer,
	}

	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 10, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		envelope, err := processor.Process(msgCtx, msg)
		if err != nil {
			retry := true
			reason := "audit persist failed"
			if errors.Is(err, ErrBadPayload) {
				slog.Error("audit: bad payload", "error", err)
				retry = false
				reason = "bad payload"
			}
			handleEventFailure(msgCtx, busClient, msg, envelope, reason, retry)
			return nil
		}

		ackMessage("audit", msg)
		return nil
	}); err != nil && err != context.Canceled {
		logger.Fatal("audit consume failed", err)
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
		logger.WithContext(ctx).Error("audit: deadletter publish failed", "error", err)
	}
}

func ackMessage(service string, msg *nats.Msg) {
	if msg == nil {
		return
	}
	if err := msg.Ack(); err != nil {
		logger.Error("ack failed", err, "service", service)
	}
}

func nakMessage(service string, msg *nats.Msg) {
	if msg == nil {
		return
	}
	if err := msg.Nak(); err != nil {
		logger.Error("nak failed", err, "service", service)
	}
}
