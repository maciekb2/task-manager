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
	tracer := otel.Tracer("deadletter")

	for {
		items, err := rdb.BRPop(ctx, 0, flow.QueueDeadLetter).Result()
		if err != nil {
			log.Printf("deadletter: BRPOP failed: %v", err)
			continue
		}
		if len(items) < 2 {
			continue
		}

		var entry flow.DeadLetter
		if err := json.Unmarshal([]byte(items[1]), &entry); err != nil {
			log.Printf("deadletter: bad payload: %v", err)
			continue
		}

		parentCtx := contextFromTraceParent(entry.Task.TraceParent)
		ctxSpan, span := tracer.Start(parentCtx, "deadletter.persist")
		span.SetAttributes(
			attribute.String("task.id", entry.Task.TaskID),
			attribute.String("deadletter.reason", entry.Reason),
			attribute.String("deadletter.source", entry.Source),
			attribute.String("queue.name", flow.QueueDeadLetter),
		)

		payload, err := json.Marshal(entry)
		if err != nil {
			log.Printf("deadletter: marshal failed: %v", err)
			span.End()
			continue
		}

		if err := rdb.RPush(ctxSpan, "dead_letter", payload).Err(); err != nil {
			log.Printf("deadletter: persist failed: %v", err)
		}

		log.Printf("deadletter: %s %s", entry.Task.TaskID, entry.Reason)
		span.End()
	}
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "redis-service:6379"
}

func contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}
