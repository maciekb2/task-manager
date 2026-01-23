package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
)

func main() {
	ctx := context.Background()
	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "notifier"})
	if err != nil {
		log.Fatalf("nats connect failed: %v", err)
	}
	defer busClient.Close()
	if _, err := busClient.EnsureStream(bus.EventsStreamConfig()); err != nil {
		log.Fatalf("nats stream setup failed: %v", err)
	}
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamEvents, "notifier"), bus.SubjectEventStatus)
	if _, err := busClient.EnsureConsumer(bus.StreamEvents, consumerCfg); err != nil {
		log.Fatalf("nats consumer setup failed: %v", err)
	}
	sub, err := busClient.PullSubscribe(bus.SubjectEventStatus, consumerCfg.Durable)
	if err != nil {
		log.Fatalf("nats subscribe failed: %v", err)
	}
	tracer := otel.Tracer("notifier")

	notifier := NewNotifier(rdb, busClient, tracer)

	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 10, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		return notifier.HandleMessage(msgCtx, &NatsMessageAdapter{Msg: msg})
	}); err != nil {
		log.Fatalf("notifier consume failed: %v", err)
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
