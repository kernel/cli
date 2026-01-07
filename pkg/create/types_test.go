package create

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ts", "typescript"},
		{"py", "python"},
		{"typescript", "typescript"},
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeLanguage(tt.input)
			assert.Equal(t, tt.expected, got, "NormalizeLanguage(%q) should return %q, got %q", tt.input, tt.expected, got)
		})
	}
}
