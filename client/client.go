// Package main implements the gRPC client for interacting with the Task Manager server.
// It demonstrates how to submit tasks and stream their status updates.
package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	pb "github.com/maciekb2/task-manager/proto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"google.golang.org/grpc"
)

func main() {
	ctx := context.Background()
	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	conn := connectToServer(ctx)
	defer conn.Close()

	client := pb.NewTaskManagerClient(conn)

	rate := producerRate()
	concurrency := producerConcurrency()
	interval := time.Second
	if rate > 0 {
		interval = time.Duration(float64(time.Second) / rate)
		if interval <= 0 {
			interval = time.Second
		}
	}

	jobs := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		workerID := i + 1
		go producerLoop(client, jobs, workerID)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		jobs <- struct{}{}
	}
}

func connectToServer(ctx context.Context) *grpc.ClientConn {
	target := serverAddr()
	backoff := 500 * time.Millisecond
	maxBackoff := 15 * time.Second
	for {
		conn, err := grpc.DialContext(ctx, target, append(dialOpts(), grpc.WithBlock())...)
		if err == nil {
			return conn
		}
		log.Printf("did not connect to %s: %v (retrying in %s)", target, err, backoff)
		time.Sleep(backoff)
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func producerLoop(client pb.TaskManagerClient, jobs <-chan struct{}, workerID int) {
	for range jobs {
		number1 := generateNumber1()
		number2 := generateNumber2()
		description := randomTaskDescription()
		priority := randomPriority()
		sendTaskWithNumbers(client, workerID, description, priority, number1, number2)
	}
}

func sendTaskWithNumbers(client pb.TaskManagerClient, workerID int, description string, priority pb.TaskPriority, number1 int, number2 int) {
	tracer := otel.Tracer("client")
	ctx, span := tracer.Start(context.Background(), "client.submit")
	idempotencyKey := generateIdempotencyKey()
	span.SetAttributes(
		attribute.String("task.description", description),
		attribute.Int("task.priority", int(priority)),
		attribute.Int("task.number1", number1),
		attribute.Int("task.number2", number2),
		attribute.String("task.idempotency_key", idempotencyKey),
		attribute.Int("producer.worker_id", workerID),
	)

	taskCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	defer span.End()

	log.Printf("Wysyłanie zadania: %s z priorytetem: %s oraz liczbami: %d, %d (idempotency: %s, worker: %d)", description, priority, number1, number2, idempotencyKey, workerID)

	res, err := client.SubmitTask(taskCtx, &pb.TaskRequest{
		TaskDescription: description,
		Priority:        priority,
		Number1:         int32(number1),
		Number2:         int32(number2),
		IdempotencyKey:  idempotencyKey,
	})
	if err != nil {
		span.RecordError(err)
		log.Printf("could not submit task: %v", err)
		return
	}

	log.Printf("Zadanie zostało wysłane z ID: %s (worker: %d)", res.TaskId, workerID)
	streamTaskStatus(client, res.TaskId)
}

func streamTaskStatus(client pb.TaskManagerClient, taskID string) {
	statusCtx, cancelStatus := context.WithCancel(context.Background())
	defer cancelStatus()

	stream, err := client.StreamTaskStatus(statusCtx, &pb.StatusRequest{TaskId: taskID})
	if err != nil {
		log.Printf("could not get status stream: %v", err)
		return
	}

	log.Println("Oczekiwanie na statusy...")
	for {
		status, err := stream.Recv()
		if err != nil {
			log.Printf("Stream zakończony: %v", err)
			break
		}
		log.Printf("Status zadania [%s]: %s", taskID, status.Status)
		if status.Status == "COMPLETED" || status.Status == "FAILED" {
			break
		}
	}
}

func serverAddr() string {
	if value := os.Getenv("TASKMANAGER_ADDR"); value != "" {
		return value
	}
	return "taskmanager-service:50051"
}

func randomTaskDescription() string {
	descriptions := []string{"Task A", "Task B", "Task C", "Critical Fix", "Routine Check"}
	return descriptions[rand.Intn(len(descriptions))]
}

func randomPriority() pb.TaskPriority {
	random := rand.Intn(3)
	switch random {
	case 0:
		return pb.TaskPriority_LOW
	case 1:
		return pb.TaskPriority_MEDIUM
	default:
		return pb.TaskPriority_HIGH
	}
}

func generateNumber1() int {
	return rand.Intn(8096-1024) + 1024
}

func generateNumber2() int {
	return rand.Intn(8096-1024) + 1024
}

func generateIdempotencyKey() string {
	return uuid.NewString()
}

func producerRate() float64 {
	value := os.Getenv("PRODUCER_RATE")
	if value == "" {
		return 1
	}
	rate, err := strconv.ParseFloat(value, 64)
	if err != nil || rate <= 0 {
		log.Printf("invalid PRODUCER_RATE=%q, using 1", value)
		return 1
	}
	return rate
}

func producerConcurrency() int {
	value := os.Getenv("PRODUCER_CONCURRENCY")
	if value == "" {
		return 1
	}
	count, err := strconv.Atoi(value)
	if err != nil || count < 1 {
		log.Printf("invalid PRODUCER_CONCURRENCY=%q, using 1", value)
		return 1
	}
	return count
}
