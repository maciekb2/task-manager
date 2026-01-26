package bus

import (
	"testing"
	"time"
)

func TestDurableName(t *testing.T) {
	tests := []struct {
		stream  string
		service string
		want    string
	}{
		{"TASKS", "worker", "tasks-worker"},
		{"tasks", "Worker", "tasks-Worker"}, // Service name is not lowercased by convention
		{"", "worker", "worker"},
		{"tasks", "", "tasks"},
		{"  TASKS  ", "  worker  ", "tasks-worker"},
	}

	for _, tt := range tests {
		got := DurableName(tt.stream, tt.service)
		if got != tt.want {
			t.Errorf("DurableName(%q, %q) = %q; want %q", tt.stream, tt.service, got, tt.want)
		}
	}
}

func TestDLQSubject(t *testing.T) {
	tests := []struct {
		stream string
		want   string
	}{
		{"TASKS", "tasks.dlq"},
		{"tasks", "tasks.dlq"},
		{"", "dlq"},
		{"  TASKS  ", "tasks.dlq"},
	}

	for _, tt := range tests {
		got := DLQSubject(tt.stream)
		if got != tt.want {
			t.Errorf("DLQSubject(%q) = %q; want %q", tt.stream, got, tt.want)
		}
	}
}

func TestMaxDeliver(t *testing.T) {
	// Backup original backoff
	origBackoff := ConsumerBackoff
	defer func() { ConsumerBackoff = origBackoff }()

	// Case 1: Short backoff, should default to ConsumerMaxDeliver (6)
	ConsumerBackoff = []time.Duration{time.Second, time.Second}
	if got := MaxDeliver(); got != ConsumerMaxDeliver {
		t.Errorf("MaxDeliver() short backoff = %d; want %d", got, ConsumerMaxDeliver)
	}

	// Case 2: Long backoff (7 items), should be 7 + 1 = 8
	ConsumerBackoff = []time.Duration{
		1, 2, 3, 4, 5, 6, 7,
	}
	if got := MaxDeliver(); got != 8 {
		t.Errorf("MaxDeliver() long backoff = %d; want 8", got)
	}

	// Case 3: Empty backoff
	ConsumerBackoff = []time.Duration{}
	if got := MaxDeliver(); got != ConsumerMaxDeliver {
		t.Errorf("MaxDeliver() empty backoff = %d; want %d", got, ConsumerMaxDeliver)
	}
}

func TestWorkerSubjectForPriority(t *testing.T) {
	tests := []struct {
		priority int32
		want     string
	}{
		{2, SubjectTaskWorkerHigh},
		{1, SubjectTaskWorkerMedium},
		{0, SubjectTaskWorkerLow},
		{-1, SubjectTaskWorkerLow},
		{100, SubjectTaskWorkerLow},
	}

	for _, tt := range tests {
		got := WorkerSubjectForPriority(tt.priority)
		if got != tt.want {
			t.Errorf("WorkerSubjectForPriority(%d) = %q; want %q", tt.priority, got, tt.want)
		}
	}
}
