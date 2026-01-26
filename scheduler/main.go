package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
)

func main() {
	logger.Setup("scheduler")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		logger.Fatal("telemetry init failed", err)
	}
	defer shutdown(ctx)

	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "scheduler"})
	if err != nil {
		logger.Fatal("nats connect failed", err)
	}
	defer busClient.Close()
	if _, err := busClient.EnsureStream(bus.TasksStreamConfig()); err != nil {
		logger.Fatal("nats stream setup failed", err)
	}
	if _, err := busClient.EnsureStream(bus.EventsStreamConfig()); err != nil {
		logger.Fatal("nats stream setup failed", err)
	}
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "scheduler"), bus.SubjectTaskSchedule)
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, consumerCfg); err != nil {
		logger.Fatal("nats consumer setup failed", err)
	}
	sub, err := busClient.PullSubscribe(bus.SubjectTaskSchedule, consumerCfg.Durable)
	if err != nil {
		logger.Fatal("nats subscribe failed", err)
	}

	svc := NewService(busClient)

	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 5, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		return svc.ProcessTask(msgCtx, &NatsMessage{Msg: msg})
	}); err != nil && err != context.Canceled {
		logger.Fatal("scheduler consume failed", err)
	}
}

func natsURL() string {
	if addr := os.Getenv("NATS_URL"); addr != "" {
		return addr
	}
	return bus.DefaultURL
}
