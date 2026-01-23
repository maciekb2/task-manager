package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
