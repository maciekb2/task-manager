package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
)

func main() {
	logger.Setup("worker")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		logger.Fatal("telemetry init failed", err)
	}
	defer shutdown(ctx)

	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "worker"})
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

	highCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "worker-high"), bus.SubjectTaskWorkerHigh)
	mediumCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "worker-medium"), bus.SubjectTaskWorkerMedium)
	lowCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "worker-low"), bus.SubjectTaskWorkerLow)
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, highCfg); err != nil {
		logger.Fatal("nats consumer setup failed", err)
	}
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, mediumCfg); err != nil {
		logger.Fatal("nats consumer setup failed", err)
	}
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, lowCfg); err != nil {
		logger.Fatal("nats consumer setup failed", err)
	}
	highSub, err := busClient.PullSubscribe(bus.SubjectTaskWorkerHigh, highCfg.Durable)
	if err != nil {
		logger.Fatal("nats subscribe failed", err)
	}
	mediumSub, err := busClient.PullSubscribe(bus.SubjectTaskWorkerMedium, mediumCfg.Durable)
	if err != nil {
		logger.Fatal("nats subscribe failed", err)
	}
	lowSub, err := busClient.PullSubscribe(bus.SubjectTaskWorkerLow, lowCfg.Durable)
	if err != nil {
		logger.Fatal("nats subscribe failed", err)
	}

	count := workerCount()
	failRate := workerFailRate()
	slog.Info("worker: starting workers", "count", count, "fail_rate", failRate)
	jobs := make(chan *nats.Msg, count*2)
	go dispatchLoop(ctx, []prioritySub{
		{subject: bus.SubjectTaskWorkerHigh, sub: highSub},
		{subject: bus.SubjectTaskWorkerMedium, sub: mediumSub},
		{subject: bus.SubjectTaskWorkerLow, sub: lowSub},
	}, jobs)

	for i := 0; i < count; i++ {
		workerID := i + 1
		go processLoop(ctx, busClient, jobs, workerID, failRate, nil)
	}

	<-ctx.Done()
	slog.Info("Shutting down worker...")
	time.Sleep(time.Second)
	slog.Info("Worker stopped.")
}
