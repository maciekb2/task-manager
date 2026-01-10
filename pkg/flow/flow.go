package flow

import "time"

const (
	QueueIngest     = "queue:ingest"
	QueueSchedule   = "queue:schedule"
	QueueStatus     = "queue:status"
	QueueResults    = "queue:results"
	QueueAudit      = "queue:audit"
	QueueDeadLetter = "queue:deadletter"

	SortedTasks = "tasks"
)

type TaskEnvelope struct {
	TaskID          string `json:"task_id"`
	TaskDescription string `json:"task_description"`
	Priority        int32  `json:"priority"`
	Number1         int32  `json:"number1"`
	Number2         int32  `json:"number2"`
	TraceParent     string `json:"traceparent,omitempty"`
	CreatedAt       string `json:"created_at"`
	Attempt         int    `json:"attempt"`
	Category        string `json:"category,omitempty"`
	Score           int32  `json:"score,omitempty"`
}

type StatusUpdate struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	TraceParent string `json:"traceparent,omitempty"`
	Timestamp   string `json:"timestamp"`
	Source      string `json:"source"`
}

type ResultEnvelope struct {
	Task        TaskEnvelope `json:"task"`
	Result      int32        `json:"result"`
	ProcessedAt string       `json:"processed_at"`
	WorkerID    int          `json:"worker_id"`
}

type AuditEvent struct {
	TaskID      string `json:"task_id"`
	Event       string `json:"event"`
	Detail      string `json:"detail,omitempty"`
	TraceParent string `json:"traceparent,omitempty"`
	Source      string `json:"source"`
	Timestamp   string `json:"timestamp"`
}

type DeadLetter struct {
	Task      TaskEnvelope `json:"task"`
	Reason    string       `json:"reason"`
	Source    string       `json:"source"`
	Timestamp string       `json:"timestamp"`
}

func StatusChannel(taskID string) string {
	return "task_status:" + taskID
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
