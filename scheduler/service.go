package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/maciekb2/task-manager/pkg/bus"
	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// BusPublisher defines the interface for publishing messages, abstraction over *bus.Client
type BusPublisher interface {
	PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header, opts ...nats.PubOpt) (*nats.PubAck, error)
}

// Message defines an interface for NATS message interaction to allow mocking
type Message interface {
	GetData() []byte
	GetHeader() nats.Header
	Ack() error
	Nak() error
	DeliveryAttempt() int
}

// NatsMessage implements Message for a real *nats.Msg
type NatsMessage struct {
	Msg *nats.Msg
}

func (m *NatsMessage) GetData() []byte {
	return m.Msg.Data
}

func (m *NatsMessage) GetHeader() nats.Header {
	return m.Msg.Header
}

func (m *NatsMessage) Ack() error {
	return m.Msg.Ack()
}

func (m *NatsMessage) Nak() error {
	return m.Msg.Nak()
}

func (m *NatsMessage) DeliveryAttempt() int {
	return bus.DeliveryAttempt(m.Msg)
}

type Service struct {
	publisher BusPublisher
	tracer    trace.Tracer
}

func NewService(publisher BusPublisher) *Service {
	return &Service{
		publisher: publisher,
		tracer:    otel.Tracer("scheduler"),
	}
}

func (s *Service) ProcessTask(msgCtx context.Context, msg Message) error {
	var task flow.TaskEnvelope
	if err := json.Unmarshal(msg.GetData(), &task); err != nil {
		slog.Error("scheduler: bad payload", "error", err)
		s.handleProcessingFailure(msgCtx, msg, flow.TaskEnvelope{}, "bad payload", false)
		return nil
	}

	if task.TraceParent == "" {
		task.TraceParent = s.traceparentFromContext(msgCtx)
	}

	parentCtx := msgCtx
	if !trace.SpanContextFromContext(parentCtx).IsValid() && task.TraceParent != "" {
		parentCtx = s.contextFromTraceParent(task.TraceParent)
	}
	ctxTask, span := s.tracer.Start(parentCtx, "scheduler.schedule")

	// If we are in production, msg is NatsMessage, which has Msg.
	if nm, ok := msg.(*NatsMessage); ok {
		bus.AnnotateSpan(span, nm.Msg)
	}

	queueName := bus.WorkerSubjectForPriority(task.Priority)
	span.SetAttributes(
		attribute.String("task.id", task.TaskID),
		attribute.Int64("task.priority", int64(task.Priority)),
		attribute.String("task.url", task.URL),
		attribute.String("task.method", task.Method),
		attribute.String("queue.source", bus.SubjectTaskSchedule),
		attribute.String("queue.target", queueName),
	)

	if _, err := s.publisher.PublishJSON(ctxTask, queueName, task, nil); err != nil {
		logger.WithContext(ctxTask).Error("scheduler: publish worker failed", "error", err)
		span.RecordError(err)
		span.End()
		s.handleProcessingFailure(msgCtx, msg, task, "publish worker failed", true)
		return nil
	}

	s.enqueueStatus(ctxTask, task, "SCHEDULED", "scheduler")
	s.enqueueAudit(ctxTask, flow.AuditEvent{
		TaskID:      task.TaskID,
		Event:       "task.scheduled",
		Detail:      "Task added to priority queue",
		TraceParent: task.TraceParent,
		Source:      "scheduler",
		Timestamp:   flow.Now(),
	})

	span.End()
	s.ackMessage("scheduler", msg)
	return nil
}

func (s *Service) enqueueStatus(ctx context.Context, task flow.TaskEnvelope, status, source string) {
	update := flow.StatusUpdate{
		TaskID:      task.TaskID,
		Status:      status,
		TraceParent: task.TraceParent,
		Timestamp:   flow.Now(),
		Source:      source,
	}
	if _, err := s.publisher.PublishJSON(ctx, bus.SubjectEventStatus, update, nil); err != nil {
		logger.WithContext(ctx).Error("scheduler: status publish failed", "error", err)
	}
}

func (s *Service) enqueueAudit(ctx context.Context, event flow.AuditEvent) {
	if _, err := s.publisher.PublishJSON(ctx, bus.SubjectEventAudit, event, nil); err != nil {
		logger.WithContext(ctx).Error("scheduler: audit publish failed", "error", err)
	}
}

func (s *Service) enqueueDeadLetter(ctx context.Context, task flow.TaskEnvelope, reason string, attempts int) {
	if attempts <= 0 {
		attempts = 1
	}
	entry := flow.DeadLetter{
		Task:      task,
		Reason:    reason,
		Attempts:  attempts,
		Source:    "scheduler",
		Timestamp: flow.Now(),
	}
	if _, err := s.publisher.PublishJSON(ctx, bus.SubjectEventDeadLetter, entry, nil); err != nil {
		logger.WithContext(ctx).Error("scheduler: deadletter publish failed", "error", err)
	}
}

func (s *Service) handleProcessingFailure(ctx context.Context, msg Message, task flow.TaskEnvelope, reason string, retry bool) {
	attempts := msg.DeliveryAttempt()
	if !retry || attempts >= bus.MaxDeliver() {
		s.enqueueDeadLetter(ctx, task, reason, attempts)
		s.ackMessage("scheduler", msg)
		return
	}
	s.nakMessage("scheduler", msg)
}

func (s *Service) ackMessage(service string, msg Message) {
	if msg == nil {
		return
	}
	if err := msg.Ack(); err != nil {
		logger.Error("ack failed", err, "service", service)
	}
}

func (s *Service) nakMessage(service string, msg Message) {
	if msg == nil {
		return
	}
	if err := msg.Nak(); err != nil {
		logger.Error("nak failed", err, "service", service)
	}
}

func (s *Service) contextFromTraceParent(traceParent string) context.Context {
	carrier := propagation.MapCarrier{"traceparent": traceParent}
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}

func (s *Service) traceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier["traceparent"]
}
