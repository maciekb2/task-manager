package main

import (
	"strconv"

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
)

func recordMetrics(priority int32) {
	p := strconv.Itoa(int(priority))
	tasksReceived.WithLabelValues(p).Inc()
}
