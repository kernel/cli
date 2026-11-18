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
				"attachment is required for authorizing a Link or Shop Pay card at a checkout and for every fill, including VGS",
				"ready Link, KERNEL, Shop Pay, and VGS cards use only advertised fill",
				"VGS outbound proxy replaces",
				"the CLI never receives the card",
				"AgentCard-only checkout aliases support egress substitution with checkout hold, approval, and replay",
				"not a fallback after fill",
			},
		},
		{
			name: "items get",
			path: []string{"items", "get"},
			want: []string{
				"returned AgentCard checkout aliases",
				"not logged in or paid",
				"credential_account in revoking",
				"amount_authority identifies the approved amount's source",
				"amount_verified and charged_* may be absent",
				"charged_kind none means the checkout produced a token or payment method",
			},
		},
		{
			name: "items invoke",
			path: []string{"items", "invoke"},
			want: []string{
				"the vault must already be attached to the browser",
				"fill is available for credential items and ready Link, KERNEL, Shop Pay, or VGS cards when advertised, not AgentCard",
				"Link, KERNEL, and Shop Pay cards do not expose aliases or support egress substitution",
				"VGS cards also expose no aliases to the CLI",
				"fill writes real credential or card values into the browser except for VGS",
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
				"payment-method-funded KERNEL cards and KERNEL payment_request items have no stored billing fields",
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

func TestVaultDeletionAndDeletedEventHelp(t *testing.T) {
	vaultDelete, _, err := newVaultsCommand().Find([]string{"delete"})
	require.NoError(t, err)
	assert.Contains(t, vaultDelete.Long, "Each 1Password credential account is ended")
	assert.Contains(t, vaultDelete.Long, "provider_revocation_pending")
	assert.Contains(t, vaultDelete.Long, "delete the vault again later")

	itemDelete, _, err := newVaultsCommand().Find([]string{"items", "delete"})
	require.NoError(t, err)
	assert.Contains(t, itemDelete.Long, "AgentCard card in recovery_required")
	assert.Contains(t, itemDelete.Long, "Deleting a managed-auth credential")
	assert.Contains(t, itemDelete.Long, "Deleting a 1Password credential account stops fills immediately")
	assert.Contains(t, itemDelete.Flags().Lookup("yes").Usage, "qualifying AgentCard recovery")

	events, _, err := newVaultsCommand().Find([]string{"items", "events"})
	require.NoError(t, err)
	assert.Contains(t, events.Long, "deleted vaults must be addressed by ID")
	assert.Contains(t, events.Long, "item IDs take precedence over keys")
	assert.Contains(t, events.Flags().Lookup("include-deleted").Usage, "--wait is ignored")
}
