package bus

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func BenchmarkProcessBatch(b *testing.B) {
	// Create a dummy client
	client := &Client{}

	// Create a batch of messages
	batchSize := 10
	msgs := make([]*nats.Msg, batchSize)
	for i := 0; i < batchSize; i++ {
		msgs[i] = &nats.Msg{
			Subject: "test.subject",
			Data:    []byte("test payload"),
			Header:  nats.Header{},
		}
	}

	opts := ConsumeOptions{
		DisableAutoAck: true, // prevent Ack() calls which would fail on dummy messages
	}
	retry := RetryPolicy{}

	// Handler that simulates work
	handler := func(ctx context.Context, msg *nats.Msg) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.processBatch(context.Background(), msgs, opts, retry, handler)
	}
}
