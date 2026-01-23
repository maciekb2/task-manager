package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestPersist(t *testing.T) {
	// Setup miniredis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	tracer := noop.NewTracerProvider().Tracer("")
	service := NewDeadLetterService(rdb, tracer)

	ctx := context.Background()
	entry := flow.DeadLetter{
		Reason: "test_reason",
		Source: "test_source",
		Task: flow.TaskEnvelope{
			TaskID: "task-123",
		},
	}

	// Test Persist
	err = service.Persist(ctx, entry, nil)
	assert.NoError(t, err)

	// Verify Redis content using miniredis direct access
	vals, err := mr.List("dead_letter")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(vals))

	var saved flow.DeadLetter
	err = json.Unmarshal([]byte(vals[0]), &saved)
	assert.NoError(t, err)

	assert.Equal(t, entry.Task.TaskID, saved.Task.TaskID)
	assert.Equal(t, entry.Reason, saved.Reason)
	assert.Equal(t, entry.Source, saved.Source)
}
