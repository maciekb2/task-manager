package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
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
		log.Printf("enricher listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("enricher server failed: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down enricher...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("enricher shutdown error: %v", err)
	}
	log.Println("Enricher stopped.")
}

func handleEnrich(w http.ResponseWriter, r *http.Request) {
	tracer := otel.Tracer("enricher")
	ctx, span := tracer.Start(r.Context(), "enricher.handle")
	defer span.End()

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req EnrichRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	span.SetAttributes(
		attribute.Int64("task.priority", int64(req.Priority)),
		attribute.String("task.url", req.URL),
		attribute.String("task.method", req.Method),
	)

	svc := NewEnricherService()
	resp := svc.Enrich(req)

	span.SetAttributes(
		attribute.String("task.category", resp.Category),
		attribute.Int64("task.score", int64(resp.Score)),
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
