package bus

import "strings"

const (
	StreamTasks  = "TASKS"
	StreamEvents = "EVENTS"
)

const (
	SubjectTaskIngest       = "tasks.ingest"
	SubjectTaskSchedule     = "tasks.schedule"
	SubjectTaskWorkerHigh   = "tasks.worker.high"
	SubjectTaskWorkerMedium = "tasks.worker.medium"
	SubjectTaskWorkerLow    = "tasks.worker.low"
	SubjectTaskResults      = "tasks.results"
)

const (
	SubjectEventStatus     = "events.status"
	SubjectEventAudit      = "events.audit"
	SubjectEventDeadLetter = "events.deadletter"
)

func DurableName(stream, service string) string {
	stream = strings.ToLower(strings.TrimSpace(stream))
	service = strings.TrimSpace(service)
	if stream == "" {
		return service
	}
	if service == "" {
		return stream
	}
	return stream + "-" + service
}

func QueueGroup(service string) string {
	return strings.TrimSpace(service)
}

func DLQSubject(stream string) string {
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return "dlq"
	}
	return strings.ToLower(stream) + ".dlq"
}

func WorkerSubjectForPriority(priority int32) string {
	switch priority {
	case 2:
		return SubjectTaskWorkerHigh
	case 1:
		return SubjectTaskWorkerMedium
	default:
		return SubjectTaskWorkerLow
	}
}
