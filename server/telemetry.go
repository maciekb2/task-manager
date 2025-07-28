package main

import (
	"context"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

var (
	tasksSubmitted metric.Int64Counter
	tasksProcessed metric.Int64Counter
	taskDuration   metric.Float64Histogram
)

func initTelemetry(ctx context.Context) (func(context.Context) error, error) {
	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(semconv.ServiceName("task-manager-server")),
	)
	if err != nil {
		return nil, err
	}

	traceExp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint("tempo:4317"))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	promExp, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExp),
	)
	otel.SetMeterProvider(mp)

	meter := otel.GetMeterProvider().Meter("taskmanager-server")
	if tasksSubmitted, err = meter.Int64Counter(
		"tasks_submitted",
		metric.WithDescription("Number of tasks submitted"),
	); err != nil {
		return nil, err
	}
	if tasksProcessed, err = meter.Int64Counter(
		"tasks_processed",
		metric.WithDescription("Number of tasks processed"),
	); err != nil {
		return nil, err
	}
	if taskDuration, err = meter.Float64Histogram(
		"task_processing_seconds",
		metric.WithDescription("Task processing duration in seconds"),
		metric.WithUnit("s"),
	); err != nil {
		return nil, err
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":2222", nil); err != nil {
			log.Printf("prometheus endpoint error: %v", err)
		}
	}()

	return tp.Shutdown, nil
}

func serverOpts() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
		grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
	}
}
