package main

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/flow"
	"go.opentelemetry.io/otel"
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
	count := workerCount()
	log.Printf("worker: starting %d workers", count)

	for i := 0; i < count; i++ {
		workerID := i + 1
		go processLoop(ctx, rdb, workerID)
	}

	select {}
}

func processLoop(ctx context.Context, rdb *redis.Client, workerID int) {
	tracer := otel.Tracer("worker")
	for {
		tasks, err := rdb.ZPopMax(ctx, flow.SortedTasks, 1).Result()
		if err != nil {
			log.Printf("worker %d: ZPOPMAX failed: %v", workerID, err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(tasks) == 0 {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		payload, ok := tasks[0].Member.(string)
		if !ok {
			log.Printf("worker %d: unexpected payload type", workerID)
			continue
		}

		var task flow.TaskEnvelope
		if err := json.Unmarshal([]byte(payload), &task); err != nil {
			log.Printf("worker %d: bad payload: %v", workerID, err)
			continue
		}

		parentCtx := contextFromTraceParent(task.TraceParent)
		ctxTask, span := tracer.Start(parentCtx, "worker.process")

		enqueueStatus(ctx, rdb, task, "IN_PROGRESS", "worker")
		enqueueAudit(ctx, rdb, flow.AuditEvent{
			TaskID:      task.TaskID,
			Event:       "task.started",
			Detail:      "Worker picked task",
			TraceParent: task.TraceParent,
			Source:      "worker",
			Timestamp:   flow.Now(),
		})

		processDuration := time.Duration(2+rand.Intn(3)) * time.Second
		time.Sleep(processDuration)

		if rand.Float32() < 0.2 {
			log.Printf("worker %d: task %s failed", workerID, task.TaskID)
			enqueueStatus(ctx, rdb, task, "FAILED", "worker")
			enqueueDeadLetter(ctx, rdb, task, "processing failed")
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
		encoded, err := json.Marshal(resultPayload)
		if err != nil {
			log.Printf("worker %d: result serialize failed: %v", workerID, err)
			enqueueDeadLetter(ctx, rdb, task, "result serialize failed")
			span.End()
			continue
		}

		if err := rdb.RPush(ctx, flow.QueueResults, encoded).Err(); err != nil {
			log.Printf("worker %d: enqueue results failed: %v", workerID, err)
			enqueueDeadLetter(ctx, rdb, task, "enqueue results failed")
			span.End()
			continue
		}

		enqueueStatus(ctx, rdb, task, "COMPLETED", "worker")
		enqueueAudit(ctx, rdb, flow.AuditEvent{
			TaskID:      task.TaskID,
			Event:       "task.completed",
			Detail:      "Worker finished task",
			TraceParent: task.TraceParent,
			Source:      "worker",
			Timestamp:   flow.Now(),
		})

		span.End()
		_ = ctxTask
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
		log.Printf("worker: status marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueStatus, payload).Err(); err != nil {
		log.Printf("worker: status enqueue failed: %v", err)
	}
}

func enqueueAudit(ctx context.Context, rdb *redis.Client, event flow.AuditEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("worker: audit marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueAudit, payload).Err(); err != nil {
		log.Printf("worker: audit enqueue failed: %v", err)
	}
}

func enqueueDeadLetter(ctx context.Context, rdb *redis.Client, task flow.TaskEnvelope, reason string) {
	entry := flow.DeadLetter{
		Task:      task,
		Reason:    reason,
		Source:    "worker",
		Timestamp: flow.Now(),
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		log.Printf("worker: deadletter marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueDeadLetter, payload).Err(); err != nil {
		log.Printf("worker: deadletter enqueue failed: %v", err)
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

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "redis-service:6379"
}
