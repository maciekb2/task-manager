package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskEnvelope_JSON(t *testing.T) {
	// Case 1: With all fields
	t1 := TaskEnvelope{
		TaskID:      "1",
		TraceParent: "trace-1",
		Category:    "high",
		Score:       100,
	}
	b1, err := json.Marshal(t1)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s1 := string(b1)
	if !strings.Contains(s1, "traceparent") {
		t.Error("Expected traceparent field")
	}
	if !strings.Contains(s1, "category") {
		t.Error("Expected category field")
	}
	if !strings.Contains(s1, "score") {
		t.Error("Expected score field")
	}

	// Case 2: Omitted fields
	t2 := TaskEnvelope{
		TaskID: "2",
	}
	b2, err := json.Marshal(t2)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s2 := string(b2)
	if strings.Contains(s2, "traceparent") {
		t.Error("Expected traceparent to be omitted")
	}
	if strings.Contains(s2, "category") {
		t.Error("Expected category to be omitted")
	}
	if strings.Contains(s2, "score") {
		t.Error("Expected score to be omitted")
	}
}

func TestResultEnvelope_JSON(t *testing.T) {
	// Case 1: With Latency
	r1 := ResultEnvelope{
		Result:    200,
		LatencyMs: 100,
	}
	b1, err := json.Marshal(r1)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s1 := string(b1)
	if !strings.Contains(s1, "latency_ms") {
		t.Error("Expected latency_ms field")
	}

	// Case 2: Zero Latency (omitted due to omitempty on int)
	r2 := ResultEnvelope{
		Result:    200,
		LatencyMs: 0,
	}
	b2, err := json.Marshal(r2)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s2 := string(b2)
	if strings.Contains(s2, "latency_ms") {
		t.Error("Expected latency_ms to be omitted for 0 value")
	}
}
