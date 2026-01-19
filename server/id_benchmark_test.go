package main

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/google/uuid"
)

// BenchmarkIDGeneration_Legacy measures the performance of the old weak ID generation.
func BenchmarkIDGeneration_Legacy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = fmt.Sprintf("%d", rand.Int())
	}
}

// BenchmarkIDGeneration_Current measures the performance of the new strong ID generation (UUID v4).
func BenchmarkIDGeneration_Current(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = uuid.NewString()
	}
}

func BenchmarkIDGeneration_Legacy_Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = fmt.Sprintf("%d", rand.Int())
		}
	})
}

func BenchmarkIDGeneration_Current_Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = uuid.NewString()
		}
	})
}
