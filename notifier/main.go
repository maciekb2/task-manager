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
	tracer := otel.Tracer("notifier")

	for {
		items, err := rdb.BRPop(ctx, 0, flow.QueueStatus).Result()
		if err != nil {
			log.Printf("notifier: BRPOP failed: %v", err)
			continue
		}
		if len(items) < 2 {
			continue
		}

		var update flow.StatusUpdate
		if err := json.Unmarshal([]byte(items[1]), &update); err != nil {
			log.Printf("notifier: bad payload: %v", err)
			continue
		}

		parentCtx := contextFromTraceParent(update.TraceParent)
		ctxSpan, span := tracer.Start(parentCtx, "notifier.update")
		span.SetAttributes(
			attribute.String("task.id", update.TaskID),
			attribute.String("task.status", update.Status),
			attribute.String("status.source", update.Source),
			attribute.String("queue.name", flow.QueueStatus),
		)

		statusKey := flow.StatusChannel(update.TaskID)
		if err := rdb.HSet(ctxSpan, statusKey, map[string]interface{}{
			"status":     update.Status,
			"updated_at": update.Timestamp,
			"source":     update.Source,
		}).Err(); err != nil {
			log.Printf("notifier: status update failed: %v", err)
		}

		if err := rdb.Publish(ctxSpan, statusKey, update.Status).Err(); err != nil {
			log.Printf("notifier: publish failed: %v", err)
		}
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
