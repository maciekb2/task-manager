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

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "worker"})
	if err != nil {
		log.Fatalf("nats connect failed: %v", err)
	}
	defer busClient.Close()
	if _, err := busClient.EnsureStream(bus.TasksStreamConfig()); err != nil {
		log.Fatalf("nats stream setup failed: %v", err)
	}
	if _, err := busClient.EnsureStream(bus.EventsStreamConfig()); err != nil {
		log.Fatalf("nats stream setup failed: %v", err)
	}

	highCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "worker-high"), bus.SubjectTaskWorkerHigh)
	mediumCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "worker-medium"), bus.SubjectTaskWorkerMedium)
	lowCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "worker-low"), bus.SubjectTaskWorkerLow)
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, highCfg); err != nil {
		log.Fatalf("nats consumer setup failed: %v", err)
	}
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, mediumCfg); err != nil {
		log.Fatalf("nats consumer setup failed: %v", err)
	}
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, lowCfg); err != nil {
		log.Fatalf("nats consumer setup failed: %v", err)
	}
	highSub, err := busClient.PullSubscribe(bus.SubjectTaskWorkerHigh, highCfg.Durable)
	if err != nil {
		log.Fatalf("nats subscribe failed: %v", err)
	}
	mediumSub, err := busClient.PullSubscribe(bus.SubjectTaskWorkerMedium, mediumCfg.Durable)
	if err != nil {
		log.Fatalf("nats subscribe failed: %v", err)
	}
	lowSub, err := busClient.PullSubscribe(bus.SubjectTaskWorkerLow, lowCfg.Durable)
	if err != nil {
		log.Fatalf("nats subscribe failed: %v", err)
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
}
