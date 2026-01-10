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

		payload, err := json.Marshal(entry)
		if err != nil {
			log.Printf("deadletter: marshal failed: %v", err)
			continue
		}

		if err := rdb.RPush(ctx, "dead_letter", payload).Err(); err != nil {
			log.Printf("deadletter: persist failed: %v", err)
		}

		log.Printf("deadletter: %s %s", entry.Task.TaskID, entry.Reason)
	}
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "redis-service:6379"
}
