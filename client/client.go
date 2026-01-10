// Package main implements the gRPC client for interacting with the Task Manager server.
// It demonstrates how to submit tasks and stream their status updates.
package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"time"

	pb "github.com/maciekb2/task-manager/proto"

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

	// Generowanie i wysyłanie losowych zadań
	for {
		number1 := generateNumber1()
		number2 := generateNumber2()
		description := randomTaskDescription()
		priority := randomPriority()
		sendTaskWithNumbers(client, description, priority, number1, number2)
		// Opóźnienie między wysyłaniem kolejnych zadań
		time.Sleep(1 * time.Second)
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

func sendTaskWithNumbers(client pb.TaskManagerClient, description string, priority pb.TaskPriority, number1 int, number2 int) {
	taskCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log.Printf("Wysyłanie zadania: %s z priorytetem: %s oraz liczbami: %d, %d", description, priority, number1, number2)

	res, err := client.SubmitTask(taskCtx, &pb.TaskRequest{
		TaskDescription: description,
		Priority:        priority,
		Number1:         int32(number1),
		Number2:         int32(number2),
	})
	if err != nil {
		log.Printf("could not submit task: %v", err)
		return
	}

	log.Printf("Zadanie zostało wysłane z ID: %s", res.TaskId)
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
