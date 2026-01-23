package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const resultStoreScript = `if redis.call("EXISTS", KEYS[1]) == 1 then return 0 end
redis.call("HSET", KEYS[1], "result", ARGV[1], "latency_ms", ARGV[2], "processed_at", ARGV[3], "worker_id", ARGV[4], "category", ARGV[5], "score", ARGV[6])
return 1`

type resultStoreMetrics struct {
	e2eLatency metric.Float64Histogram
}

var getDeliveryAttempt = bus.DeliveryAttempt

type BusClient interface {
	PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error)
}

func main() {
	logger.Setup("result-store")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		logger.Fatal("telemetry init failed", err)
	}
	defer shutdown(ctx)
	metrics, err := initMetrics()
	if err != nil {
		logger.Fatal("metrics init failed", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	srv := startHTTPServer(rdb)
	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "result-store"})
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
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "result-store"), bus.SubjectTaskResults)
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, consumerCfg); err != nil {
		logger.Fatal("nats consumer setup failed", err)
	}
	sub, err := busClient.PullSubscribe(bus.SubjectTaskResults, consumerCfg.Durable)
	if err != nil {
		logger.Fatal("nats subscribe failed", err)
	}

	tracer := otel.Tracer("result-store")
	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 5, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		return ProcessMessage(msgCtx, msg, rdb, busClient, tracer, metrics)
	}); err != nil && err != context.Canceled {
		logger.Fatal("result-store consume failed", err)
	}

	slog.Info("Shutting down result-store http...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("result-store http shutdown error", err)
	}
	slog.Info("Result-store stopped.")
}

func ProcessMessage(msgCtx context.Context, msg *nats.Msg, rdb *redis.Client, busClient BusClient, tracer trace.Tracer, metrics resultStoreMetrics) error {
	var result flow.ResultEnvelope
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		slog.Error("result-store: bad payload", "error", err)
		handleProcessingFailure(msgCtx, busClient, msg, flow.TaskEnvelope{}, "bad payload", false)
		return nil
	}

	if result.Task.TraceParent == "" {
		result.Task.TraceParent = traceparentFromContext(msgCtx)
	}

	parentCtx := msgCtx
	if !trace.SpanContextFromContext(parentCtx).IsValid() && result.Task.TraceParent != "" {
		parentCtx = contextFromTraceParent(result.Task.TraceParent)
	}
	ctxTask, span := tracer.Start(parentCtx, "result-store.persist")
	bus.AnnotateSpan(span, msg)
	span.SetAttributes(
		attribute.String("task.id", result.Task.TaskID),
		attribute.Int("worker.id", result.WorkerID),
		attribute.Int64("task.result", int64(result.Result)),
		attribute.Int64("task.latency_ms", result.LatencyMs),
		attribute.String("queue.source", bus.SubjectTaskResults),
	)

	key := "task_result:" + result.Task.TaskID
	span.SetAttributes(attribute.String("redis.key", key))
	stored, err := storeResultOnce(ctxTask, rdb, key, result)
	if err != nil {
		logger.WithContext(ctxTask).Error("result-store: persist failed", "error", err)
		span.RecordError(err)
		span.End()
		handleProcessingFailure(msgCtx, busClient, msg, result.Task, "persist failed", true)
		return nil
	}
	if !stored {
		span.SetAttributes(attribute.Bool("result.duplicate", true))
	}

	recordE2ELatency(ctxTask, metrics, result, span)

	if err := enqueueAudit(ctxTask, busClient, flow.AuditEvent{
		TaskID:      result.Task.TaskID,
		Event:       "task.stored",
		Detail:      "Result persisted",
		TraceParent: result.Task.TraceParent,
		Source:      "result-store",
		Timestamp:   flow.Now(),
	}); err != nil {
		logger.WithContext(ctxTask).Error("result-store: audit publish failed", "error", err)
		span.RecordError(err)
		span.End()
		handleProcessingFailure(msgCtx, busClient, msg, result.Task, "audit publish failed", true)
		return nil
	}

	span.End()
	ackMessage("result-store", msg)
	return nil
}

func initMetrics() (resultStoreMetrics, error) {
	meter := otel.Meter("result-store")
	e2eLatency, err := meter.Float64Histogram(
		"taskmanager_task_e2e_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("End-to-end task latency from submit to persist."),
	)
	if err != nil {
		return resultStoreMetrics{}, err
	}
	return resultStoreMetrics{e2eLatency: e2eLatency}, nil
}

func recordE2ELatency(ctx context.Context, metrics resultStoreMetrics, result flow.ResultEnvelope, span trace.Span) {
	if metrics.e2eLatency == nil {
		return
	}
	createdAt, err := time.Parse(time.RFC3339Nano, result.Task.CreatedAt)
	if err != nil {
		return
	}
	latency := time.Since(createdAt).Seconds()
	if latency < 0 {
		return
	}
	metrics.e2eLatency.Record(ctx, latency, metric.WithAttributes(attribute.Int64("task.priority", int64(result.Task.Priority))))
	if span != nil {
		span.SetAttributes(attribute.Float64("task.e2e_seconds", latency))
	}
}

func startHTTPServer(rdb *redis.Client) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/results/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/results/")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		key := "task_result:" + id
		data, err := rdb.HGetAll(ctx, key).Result()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if len(data) == 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(data)
	})

	addr := ":" + resultPort()
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		slog.Info("result-store http listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("result-store http failed", err)
		}
	}()
	return srv
}

func enqueueAudit(ctx context.Context, busClient BusClient, event flow.AuditEvent) error {
	_, err := busClient.PublishJSON(ctx, bus.SubjectEventAudit, event, nil)
	return err
}

func contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}

func resultPort() string {
	if port := os.Getenv("RESULT_STORE_PORT"); port != "" {
		return port
	}
	return "8082"
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

func traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}

func storeResultOnce(ctx context.Context, rdb *redis.Client, key string, result flow.ResultEnvelope) (bool, error) {
	res, err := rdb.Eval(ctx, resultStoreScript, []string{key},
		result.Result,
		result.LatencyMs,
		result.ProcessedAt,
		result.WorkerID,
		result.Task.Category,
		result.Task.Score,
	).Result()
	if err != nil {
		return false, err
	}
	switch value := res.(type) {
	case int64:
		return value == 1, nil
	case bool:
		return value, nil
	default:
		return false, nil
	}
}

func handleProcessingFailure(ctx context.Context, busClient BusClient, msg *nats.Msg, task flow.TaskEnvelope, reason string, retry bool) {
	attempts := getDeliveryAttempt(msg)
	if !retry || attempts >= bus.MaxDeliver() {
		enqueueDeadLetter(ctx, busClient, task, reason, attempts)
		ackMessage("result-store", msg)
		return
	}
	nakMessage("result-store", msg)
}

func enqueueDeadLetter(ctx context.Context, busClient BusClient, task flow.TaskEnvelope, reason string, attempts int) {
	if attempts <= 0 {
		attempts = 1
	}
	entry := flow.DeadLetter{
		Task:      task,
		Reason:    reason,
		Attempts:  attempts,
		Source:    "result-store",
		Timestamp: flow.Now(),
	}
	if _, err := busClient.PublishJSON(ctx, bus.SubjectEventDeadLetter, entry, nil); err != nil {
		logger.WithContext(ctx).Error("result-store: deadletter publish failed", "error", err)
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
