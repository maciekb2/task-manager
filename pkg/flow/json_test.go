package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskEnvelope_JSON(t *testing.T) {
	// Case 1: All fields present
	task := TaskEnvelope{
		TaskID:          "123",
		TaskDescription: "Desc",
		Priority:        1,
		URL:             "http://test.com",
		Method:          "GET",
		TraceParent:     "tp-123",
		CreatedAt:       "now",
		Category:        "cat",
		Score:           10,
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	str := string(data)
	if !strings.Contains(str, `"traceparent":"tp-123"`) {
		t.Error("Expected traceparent field")
	}
	if !strings.Contains(str, `"category":"cat"`) {
		t.Error("Expected category field")
	}

	// Case 2: Optional fields empty
	taskEmpty := TaskEnvelope{
		TaskID: "123",
	}
	dataEmpty, err := json.Marshal(taskEmpty)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	strEmpty := string(dataEmpty)
	if strings.Contains(strEmpty, `"traceparent"`) {
		t.Error("Expected traceparent to be omitted")
	}
	if strings.Contains(strEmpty, `"category"`) {
		t.Error("Expected category to be omitted")
	}
	if strings.Contains(strEmpty, `"score"`) {
		t.Error("Expected score to be omitted")
	}
}

func TestResultEnvelope_JSON(t *testing.T) {
	// Case 1: Latency present
	res := ResultEnvelope{
		Result:    200,
		LatencyMs: 100,
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"latency_ms":100`) {
		t.Error("Expected latency_ms field")
	}

	// Case 2: Latency 0 (should be omitted due to omitempty?)
	// Let's check struct definition: `json:"latency_ms,omitempty"`
	// If LatencyMs is 0, it will be omitted.
	resZero := ResultEnvelope{
		Result: 200,
	}
	dataZero, err := json.Marshal(resZero)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if strings.Contains(string(dataZero), `"latency_ms"`) {
		t.Error("Expected latency_ms to be omitted when 0")
	}
}
