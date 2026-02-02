package flow

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskEnvelope_JSON_Contract(t *testing.T) {
	// Setup a task with known values
	task := TaskEnvelope{
		TaskID:          "test-123",
		TaskDescription: "test description",
		Priority:        2,
		URL:             "http://test.com",
		Method:          "POST",
		CreatedAt:       "2023-01-01T00:00:00Z",
		Attempt:         1,
		// Leave TraceParent, Category, Score empty to test omitempty
	}

	// Marshal
	data, err := json.Marshal(task)
	assert.NoError(t, err)

	// Unmarshal to generic map to verify keys explicitly
	var jsonMap map[string]interface{}
	err = json.Unmarshal(data, &jsonMap)
	assert.NoError(t, err)

	// Assertions
	assert.Equal(t, "test-123", jsonMap["task_id"])
	assert.Equal(t, "test description", jsonMap["task_description"])
	assert.Equal(t, float64(2), jsonMap["priority"]) // JSON numbers unmarshal as float64
	assert.Equal(t, "http://test.com", jsonMap["url"])
	assert.Equal(t, "POST", jsonMap["method"])
	assert.Equal(t, "2023-01-01T00:00:00Z", jsonMap["created_at"])
	assert.Equal(t, float64(1), jsonMap["attempt"])

	// Verify omitempty fields are missing
	_, hasTrace := jsonMap["traceparent"]
	assert.False(t, hasTrace, "traceparent should be omitted when empty")
	_, hasCat := jsonMap["category"]
	assert.False(t, hasCat, "category should be omitted when empty")
	_, hasScore := jsonMap["score"]
	assert.False(t, hasScore, "score should be omitted when empty")
}

func TestResultEnvelope_JSON_Contract(t *testing.T) {
	result := ResultEnvelope{
		Task:        TaskEnvelope{TaskID: "task-1"},
		Result:      200,
		LatencyMs:   150,
		ProcessedAt: "2023-01-01T00:00:05Z",
		WorkerID:    5,
	}

	data, err := json.Marshal(result)
	assert.NoError(t, err)

	var jsonMap map[string]interface{}
	err = json.Unmarshal(data, &jsonMap)
	assert.NoError(t, err)

	// Check nested task
	taskMap, ok := jsonMap["task"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "task-1", taskMap["task_id"])

	assert.Equal(t, float64(200), jsonMap["result"])
	assert.Equal(t, float64(150), jsonMap["latency_ms"])
	assert.Equal(t, "2023-01-01T00:00:05Z", jsonMap["processed_at"])
	assert.Equal(t, float64(5), jsonMap["worker_id"])
}

func TestTaskEnvelope_JSON_Full(t *testing.T) {
	// Test with all fields populated
	task := TaskEnvelope{
		TaskID:      "id",
		TraceParent: "trace-1",
		Category:    "cat",
		Score:       100,
	}

	data, err := json.Marshal(task)
	assert.NoError(t, err)

	var jsonMap map[string]interface{}
	err = json.Unmarshal(data, &jsonMap)
	assert.NoError(t, err)

	assert.Equal(t, "trace-1", jsonMap["traceparent"])
	assert.Equal(t, "cat", jsonMap["category"])
	assert.Equal(t, float64(100), jsonMap["score"])
}
