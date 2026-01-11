package bus

import (
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	AttrMessageSubject      = "message.subject"
	AttrMessageStream       = "message.stream"
	AttrMessageConsumer     = "message.consumer"
	AttrMessageDeliverCount = "message.deliver_count"
)

func MessageAttributes(msg *nats.Msg) []attribute.KeyValue {
	if msg == nil {
		return nil
	}

	attrs := []attribute.KeyValue{attribute.String(AttrMessageSubject, msg.Subject)}
	meta, err := msg.Metadata()
	if err != nil || meta == nil {
		return attrs
	}
	if meta.Stream != "" {
		attrs = append(attrs, attribute.String(AttrMessageStream, meta.Stream))
	}
	if meta.Consumer != "" {
		attrs = append(attrs, attribute.String(AttrMessageConsumer, meta.Consumer))
	}
	if meta.NumDelivered > 0 {
		attrs = append(attrs, attribute.Int64(AttrMessageDeliverCount, int64(meta.NumDelivered)))
	}
	return attrs
}

func AnnotateSpan(span trace.Span, msg *nats.Msg) {
	if span == nil || msg == nil {
		return
	}
	attrs := MessageAttributes(msg)
	if len(attrs) == 0 {
		return
	}
	span.SetAttributes(attrs...)
}
