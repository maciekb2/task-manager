package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var httpClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

// BusClient interface for mocking
type BusClient interface {
	Close()
	EnsureStream(cfg *nats.StreamConfig) (*nats.StreamInfo, error)
	EnsureConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error)
	PullSubscribe(subject, durable string, opts ...nats.SubOpt) (*nats.Subscription, error)
	Consume(ctx context.Context, sub *nats.Subscription, opts bus.ConsumeOptions, handler bus.Handler) error
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
		log.Fatalf("ingest run failed: %v", err)
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

	busClient, err := busConnect(bus.Config{URL: natsURL(), Name: "ingest"})
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
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "ingest"), bus.SubjectTaskIngest)
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, consumerCfg); err != nil {
		return err
	}
	sub, err := busClient.PullSubscribe(bus.SubjectTaskIngest, consumerCfg.Durable)
	if err != nil {
		return err
	}

	enricher := NewHttpEnricher(httpClient, enricherURL())
	svc := NewIngestService(busClient, enricher)

	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 5, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		return svc.ProcessMessage(msgCtx, NewNatsMessageAdapter(msg))
	}); err != nil && err != context.Canceled {
		return err
	}
	return nil
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
