package main

import (
	"context"
	"io"
	"os"
	"testing"

	pb "github.com/maciekb2/task-manager/proto"
	"google.golang.org/grpc"
)

// mockStream simulates the stream returned by StreamTaskStatus
type mockStream struct {
	grpc.ClientStream
	statusResponses []*pb.StatusResponse
	current         int
}

func (m *mockStream) Recv() (*pb.StatusResponse, error) {
	if m.current >= len(m.statusResponses) {
		return nil, io.EOF
	}
	res := m.statusResponses[m.current]
	m.current++
	return res, nil
}

// mockTaskManagerClient mocks the TaskManagerClient interface
type mockTaskManagerClient struct {
	submitTaskCalled    bool
	submitTaskReq       *pb.TaskRequest
	streamTaskStatusCalled bool
	streamTaskStatusReq *pb.StatusRequest
}

func (m *mockTaskManagerClient) SubmitTask(ctx context.Context, in *pb.TaskRequest, opts ...grpc.CallOption) (*pb.TaskResponse, error) {
	m.submitTaskCalled = true
	m.submitTaskReq = in
	return &pb.TaskResponse{TaskId: "test-task-id"}, nil
}

func (m *mockTaskManagerClient) StreamTaskStatus(ctx context.Context, in *pb.StatusRequest, opts ...grpc.CallOption) (pb.TaskManager_StreamTaskStatusClient, error) {
	m.streamTaskStatusCalled = true
	m.streamTaskStatusReq = in
	return &mockStream{
		statusResponses: []*pb.StatusResponse{
			{Status: "PENDING"},
			{Status: "PROCESSING"},
			{Status: "COMPLETED"},
		},
	}, nil
}

func TestSendTaskWithURL(t *testing.T) {
	mockClient := &mockTaskManagerClient{}

	workerID := 1
	description := "Test Task"
	priority := pb.TaskPriority_HIGH
	url := "http://example.com"
	method := "GET"

	SendTaskWithURL(mockClient, workerID, description, priority, url, method)

	if !mockClient.submitTaskCalled {
		t.Error("Expected SubmitTask to be called")
	}

	if mockClient.submitTaskReq.TaskDescription != description {
		t.Errorf("Expected description %s, got %s", description, mockClient.submitTaskReq.TaskDescription)
	}

	if mockClient.submitTaskReq.Priority != priority {
		t.Errorf("Expected priority %s, got %s", priority, mockClient.submitTaskReq.Priority)
	}

	if !mockClient.streamTaskStatusCalled {
		t.Error("Expected StreamTaskStatus to be called")
	}

	if mockClient.streamTaskStatusReq.TaskId != "test-task-id" {
		t.Errorf("Expected TaskId test-task-id, got %s", mockClient.streamTaskStatusReq.TaskId)
	}
}

func TestProducerRate(t *testing.T) {
	os.Setenv("PRODUCER_RATE", "10.5")
	defer os.Unsetenv("PRODUCER_RATE")

	rate := producerRate()
	if rate != 10.5 {
		t.Errorf("Expected rate 10.5, got %f", rate)
	}

	os.Setenv("PRODUCER_RATE", "invalid")
	rate = producerRate()
	if rate != 1.0 {
		t.Errorf("Expected default rate 1.0 for invalid input, got %f", rate)
	}
}

func TestProducerConcurrency(t *testing.T) {
	os.Setenv("PRODUCER_CONCURRENCY", "5")
	defer os.Unsetenv("PRODUCER_CONCURRENCY")

	conc := producerConcurrency()
	if conc != 5 {
		t.Errorf("Expected concurrency 5, got %d", conc)
	}

	os.Setenv("PRODUCER_CONCURRENCY", "0")
	conc = producerConcurrency()
	if conc != 1 {
		t.Errorf("Expected default concurrency 1 for invalid input, got %d", conc)
	}
}
