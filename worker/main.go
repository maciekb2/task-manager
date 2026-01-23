package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/nats-io/nats.go"
)

// BusClient interface for mocking
type BusClient interface {
	Close()
	EnsureStream(cfg *nats.StreamConfig) (*nats.StreamInfo, error)
	EnsureConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error)
	PullSubscribe(subject, durable string, opts ...nats.SubOpt) (*nats.Subscription, error)
	PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error)
}

// Variables for mocking
var (
	busConnect = func(cfg bus.Config) (BusClient, error) {
		return bus.Connect(cfg)
	}
	initTelemetryFunc = initTelemetry
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("worker run failed: %v", err)
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

	busClient, err := busConnect(bus.Config{URL: natsURL(), Name: "worker"})
	if err != nil {
		return err
	}
	defer busClient.Close()

	if _, err := busClient.EnsureStream(bus.TasksStreamConfig()); err != nil {
		return err
	}
	if _, err := busClient.EnsureStream(bus.EventsStreamConfig()); err != nil {
		return err
	}

	highCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "worker-high"), bus.SubjectTaskWorkerHigh)
	mediumCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "worker-medium"), bus.SubjectTaskWorkerMedium)
	lowCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "worker-low"), bus.SubjectTaskWorkerLow)
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, highCfg); err != nil {
		return err
	}
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, mediumCfg); err != nil {
		return err
	}
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, lowCfg); err != nil {
		return err
	}
	highSub, err := busClient.PullSubscribe(bus.SubjectTaskWorkerHigh, highCfg.Durable)
	if err != nil {
		return err
	}
	mediumSub, err := busClient.PullSubscribe(bus.SubjectTaskWorkerMedium, mediumCfg.Durable)
	if err != nil {
		return err
	}
	lowSub, err := busClient.PullSubscribe(bus.SubjectTaskWorkerLow, lowCfg.Durable)
	if err != nil {
		return err
	}

	count := workerCount()
	failRate := workerFailRate()
	log.Printf("worker: starting %d workers (fail rate %.2f)", count, failRate)
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
	log.Println("Shutting down worker...")
	time.Sleep(time.Second)
	log.Println("Worker stopped.")
	return nil
}
