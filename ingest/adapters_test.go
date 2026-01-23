package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
)

func TestNatsMessageAdapter(t *testing.T) {
	data := []byte("test")
	header := nats.Header{"foo": []string{"bar"}}
	msg := &nats.Msg{
		Subject: "test.subject",
		Data:    data,
		Header:  header,
	}
	adapter := NewNatsMessageAdapter(msg)

	if string(adapter.GetData()) != "test" {
		t.Error("GetData mismatch")
	}
	if adapter.GetSubject() != "test.subject" {
		t.Error("GetSubject mismatch")
	}
	if adapter.GetHeaders().Get("foo") != "bar" {
		t.Error("GetHeaders mismatch")
	}

	_ = adapter.Ack()
	_ = adapter.Nak()
	_, _ = adapter.Metadata()
}

func TestHttpEnricher_Enrich(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req flow.TaskEnvelope
		json.NewDecoder(r.Body).Decode(&req)

		resp := enrichResponse{
			Category: "high",
			Score:    100,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := ts.Client()
	enricher := NewHttpEnricher(client, ts.URL)

	task := flow.TaskEnvelope{TaskID: "1"}
	enriched, err := enricher.Enrich(context.Background(), task)
	if err != nil {
		t.Fatalf("enrich failed: %v", err)
	}

	if enriched.Category != "high" {
		t.Errorf("expected category high, got %s", enriched.Category)
	}
	if enriched.Score != 100 {
		t.Errorf("expected score 100, got %d", enriched.Score)
	}
}

func TestHttpEnricher_Enrich_Failure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := ts.Client()
	enricher := NewHttpEnricher(client, ts.URL)

	task := flow.TaskEnvelope{TaskID: "1"}
	_, err := enricher.Enrich(context.Background(), task)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestHttpEnricher_Enrich_ClientError(t *testing.T) {
	// Invalid URL
	enricher := NewHttpEnricher(&http.Client{}, "http://invalid-url-@@@")
	_, err := enricher.Enrich(context.Background(), flow.TaskEnvelope{})
	if err == nil {
		t.Error("expected error on new request, got nil")
	}

	// Request fails (closed server)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close() // Close immediately

	enricher2 := NewHttpEnricher(ts.Client(), ts.URL)
	_, err = enricher2.Enrich(context.Background(), flow.TaskEnvelope{})
	if err == nil {
		t.Error("expected error on do, got nil")
	}
}

func TestHttpEnricher_Enrich_DecodeError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	}))
	defer ts.Close()

	enricher := NewHttpEnricher(ts.Client(), ts.URL)
	_, err := enricher.Enrich(context.Background(), flow.TaskEnvelope{})
	if err == nil {
		t.Error("expected error on decode, got nil")
	}
}
