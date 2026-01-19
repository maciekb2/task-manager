package main

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	pb "github.com/maciekb2/task-manager/proto"
)

func TestUnboundedGoroutines(t *testing.T) {
	// Start miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("could not start miniredis: %v", err)
	}
	defer mr.Close()

	// Initialize server
	s := newServer(mr.Addr())

	ctx := context.Background()
	// Start workers
	numWorkers := 5
	s.StartWorkers(ctx, numWorkers)

	// Measure baseline goroutines
	// Give some time for things to settle
	time.Sleep(100 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()

	t.Logf("Baseline goroutines: %d", baselineGoroutines)

	numTasks := 50

	for i := 0; i < numTasks; i++ {
		req := &pb.TaskRequest{
			TaskDescription: fmt.Sprintf("task %d", i),
			Priority:        pb.TaskPriority_HIGH,
		}
		_, err := s.SubmitTask(ctx, req)
		if err != nil {
			t.Fatalf("failed to submit task: %v", err)
		}
	}

	// Allow some time for processing to start (if any)
	time.Sleep(500 * time.Millisecond)

	currentGoroutines := runtime.NumGoroutine()
	t.Logf("Current goroutines: %d", currentGoroutines)

	// We expect the number of goroutines to be stable.
	// Since we started workers BEFORE baseline, the count should not increase significantly.
	// If the leak was present, we would see +50 goroutines.
	if currentGoroutines > baselineGoroutines+5 {
		t.Fatalf("Goroutine leak detected! Started with %d, ended with %d. Difference: %d", baselineGoroutines, currentGoroutines, currentGoroutines-baselineGoroutines)
	} else {
		t.Logf("Success: Goroutine count is stable.")
	}
}
