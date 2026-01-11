package bus

import (
	"time"

	"github.com/nats-io/nats.go"
)

const (
	StreamMaxAge   = 24 * time.Hour
	StreamMaxMsgs  = 1_000_000
	StreamMaxBytes = 512 * 1024 * 1024
)

const (
	ConsumerAckWait    = 30 * time.Second
	ConsumerMaxDeliver = 6
)

var ConsumerBackoff = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	1 * time.Minute,
}

func TasksStreamConfig() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      StreamTasks,
		Subjects:  TaskSubjects(),
		Retention: nats.LimitsPolicy,
		MaxAge:    StreamMaxAge,
		MaxMsgs:   StreamMaxMsgs,
		MaxBytes:  StreamMaxBytes,
		Discard:   nats.DiscardOld,
		Storage:   nats.FileStorage,
	}
}

func EventsStreamConfig() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      StreamEvents,
		Subjects:  EventSubjects(),
		Retention: nats.LimitsPolicy,
		MaxAge:    StreamMaxAge,
		MaxMsgs:   StreamMaxMsgs,
		MaxBytes:  StreamMaxBytes,
		Discard:   nats.DiscardOld,
		Storage:   nats.FileStorage,
	}
}

func DefaultConsumerConfig(durable, filterSubject string) *nats.ConsumerConfig {
	maxDeliver := MaxDeliver()
	cfg := &nats.ConsumerConfig{
		Durable:       durable,
		Name:          durable,
		FilterSubject: filterSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       ConsumerAckWait,
		MaxDeliver:    maxDeliver,
		DeliverPolicy: nats.DeliverAllPolicy,
		ReplayPolicy:  nats.ReplayInstantPolicy,
	}
	if len(ConsumerBackoff) > 0 {
		cfg.BackOff = append([]time.Duration(nil), ConsumerBackoff...)
	}
	return cfg
}

func MaxDeliver() int {
	maxDeliver := ConsumerMaxDeliver
	if len(ConsumerBackoff) >= maxDeliver {
		return len(ConsumerBackoff) + 1
	}
	return maxDeliver
}

func TaskSubjects() []string {
	return []string{
		SubjectTaskIngest,
		SubjectTaskSchedule,
		SubjectTaskWorkerHigh,
		SubjectTaskWorkerMedium,
		SubjectTaskWorkerLow,
		SubjectTaskResults,
	}
}

func EventSubjects() []string {
	return []string{
		SubjectEventStatus,
		SubjectEventAudit,
		SubjectEventDeadLetter,
	}
}
