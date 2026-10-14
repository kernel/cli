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
				"attachment is required for authorizing a Link card at a checkout and for fill",
				"ready Link and kernel cards use only advertised fill",
				"neither exposes aliases or supports egress substitution",
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
				"the vault must already be attached to the browser",
				"fill is available for credential items and ready Link or kernel cards when advertised, not AgentCard",
				"Link and kernel cards do not expose aliases or support egress substitution",
				"fill writes real values into the browser; unrestricted browser/cdp access can read them",
				"fill never explicitly submits forms or clicks buttons",
				"input/change events may trigger site behavior",
				"not website acceptance, login, or payment success",
				"failed may leave partial writes; unknown quarantines the browser",
				"never in an automatic loop. never fall back to aliases",
				"requests are not automatically retried",
				"use only when advertised for an AgentCard card",
				"report one observed merchant outcome",
				"unknown means no reliable merchant outcome",
				"restart_verification takes no parameters",
				"kernel cards have no stored billing fields",
				"billing_* returns field_unavailable (400) before any write",
				"enter billing details on the merchant page",
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
			if tt.name == "items invoke" {
				for _, want := range []string{"confirm_transaction --params", "restart_verification --open", `"field":"number","selector":"#cardNumber"`, `"field":"expiration","format":"MM/YY","selector":"#cardExpiry"`, `"field":"cvc","selector":"#cardCvc"`} {
					assert.Contains(t, cmd.Example, want)
				}
			}
			assert.NotContains(t, text, "does not observe merchant charges")
			assert.NotContains(t, text, "never replace an item in recovery_required")
			assert.NotContains(t, text, "aliases are an alternative")
			assert.NotContains(t, text, "explicitly chosen egress-substitution")
		})
	}
}
