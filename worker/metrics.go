package main

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	tasksProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tasks_processed_total",
			Help: "The total number of tasks processed",
		},
		[]string{"priority", "status", "worker_id"},
	)

	taskProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "task_processing_duration_seconds",
			Help:    "Duration of task processing",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"priority"},
	)
)

func recordMetrics(priority int32, status string, workerID int, durationSeconds float64) {
	p := strconv.Itoa(int(priority))
	w := strconv.Itoa(workerID)
	tasksProcessed.WithLabelValues(p, status, w).Inc()
	taskProcessingDuration.WithLabelValues(p).Observe(durationSeconds)
}
