package cmd

import (
	"encoding/json"
	"fmt"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/spf13/cobra"
)

type vaultProviderConfigReference struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type vaultLinkClient struct {
	Type           string          `json:"type"`
	ProviderConfig json.RawMessage `json:"provider_config"`
}

type vaultLinkTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

type vaultLinkAuthorization struct {
	Method       string           `json:"method"`
	Client       vaultLinkClient  `json:"client"`
	Tokens       *vaultLinkTokens `json:"tokens,omitempty"`
	TokenRefresh string           `json:"token_refresh,omitempty"`
	fields       map[string]json.RawMessage
}

func vaultWalletSpecFromFlags(cmd *cobra.Command) (kernel.VaultItemUpsertParamsBodyWalletSpecUnion, error) {
	spec, err := vaultSpecFromFlags(cmd)
	if err != nil {
		return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, err
	}
	reference, err := vaultProviderConfigFromFlags(cmd)
	if err != nil {
		return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, err
	}
	provider, _ := cmd.Flags().GetString("provider")
	if provider != "link" && cmd.Flags().Changed("token-refresh") {
		return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, fmt.Errorf("--token-refresh is only for imported Link wallet grants")
	}
	switch provider {
	case "kernel":
		if reference != nil || cmd.Flags().Changed("tokens-file") {
			return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, fmt.Errorf("kernel wallets do not use provider configurations or tokens")
		}
	case "shop_pay":
		if reference != nil || cmd.Flags().Changed("tokens-file") {
			return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, fmt.Errorf("Shop Pay wallets use Kernel-managed OAuth and do not accept provider configurations or tokens")
		}
		err = configureShopPayWallet(spec)
	case "agentcard":
		if cmd.Flags().Changed("tokens-file") {
			return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, fmt.Errorf("--tokens-file is only for imported Link wallet grants")
		}
		err = configureAgentCardWallet(spec, reference)
	case "vgs":
		if cmd.Flags().Changed("tokens-file") {
			return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, fmt.Errorf("VGS wallets use a vault provider configuration and do not accept tokens")
		}
		err = configureVgsWallet(spec, reference)
	case "link":
		err = configureLinkWallet(cmd, spec, reference)
	}
	if err != nil {
		return kernel.VaultItemUpsertParamsBodyWalletSpecUnion{}, err
	}
	return param.Override[kernel.VaultItemUpsertParamsBodyWalletSpecUnion](spec), nil
}

func vaultProviderConfigFromFlags(cmd *cobra.Command) (json.RawMessage, error) {
	var reference vaultProviderConfigReference
	var value string
	switch {
	case cmd.Flags().Changed("provider-config-id"):
		value, _ = cmd.Flags().GetString("provider-config-id")
		reference.ID = value
	case cmd.Flags().Changed("provider-config-name"):
		value, _ = cmd.Flags().GetString("provider-config-name")
		reference.Name = value
	default:
		return nil, nil
	}
	if err := validateVaultName(value, "provider configuration reference"); err != nil {
		return nil, err
	}
	return json.Marshal(reference)
}

func configureShopPayWallet(spec map[string]json.RawMessage) error {
	raw, ok := spec["authorization"]
	if !ok {
		return fmt.Errorf("Shop Pay spec.authorization is required with method oauth and client.type kernel_managed")
	}
	var authorization struct {
		Method string `json:"method"`
		Client struct {
			Type string `json:"type"`
		} `json:"client"`
	}
	if json.Unmarshal(raw, &authorization) != nil || authorization.Method != "oauth" || authorization.Client.Type != "kernel_managed" {
		return fmt.Errorf("Shop Pay spec.authorization must use method oauth and client.type kernel_managed")
	}
	return nil
}

func configureAgentCardWallet(spec map[string]json.RawMessage, reference json.RawMessage) error {
	if reference != nil {
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

func configureVgsWallet(spec map[string]json.RawMessage, reference json.RawMessage) error {
	if reference != nil {
		if _, exists := spec["provider_config"]; exists {
			return fmt.Errorf("select a provider config through flags or spec, not both")
		}
		spec["provider_config"] = reference
	}
	ref, exists := spec["provider_config"]
	if !exists {
		return fmt.Errorf("VGS wallets require --provider-config-id, --provider-config-name, or spec.provider_config")
	}
	return validateVaultConfigReference(ref)
}

func configureLinkWallet(cmd *cobra.Command, spec map[string]json.RawMessage, reference json.RawMessage) error {
	auth, err := vaultLinkAuthorizationFromSpec(spec["authorization"], reference)
	if err != nil {
		return err
	}
	tokenRefreshFlag, tokenRefreshSet := "", cmd.Flags().Changed("token-refresh")
	_, tokenRefreshInSpec := auth.fields["token_refresh"]
	if tokenRefreshSet {
		tokenRefreshFlag, _ = cmd.Flags().GetString("token-refresh")
		if tokenRefreshInSpec && auth.TokenRefresh != tokenRefreshFlag {
			return fmt.Errorf("spec.authorization.token_refresh must match --token-refresh")
		}
		auth.TokenRefresh = tokenRefreshFlag
	}
	if (tokenRefreshSet || tokenRefreshInSpec) && auth.TokenRefresh != "kernel" && auth.TokenRefresh != "customer" {
		return fmt.Errorf("Link authorization.token_refresh must be kernel or customer")
	}
	if auth.Client.Type != "customer_managed" {
		if cmd.Flags().Changed("tokens-file") {
			return fmt.Errorf("--tokens-file requires a customer-managed Link provider configuration")
		}
		if tokenRefreshSet || tokenRefreshInSpec {
			return fmt.Errorf("token_refresh applies only to imported Link grants with a customer-managed provider configuration")
		}
		return nil
	}
	if auth.Method != "oauth" {
		return fmt.Errorf("imported Link authorization.method must be oauth")
	}
	if err := validateVaultConfigReference(auth.Client.ProviderConfig); err != nil {
		return err
	}
	effectiveTokenRefresh := auth.TokenRefresh
	if effectiveTokenRefresh == "" {
		effectiveTokenRefresh = "kernel"
	}
	var tokens map[string]string
	if effectiveTokenRefresh == "customer" {
		tokens, err = readVaultSecrets(cmd, "tokens-file", "access_token")
	} else {
		tokens, err = readVaultSecrets(cmd, "tokens-file", "access_token", "refresh_token")
	}
	if err != nil {
		return err
	}
	spec["authorization"], err = auth.withTokens(vaultLinkTokens{AccessToken: tokens["access_token"], RefreshToken: tokens["refresh_token"]})
	return err
}

func vaultLinkAuthorizationFromSpec(raw, reference json.RawMessage) (vaultLinkAuthorization, error) {
	if reference != nil {
		if raw != nil {
			return vaultLinkAuthorization{}, fmt.Errorf("with Link config selection flags, omit spec.authorization; supply the grant through --tokens-file")
		}
		return vaultLinkAuthorization{Method: "oauth", Client: vaultLinkClient{Type: "customer_managed", ProviderConfig: reference}}, nil
	}
	var auth vaultLinkAuthorization
	if raw == nil {
		return auth, nil
	}
	if json.Unmarshal(raw, &auth.fields) != nil {
		return auth, fmt.Errorf("spec.authorization must be an OAuth authorization object")
	}
	if method, exists := auth.fields["method"]; exists && json.Unmarshal(method, &auth.Method) != nil {
		return auth, fmt.Errorf("spec.authorization.method must be a string")
	}
	if client, exists := auth.fields["client"]; exists && json.Unmarshal(client, &auth.Client) != nil {
		return auth, fmt.Errorf("spec.authorization.client must be an OAuth client object")
	}
	if tokenRefresh, exists := auth.fields["token_refresh"]; exists && json.Unmarshal(tokenRefresh, &auth.TokenRefresh) != nil {
		return auth, fmt.Errorf("spec.authorization.token_refresh must be a string")
	}
	return auth, nil
}

func (auth vaultLinkAuthorization) withTokens(tokens vaultLinkTokens) (json.RawMessage, error) {
	auth.Tokens = &tokens
	if auth.fields == nil {
		return json.Marshal(auth)
	}
	// Only file- and flag-supplied fields change; preserve opaque fields and omissions.
	raw, err := json.Marshal(tokens)
	if err != nil {
		return nil, err
	}
	auth.fields["tokens"] = raw
	if auth.TokenRefresh != "" {
		raw, err = json.Marshal(auth.TokenRefresh)
		if err != nil {
			return nil, err
		}
		auth.fields["token_refresh"] = raw
	}
	return json.Marshal(auth.fields)
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
