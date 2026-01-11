package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
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

type enrichResponse struct {
	Category string `json:"category"`
	Score    int32  `json:"score"`
}

var httpClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

func main() {
	ctx := context.Background()
	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "ingest"})
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
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "ingest"), bus.SubjectTaskIngest)
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, consumerCfg); err != nil {
		log.Fatalf("nats consumer setup failed: %v", err)
	}
	sub, err := busClient.PullSubscribe(bus.SubjectTaskIngest, consumerCfg.Durable)
	if err != nil {
		log.Fatalf("nats subscribe failed: %v", err)
	}

	tracer := otel.Tracer("ingest")
	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 5, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		var task flow.TaskEnvelope
		if err := json.Unmarshal(msg.Data, &task); err != nil {
			log.Printf("ingest: bad payload: %v", err)
			handleProcessingFailure(msgCtx, busClient, msg, flow.TaskEnvelope{}, "bad payload", false)
			return nil
		}

		if task.TraceParent == "" {
			task.TraceParent = traceparentFromContext(msgCtx)
		}

		parentCtx := msgCtx
		if !trace.SpanContextFromContext(parentCtx).IsValid() && task.TraceParent != "" {
			parentCtx = contextFromTraceParent(task.TraceParent)
		}
		ctxTask, span := tracer.Start(parentCtx, "ingest.process")
		bus.AnnotateSpan(span, msg)
		span.SetAttributes(
			attribute.String("task.id", task.TaskID),
			attribute.Int64("task.priority", int64(task.Priority)),
			attribute.Int64("task.number1", int64(task.Number1)),
			attribute.Int64("task.number2", int64(task.Number2)),
			attribute.String("queue.source", bus.SubjectTaskIngest),
			attribute.String("queue.target", bus.SubjectTaskSchedule),
		)

		if strings.TrimSpace(task.TaskDescription) == "" {
			span.SetAttributes(attribute.String("task.validation", "missing_description"))
			span.RecordError(errInvalidTask("missing description"))
			enqueueDeadLetter(ctxTask, busClient, task, "missing description", bus.DeliveryAttempt(msg))
			enqueueStatus(ctxTask, busClient, task, "REJECTED", "ingest")
			ackMessage("ingest", msg)
			span.End()
			return nil
		}

		enriched, err := enrichTask(ctxTask, task)
		if err != nil {
			log.Printf("ingest: enrich failed: %v", err)
			span.RecordError(err)
		} else {
			task = enriched
			span.SetAttributes(
				attribute.String("task.category", task.Category),
				attribute.Int64("task.score", int64(task.Score)),
			)
			enqueueStatus(ctxTask, busClient, task, "ENRICHED", "ingest")
		}

		task.Attempt++
		if _, err := busClient.PublishJSON(ctxTask, bus.SubjectTaskSchedule, task, nil); err != nil {
			log.Printf("ingest: publish schedule failed: %v", err)
			span.RecordError(err)
			span.End()
			handleProcessingFailure(msgCtx, busClient, msg, task, "publish schedule failed", true)
			return nil
		}

		enqueueStatus(ctxTask, busClient, task, "VALIDATED", "ingest")
		enqueueAudit(ctxTask, busClient, flow.AuditEvent{
			TaskID:      task.TaskID,
			Event:       "task.validated",
			Detail:      "Task validated and forwarded",
			TraceParent: task.TraceParent,
			Source:      "ingest",
			Timestamp:   flow.Now(),
		})

		span.End()
		ackMessage("ingest", msg)
		return nil
	}); err != nil {
		log.Fatalf("ingest consume failed: %v", err)
	}
}

func enrichTask(ctx context.Context, task flow.TaskEnvelope) (flow.TaskEnvelope, error) {
	payload, err := json.Marshal(task)
	if err != nil {
		return task, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, enricherURL(), bytes.NewReader(payload))
	if err != nil {
		return task, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return task, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return task, errInvalidTask("enricher rejected payload")
	}

	var result enrichResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return task, err
	}

	task.Category = result.Category
	task.Score = result.Score
	return task, nil
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
		log.Printf("ingest: status publish failed: %v", err)
	}
}

func enqueueAudit(ctx context.Context, busClient *bus.Client, event flow.AuditEvent) {
	if _, err := busClient.PublishJSON(ctx, bus.SubjectEventAudit, event, nil); err != nil {
		log.Printf("ingest: audit publish failed: %v", err)
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
		Source:    "ingest",
		Timestamp: flow.Now(),
	}
	if _, err := busClient.PublishJSON(ctx, bus.SubjectEventDeadLetter, entry, nil); err != nil {
		log.Printf("ingest: deadletter publish failed: %v", err)
	}
}

func contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
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

type errInvalidTask string

func (e errInvalidTask) Error() string { return string(e) }

func traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}

func handleProcessingFailure(ctx context.Context, busClient *bus.Client, msg *nats.Msg, task flow.TaskEnvelope, reason string, retry bool) {
	attempts := bus.DeliveryAttempt(msg)
	if !retry || attempts >= bus.MaxDeliver() {
		enqueueDeadLetter(ctx, busClient, task, reason, attempts)
		ackMessage("ingest", msg)
		return
	}
	nakMessage("ingest", msg)
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
