package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maciekb2/task-manager/pkg/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type enrichRequest struct {
	TaskDescription string `json:"task_description"`
	Priority        int32  `json:"priority"`
	URL             string `json:"url"`
	Method          string `json:"method"`
}

type enrichResponse struct {
	Category string `json:"category"`
	Score    int32  `json:"score"`
}

func main() {
	logger.Setup("enricher")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		logger.Fatal("telemetry init failed", err)
	}
	defer shutdown(ctx)

	mux := http.NewServeMux()
	mux.Handle("/enrich", otelhttp.NewHandler(http.HandlerFunc(handleEnrich), "enricher.http"))

	addr := ":" + port()
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		slog.Info("enricher listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("enricher server failed", err)
		}
	}()

	<-ctx.Done()
	slog.Info("Shutting down enricher...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("enricher shutdown error", err)
	}
	slog.Info("Enricher stopped.")
}

func handleEnrich(w http.ResponseWriter, r *http.Request) {
	tracer := otel.Tracer("enricher")
	ctx, span := tracer.Start(r.Context(), "enricher.handle")
	defer span.End()

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req enrichRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	span.SetAttributes(
		attribute.Int64("task.priority", int64(req.Priority)),
		attribute.String("task.url", req.URL),
		attribute.String("task.method", req.Method),
	)

	category := "low"
	if req.Priority >= 2 {
		category = "high"
	} else if req.Priority == 1 {
		category = "medium"
	}

	score := int32(len(req.URL)) + req.Priority*100
	resp := enrichResponse{Category: category, Score: score}
	span.SetAttributes(
		attribute.String("task.category", category),
		attribute.Int64("task.score", int64(score)),
	)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		_ = ctx
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func port() string {
	if value := os.Getenv("ENRICHER_PORT"); value != "" {
		return value
	}
	return "8080"
}
