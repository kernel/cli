package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultPaymentHelpProviderBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name string
		path []string
		want []string
	}{
		{
			name: "vaults",
			want: []string{
				"attachment is required for fill",
				"Ready Link cards use only advertised fill",
				"Link cards do not expose aliases or support egress substitution",
				"AgentCard-only checkout aliases support egress substitution with checkout hold, approval, and replay",
				"not a fallback after fill",
			},
		},
		{
			name: "items get",
			path: []string{"items", "get"},
			want: []string{"returned AgentCard checkout aliases", "not logged in or paid"},
		},
		{
			name: "items invoke",
			path: []string{"items", "invoke"},
			want: []string{
				"The vault must already be attached to the browser",
				"Fill is available for credential items and ready Link cards when advertised, not AgentCard",
				"Link cards do not expose aliases or support egress substitution",
				"Fill writes real values into the browser; unrestricted browser/CDP access can read them",
				"Fill never explicitly submits forms or clicks buttons",
				"input/change events may trigger site behavior",
				"not website acceptance, login, or payment success",
				"failed may leave partial writes; unknown quarantines the browser",
				"Never automatically retry or fall back to aliases",
				"Requests are not automatically retried",
				"Use only when advertised for an AgentCard card",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, err := newVaultsCommand().Find(tt.path)
			require.NoError(t, err)
			text := strings.Join(strings.Fields(cmd.Long), " ")
			for _, want := range tt.want {
				assert.Contains(t, text, want)
			}
			assert.NotContains(t, text, "aliases are an alternative")
			assert.NotContains(t, text, "explicitly chosen egress-substitution")
		})
	}
}
