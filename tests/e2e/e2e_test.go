package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	pb "github.com/maciekb2/task-manager/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestSubmitAndVerifyTask(t *testing.T) {
	addr := os.Getenv("TASKMANAGER_ADDR")
	if addr == "" {
		addr = "envoy:50051"
	}

	log.Printf("Attempting to connect to %s", addr)

	var conn *grpc.ClientConn
	var err error

	// NewClient returns immediately. We rely on the retry loop in SubmitTask to handle initial connection delays.
	conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer conn.Close()

	client := pb.NewTaskManagerClient(conn)

	// Submit Task
	log.Println("Submitting task...")
	var res *pb.TaskResponse

	// Retry loop for submission (service might be starting up)
	for i := 0; i < 30; i++ {
		subCtx, subCancel := context.WithTimeout(context.Background(), 5*time.Second)
		res, err = client.SubmitTask(subCtx, &pb.TaskRequest{
			TaskDescription: "E2E Test Task",
			Priority:        pb.TaskPriority_HIGH,
			Url:             "https://example.com",
			Method:          "GET",
			IdempotencyKey:  fmt.Sprintf("e2e-%d", time.Now().UnixNano()),
		})
		subCancel()
		if err == nil {
			break
		}
		log.Printf("SubmitTask failed: %v. Retrying...", err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		t.Fatalf("Failed to submit task after retries: %v", err)
	}

	log.Printf("Task submitted. ID: %s", res.TaskId)

	// Stream Status
	streamCtx, streamCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer streamCancel()

	stream, err := client.StreamTaskStatus(streamCtx, &pb.StatusRequest{TaskId: res.TaskId})
	if err != nil {
		t.Fatalf("Failed to start status stream: %v", err)
	}

	for {
		status, err := stream.Recv()
		if err != nil {
			t.Fatalf("Error receiving status: %v", err)
		}
		log.Printf("Task Status: %s", status.Status)
		if status.Status == "COMPLETED" || status.Status == "FAILED" {
			log.Printf("Task finished with status: %s", status.Status)
			break
		}
	}
}
