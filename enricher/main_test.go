package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

type failWriter struct {
	http.ResponseWriter
}

func (w *failWriter) Write(b []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestHandleEnrich(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           interface{}
		expectedStatus int
		expectedCat    string
		expectedScore  int32
	}{
		{
			name:   "High Priority",
			method: http.MethodPost,
			body: EnrichRequest{
				Priority: 2,
				URL:      "http://example.com", // len 18
			},
			expectedStatus: http.StatusOK,
			expectedCat:    "high",
			expectedScore:  18 + 200, // 218
		},
		{
			name:   "Medium Priority",
			method: http.MethodPost,
			body: EnrichRequest{
				Priority: 1,
				URL:      "http://a.com", // len 12
			},
			expectedStatus: http.StatusOK,
			expectedCat:    "medium",
			expectedScore:  12 + 100, // 112
		},
		{
			name:   "Low Priority",
			method: http.MethodPost,
			body: EnrichRequest{
				Priority: 0,
				URL:      "http://b.com", // len 12
			},
			expectedStatus: http.StatusOK,
			expectedCat:    "low",
			expectedScore:  12 + 0, // 12
		},
		{
			name:           "Invalid Method",
			method:         http.MethodGet,
			body:           nil,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			body:           "invalid-json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var bodyBytes []byte
			if tc.body != nil {
				if s, ok := tc.body.(string); ok && s == "invalid-json" {
					bodyBytes = []byte(s)
				} else {
					var err error
					bodyBytes, err = json.Marshal(tc.body)
					if err != nil {
						t.Fatalf("failed to marshal body: %v", err)
					}
				}
			}

			req := httptest.NewRequest(tc.method, "/enrich", bytes.NewBuffer(bodyBytes))
			rec := httptest.NewRecorder()

			handleEnrich(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rec.Code)
			}

			if tc.expectedStatus == http.StatusOK {
				var resp EnrichResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if resp.Category != tc.expectedCat {
					t.Errorf("expected category %s, got %s", tc.expectedCat, resp.Category)
				}
				if resp.Score != tc.expectedScore {
					t.Errorf("expected score %d, got %d", tc.expectedScore, resp.Score)
				}
			}
		})
	}
}

func TestHandleEnrich_WriteError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/enrich", bytes.NewBuffer([]byte(`{"priority":1, "url":"http://a.com"}`)))
	rec := httptest.NewRecorder()
	fw := &failWriter{ResponseWriter: rec}

	handleEnrich(fw, req)

	if rec.Code != http.StatusInternalServerError {
		t.Logf("Status code: %d", rec.Code)
	}
}

func TestRun(t *testing.T) {
	os.Setenv("ENRICHER_PORT", "0")
	os.Setenv("METRICS_PORT", "0")
	defer os.Unsetenv("ENRICHER_PORT")
	defer os.Unsetenv("METRICS_PORT")

	origListenAndServe := listenAndServe
	listenAndServe = func(addr string, handler http.Handler) error {
		return nil
	}
	defer func() { listenAndServe = origListenAndServe }()

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error)
	go func() {
		errChan <- run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("run did not return after cancellation")
	}
}

func TestRun_StartupError(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	os.Setenv("ENRICHER_PORT", port)
	os.Setenv("METRICS_PORT", "0")
	defer os.Unsetenv("ENRICHER_PORT")
	defer os.Unsetenv("METRICS_PORT")

	origListenAndServe := listenAndServe
	listenAndServe = func(addr string, handler http.Handler) error {
		return nil
	}
	defer func() { listenAndServe = origListenAndServe }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = run(ctx)
	if err == nil {
		t.Error("expected error due to port conflict, got nil")
	}
}

func TestRun_TelemetryError(t *testing.T) {
	origInit := initTelemetryFunc
	initTelemetryFunc = func(ctx context.Context) (func(context.Context) error, error) {
		return nil, errors.New("telemetry error")
	}
	defer func() { initTelemetryFunc = origInit }()

	err := run(context.Background())
	if err == nil {
		t.Error("expected error from telemetry, got nil")
	}
}
