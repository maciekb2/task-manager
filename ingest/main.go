package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/flow"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
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

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	tracer := otel.Tracer("ingest")

	for {
		items, err := rdb.BRPop(ctx, 0, flow.QueueIngest).Result()
		if err != nil {
			log.Printf("ingest: BRPOP failed: %v", err)
			continue
		}
		if len(items) < 2 {
			continue
		}

		var task flow.TaskEnvelope
		if err := json.Unmarshal([]byte(items[1]), &task); err != nil {
			log.Printf("ingest: bad payload: %v", err)
			continue
		}

		parentCtx := contextFromTraceParent(task.TraceParent)
		ctxTask, span := tracer.Start(parentCtx, "ingest.process")
		span.SetAttributes(
			attribute.String("task.id", task.TaskID),
			attribute.Int64("task.priority", int64(task.Priority)),
			attribute.Int64("task.number1", int64(task.Number1)),
			attribute.Int64("task.number2", int64(task.Number2)),
			attribute.String("queue.name", flow.QueueIngest),
		)

		if strings.TrimSpace(task.TaskDescription) == "" {
			span.SetAttributes(attribute.String("task.validation", "missing_description"))
			span.RecordError(errInvalidTask("missing description"))
			enqueueDeadLetter(ctxTask, rdb, task, "missing description")
			enqueueStatus(ctxTask, rdb, task, "REJECTED", "ingest")
			span.End()
			continue
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
			enqueueStatus(ctxTask, rdb, task, "ENRICHED", "ingest")
		}

		task.Attempt++
		payload, err := json.Marshal(task)
		if err != nil {
			log.Printf("ingest: serialize failed: %v", err)
			span.End()
			continue
		}

		if err := rdb.RPush(ctxTask, flow.QueueSchedule, payload).Err(); err != nil {
			log.Printf("ingest: enqueue failed: %v", err)
			enqueueDeadLetter(ctxTask, rdb, task, "enqueue schedule failed")
			span.End()
			continue
		}

		enqueueStatus(ctxTask, rdb, task, "VALIDATED", "ingest")
		enqueueAudit(ctxTask, rdb, flow.AuditEvent{
			TaskID:      task.TaskID,
			Event:       "task.validated",
			Detail:      "Task validated and forwarded",
			TraceParent: task.TraceParent,
			Source:      "ingest",
			Timestamp:   flow.Now(),
		})

		span.End()
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
		log.Printf("ingest: status marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueStatus, payload).Err(); err != nil {
		log.Printf("ingest: status enqueue failed: %v", err)
	}
}

func enqueueAudit(ctx context.Context, rdb *redis.Client, event flow.AuditEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("ingest: audit marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueAudit, payload).Err(); err != nil {
		log.Printf("ingest: audit enqueue failed: %v", err)
	}
}

func enqueueDeadLetter(ctx context.Context, rdb *redis.Client, task flow.TaskEnvelope, reason string) {
	entry := flow.DeadLetter{
		Task:      task,
		Reason:    reason,
		Source:    "ingest",
		Timestamp: flow.Now(),
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		log.Printf("ingest: deadletter marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueDeadLetter, payload).Err(); err != nil {
		log.Printf("ingest: deadletter enqueue failed: %v", err)
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

func enricherURL() string {
	if url := os.Getenv("ENRICHER_URL"); url != "" {
		return url
	}
	return "http://enricher:8080/enrich"
}

type errInvalidTask string

func (e errInvalidTask) Error() string { return string(e) }
