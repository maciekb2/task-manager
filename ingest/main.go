package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var httpClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

func main() {
	logger.Setup("ingest")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		logger.Fatal("telemetry init failed", err)
	}
	defer shutdown(ctx)

	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "ingest"})
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
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "ingest"), bus.SubjectTaskIngest)
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, consumerCfg); err != nil {
		logger.Fatal("nats consumer setup failed", err)
	}
	sub, err := busClient.PullSubscribe(bus.SubjectTaskIngest, consumerCfg.Durable)
	if err != nil {
		logger.Fatal("nats subscribe failed", err)
	}

	enricher := NewHttpEnricher(httpClient, enricherURL())
	svc := NewIngestService(busClient, enricher)

	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 5, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		return svc.ProcessMessage(msgCtx, NewNatsMessageAdapter(msg))
	}); err != nil && err != context.Canceled {
		logger.Fatal("ingest consume failed", err)
	}
}

func natsURL() string {
	if addr := os.Getenv("NATS_URL"); addr != "" {
		return addr
	}
	return bus.DefaultURL
}

func enricherURL() string {
	if url := os.Getenv("ENRICHER_URL"); url != "" {
		return url
	}
	return "http://enricher:8080/enrich"
}
