package bus

import (
	"testing"
	"time"
	"encoding/base64"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
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

func TestBackoffFor(t *testing.T) {
	policy := RetryPolicy{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
		MaxRetries:     5,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1 * time.Second}, // Capped by MaxBackoff
		{6, 1 * time.Second}, // Capped
	}

	for _, tc := range tests {
		got := backoffFor(policy, tc.attempt)
		if got != tc.expected {
			t.Errorf("attempt %d: expected %v, got %v", tc.attempt, tc.expected, got)
		}
	}
}

func TestBuildDLQMessage(t *testing.T) {
	data := []byte("hello world")
	msg := &nats.Msg{
		Subject: "test.subject",
		Data:    data,
		Header:  nats.Header{"X-Test": []string{"true"}},
	}
	// Note: We can't easily mock msg.Metadata() without a real JS response or interface,
	// but BuildDLQMessage handles nil metadata gracefully.

	reason := "test failure"
	dlq := BuildDLQMessage(msg, reason)

	if dlq.Subject != "test.subject" {
		t.Errorf("expected subject test.subject, got %s", dlq.Subject)
	}
	if dlq.Reason != reason {
		t.Errorf("expected reason %s, got %s", reason, dlq.Reason)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	if dlq.Payload != encoded {
		t.Errorf("expected payload %s, got %s", encoded, dlq.Payload)
	}
	if val := dlq.Headers["X-Test"]; len(val) != 1 || val[0] != "true" {
		t.Errorf("header mismatch")
	}
}

func TestCloneHeaders(t *testing.T) {
	original := nats.Header{
		"Key1": []string{"Val1", "Val2"},
		"Key2": []string{"Val3"},
	}

	cloned := cloneHeaders(original)

	// Verify content
	if len(cloned) != len(original) {
		t.Errorf("length mismatch")
	}

	// Verify deep copy
	original["Key1"][0] = "Modified"
	if cloned["Key1"][0] == "Modified" {
		t.Error("cloneHeaders did not perform deep copy")
	}
}
