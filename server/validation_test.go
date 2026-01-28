package main

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	pb "github.com/maciekb2/task-manager/proto"
)

func TestSubmitTask_MethodDefaulting(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mockBus := &MockBus{}
	srv := newServer(rdb, mockBus)
	ctx := context.Background()

	// Task with empty Method
	req := &pb.TaskRequest{
		TaskDescription: "Default Method Task",
		Url:             "http://example.com",
		Method:          "", // Should default to GET
	}

	resp, err := srv.SubmitTask(ctx, req)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// Verify Redis
	taskKey := "task:" + resp.TaskId
	val := mr.HGet(taskKey, "method")
	if val != "GET" {
		t.Errorf("expected method 'GET', got '%s'", val)
	}

	// Verify Bus
	foundIngest := false
	for _, msg := range mockBus.Published {
		if msg.Subject == bus.SubjectTaskIngest {
			taskEnv, ok := msg.Payload.(flow.TaskEnvelope)
			if ok {
				foundIngest = true
				if taskEnv.Method != "GET" {
					t.Errorf("expected bus payload method 'GET', got '%s'", taskEnv.Method)
				}
			}
		}
	}
	if !foundIngest {
		t.Error("TaskIngest message not published")
	}
}
