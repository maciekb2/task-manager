package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNatsURL(t *testing.T) {
	os.Setenv("NATS_URL", "nats://foo:4222")
	defer os.Unsetenv("NATS_URL")
	assert.Equal(t, "nats://foo:4222", natsURL())

	os.Unsetenv("NATS_URL")
	assert.Equal(t, "nats://nats:4222", natsURL())
}
