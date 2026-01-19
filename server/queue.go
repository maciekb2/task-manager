package main

import (
	"context"
	"sync"

	"github.com/go-redis/redis/v8"
)

type TaskQueue interface {
	Push(ctx context.Context, taskID string, priority int) error
	Pop(ctx context.Context) (string, error)
}

type RedisQueue struct {
	client *redis.Client
}

func NewRedisQueue(client *redis.Client) *RedisQueue {
	return &RedisQueue{client: client}
}

func (q *RedisQueue) Push(ctx context.Context, taskID string, priority int) error {
	return q.client.ZAdd(ctx, "tasks", &redis.Z{
		Score:  float64(priority),
		Member: taskID,
	}).Err()
}

func (q *RedisQueue) Pop(ctx context.Context) (string, error) {
	result, err := q.client.BZPopMax(ctx, 0, "tasks").Result()
	if err != nil {
		return "", err
	}
	return result.Member.(string), nil
}

// MemoryQueue for testing
type MemoryQueue struct {
	ch chan string
	mu sync.Mutex
}

func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		ch: make(chan string, 100),
	}
}

func (q *MemoryQueue) Push(ctx context.Context, taskID string, priority int) error {
	q.ch <- taskID
	return nil
}

func (q *MemoryQueue) Pop(ctx context.Context) (string, error) {
	select {
	case taskID := <-q.ch:
		return taskID, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
