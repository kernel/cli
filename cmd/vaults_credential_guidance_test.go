package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialHumanGuidance(t *testing.T) {
	for _, operation := range []string{"create", "get", "collect"} {
		for _, status := range []string{"pending_collection", "ready"} {
			t.Run(operation+"/"+status, func(t *testing.T) {
				fixture := strings.Replace(credentialFixture, "pending_collection", status, 1)
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, fixture)
				})
				args := []string{"vaults", "items", "get", "user", "login"}
				switch operation {
				case "create":
					args = []string{"vaults", "credentials", "create", "user", "login", "--spec-file", credentialSpecFile(t, `{"fields":{"password":{"type":"password"}}}`)}
				case "collect":
					args = []string{"vaults", "items", "invoke", "user", "login", "collect"}
				}
				stdout, human, err := executeVaultCommand(t, client, args...)
				require.NoError(t, err)
				output := stdout + human
				assert.Contains(t, output, "Share the collection URL with the user")
				assert.Contains(t, output, "items get --wait 60")
				assert.Contains(t, output, "compare versions without --wait")
				assert.Contains(t, output, "not that login succeeded")
				assert.Contains(t, output, "Do not use credential items for credit card data")
				assert.Contains(t, output, "Use wallet and card item types")
				assert.Contains(t, output, "Available operation: fill")
				assert.NotContains(t, output, "with the provider")
				assert.NotContains(t, output, "OAuth codes")
				assert.NotContains(t, output, "never-print")
			})
		}
	}
}
