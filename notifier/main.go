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

		statusKey := flow.StatusChannel(update.TaskID)
		if err := rdb.HSet(ctx, statusKey, map[string]interface{}{
			"status":     update.Status,
			"updated_at": update.Timestamp,
			"source":     update.Source,
		}).Err(); err != nil {
			log.Printf("notifier: status update failed: %v", err)
		}

		if err := rdb.Publish(ctx, statusKey, update.Status).Err(); err != nil {
			log.Printf("notifier: publish failed: %v", err)
		}
	}
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "redis-service:6379"
}
