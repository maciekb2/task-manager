package main

import "testing"

func TestEnricherService_Enrich(t *testing.T) {
	svc := NewEnricherService()

	tests := []struct {
		name     string
		req      EnrichRequest
		expected EnrichResponse
	}{
		{
			name: "High Priority",
			req: EnrichRequest{
				Priority: 2,
				URL:      "http://example.com", // len 18
			},
			expected: EnrichResponse{
				Category: "high",
				Score:    218,
			},
		},
		{
			name: "Medium Priority",
			req: EnrichRequest{
				Priority: 1,
				URL:      "http://a.com", // len 12
			},
			expected: EnrichResponse{
				Category: "medium",
				Score:    112,
			},
		},
		{
			name: "Low Priority",
			req: EnrichRequest{
				Priority: 0,
				URL:      "http://b.com", // len 12
			},
			expected: EnrichResponse{
				Category: "low",
				Score:    12,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := svc.Enrich(tt.req)
			if resp.Category != tt.expected.Category {
				t.Errorf("expected category %s, got %s", tt.expected.Category, resp.Category)
			}
			if resp.Score != tt.expected.Score {
				t.Errorf("expected score %d, got %d", tt.expected.Score, resp.Score)
			}
		})
	}
}
