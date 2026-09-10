package cmd

import (
	"encoding/json"
	"fmt"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/spf13/cobra"
)

func vaultWalletSpecFromFlags(cmd *cobra.Command) (kernel.VaultItemUpsertParamsBodyWalletSpecUnion, error) {
	spec, err := vaultSpecFromFlags(cmd)
	if err != nil {
		return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, err
	}
	if err := configureVaultWallet(cmd, spec); err != nil {
		return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, err
	}
	return param.Override[kernel.VaultItemUpsertParamsBodyWalletSpecUnion](spec), nil
}

func configureVaultWallet(cmd *cobra.Command, spec map[string]json.RawMessage) error {
	provider, _ := cmd.Flags().GetString("provider")
	id, _ := cmd.Flags().GetString("provider-config-id")
	name, _ := cmd.Flags().GetString("provider-config-name")
	selected := cmd.Flags().Changed("provider-config-id") || cmd.Flags().Changed("provider-config-name")
	var reference json.RawMessage
	if selected {
		field, value := "id", id
		if cmd.Flags().Changed("provider-config-name") {
			field, value = "name", name
		}
		if err := validateVaultName(value, "provider configuration reference"); err != nil {
			return err
		}
		reference, _ = json.Marshal(map[string]string{field: value})
	}
	if provider == "agentcard" {
		if cmd.Flags().Changed("tokens-file") {
			return fmt.Errorf("--tokens-file is only for imported Link wallet grants")
		}
		if selected {
			if _, exists := spec["provider_config"]; exists {
				return fmt.Errorf("select a provider config through flags or spec, not both")
			}
			spec["provider_config"] = reference
		}
		if ref, exists := spec["provider_config"]; exists {
			return validateVaultConfigReference(ref)
		}
		return nil
	}
	if selected {
		if _, exists := spec["authorization"]; exists {
			return fmt.Errorf("with Link config selection flags, omit spec.authorization; supply the grant through --tokens-file")
		}
		spec["authorization"], _ = json.Marshal(map[string]any{"method": "oauth", "client": map[string]any{"type": "customer_managed", "provider_config": reference}})
	}
	var auth struct {
		Method string `json:"method"`
		Client struct {
			Type           string          `json:"type"`
			ProviderConfig json.RawMessage `json:"provider_config"`
		} `json:"client"`
	}
	if raw, exists := spec["authorization"]; exists && json.Unmarshal(raw, &auth) != nil {
		return fmt.Errorf("spec.authorization must be an OAuth authorization object")
	}
	imported := auth.Client.Type == "customer_managed"
	if !imported {
		if cmd.Flags().Changed("tokens-file") {
			return fmt.Errorf("--tokens-file requires a customer-managed Link provider configuration")
		}
		return nil
	}
	if auth.Method != "oauth" {
		return fmt.Errorf("imported Link authorization.method must be oauth")
	}
	if err := validateVaultConfigReference(auth.Client.ProviderConfig); err != nil {
		return err
	}
	tokens, err := readVaultSecrets(cmd, "tokens-file", "access_token", "refresh_token")
	if err != nil {
		return err
	}
	var authorization map[string]json.RawMessage
	_ = json.Unmarshal(spec["authorization"], &authorization)
	authorization["tokens"], _ = json.Marshal(tokens)
	spec["authorization"], _ = json.Marshal(authorization)
	return nil
}

func validateVaultConfigReference(raw json.RawMessage) error {
	var ref map[string]string
	if json.Unmarshal(raw, &ref) != nil || len(ref) != 1 {
		return fmt.Errorf("provider_config must contain exactly one of id or name")
	}
	for field, value := range ref {
		if field != "id" && field != "name" {
			return fmt.Errorf("provider_config must contain exactly one of id or name")
		}
		if err := validateVaultName(value, "provider configuration reference"); err != nil {
			return err
		}
	}
	return nil
}
