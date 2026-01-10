package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/flow"
)

func main() {
	ctx := context.Background()
	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})

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

		payload, err := json.Marshal(event)
		if err != nil {
			log.Printf("audit: marshal failed: %v", err)
			continue
		}

		if err := rdb.RPush(ctx, "audit_events", payload).Err(); err != nil {
			log.Printf("audit: persist failed: %v", err)
		}

		log.Printf("audit: %s %s", event.Event, event.TaskID)
	}
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "redis-service:6379"
}
