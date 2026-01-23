package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
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
	batchSize := workerBatchSize()
	failRate := workerFailRate()
	log.Printf("worker: starting %d workers (batch size %d, fail rate %.2f)", count, batchSize, failRate)
	jobs := make(chan *nats.Msg, count*2)
	go dispatchLoop(ctx, []prioritySub{
		{subject: bus.SubjectTaskWorkerHigh, sub: highSub},
		{subject: bus.SubjectTaskWorkerMedium, sub: mediumSub},
		{subject: bus.SubjectTaskWorkerLow, sub: lowSub},
	}, jobs, batchSize)

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

func dispatchLoop(ctx context.Context, subs []prioritySub, out chan<- *nats.Msg, batchSize int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		dispatched := false
		for _, entry := range subs {
			msgs, err := fetchBatch(entry.sub, batchSize, 300*time.Millisecond)
			if err != nil {
				log.Printf("worker: fetch from %s failed: %v", entry.subject, err)
				time.Sleep(500 * time.Millisecond)
				dispatched = true
				break
			}
			if len(msgs) == 0 {
				continue
			}
			for _, msg := range msgs {
				out <- msg
			}
			dispatched = true
			break
		}
		if !dispatched {
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func fetchBatch(sub *nats.Subscription, batchSize int, wait time.Duration) ([]*nats.Msg, error) {
	msgs, err := sub.Fetch(batchSize, nats.MaxWait(wait))
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			return nil, nil
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, err
	}
	return msgs, nil
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
			start := time.Now()
			span.SetAttributes(
				attribute.String("task.id", task.TaskID),
				attribute.Int64("task.priority", int64(task.Priority)),
				attribute.Int("worker.id", workerID),
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

			checksum, iterations := simulateWork(task.Number1, task.Number2, task.Priority)
			ioDelay := time.Duration(200+rand.Intn(400)) * time.Millisecond
			time.Sleep(ioDelay)
			span.SetAttributes(
				attribute.Int("task.work.iterations", iterations),
				attribute.Int64("task.work.checksum", checksum),
				attribute.Float64("task.processing_seconds", time.Since(start).Seconds()),
				attribute.Float64("task.simulated_io_ms", float64(ioDelay.Milliseconds())),
			)

			if rand.Float64() < failRate {
				log.Printf("worker %d: task %s failed", workerID, task.TaskID)
				span.SetAttributes(attribute.String("task.outcome", "failed"))
				enqueueStatus(ctxTask, busClient, task, "FAILED", "worker")
				enqueueDeadLetter(ctxTask, busClient, task, "processing failed", bus.DeliveryAttempt(msg))
				ackMessage("worker", msg)
				span.End()
				continue
			}

			result := int32(task.Number1 + task.Number2)
			resultPayload := flow.ResultEnvelope{
				Task:        task,
				Result:      result,
				ProcessedAt: flow.Now(),
				WorkerID:    workerID,
			}
			if _, err := busClient.PublishJSON(ctxTask, bus.SubjectTaskResults, resultPayload, nil); err != nil {
				log.Printf("worker %d: publish results failed: %v", workerID, err)
				span.RecordError(err)
				handleProcessingFailure(ctxTask, busClient, msg, task, "publish results failed", true)
				span.End()
				continue
			}

			span.SetAttributes(
				attribute.String("task.outcome", "completed"),
				attribute.Int64("task.result", int64(result)),
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
			span.End()
		}
	}
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

func workerBatchSize() int {
	const defaultBatchSize = 10
	value := os.Getenv("WORKER_BATCH_SIZE")
	if value == "" {
		return defaultBatchSize
	}
	batchSize, err := strconv.Atoi(value)
	if err != nil || batchSize < 1 {
		log.Printf("worker: invalid WORKER_BATCH_SIZE=%q, using %d", value, defaultBatchSize)
		return defaultBatchSize
	}
	return batchSize
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

func simulateWork(number1, number2 int32, priority int32) (int64, int) {
	base := int(number1+number2) % 5000
	iterations := 5000 + base + int(priority)*1000
	var hash uint64 = 1469598103934665603
	for i := 0; i < iterations; i++ {
		hash ^= uint64(number1) + uint64(number2) + uint64(i)
		hash *= 1099511628211
		hash ^= hash >> 32
	}
	return int64(hash & 0x7fffffffffffffff), iterations
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
