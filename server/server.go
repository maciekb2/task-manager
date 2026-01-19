// Package main implements the gRPC server for the Task Manager application.
// This server handles task submission, status monitoring, and task processing.

package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	pb "github.com/maciekb2/task-manager/proto"

	"google.golang.org/grpc"
)

type task struct {
	id          string
	description string
	priority    pb.TaskPriority
	status      string
}

type server struct {
	pb.UnimplementedTaskManagerServer
	rdb         *redis.Client
	mu          sync.Mutex
	subscribers map[string]chan string
}

func newServer(redisAddr string) *server {
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	return &server{
		rdb:         rdb,
		subscribers: make(map[string]chan string),
	}
}

func (s *server) SubmitTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error) {
	taskID := uuid.NewString()
	newTask := &task{
		id:          taskID,
		description: req.TaskDescription,
		priority:    req.Priority,
		status:      "QUEUED",
	}

	// Store the new task in Redis
	if err := s.rdb.HSet(ctx, "task:"+taskID, map[string]interface{}{
		"id":          newTask.id,
		"description": newTask.description,
		"priority":    int32(newTask.priority),
		"status":      newTask.status,
	}).Err(); err != nil {
		return nil, fmt.Errorf("could not store task: %v", err)
	}

	s.mu.Lock()
	if _, exists := s.subscribers[taskID]; !exists {
		s.subscribers[taskID] = make(chan string, 10)
	}
	s.mu.Unlock()

	// Dodawanie zadania do kolejki Redis
	if err := s.rdb.ZAdd(ctx, "tasks", &redis.Z{
		Score:  float64(req.Priority), // Priorytet jako score
		Member: taskID,
	}).Err(); err != nil {
		return nil, fmt.Errorf("could not add task to queue: %v", err)
	}

	return &pb.TaskResponse{TaskId: taskID}, nil
}

func (s *server) CheckTaskStatus(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	// Pobieranie statusu z Redisa
	status, err := s.rdb.HGet(ctx, "task_status:"+req.TaskId, "status").Result()
	if err != nil {
		return &pb.StatusResponse{Status: "UNKNOWN TASK"}, nil
	}

	return &pb.StatusResponse{Status: status}, nil
}

func (s *server) StartWorkers(ctx context.Context, count int) {
	for i := 0; i < count; i++ {
		go s.processTasks(ctx)
	}
}

func (s *server) StreamTaskStatus(req *pb.StatusRequest, stream pb.TaskManager_StreamTaskStatusServer) error {
	s.mu.Lock()
	ch, exists := s.subscribers[req.TaskId]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("task not found")
	}

	for status := range ch {
		if err := stream.Send(&pb.StatusResponse{Status: status}); err != nil {
			return err
		}
		if status == "COMPLETED" || status == "FAILED" {
			break
		}
	}
	return nil
}

func (s *server) processTasks(ctx context.Context) {
	for {
		// Pobieranie zadania o najwyższym priorytecie z kolejki Redis
		tasks, err := s.rdb.ZPopMax(ctx, "tasks", 1).Result()
		if err != nil {
			log.Printf("Could not get task from queue: %v", err)
			time.Sleep(1 * time.Second) // Odczekanie przed ponowną próbą
			continue
		}

		if len(tasks) == 0 {
			time.Sleep(1 * time.Second) // Odczekanie, jeśli kolejka jest pusta
			continue
		}

		taskID := tasks[0].Member.(string)

		// Aktualizacja statusu na IN_PROGRESS
		s.updateTaskStatus(ctx, taskID, "IN_PROGRESS")
		time.Sleep(5 * time.Second) // Symulacja przetwarzania

		// Symulacja sukcesu lub porażki
		if rand.Float32() < 0.8 {
			s.updateTaskStatus(ctx, taskID, "COMPLETED")
		} else {
			s.updateTaskStatus(ctx, taskID, "FAILED")
		}
	}
}

func (s *server) updateTaskStatus(ctx context.Context, taskID, status string) {
	s.mu.Lock()
	if ch, ok := s.subscribers[taskID]; ok {
		ch <- status
	}
	s.mu.Unlock()

	// Aktualizacja statusu w Redisie
	if err := s.rdb.HSet(ctx, "task_status:"+taskID, "status", status).Err(); err != nil {
		log.Printf("Could not update task status: %v", err)
	}
}

func main() {
	ctx := context.Background()
	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(serverOpts()...)
	srv := newServer("redis-service:6379")

	workerCount := 5
	if countStr := os.Getenv("WORKER_COUNT"); countStr != "" {
		if count, err := strconv.Atoi(countStr); err == nil && count > 0 {
			workerCount = count
		}
	}
	srv.StartWorkers(ctx, workerCount)
	pb.RegisterTaskManagerServer(grpcServer, srv)

	log.Println("Serwer gRPC działa na porcie :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
