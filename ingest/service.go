package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type IngestService struct {
	pub      Publisher
	enricher Enricher
	tracer   trace.Tracer
}

func NewIngestService(pub Publisher, enricher Enricher) *IngestService {
	return &IngestService{
		pub:      pub,
		enricher: enricher,
		tracer:   otel.Tracer("ingest"),
	}
}

func (s *IngestService) ProcessMessage(msgCtx context.Context, msg Message) error {
	var task flow.TaskEnvelope
	if err := json.Unmarshal(msg.GetData(), &task); err != nil {
		log.Printf("ingest: bad payload: %v", err)
		s.handleProcessingFailure(msgCtx, msg, flow.TaskEnvelope{}, "bad payload", false)
		return nil
	}

	if task.TraceParent == "" {
		task.TraceParent = traceparentFromContext(msgCtx)
	}

	parentCtx := msgCtx
	if !trace.SpanContextFromContext(parentCtx).IsValid() && task.TraceParent != "" {
		parentCtx = contextFromTraceParent(task.TraceParent)
	}
	ctxTask, span := s.tracer.Start(parentCtx, "ingest.process")
	s.annotateSpan(span, msg)
	span.SetAttributes(
		attribute.String("task.id", task.TaskID),
		attribute.Int64("task.priority", int64(task.Priority)),
		attribute.String("task.url", task.URL),
		attribute.String("task.method", task.Method),
		attribute.String("queue.source", bus.SubjectTaskIngest),
		attribute.String("queue.target", bus.SubjectTaskSchedule),
	)

	if strings.TrimSpace(task.TaskDescription) == "" {
		span.SetAttributes(attribute.String("task.validation", "missing_description"))
		span.RecordError(errInvalidTask("missing description"))
		s.enqueueDeadLetter(ctxTask, task, "missing description", s.getDeliveryAttempt(msg))
		s.enqueueStatus(ctxTask, task, "REJECTED", "ingest")
		s.ackMessage(msg)
		span.End()
		return nil
	}

	enriched, err := s.enricher.Enrich(ctxTask, task)
	if err != nil {
		log.Printf("ingest: enrich failed: %v", err)
		span.RecordError(err)
	} else {
		task = enriched
		span.SetAttributes(
			attribute.String("task.category", task.Category),
			attribute.Int64("task.score", int64(task.Score)),
		)
		s.enqueueStatus(ctxTask, task, "ENRICHED", "ingest")
	}

	task.Attempt++
	if _, err := s.pub.PublishJSON(ctxTask, bus.SubjectTaskSchedule, task, nil); err != nil {
		log.Printf("ingest: publish schedule failed: %v", err)
		span.RecordError(err)
		span.End()
		s.handleProcessingFailure(msgCtx, msg, task, "publish schedule failed", true)
		return nil
	}

	s.enqueueStatus(ctxTask, task, "VALIDATED", "ingest")
	s.enqueueAudit(ctxTask, flow.AuditEvent{
		TaskID:      task.TaskID,
		Event:       "task.validated",
		Detail:      "Task validated and forwarded",
		TraceParent: task.TraceParent,
		Source:      "ingest",
		Timestamp:   flow.Now(),
	})

	span.End()
	s.ackMessage(msg)
	return nil
}

func (s *IngestService) handleProcessingFailure(ctx context.Context, msg Message, task flow.TaskEnvelope, reason string, retry bool) {
	attempts := s.getDeliveryAttempt(msg)
	if !retry || attempts >= bus.MaxDeliver() {
		s.enqueueDeadLetter(ctx, task, reason, attempts)
		s.ackMessage(msg)
		return
	}
	s.nakMessage(msg)
}

func (s *IngestService) enqueueStatus(ctx context.Context, task flow.TaskEnvelope, status, source string) {
	update := flow.StatusUpdate{
		TaskID:      task.TaskID,
		Status:      status,
		TraceParent: task.TraceParent,
		Timestamp:   flow.Now(),
		Source:      source,
	}
	if _, err := s.pub.PublishJSON(ctx, bus.SubjectEventStatus, update, nil); err != nil {
		log.Printf("ingest: status publish failed: %v", err)
	}
}

func (s *IngestService) enqueueAudit(ctx context.Context, event flow.AuditEvent) {
	if _, err := s.pub.PublishJSON(ctx, bus.SubjectEventAudit, event, nil); err != nil {
		log.Printf("ingest: audit publish failed: %v", err)
	}
}

func (s *IngestService) enqueueDeadLetter(ctx context.Context, task flow.TaskEnvelope, reason string, attempts int) {
	if attempts <= 0 {
		attempts = 1
	}
	entry := flow.DeadLetter{
		Task:      task,
		Reason:    reason,
		Attempts:  attempts,
		Source:    "ingest",
		Timestamp: flow.Now(),
	}
	if _, err := s.pub.PublishJSON(ctx, bus.SubjectEventDeadLetter, entry, nil); err != nil {
		log.Printf("ingest: deadletter publish failed: %v", err)
	}
}

func (s *IngestService) getDeliveryAttempt(msg Message) int {
	meta, err := msg.Metadata()
	if err != nil || meta == nil {
		return 1
	}
	return int(meta.NumDelivered)
}

func (s *IngestService) ackMessage(msg Message) {
	if msg == nil {
		return
	}
	if err := msg.Ack(); err != nil {
		log.Printf("ingest: ack failed: %v", err)
	}
}

func (s *IngestService) nakMessage(msg Message) {
	if msg == nil {
		return
	}
	if err := msg.Nak(); err != nil {
		log.Printf("ingest: nak failed: %v", err)
	}
}

func (s *IngestService) annotateSpan(span trace.Span, msg Message) {
	if span == nil || msg == nil {
		return
	}
	attrs := []attribute.KeyValue{attribute.String("message.subject", msg.GetSubject())}
	meta, err := msg.Metadata()
	if err == nil && meta != nil {
		if meta.Stream != "" {
			attrs = append(attrs, attribute.String("message.stream", meta.Stream))
		}
		if meta.Consumer != "" {
			attrs = append(attrs, attribute.String("message.consumer", meta.Consumer))
		}
		if meta.NumDelivered > 0 {
			attrs = append(attrs, attribute.Int64("message.deliver_count", int64(meta.NumDelivered)))
		}
	}
	span.SetAttributes(attrs...)
}

// Helpers reused from main.go (which will be removed/modified in main.go)

func contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}

func traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}

type errInvalidTask string

func (e errInvalidTask) Error() string { return string(e) }
