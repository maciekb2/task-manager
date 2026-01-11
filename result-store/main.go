package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

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
	go startHTTPServer(rdb)

	tracer := otel.Tracer("result-store")
	for {
		items, err := rdb.BRPop(ctx, 0, flow.QueueResults).Result()
		if err != nil {
			log.Printf("result-store: BRPOP failed: %v", err)
			continue
		}
		if len(items) < 2 {
			continue
		}

		var result flow.ResultEnvelope
		if err := json.Unmarshal([]byte(items[1]), &result); err != nil {
			log.Printf("result-store: bad payload: %v", err)
			continue
		}

		parentCtx := contextFromTraceParent(result.Task.TraceParent)
		ctxTask, span := tracer.Start(parentCtx, "result-store.persist")
		span.SetAttributes(
			attribute.String("task.id", result.Task.TaskID),
			attribute.Int("worker.id", result.WorkerID),
			attribute.Int64("task.result", int64(result.Result)),
			attribute.String("queue.source", flow.QueueResults),
		)

		key := "task_result:" + result.Task.TaskID
		span.SetAttributes(attribute.String("redis.key", key))
		if err := rdb.HSet(ctxTask, key, map[string]interface{}{
			"result":       result.Result,
			"processed_at": result.ProcessedAt,
			"worker_id":    result.WorkerID,
			"category":     result.Task.Category,
			"score":        result.Task.Score,
		}).Err(); err != nil {
			log.Printf("result-store: persist failed: %v", err)
			span.End()
			continue
		}

		enqueueAudit(ctxTask, rdb, flow.AuditEvent{
			TaskID:      result.Task.TaskID,
			Event:       "task.stored",
			Detail:      "Result persisted",
			TraceParent: result.Task.TraceParent,
			Source:      "result-store",
			Timestamp:   flow.Now(),
		})

		span.End()
	}
}

func startHTTPServer(rdb *redis.Client) {
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
	log.Printf("result-store http listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("result-store http failed: %v", err)
	}
}

func enqueueAudit(ctx context.Context, rdb *redis.Client, event flow.AuditEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("result-store: audit marshal failed: %v", err)
		return
	}
	if err := rdb.RPush(ctx, flow.QueueAudit, payload).Err(); err != nil {
		log.Printf("result-store: audit enqueue failed: %v", err)
	}
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
