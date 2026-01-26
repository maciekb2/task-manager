package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-redis/redis/v8"
	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var ErrBadPayload = errors.New("bad payload")

type AuditProcessor struct {
	RDB    redis.Cmdable
	Tracer trace.Tracer
}

func (p *AuditProcessor) Process(ctx context.Context, msg *nats.Msg) (flow.TaskEnvelope, error) {
	var event flow.AuditEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		return flow.TaskEnvelope{}, ErrBadPayload
	}

	if event.TraceParent == "" {
		event.TraceParent = traceparentFromContext(ctx)
	}

	parentCtx := ctx
	if !trace.SpanContextFromContext(parentCtx).IsValid() && event.TraceParent != "" {
		parentCtx = contextFromTraceParent(event.TraceParent)
	}

	ctxSpan, span := p.Tracer.Start(parentCtx, "audit.persist")
	defer span.End()

	bus.AnnotateSpan(span, msg)
	span.SetAttributes(
		attribute.String("task.id", event.TaskID),
		attribute.String("audit.event", event.Event),
		attribute.String("audit.source", event.Source),
		attribute.String("queue.name", bus.SubjectEventAudit),
	)

	payload, err := json.Marshal(event)
	if err != nil {
		return flow.TaskEnvelope{}, err
	}

	if err := p.RDB.RPush(ctxSpan, "audit_events", payload).Err(); err != nil {
		span.RecordError(err)
		return flow.TaskEnvelope{TaskID: event.TaskID, TraceParent: event.TraceParent}, err
	}

	logger.WithContext(ctxSpan).Info("audit: event processed", "event", event.Event, "task_id", event.TaskID)
	return flow.TaskEnvelope{TaskID: event.TaskID, TraceParent: event.TraceParent}, nil
}

func contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}

func traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}
