package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
)

func main() {
	logger.Setup("notifier")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		logger.Fatal("telemetry init failed", err)
	}
	defer shutdown(ctx)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "notifier"})
	if err != nil {
		logger.Fatal("nats connect failed", err)
	}
	defer busClient.Close()
	if _, err := busClient.EnsureStream(bus.EventsStreamConfig()); err != nil {
		logger.Fatal("nats stream setup failed", err)
	}
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamEvents, "notifier"), bus.SubjectEventStatus)
	if _, err := busClient.EnsureConsumer(bus.StreamEvents, consumerCfg); err != nil {
		logger.Fatal("nats consumer setup failed", err)
	}
	sub, err := busClient.PullSubscribe(bus.SubjectEventStatus, consumerCfg.Durable)
	if err != nil {
		logger.Fatal("nats subscribe failed", err)
	}
	tracer := otel.Tracer("notifier")

	notifier := NewNotifier(rdb, busClient, tracer)

	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 10, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		return notifier.HandleMessage(msgCtx, &NatsMessageAdapter{Msg: msg})
	}); err != nil && err != context.Canceled {
		logger.Fatal("notifier consume failed", err)
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
