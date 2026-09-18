package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
)

const vaultCredentialHelp = `Create credentials from the fields observed on a website.

Do not use credential items to store, collect, or fill credit card data, including
card numbers (PANs), security codes (CVV/CVC), or expiration dates. Use wallet and
card item types for credit cards and payment checkout instead.

First create a vault for the end user and attach it with browsers create --vault.
Use a protected JSON file or stdin, never secret values in shell arguments.
The spec contains description and fields as an ordered array of named definitions.
Inspect the website and list fields in its natural top-to-bottom order because the
user-facing collection form renders that order unchanged. Field types are text,
email, password, and totp; definitions accept name, label, required, sensitive, and value.
Optional label is non-secret display text for the form; it never changes value keys,
updates, or fills. Use a single trimmed line of at most 128 UTF-8 bytes.
Set description to the recognizable site name only, e.g. "Hacker News", not
"Hacker News sign-in credentials". This text is the user-facing form title.
Set sensitive:false explicitly for ordinary usernames and email addresses.
Reserve sensitive:true for secrets such as passwords, API tokens, and TOTP seeds.
Password and totp must be sensitive. Omitted sensitive defaults to true for safety.
Omit required values to receive a collection URL to present to the user.
Poll items get --wait 60 until state.status is ready, then use items invoke fill.
Ready means populated, not a successful login. An agent controlling the browser
can read filled values. TOTP seeds must not be collected through the hosted form.
Get/list output includes definitions, has_value, and explicitly non-sensitive text/email values.
Sensitive values and TOTP seeds are omitted.
Collection URLs are bearer credentials: share only with the intended user.`

func newVaultCredentialsCommand() *cobra.Command {
	group := &cobra.Command{Use: "credentials", Short: "Collect, update, and fill user credentials", Long: vaultCredentialHelp}
	for _, update := range []bool{false, true} {
		name, short := "create", "Create a credential and return its collection URL"
		if update {
			name, short = "update", "Update credential values or description using an expected version"
		}
		cmd := &cobra.Command{Use: name + " <vault> <key> --spec-file <path|->", Short: short, Args: cobra.ExactArgs(2), PreRunE: vaultPreRun, Long: vaultCredentialHelp,
			RunE: func(cmd *cobra.Command, args []string) error {
				data, err := readVaultSpecFile(cmd)
				if err != nil {
					return err
				}
				version, _ := cmd.Flags().GetInt64("version")
				open, _ := cmd.Flags().GetBool("open")
				expectedID, _ := cmd.Flags().GetString("expected-item-id")
				if cmd.Flags().Changed("expected-item-id") && strings.TrimSpace(expectedID) == "" {
					return fmt.Errorf("--expected-item-id must not be empty")
				}
				return getVaultsHandler(cmd).saveCredential(cmd.Context(), args[0], args[1], data, update, version, expectedID, vaultOutput(cmd), open)
			},
		}
		if update {
			cmd.Long += "\nUpdate spec fields are an object keyed by field name, not the ordered array used on create.\nUpdate preserves omitted fields, replaces nonempty string values, and clears supported values with null or an empty string. Clearing a required text/email/password field returns pending_collection; form submissions still require a nonempty value.\nField definitions are immutable. Do not automatically retry version conflicts."
			cmd.Flags().Int64("version", 0, "Expected version from items get (required; never auto-refreshed)")
			_ = cmd.MarkFlagRequired("version")
			cmd.Flags().String("expected-item-id", "", "Immutable item ID from the original read; reject an update if the key now refers to a replacement item")
			cmd.Example = "  kernel vaults credentials update user-vault login --version 2 --spec-file changes.json"
		} else {
			cmd.Example = `  kernel vaults credentials create user-vault login --spec-file - <<'JSON'
{"description":"Hacker News","fields":[{"name":"username","type":"text","required":true,"sensitive":false},{"name":"password","type":"password","required":true,"sensitive":true}]}
JSON`
		}
		cmd.Flags().String("spec-file", "", "Credential spec JSON file (use '-' for stdin; maximum 128 KiB)")
		_ = cmd.MarkFlagRequired("spec-file")
		cmd.Flags().Bool("open", false, "Open the returned HTTPS collection URL")
		addVaultJSONOutputFlag(cmd)
		group.AddCommand(cmd)
	}
	return group
}

func readVaultSpecFile(cmd *cobra.Command) ([]byte, error) {
	path, _ := cmd.Flags().GetString("spec-file")
	if path == "" {
		return nil, fmt.Errorf("--spec-file is required (use '-' for stdin)")
	}
	var reader io.Reader = cmd.InOrStdin()
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("could not open --spec-file")
		}
		defer f.Close()
		reader = f
	}
	const limit = 128 * 1024
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || len(data) > limit {
		return nil, fmt.Errorf("could not read --spec-file (maximum 128 KiB)")
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return nil, fmt.Errorf("--spec-file must contain a JSON object")
	}
	return data, nil
}

func (c VaultsCmd) saveCredential(ctx context.Context, vault, key string, data []byte, update bool, version int64, expectedID, output string, open bool) error {
	var item *kernel.VaultItemUnion
	var err error
	if update {
		if version < 1 {
			return fmt.Errorf("--version must be positive")
		}
		var spec kernel.CredentialVaultItemSpecUpdateParam
		if json.Unmarshal(data, &spec) != nil {
			return fmt.Errorf("invalid credential update spec")
		}
		request := kernel.CredentialVaultItemUpdateRequestParam{Type: "credential", Version: version, Spec: spec}
		if expectedID != "" {
			request.ExpectedItemID = kernel.String(expectedID)
		}
		item, err = c.vaults.Items.Update(ctx, key, kernel.VaultItemUpdateParams{IDOrName: vault, OfCredentialVaultItemUpdateRequest: &request}, option.WithMaxRetries(0))
	} else {
		var spec kernel.CredentialVaultItemSpecInputParam
		if json.Unmarshal(data, &spec) != nil || len(spec.Fields) == 0 {
			if credentialSpecUsesKeyedFields(data) {
				return fmt.Errorf("credential spec fields must be an ordered array of definitions carrying a name, not an object keyed by name")
			}
			return fmt.Errorf("credential spec requires fields")
		}
		// Names key values, updates, and fills; reject specs the form cannot address.
		for _, field := range spec.Fields {
			if strings.TrimSpace(field.Name) == "" {
				return fmt.Errorf("every credential spec field requires a name")
			}
		}
		item, err = c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{IDOrName: vault, OfCredential: &kernel.CredentialVaultItemRequestParam{Type: "credential", Spec: spec}}, option.WithMaxRetries(0))
	}
	if err != nil {
		return vaultCredentialError(err)
	}
	return c.showItem(item, output, open)
}

// The create spec moved from fields keyed by name to an ordered array; point
// callers still sending the object form at the replacement shape.
func credentialSpecUsesKeyedFields(data []byte) bool {
	var object struct {
		Fields json.RawMessage `json:"fields"`
	}
	if json.Unmarshal(data, &object) != nil {
		return false
	}
	fields := bytes.TrimSpace(object.Fields)
	return len(fields) > 0 && fields[0] == '{'
}
