package cmd

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBoolFlag(t *testing.T) {
	tests := []struct {
		name  string
		setTo string
		want  BoolFlag
	}{
		{name: "omitted", want: BoolFlag{}},
		{name: "true", setTo: "true", want: BoolFlag{Set: true, Value: true}},
		{name: "false", setTo: "false", want: BoolFlag{Set: true, Value: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := pflag.NewFlagSet(t.Name(), pflag.ContinueOnError)
			flags.Bool("record-session", false, "")
			if tt.setTo != "" {
				require.NoError(t, flags.Set("record-session", tt.setTo))
			}

			assert.Equal(t, tt.want, readBoolFlag(flags, "record-session"))
		})
	}
}
