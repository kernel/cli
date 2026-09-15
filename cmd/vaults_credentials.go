package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/spf13/cobra"
)

var vaultCredentialFieldTypes = []string{"text", "email", "password", "totp"}

// CreateCredential stores a credential item without a wallet or provider. Values
// arrive through a file so secrets never appear in shell arguments; omitted
// required values leave the item pending_collection with a hosted form action.
func (c VaultsCmd) CreateCredential(ctx context.Context, vault, key string, spec map[string]json.RawMessage, output string, open bool) error {
	item, err := c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{
		IDOrName: vault,
		OfCredential: &kernel.CredentialVaultItemRequestParam{
			Type: kernel.CredentialVaultItemRequestTypeCredential,
			Spec: param.Override[kernel.CredentialVaultItemSpecInputParam](spec),
		},
	}, option.WithMaxRetries(0))
	if err != nil {
		return vaultCredentialItemError(err)
	}
	return c.showItem(item, output, open)
}

// UpdateCredential atomically replaces selected values and the description.
// --version is the expected current item version, so a concurrent edit returns
// 409 instead of silently overwriting it.
func (c VaultsCmd) UpdateCredential(ctx context.Context, vault, key string, version int64, expectedItemID string, spec map[string]json.RawMessage, output string, open bool) error {
	request := kernel.CredentialVaultItemUpdateRequestParam{
		Type:    kernel.CredentialVaultItemUpdateRequestTypeCredential,
		Version: version,
		Spec:    param.Override[kernel.CredentialVaultItemSpecUpdateParam](spec),
	}
	if expectedItemID != "" {
		request.ExpectedItemID = kernel.Opt(expectedItemID)
	}
	item, err := c.vaults.Items.Update(ctx, key, kernel.VaultItemUpdateParams{IDOrName: vault, OfCredentialVaultItemUpdateRequest: &request}, option.WithMaxRetries(0))
	if err != nil {
		return vaultCredentialItemError(err)
	}
	return c.showItem(item, output, open)
}

// The spec is forwarded without defaults or normalization, like card and wallet
// specs. Only the shape the CLI must reason about is checked here.
func vaultCredentialSpecFromFlags(cmd *cobra.Command) (map[string]json.RawMessage, error) {
	raw, _ := cmd.Flags().GetString("spec")
	var spec map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &spec) != nil || spec == nil {
		return nil, fmt.Errorf("--spec must be a JSON object with fields and an optional description")
	}
	for key := range spec {
		if key != "fields" && key != "description" {
			return nil, fmt.Errorf("--spec supports only description and fields")
		}
	}
	fields, err := vaultCredentialFieldsFromSpec(spec["fields"])
	if err != nil {
		return nil, err
	}
	if cmd.Flags().Changed("values-file") {
		values, err := readVaultFieldValues(cmd, "values-file")
		if err != nil {
			return nil, err
		}
		if err := vaultCredentialApplyValues(fields, values); err != nil {
			return nil, err
		}
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	spec["fields"] = encoded
	return spec, nil
}

func vaultCredentialFieldsFromSpec(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if raw == nil || json.Unmarshal(raw, &fields) != nil || len(fields) < 1 || len(fields) > 32 {
		return nil, fmt.Errorf("--spec.fields must be a JSON object declaring 1-32 fields")
	}
	for name, raw := range fields {
		if !vaultFieldNamePattern.MatchString(name) {
			return nil, fmt.Errorf("--spec.fields names must match [a-zA-Z][a-zA-Z0-9_]{0,63}")
		}
		var field map[string]json.RawMessage
		if json.Unmarshal(raw, &field) != nil || field == nil {
			return nil, fmt.Errorf("--spec.fields[%q] must be a JSON object", name)
		}
		if _, ok := field["value"]; ok {
			return nil, fmt.Errorf("--spec must not contain field values; supply them with --values-file")
		}
		var fieldType string
		if json.Unmarshal(field["type"], &fieldType) != nil || !slices.Contains(vaultCredentialFieldTypes, fieldType) {
			return nil, fmt.Errorf("--spec.fields[%q].type must be text, email, password, or totp", name)
		}
	}
	return fields, nil
}

// Creation rejects null and empty values, so refuse them before sending a
// request that cannot succeed.
func vaultCredentialApplyValues(fields map[string]json.RawMessage, values map[string]*string) error {
	for _, name := range sortedVaultFieldNames(values) {
		raw, declared := fields[name]
		if !declared {
			return fmt.Errorf("--values-file field %q is not declared in --spec.fields", name)
		}
		if values[name] == nil || *values[name] == "" {
			return fmt.Errorf("--values-file value for %q must be a non-empty string; omit the field to leave it unset", name)
		}
		var field map[string]json.RawMessage
		if err := json.Unmarshal(raw, &field); err != nil {
			return err
		}
		encoded, err := json.Marshal(*values[name])
		if err != nil {
			return err
		}
		field["value"] = encoded
		if fields[name], err = json.Marshal(field); err != nil {
			return err
		}
	}
	return nil
}

// An update must change something: the API rejects an empty spec.
func vaultCredentialUpdateSpecFromFlags(cmd *cobra.Command) (map[string]json.RawMessage, error) {
	spec := make(map[string]json.RawMessage)
	if cmd.Flags().Changed("description") {
		description, _ := cmd.Flags().GetString("description")
		encoded, err := json.Marshal(description)
		if err != nil {
			return nil, err
		}
		spec["description"] = encoded
	}
	if cmd.Flags().Changed("values-file") {
		values, err := readVaultFieldValues(cmd, "values-file")
		if err != nil {
			return nil, err
		}
		fields := make(map[string]json.RawMessage, len(values))
		for _, name := range sortedVaultFieldNames(values) {
			encoded, err := json.Marshal(map[string]*string{"value": values[name]})
			if err != nil {
				return nil, err
			}
			fields[name] = encoded
		}
		if spec["fields"], err = json.Marshal(fields); err != nil {
			return nil, err
		}
	}
	if len(spec) == 0 {
		return nil, fmt.Errorf("update requires --description, --values-file, or both")
	}
	return spec, nil
}

func sortedVaultFieldNames[T any](values map[string]T) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
