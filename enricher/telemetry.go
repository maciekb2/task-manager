package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func initTelemetry(ctx context.Context) (func(context.Context) error, error) {
	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(semconv.ServiceName("task-enricher")),
	)
	if err != nil {
		return nil, err
	}

	traceExp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint(otelEndpoint()))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	promExp, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExp),
	)
	otel.SetMeterProvider(mp)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":"+metricsPort(), nil); err != nil {
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

func otelEndpoint() string {
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		if strings.HasPrefix(endpoint, "http://") {
			return strings.TrimPrefix(endpoint, "http://")
		}
		if strings.HasPrefix(endpoint, "https://") {
			return strings.TrimPrefix(endpoint, "https://")
		}
		return endpoint
	}
	return "tempo:4317"
}

func metricsPort() string {
	if port := os.Getenv("METRICS_PORT"); port != "" {
		return port
	}
	return "2222"
}
