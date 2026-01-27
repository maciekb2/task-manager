package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maciekb2/task-manager/pkg/flow"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
)

func TestHttpEnricher_Enrich(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse string
		serverStatus   int
		wantErr        bool
		wantCategory   string
		wantScore      int32
	}{
		{
			name:           "Success",
			serverResponse: `{"category": "finance", "score": 85}`,
			serverStatus:   http.StatusOK,
			wantErr:        false,
			wantCategory:   "finance",
			wantScore:      85,
		},
		{
			name:           "ServerError",
			serverResponse: "",
			serverStatus:   http.StatusInternalServerError,
			wantErr:        true,
		},
		{
			name:           "InvalidJSON",
			serverResponse: `{"category": "finance", "score": "invalid"}`,
			serverStatus:   http.StatusOK,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			enricher := NewHttpEnricher(server.Client(), server.URL)
			task := flow.TaskEnvelope{
				TaskID: "123",
			}

			got, err := enricher.Enrich(context.Background(), task)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCategory, got.Category)
				assert.Equal(t, tt.wantScore, got.Score)
			}
		})
	}
}

func TestNatsMessageAdapter_Getters(t *testing.T) {
	data := []byte("payload")
	msg := &nats.Msg{
		Subject: "test.subject",
		Data:    data,
		Header:  nats.Header{"key": []string{"val"}},
	}
	adapter := NewNatsMessageAdapter(msg)

	assert.Equal(t, data, adapter.GetData())
	assert.Equal(t, "test.subject", adapter.GetSubject())
	assert.Equal(t, nats.Header{"key": []string{"val"}}, adapter.GetHeaders())
}
