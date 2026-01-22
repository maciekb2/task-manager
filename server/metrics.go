package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	tasksReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tasks_received_total",
			Help: "The total number of tasks received",
		},
		[]string{"priority"},
	)

	taskIngestionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "task_ingestion_duration_seconds",
			Help:    "Duration of task ingestion",
			Buckets: prometheus.DefBuckets,
		},
	)
)
