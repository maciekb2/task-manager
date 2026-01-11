package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
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

	busClient, err := bus.Connect(bus.Config{URL: natsURL(), Name: "scheduler"})
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
	consumerCfg := bus.DefaultConsumerConfig(bus.DurableName(bus.StreamTasks, "scheduler"), bus.SubjectTaskSchedule)
	if _, err := busClient.EnsureConsumer(bus.StreamTasks, consumerCfg); err != nil {
		log.Fatalf("nats consumer setup failed: %v", err)
	}
	sub, err := busClient.PullSubscribe(bus.SubjectTaskSchedule, consumerCfg.Durable)
	if err != nil {
		log.Fatalf("nats subscribe failed: %v", err)
	}
	tracer := otel.Tracer("scheduler")

	if err := busClient.Consume(ctx, sub, bus.ConsumeOptions{Batch: 5, MaxWait: 5 * time.Second, DisableAutoAck: true}, func(msgCtx context.Context, msg *nats.Msg) error {
		var task flow.TaskEnvelope
		if err := json.Unmarshal(msg.Data, &task); err != nil {
			log.Printf("scheduler: bad payload: %v", err)
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
		ctxTask, span := tracer.Start(parentCtx, "scheduler.schedule")
		bus.AnnotateSpan(span, msg)
		queueName := bus.WorkerSubjectForPriority(task.Priority)
		span.SetAttributes(
			attribute.String("task.id", task.TaskID),
			attribute.Int64("task.priority", int64(task.Priority)),
			attribute.String("queue.source", bus.SubjectTaskSchedule),
			attribute.String("queue.target", queueName),
		)

		if _, err := busClient.PublishJSON(ctxTask, queueName, task, nil); err != nil {
			log.Printf("scheduler: publish worker failed: %v", err)
			span.RecordError(err)
			span.End()
			handleProcessingFailure(msgCtx, busClient, msg, task, "publish worker failed", true)
			return nil
		}

		enqueueStatus(ctxTask, busClient, task, "SCHEDULED", "scheduler")
		enqueueAudit(ctxTask, busClient, flow.AuditEvent{
			TaskID:      task.TaskID,
			Event:       "task.scheduled",
			Detail:      "Task added to priority queue",
			TraceParent: task.TraceParent,
			Source:      "scheduler",
			Timestamp:   flow.Now(),
		})

		span.End()
		ackMessage("scheduler", msg)
		return nil
	}); err != nil {
		log.Fatalf("scheduler consume failed: %v", err)
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
		log.Printf("scheduler: status publish failed: %v", err)
	}
}

func enqueueAudit(ctx context.Context, busClient *bus.Client, event flow.AuditEvent) {
	if _, err := busClient.PublishJSON(ctx, bus.SubjectEventAudit, event, nil); err != nil {
		log.Printf("scheduler: audit publish failed: %v", err)
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
		Source:    "scheduler",
		Timestamp: flow.Now(),
	}
	if _, err := busClient.PublishJSON(ctx, bus.SubjectEventDeadLetter, entry, nil); err != nil {
		log.Printf("scheduler: deadletter publish failed: %v", err)
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

func traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}

func handleProcessingFailure(ctx context.Context, busClient *bus.Client, msg *nats.Msg, task flow.TaskEnvelope, reason string, retry bool) {
	attempts := bus.DeliveryAttempt(msg)
	if !retry || attempts >= bus.MaxDeliver() {
		enqueueDeadLetter(ctx, busClient, task, reason, attempts)
		ackMessage("scheduler", msg)
		return
	}
	nakMessage("scheduler", msg)
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
