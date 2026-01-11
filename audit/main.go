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
	tracer := otel.Tracer("audit")

	for {
		items, err := rdb.BRPop(ctx, 0, flow.QueueAudit).Result()
		if err != nil {
			log.Printf("audit: BRPOP failed: %v", err)
			continue
		}
		if len(items) < 2 {
			continue
		}

		var event flow.AuditEvent
		if err := json.Unmarshal([]byte(items[1]), &event); err != nil {
			log.Printf("audit: bad payload: %v", err)
			continue
		}

		parentCtx := contextFromTraceParent(event.TraceParent)
		ctxSpan, span := tracer.Start(parentCtx, "audit.persist")
		span.SetAttributes(
			attribute.String("task.id", event.TaskID),
			attribute.String("audit.event", event.Event),
			attribute.String("audit.source", event.Source),
			attribute.String("queue.name", flow.QueueAudit),
		)

		payload, err := json.Marshal(event)
		if err != nil {
			log.Printf("audit: marshal failed: %v", err)
			span.End()
			continue
		}

		if err := rdb.RPush(ctxSpan, "audit_events", payload).Err(); err != nil {
			log.Printf("audit: persist failed: %v", err)
		}

		log.Printf("audit: %s %s", event.Event, event.TaskID)
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
