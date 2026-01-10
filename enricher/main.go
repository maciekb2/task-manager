package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type enrichRequest struct {
	TaskDescription string `json:"task_description"`
	Priority        int32  `json:"priority"`
	Number1         int32  `json:"number1"`
	Number2         int32  `json:"number2"`
}

type enrichResponse struct {
	Category string `json:"category"`
	Score    int32  `json:"score"`
}

func main() {
	ctx := context.Background()
	shutdown, err := initTelemetry(ctx)
	if err != nil {
		log.Fatalf("telemetry init failed: %v", err)
	}
	defer shutdown(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/enrich", handleEnrich)

	addr := ":" + port()
	log.Printf("enricher listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("enricher server failed: %v", err)
	}
}

func handleEnrich(w http.ResponseWriter, r *http.Request) {
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(r.Header))
	tracer := otel.Tracer("enricher")
	ctx, span := tracer.Start(ctx, "enricher.handle")
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

	category := "low"
	if req.Priority >= 2 {
		category = "high"
	} else if req.Priority == 1 {
		category = "medium"
	}

	score := req.Number1 + req.Number2 + req.Priority*100
	resp := enrichResponse{Category: category, Score: score}

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
