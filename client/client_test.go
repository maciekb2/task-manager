package main

import (
	"context"
	"io"
	"testing"
	"time"

	pb "github.com/maciekb2/task-manager/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MockClient mocks the TaskManagerClient
type MockClient struct {
	pb.TaskManagerClient
	sleepDuration time.Duration
}

func (m *MockClient) SubmitTask(ctx context.Context, in *pb.TaskRequest, opts ...grpc.CallOption) (*pb.TaskResponse, error) {
	return &pb.TaskResponse{TaskId: "mock-id"}, nil
}

func (m *MockClient) StreamTaskStatus(ctx context.Context, in *pb.StatusRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.StatusResponse], error) {
	return &MockStream{sleepDuration: m.sleepDuration}, nil
}

// MockStream mocks the stream
type MockStream struct {
	sleepDuration time.Duration
	done          bool
}

func (m *MockStream) Recv() (*pb.StatusResponse, error) {
	if m.done {
		return nil, io.EOF
	}
	// Simulate server processing time
	time.Sleep(m.sleepDuration)
	m.done = true
	return &pb.StatusResponse{Status: "COMPLETED"}, nil
}

func (m *MockStream) Header() (metadata.MD, error)  { return nil, nil }
func (m *MockStream) Trailer() metadata.MD          { return nil }
func (m *MockStream) CloseSend() error              { return nil }
func (m *MockStream) Context() context.Context      { return context.Background() }
func (m *MockStream) SendMsg(msg interface{}) error { return nil }
func (m *MockStream) RecvMsg(msg interface{}) error { return nil }

func BenchmarkSynchronousLoop(b *testing.B) {
	// Task takes 50ms, Loop sleeps 10ms.
	// Sync: 60ms per iter.
	client := &MockClient{sleepDuration: 50 * time.Millisecond}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mimic the synchronous call in main()
		sendTaskWithNumbers(client, "desc", pb.TaskPriority_HIGH, 1, 2)
		time.Sleep(10 * time.Millisecond)
	}
}

func BenchmarkAsynchronousLoop(b *testing.B) {
	// Task takes 50ms, Loop sleeps 10ms.
	// Async: 10ms per iter (task runs in bg).
	client := &MockClient{sleepDuration: 50 * time.Millisecond}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mimic the asynchronous call we plan to implement
		go sendTaskWithNumbers(client, "desc", pb.TaskPriority_HIGH, 1, 2)
		time.Sleep(10 * time.Millisecond)
	}
}
