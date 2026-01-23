package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestProcess(t *testing.T) {
	// Setup Mock Redis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	// Setup Tracer
	tracer := otel.Tracer("test")

	// Processor
	proc := &AuditProcessor{
		RDB:    rdb,
		Tracer: tracer,
	}

	ctx := context.Background()

	tests := []struct {
		name        string
		msgData     []byte
		setupRedis  func()
		expectErr   error
		expectRedis int
		expectEvent *flow.AuditEvent
	}{
		{
			name: "Valid Event",
			msgData: func() []byte {
				e := flow.AuditEvent{
					TaskID: "task-123",
					Event:  "created",
					Source: "test",
				}
				b, _ := json.Marshal(e)
				return b
			}(),
			expectErr:   nil,
			expectRedis: 1,
			expectEvent: &flow.AuditEvent{
				TaskID: "task-123",
				Event:  "created",
				Source: "test",
			},
		},
		{
			name: "Invalid JSON",
			msgData: []byte("invalid-json"),
			expectErr: ErrBadPayload,
			expectRedis: 0,
		},
		{
			name: "Redis Error",
			msgData: func() []byte {
				e := flow.AuditEvent{TaskID: "task-456"}
				b, _ := json.Marshal(e)
				return b
			}(),
			setupRedis: func() {
				mr.SetError("mock redis error")
			},
			expectErr:   errors.New("mock redis error"),
			expectRedis: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset Redis state
			mr.FlushAll()
			mr.SetError("")

			if tt.setupRedis != nil {
				tt.setupRedis()
			}

			msg := &nats.Msg{
				Subject: "test",
				Data:    tt.msgData,
			}

			envelope, err := proc.Process(ctx, msg)

			if tt.expectErr != nil {
				if tt.expectErr == ErrBadPayload {
					assert.Equal(t, ErrBadPayload, err)
				} else {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectErr.Error())
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectEvent.TaskID, envelope.TaskID)
			}

			if tt.expectRedis > 0 {
				len, err := rdb.LLen(ctx, "audit_events").Result()
				assert.NoError(t, err)
				assert.Equal(t, int64(tt.expectRedis), len)

				// Check content
				val, err := rdb.LPop(ctx, "audit_events").Result()
				assert.NoError(t, err)

				var actual flow.AuditEvent
				err = json.Unmarshal([]byte(val), &actual)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectEvent.TaskID, actual.TaskID)
			}
		})
	}
}
