package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/flow"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

func main() {
	ctx := context.Background()
	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	tracer := otel.Tracer("scheduler")

	for {
		items, err := rdb.BRPop(ctx, 0, flow.QueueSchedule).Result()
		if err != nil {
			log.Printf("scheduler: BRPOP failed: %v", err)
			continue
		}
		if len(items) < 2 {
			continue
		}

		var task flow.TaskEnvelope
		if err := json.Unmarshal([]byte(items[1]), &task); err != nil {
			log.Printf("scheduler: bad payload: %v", err)
			continue
		}

		parentCtx := contextFromTraceParent(task.TraceParent)
		ctxTask, span := tracer.Start(parentCtx, "scheduler.schedule")
		queueName := flow.WorkerQueueForPriority(task.Priority)
		span.SetAttributes(
			attribute.String("task.id", task.TaskID),
			attribute.Int64("task.priority", int64(task.Priority)),
			attribute.String("queue.source", flow.QueueSchedule),
			attribute.String("queue.target", queueName),
		)

		payload, err := json.Marshal(task)
		if err != nil {
			log.Printf("scheduler: serialize failed: %v", err)
			span.End()
			continue
		}

		if err := rdb.RPush(ctxTask, queueName, payload).Err(); err != nil {
			log.Printf("scheduler: enqueue failed: %v", err)
			enqueueDeadLetter(ctxTask, rdb, task, "enqueue worker queue failed")
			span.End()
			continue
		}

		enqueueStatus(ctxTask, rdb, task, "SCHEDULED", "scheduler")
		enqueueAudit(ctxTask, rdb, flow.AuditEvent{
			TaskID:      task.TaskID,
			Event:       "task.scheduled",
			Detail:      "Task added to priority queue",
			TraceParent: task.TraceParent,
			Source:      "scheduler",
			Timestamp:   flow.Now(),
		})

		span.End()
	}
}

func enqueueStatus(ctx context.Context, rdb *redis.Client, task flow.TaskEnvelope, status, source string) {
	update := flow.StatusUpdate{
		TaskID:      task.TaskID,
		Status:      status,
		TraceParent: task.TraceParent,
		Timestamp:   flow.Now(),
		Source:      source,
	}
	payload, err := json.Marshal(update)
	if err != nil {
		log.Printf("scheduler: status marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueStatus, payload).Err(); err != nil {
		log.Printf("scheduler: status enqueue failed: %v", err)
	}
}

func enqueueAudit(ctx context.Context, rdb *redis.Client, event flow.AuditEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("scheduler: audit marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueAudit, payload).Err(); err != nil {
		log.Printf("scheduler: audit enqueue failed: %v", err)
	}
}

func enqueueDeadLetter(ctx context.Context, rdb *redis.Client, task flow.TaskEnvelope, reason string) {
	entry := flow.DeadLetter{
		Task:      task,
		Reason:    reason,
		Source:    "scheduler",
		Timestamp: flow.Now(),
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		log.Printf("scheduler: deadletter marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueDeadLetter, payload).Err(); err != nil {
		log.Printf("scheduler: deadletter enqueue failed: %v", err)
	}
}

func contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "redis-service:6379"
}
