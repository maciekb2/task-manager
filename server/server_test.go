package main

import (
	"testing"
)

func TestMemoryLeak(t *testing.T) {
	s := &server{
		subscribers: make(map[string]chan string),
		// rdb is nil, but we are not calling methods that use it
	}

	taskID := "test-task"
	s.subscribers[taskID] = make(chan string, 10)

	// Call notifySubscriber directly to test cleanup logic without needing Redis
	s.notifySubscriber(taskID, "COMPLETED")

	s.mu.Lock()
	_, exists := s.subscribers[taskID]
	s.mu.Unlock()

	if exists {
		t.Fatalf("Memory leak detected: subscriber for task %s was not removed", taskID)
	}
}
