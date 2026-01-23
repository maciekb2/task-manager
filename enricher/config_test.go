package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPort(t *testing.T) {
	os.Setenv("ENRICHER_PORT", "9090")
	defer os.Unsetenv("ENRICHER_PORT")
	assert.Equal(t, "9090", port())

	os.Unsetenv("ENRICHER_PORT")
	assert.Equal(t, "8080", port())
}
