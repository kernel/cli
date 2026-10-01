package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultRecoveryActionDisplayPolicy(t *testing.T) {
	for _, status := range []string{"requested", "recovery_required"} {
		for _, command := range []string{"get", "list"} {
			for _, output := range []string{"", "json"} {
				t.Run(status+"/"+command+"/"+output, func(t *testing.T) {
					body := strings.ReplaceAll(requestedCardFixture, `"status":"requested"`, `"status":"`+status+`"`)
					var fields map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(body), &fields))
					fields["action"] = json.RawMessage(`{"name":"spend_approval","url":"https://example.test/approve"}`)
					bodyBytes, err := json.Marshal(fields)
					require.NoError(t, err)
					body = string(bodyBytes)
					if command == "list" {
						body = "[" + body + "]"
					}
					client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, body)
					})
					args := []string{"vaults", "items", command, "checkout"}
					if command == "get" {
						args = append(args, "order-1")
					}
					if output != "" {
						args = append(args, "-o", output)
					}
					out, human, err := executeVaultInputCommand(t, client, "", args...)
					require.NoError(t, err)
					if output == "json" {
						assert.Contains(t, out, `"spend_approval"`)
						assert.Contains(t, out, `"available_operations"`)
						assert.Contains(t, out, `"authorize"`)
						assert.Empty(t, human)
					} else if status == "recovery_required" {
						assert.NotContains(t, human, "spend_approval")
						assert.NotContains(t, human, "Required action")
						assert.NotContains(t, human, "https://example.test/approve")
						assert.NotContains(t, human, "Available operation:")
						assert.NotContains(t, human, "Invoke:")
					} else {
						assert.Contains(t, human, "spend_approval")
					}
				})
			}
		}
	}
}
