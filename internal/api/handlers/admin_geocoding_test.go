package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		seconds  int
		expected string
	}{
		{"under 1 minute", 30, "30s"},
		{"exactly 1 minute", 60, "1m"},
		{"5 minutes", 300, "5m"},
		{"1 hour", 3600, "1h"},
		{"1 hour 30 minutes", 5400, "1h30m"},
		{"2 hours", 7200, "2h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.seconds)
			assert.Equal(t, tt.expected, result)
		})
	}
}
