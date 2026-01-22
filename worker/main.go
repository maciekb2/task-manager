package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	ctx := context.Background()
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
		go processLoop(ctx, busClient, jobs, workerID, failRate)
	}

	select {}
}

type prioritySub struct {
	subject string
	sub     *nats.Subscription
}

func dispatchLoop(ctx context.Context, subs []prioritySub, out chan<- *nats.Msg) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		dispatched := false
		for _, entry := range subs {
			msg, err := fetchOne(entry.sub, 300*time.Millisecond)
			if err != nil {
				log.Printf("worker: fetch from %s failed: %v", entry.subject, err)
				time.Sleep(500 * time.Millisecond)
				dispatched = true
				break
			}
			if msg == nil {
				continue
			}
			out <- msg
			dispatched = true
			break
		}
		if !dispatched {
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func fetchOne(sub *nats.Subscription, wait time.Duration) (*nats.Msg, error) {
	msgs, err := sub.Fetch(1, nats.MaxWait(wait))
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			return nil, nil
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	return msgs[0], nil
}

func processLoop(ctx context.Context, busClient *bus.Client, jobs <-chan *nats.Msg, workerID int, failRate float64) {
	tracer := otel.Tracer("worker")
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-jobs:
			if !ok || msg == nil {
				continue
			}

			msgCtx := bus.ContextFromHeaders(ctx, msg.Header)
			var task flow.TaskEnvelope
			if err := json.Unmarshal(msg.Data, &task); err != nil {
				log.Printf("worker %d: bad payload: %v", workerID, err)
				handleProcessingFailure(msgCtx, busClient, msg, flow.TaskEnvelope{}, "bad payload", false)
				continue
			}
			if task.TraceParent == "" {
				task.TraceParent = traceparentFromContext(msgCtx)
			}

			parentCtx := msgCtx
			if !trace.SpanContextFromContext(parentCtx).IsValid() && task.TraceParent != "" {
				parentCtx = contextFromTraceParent(task.TraceParent)
			}
			ctxTask, span := tracer.Start(parentCtx, "worker.process")
			bus.AnnotateSpan(span, msg)
			span.SetAttributes(
				attribute.String("task.id", task.TaskID),
				attribute.Int64("task.priority", int64(task.Priority)),
				attribute.Int("worker.id", workerID),
				attribute.String("task.url", task.URL),
				attribute.String("task.method", task.Method),
				attribute.String("queue.source", msg.Subject),
				attribute.String("queue.target", bus.SubjectTaskResults),
			)

			enqueueStatus(ctxTask, busClient, task, "IN_PROGRESS", "worker")
			enqueueAudit(ctxTask, busClient, flow.AuditEvent{
				TaskID:      task.TaskID,
				Event:       "task.started",
				Detail:      "Worker picked task",
				TraceParent: task.TraceParent,
				Source:      "worker",
				Timestamp:   flow.Now(),
			})

			statusCode, latencyMs, checkErr := performHttpCheck(ctxTask, task.URL, task.Method)

			span.SetAttributes(
				attribute.Int("http.status_code", statusCode),
				attribute.Int64("http.latency_ms", latencyMs),
			)

			if checkErr != nil {
				log.Printf("worker %d: task %s http check failed: %v", workerID, task.TaskID, checkErr)
				span.RecordError(checkErr)
				// We don't necessarily fail the task processing in terms of NATS if the HTTP check fails (e.g. 404 or DNS error),
				// but we might want to mark the outcome as such.
				// However, if we want to simulate failure for the demo:
			}

			if rand.Float64() < failRate {
				log.Printf("worker %d: task %s failed (simulated)", workerID, task.TaskID)
				span.SetAttributes(attribute.String("task.outcome", "failed"))
				enqueueStatus(ctxTask, busClient, task, "FAILED", "worker")
				enqueueDeadLetter(ctxTask, busClient, task, "processing failed", bus.DeliveryAttempt(msg))
				ackMessage("worker", msg)
				recordMetrics(task.Priority, "failed", workerID, time.Since(start).Seconds())
				span.End()
				continue
			}

			resultPayload := flow.ResultEnvelope{
				Task:        task,
				Result:      int32(statusCode),
				LatencyMs:   latencyMs,
				ProcessedAt: flow.Now(),
				WorkerID:    workerID,
			}
			if _, err := busClient.PublishJSON(ctxTask, bus.SubjectTaskResults, resultPayload, nil); err != nil {
				log.Printf("worker %d: publish results failed: %v", workerID, err)
				span.RecordError(err)
				handleProcessingFailure(ctxTask, busClient, msg, task, "publish results failed", true)
				recordMetrics(task.Priority, "failed", workerID, time.Since(start).Seconds())
				span.End()
				continue
			}

			span.SetAttributes(
				attribute.String("task.outcome", "completed"),
				attribute.Int("task.result", statusCode),
			)
			enqueueStatus(ctxTask, busClient, task, "COMPLETED", "worker")
			enqueueAudit(ctxTask, busClient, flow.AuditEvent{
				TaskID:      task.TaskID,
				Event:       "task.completed",
				Detail:      "Worker finished task",
				TraceParent: task.TraceParent,
				Source:      "worker",
				Timestamp:   flow.Now(),
			})

			ackMessage("worker", msg)
			recordMetrics(task.Priority, "completed", workerID, time.Since(start).Seconds())
			span.End()
		}
	}
}

func performHttpCheck(ctx context.Context, url, method string) (int, int64, error) {
	if method == "" {
		method = "GET"
	}
	client := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   10 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, 0, err
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return 0, latency, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, latency, nil
}

func enqueueStatus(ctx context.Context, busClient *bus.Client, task flow.TaskEnvelope, status, source string) {
	update := flow.StatusUpdate{
		TaskID:      task.TaskID,
		Status:      status,
		TraceParent: task.TraceParent,
		Timestamp:   flow.Now(),
		Source:      source,
	}
	if _, err := busClient.PublishJSON(ctx, bus.SubjectEventStatus, update, nil); err != nil {
		log.Printf("worker: status publish failed: %v", err)
	}
}

func enqueueAudit(ctx context.Context, busClient *bus.Client, event flow.AuditEvent) {
	if _, err := busClient.PublishJSON(ctx, bus.SubjectEventAudit, event, nil); err != nil {
		log.Printf("worker: audit publish failed: %v", err)
	}
}

func enqueueDeadLetter(ctx context.Context, busClient *bus.Client, task flow.TaskEnvelope, reason string, attempts int) {
	if attempts <= 0 {
		attempts = 1
	}
	entry := flow.DeadLetter{
		Task:      task,
		Reason:    reason,
		Attempts:  attempts,
		Source:    "worker",
		Timestamp: flow.Now(),
	}
	if _, err := busClient.PublishJSON(ctx, bus.SubjectEventDeadLetter, entry, nil); err != nil {
		log.Printf("worker: deadletter publish failed: %v", err)
	}
}

func contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}

func workerCount() int {
	const defaultWorkers = 2
	value := os.Getenv("WORKER_COUNT")
	if value == "" {
		return defaultWorkers
	}
	count, err := strconv.Atoi(value)
	if err != nil || count < 1 {
		log.Printf("worker: invalid WORKER_COUNT=%q, using %d", value, defaultWorkers)
		return defaultWorkers
	}
	return count
}

func workerFailRate() float64 {
	const defaultRate = 0.05
	value := strings.TrimSpace(os.Getenv("WORKER_FAIL_RATE"))
	if value == "" {
		return defaultRate
	}
	rate, err := strconv.ParseFloat(value, 64)
	if err != nil || rate < 0 || rate > 1 {
		log.Printf("worker: invalid WORKER_FAIL_RATE=%q, using %.2f", value, defaultRate)
		return defaultRate
	}
	return rate
}

func natsURL() string {
	if addr := os.Getenv("NATS_URL"); addr != "" {
		return addr
	}
	return bus.DefaultURL
}

func handleProcessingFailure(ctx context.Context, busClient *bus.Client, msg *nats.Msg, task flow.TaskEnvelope, reason string, retry bool) {
	attempts := bus.DeliveryAttempt(msg)
	if !retry || attempts >= bus.MaxDeliver() {
		enqueueDeadLetter(ctx, busClient, task, reason, attempts)
		ackMessage("worker", msg)
		return
	}
	nakMessage("worker", msg)
}

func traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}

func ackMessage(service string, msg *nats.Msg) {
	if msg == nil {
		return
	}
	if err := msg.Ack(); err != nil {
		log.Printf("%s: ack failed: %v", service, err)
	}
}

func nakMessage(service string, msg *nats.Msg) {
	if msg == nil {
		return
	}
	if err := msg.Nak(); err != nil {
		log.Printf("%s: nak failed: %v", service, err)
	}
}
