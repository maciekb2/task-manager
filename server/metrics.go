package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	tasksCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "task_manager_tasks_created_total",
		Help: "The total number of tasks created",
	}, []string{"priority"})

	tasksProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "task_manager_tasks_processed_total",
		Help: "The total number of tasks processed",
	}, []string{"status"})

	taskProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "task_manager_task_processing_duration_seconds",
		Help:    "Duration of task processing",
		Buckets: prometheus.DefBuckets,
	})

	activeWorkers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "task_manager_active_workers",
		Help: "Number of currently active workers",
	})
)
