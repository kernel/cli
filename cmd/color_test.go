package cmd

import "testing"

func TestShouldEnableColor(t *testing.T) {
	tests := []struct {
		name       string
		noColorEnv string
		isTTY      bool
		want       bool
	}{
		{"tty, no env", "", true, true},
		{"not tty, no env", "", false, false},
		{"tty, NO_COLOR set", "1", true, false},
		{"not tty, NO_COLOR set", "1", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldEnableColor(tt.noColorEnv, tt.isTTY); got != tt.want {
				t.Errorf("shouldEnableColor(%q, %v) = %v, want %v", tt.noColorEnv, tt.isTTY, got, tt.want)
			}
		})
	}
}
