package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOtelEndpoint(t *testing.T) {
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	defer os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	assert.Equal(t, "localhost:4317", otelEndpoint())

	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	assert.Equal(t, "tempo:4317", otelEndpoint())
}

func TestMetricsPort(t *testing.T) {
	os.Setenv("METRICS_PORT", "9999")
	defer os.Unsetenv("METRICS_PORT")
	assert.Equal(t, "9999", metricsPort())

	os.Unsetenv("METRICS_PORT")
	assert.Equal(t, "2222", metricsPort())
}

func TestServerOpts(t *testing.T) {
	opts := serverOpts()
	assert.Len(t, opts, 2)
}

func TestInitTelemetry(t *testing.T) {
	// Mock listenAndServe
	origListenAndServe := listenAndServe
	listenAndServe = func(addr string, handler http.Handler) error {
		return nil
	}
	defer func() { listenAndServe = origListenAndServe }()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, shutdown)

	shutdown(ctx)
}
