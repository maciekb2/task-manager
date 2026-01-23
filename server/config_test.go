package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedisAddr(t *testing.T) {
	os.Setenv("REDIS_ADDR", "localhost:6379")
	defer os.Unsetenv("REDIS_ADDR")
	assert.Equal(t, "localhost:6379", redisAddr())

	os.Unsetenv("REDIS_ADDR")
	assert.Equal(t, "redis-service:6379", redisAddr())
}

func TestNatsURL(t *testing.T) {
	os.Setenv("NATS_URL", "nats://foo:4222")
	defer os.Unsetenv("NATS_URL")
	assert.Equal(t, "nats://foo:4222", natsURL())

	os.Unsetenv("NATS_URL")
	assert.Equal(t, "nats://nats:4222", natsURL())
}
