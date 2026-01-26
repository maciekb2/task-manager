package flow

import (
	"testing"
	"time"
)

func TestWorkerQueueForPriority(t *testing.T) {
	tests := []struct {
		priority int32
		want     string
	}{
		{2, QueueWorkerHigh},
		{1, QueueWorkerMedium},
		{0, QueueWorkerLow},
		{-1, QueueWorkerLow},
		{100, QueueWorkerLow},
	}

	for _, tt := range tests {
		got := WorkerQueueForPriority(tt.priority)
		if got != tt.want {
			t.Errorf("WorkerQueueForPriority(%d) = %q; want %q", tt.priority, got, tt.want)
		}
	}
}

func TestWorkerQueuesByPriority(t *testing.T) {
	got := WorkerQueuesByPriority()
	if len(got) != 3 {
		t.Fatalf("WorkerQueuesByPriority returned %d queues; want 3", len(got))
	}
	if got[0] != QueueWorkerHigh {
		t.Errorf("First queue should be High, got %q", got[0])
	}
	if got[1] != QueueWorkerMedium {
		t.Errorf("Second queue should be Medium, got %q", got[1])
	}
	if got[2] != QueueWorkerLow {
		t.Errorf("Third queue should be Low, got %q", got[2])
	}
}

func TestStatusChannel(t *testing.T) {
	got := StatusChannel("123")
	want := "task_status:123"
	if got != want {
		t.Errorf("StatusChannel(123) = %q; want %q", got, want)
	}
}

func TestNow(t *testing.T) {
	ts := Now()
	_, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t.Errorf("Now() returned invalid RFC3339Nano string: %q, error: %v", ts, err)
	}
}
