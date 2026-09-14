package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
)

const vaultCredentialHelp = `Create credentials from the sensitive fields observed on a website.
First create a vault for the end user and attach it with browsers create --vault.
Use a protected JSON file or stdin, never secret values in shell arguments.
The spec contains description and fields keyed by name. Field types are text,
email, password, and totp; definitions accept required, sensitive, and value.
Omit required values to receive a collection URL to present to the user.
Poll items get --wait 60 until state.status is ready, then use items invoke fill.
Ready means populated, not a successful login. An agent controlling the browser
can read filled values. TOTP seeds must not be collected through the hosted form.
Get/list output includes definitions and has_value, not stored field values.
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
				return getVaultsHandler(cmd).saveCredential(cmd.Context(), args[0], args[1], data, update, version, vaultOutput(cmd), open)
			},
		}
		if update {
			cmd.Long += "\nUpdate preserves omitted fields, replaces string values, and clears supported values with null.\nField definitions are immutable. Do not automatically retry version conflicts."
			cmd.Flags().Int64("version", 0, "Expected version from items get (required; never auto-refreshed)")
			_ = cmd.MarkFlagRequired("version")
			cmd.Example = "  kernel vaults credentials update user-vault login --version 2 --spec-file changes.json"
		} else {
			cmd.Example = `  kernel vaults credentials create user-vault login --spec-file - <<'JSON'
{"fields":{"username":{"type":"email","required":true},"password":{"type":"password","required":true}}}
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

func (c VaultsCmd) saveCredential(ctx context.Context, vault, key string, data []byte, update bool, version int64, output string, open bool) error {
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
		item, err = c.vaults.Items.Update(ctx, key, kernel.VaultItemUpdateParams{IDOrName: vault, OfCredential: &kernel.CredentialVaultItemUpdateRequestParam{Type: "credential", Version: version, Spec: spec}}, option.WithMaxRetries(0))
	} else {
		var spec kernel.CredentialVaultItemSpecInputParam
		if json.Unmarshal(data, &spec) != nil || len(spec.Fields) == 0 {
			return fmt.Errorf("credential spec requires fields")
		}
		item, err = c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{IDOrName: vault, OfCredential: &kernel.CredentialVaultItemRequestParam{Type: "credential", Spec: spec}}, option.WithMaxRetries(0))
	}
	if err != nil {
		return vaultCredentialError(err)
	}
	return c.showItem(item, output, open)
}

func vaultFillError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) {
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(apiErr.RawJSON()), &body) == nil {
			switch body.Code {
			case "invalid_request", "invalid_selector", "duplicate_target", "timeout", "target_changed", "page_not_found", "ambiguous_page", "element_not_found", "ambiguous_selector", "element_not_editable", "option_not_found", "field_unavailable", "conflict", "destination_denied", "execution_failed":
				return fmt.Errorf("fill failed: %s (HTTP %d); inspect the browser and item before further action; do not automatically retry", body.Code, apiErr.StatusCode)
			}
		}
	}
	return vaultCredentialError(err)
}

func printVaultFill(result *kernel.VaultItemOperationResponseUnion, expectedFields int) error {
	raw, err := filterVaultJSON(json.RawMessage(result.RawJSON()), vaultOutputFields{"type": nil, "status": nil, "fields": vaultFieldsOf("index status error_code")})
	if err != nil {
		return fmt.Errorf("invalid fill response")
	}
	var outcome kernel.FillVaultItemOperationResult
	if json.Unmarshal(raw, &outcome) != nil || outcome.Type != "fill" || len(outcome.Fields) != expectedFields {
		return fmt.Errorf("invalid fill response")
	}
	switch outcome.Status {
	case "completed", "failed", "unknown":
	default:
		return fmt.Errorf("invalid fill response")
	}
	for i, field := range outcome.Fields {
		if field.Index != int64(i) || (outcome.Status == "completed" && field.Status != "filled") {
			return fmt.Errorf("invalid fill response")
		}
		switch field.Status {
		case "filled", "failed", "not_attempted", "unknown":
		default:
			return fmt.Errorf("invalid fill response")
		}
		switch field.ErrorCode {
		case "", "target_changed", "element_not_found", "ambiguous_selector", "element_not_editable", "option_not_found", "timeout", "execution_failed":
		default:
			return fmt.Errorf("invalid fill response")
		}
	}
	if err := printVaultJSON(raw); err != nil {
		return err
	}
	if outcome.Status != "completed" {
		return fmt.Errorf("fill did not complete; inspect field outcomes and do not automatically retry or fall back")
	}
	return nil
}
