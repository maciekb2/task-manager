package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type DeadLetterService struct {
	rdb    redis.Cmdable
	tracer trace.Tracer
}

func NewDeadLetterService(rdb redis.Cmdable, tracer trace.Tracer) *DeadLetterService {
	return &DeadLetterService{
		rdb:    rdb,
		tracer: tracer,
	}
}

func (s *DeadLetterService) Persist(ctx context.Context, entry flow.DeadLetter, msg *nats.Msg) error {
	ctxSpan, span := s.tracer.Start(ctx, "deadletter.persist")
	if msg != nil {
		bus.AnnotateSpan(span, msg)
	}
	defer span.End()

	span.SetAttributes(
		attribute.String("task.id", entry.Task.TaskID),
		attribute.String("deadletter.reason", entry.Reason),
		attribute.String("deadletter.source", entry.Source),
		attribute.String("queue.name", bus.SubjectEventDeadLetter),
	)

	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}

	if err := s.rdb.RPush(ctxSpan, "dead_letter", payload).Err(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("persist failed: %w", err)
	}

	return nil
}
