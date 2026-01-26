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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func getClient(t *testing.T) (*grpc.ClientConn, pb.TaskManagerClient) {
	addr := os.Getenv("TASKMANAGER_ADDR")
	if addr == "" {
		addr = "envoy:50051"
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	return conn, pb.NewTaskManagerClient(conn)
}

func waitForService(t *testing.T, client pb.TaskManagerClient) {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := client.SubmitTask(ctx, &pb.TaskRequest{
			TaskDescription: "Health Check",
			Priority:        pb.TaskPriority_LOW,
			Url:             "http://example.com",
			Method:          "GET",
			IdempotencyKey:  fmt.Sprintf("health-%d", time.Now().UnixNano()),
		})
		cancel()
		if err == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("Service did not become ready in time")
}

func TestE2E(t *testing.T) {
	conn, client := getClient(t)
	defer conn.Close()

	log.Println("Waiting for service to be ready...")
	waitForService(t, client)
	log.Println("Service is ready.")

	t.Run("SubmitAndVerifyTask", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		res, err := client.SubmitTask(ctx, &pb.TaskRequest{
			TaskDescription: "E2E Test Task",
			Priority:        pb.TaskPriority_HIGH,
			Url:             "https://example.com",
			Method:          "GET",
			IdempotencyKey:  fmt.Sprintf("e2e-%d", time.Now().UnixNano()),
		})
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}
		log.Printf("Task submitted. ID: %s", res.TaskId)

		waitForStatus(t, client, res.TaskId, []string{"COMPLETED", "FAILED"})
	})

	t.Run("SubmitTask_UnreachableTarget", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		res, err := client.SubmitTask(ctx, &pb.TaskRequest{
			TaskDescription: "Unreachable Task",
			Priority:        pb.TaskPriority_MEDIUM,
			Url:             "http://localhost:59999",
			Method:          "GET",
			IdempotencyKey:  fmt.Sprintf("e2e-fail-%d", time.Now().UnixNano()),
		})
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}
		log.Printf("Unreachable Task submitted. ID: %s", res.TaskId)

		// Worker handles unreachable targets gracefully, usually marking as COMPLETED (with internal error result)
		waitForStatus(t, client, res.TaskId, []string{"COMPLETED", "FAILED"})
	})

	t.Run("Idempotency", func(t *testing.T) {
		key := fmt.Sprintf("idem-%d", time.Now().UnixNano())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// First submission
		res1, err := client.SubmitTask(ctx, &pb.TaskRequest{
			TaskDescription: "Idem Task 1",
			Priority:        pb.TaskPriority_LOW,
			Url:             "http://example.com",
			Method:          "GET",
			IdempotencyKey:  key,
		})
		if err != nil {
			t.Fatalf("First submission failed: %v", err)
		}

		// Second submission with same key
		res2, err := client.SubmitTask(ctx, &pb.TaskRequest{
			TaskDescription: "Idem Task 2",
			Priority:        pb.TaskPriority_LOW,
			Url:             "http://example.com",
			Method:          "GET",
			IdempotencyKey:  key,
		})
		if err != nil {
			t.Fatalf("Second submission failed: %v", err)
		}

		if res1.TaskId != res2.TaskId {
			t.Errorf("Expected same TaskID for idempotent request. Got %s and %s", res1.TaskId, res2.TaskId)
		}

		// Third submission with different key
		res3, err := client.SubmitTask(ctx, &pb.TaskRequest{
			TaskDescription: "Idem Task 3",
			Priority:        pb.TaskPriority_LOW,
			Url:             "http://example.com",
			Method:          "GET",
			IdempotencyKey:  key + "-diff",
		})
		if err != nil {
			t.Fatalf("Third submission failed: %v", err)
		}

		if res1.TaskId == res3.TaskId {
			t.Errorf("Expected different TaskID for different key. Got %s", res1.TaskId)
		}
	})

	t.Run("MissingURL", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := client.SubmitTask(ctx, &pb.TaskRequest{
			TaskDescription: "Invalid Task",
			Priority:        pb.TaskPriority_LOW,
			Url:             "", // Missing URL
			Method:          "GET",
			IdempotencyKey:  fmt.Sprintf("invalid-%d", time.Now().UnixNano()),
		})

		if err == nil {
			t.Fatal("Expected error for missing URL, got nil")
		}

		// Check if it's a gRPC error (although server.go returns fmt.Errorf, gRPC wraps it)
		// Ideally we check for Unknown or InvalidArgument.
		// server.go returns fmt.Errorf("url is required"), which translates to Unknown usually.
		st, ok := status.FromError(err)
		if !ok {
			t.Logf("Got error but not status error: %v", err)
		} else {
			if st.Code() != codes.Unknown && st.Code() != codes.InvalidArgument {
				t.Logf("Got unexpected status code: %s", st.Code())
			}
		}
	})
}

func waitForStatus(t *testing.T, client pb.TaskManagerClient, taskID string, targetStatuses []string) {
	streamCtx, streamCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer streamCancel()

	stream, err := client.StreamTaskStatus(streamCtx, &pb.StatusRequest{TaskId: taskID})
	if err != nil {
		t.Fatalf("Failed to start status stream: %v", err)
	}

	for {
		status, err := stream.Recv()
		if err != nil {
			t.Fatalf("Error receiving status: %v", err)
		}
		log.Printf("[%s] Task Status: %s", taskID, status.Status)
		for _, ts := range targetStatuses {
			if status.Status == ts {
				return
			}
		}
	}
}
