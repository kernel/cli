package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var vaultProviderConfigFields = vaultFieldsOf("id name provider client_id publishable_key test_mode vault_id environment created_at updated_at")

type VaultProviderConfigsCmd struct {
	configs  *kernel.VaultProviderConfigService
	prompter interactive.Prompter
}

func init() {
	rootCmd.AddCommand(newVaultProviderConfigsCommand())
}

func getVaultProviderConfigsHandler(cmd *cobra.Command) VaultProviderConfigsCmd {
	client := getKernelClient(cmd)
	return VaultProviderConfigsCmd{configs: &client.VaultProviderConfigs, prompter: interactive.NewPrompter()}
}

func newVaultProviderConfigsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "vault-provider-configs", Short: "Manage organization-owned vault provider configurations",
		Long: `Manage Link OAuth clients, AgentCard application credentials, and VGS service accounts shared across projects.
Config credentials are not wallet grants. Link grants are imported separately when creating a wallet.
VGS configurations identify one customer-managed vault and environment for alias reveal.
Writes require organization-scoped authentication; --project does not elevate a project API key.
Provider and client ID are immutable. VGS vault ID and environment are immutable too.
Rename or rotate a secret without changing wallet bindings; rotation revalidates the same provider identity and mode.
Secrets are accepted only from a file or stdin and are never displayed. Protect credential files.`,
	}
	preRun := func(cmd *cobra.Command, args []string) error {
		if err := validateJSONOutput(vaultOutput(cmd)); err != nil {
			return err
		}
		if len(args) > 0 {
			return validateVaultName(args[0], "configuration ID or name")
		}
		return nil
	}
	create := &cobra.Command{Use: "create --name <name> --provider <link|agentcard|vgs> --credentials-file <path|->", Short: "Register a provider configuration", Args: cobra.NoArgs, PreRunE: preRun,
		Long: "Register a configuration; duplicate names return a conflict without replacing credentials.\n--credentials-file must contain client_id and client_secret strings. For VGS it must also contain vault_id and environment (sandbox or live); the service account needs aliases:read access to that vault. Kernel validates the VGS scope and vault access before storing anything.\nUse a protected file or pipe from a secret manager; never put credentials in shell arguments.\nFor Link, pass --publishable-key so imported wallet grants keep working after their access token expires.",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			if err := validateVaultName(name, "--name"); err != nil {
				return err
			}
			provider, _ := cmd.Flags().GetString("provider")
			if provider != "link" && provider != "agentcard" && provider != "vgs" {
				return fmt.Errorf("--provider must be link, agentcard, or vgs")
			}
			publishableKey, _ := cmd.Flags().GetString("publishable-key")
			if cmd.Flags().Changed("publishable-key") && (provider != "link" || publishableKey == "") {
				return fmt.Errorf("--publishable-key requires --provider link and a non-empty value")
			}
			credentialFields := []string{"client_id", "client_secret"}
			if provider == "vgs" {
				credentialFields = append(credentialFields, "vault_id", "environment")
			}
			credentials, err := readVaultSecrets(cmd, "credentials-file", credentialFields...)
			if err != nil {
				return err
			}
			params := kernel.VaultProviderConfigNewParams{}
			switch provider {
			case "link":
				linkCredentials := kernel.VaultProviderConfigNewParamsBodyLinkCredentials{ClientID: credentials["client_id"], ClientSecret: credentials["client_secret"]}
				if publishableKey != "" {
					linkCredentials.PublishableKey = kernel.Opt(publishableKey)
				}
				params.OfLink = &kernel.VaultProviderConfigNewParamsBodyLink{Name: name, Credentials: linkCredentials}
			case "agentcard":
				params.OfAgentcard = &kernel.VaultProviderConfigNewParamsBodyAgentcard{Name: name, Credentials: kernel.VaultProviderConfigNewParamsBodyAgentcardCredentials{ClientID: credentials["client_id"], ClientSecret: credentials["client_secret"]}}
			case "vgs":
				params.OfVgs = &kernel.VaultProviderConfigNewParamsBodyVgs{Name: name, Credentials: kernel.VaultProviderConfigNewParamsBodyVgsCredentials{
					ClientID: credentials["client_id"], ClientSecret: credentials["client_secret"],
					VaultID: credentials["vault_id"], Environment: credentials["environment"],
				}}
			}
			c := getVaultProviderConfigsHandler(cmd)
			config, err := c.configs.New(cmd.Context(), params, vaultWriteRequestOptions()...)
			if err != nil {
				return vaultCredentialError(err)
			}
			return printVaultProviderConfig(config, vaultOutput(cmd))
		}}
	create.Flags().String("name", "", "Organization-unique configuration name (required)")
	create.Flags().String("provider", "", "Provider: link, agentcard, or vgs (required)")
	create.Flags().String("credentials-file", "", "Read provider credential JSON from a file (use '-' for stdin); VGS also requires vault_id and environment")
	create.Flags().String("publishable-key", "", "Link only. Stripe publishable key for the account that owns the Link OAuth client; required for Kernel to refresh or revoke imported wallet grants")
	for _, flag := range []string{"name", "provider", "credentials-file"} {
		_ = create.MarkFlagRequired(flag)
	}
	addVaultJSONOutputFlag(create)

	get := &cobra.Command{Use: "get <id-or-name>", Aliases: []string{"show"}, Short: "Show public configuration fields", Args: cobra.ExactArgs(1), PreRunE: preRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getVaultProviderConfigsHandler(cmd)
			config, err := c.configs.Get(cmd.Context(), args[0], option.WithMaxRetries(0))
			if err != nil {
				return vaultCredentialError(err)
			}
			return printVaultProviderConfig(config, vaultOutput(cmd))
		}}
	addVaultJSONOutputFlag(get)

	list := &cobra.Command{Use: "list", Short: "List configurations in the organization", Args: cobra.NoArgs, PreRunE: preRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, _ := cmd.Flags().GetInt64("limit")
			offset, _ := cmd.Flags().GetInt64("offset")
			return getVaultProviderConfigsHandler(cmd).List(cmd.Context(), limit, offset, vaultOutput(cmd))
		}}
	list.Flags().Int64("limit", 20, "Maximum configurations to return (1-100)")
	list.Flags().Int64("offset", 0, "Number of configurations to skip")
	addVaultJSONOutputFlag(list)

	update := &cobra.Command{Use: "update <id-or-name>", Short: "Rename a configuration or rotate its secret", Args: cobra.ExactArgs(1), PreRunE: preRun,
		Long: "Omitted fields remain unchanged. --credentials-file accepts only a client_secret JSON string field.\n--publishable-key sets the Stripe publishable key on Link configurations.\nProvider and client ID cannot change. AgentCard mode is preserved. VGS vault_id and environment are immutable; rotating its secret repeats the aliases:read and vault-access checks against the same vault and environment. Rotation affects all wallets using this configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("credentials-file") && !cmd.Flags().Changed("publishable-key") {
				return fmt.Errorf("provide --name, --credentials-file, or --publishable-key")
			}
			params := kernel.VaultProviderConfigUpdateParams{}
			if cmd.Flags().Changed("name") {
				name, _ := cmd.Flags().GetString("name")
				if err := validateVaultName(name, "--name"); err != nil {
					return err
				}
				params.Name = kernel.Opt(name)
			}
			if cmd.Flags().Changed("credentials-file") {
				credentials, err := readVaultSecrets(cmd, "credentials-file", "client_secret")
				if err != nil {
					return err
				}
				params.Credentials.ClientSecret = kernel.Opt(credentials["client_secret"])
			}
			if cmd.Flags().Changed("publishable-key") {
				publishableKey, _ := cmd.Flags().GetString("publishable-key")
				if publishableKey == "" {
					return fmt.Errorf("--publishable-key must be non-empty")
				}
				params.Credentials.PublishableKey = kernel.Opt(publishableKey)
			}
			c := getVaultProviderConfigsHandler(cmd)
			config, err := c.configs.Update(cmd.Context(), args[0], params, vaultWriteRequestOptions()...)
			if err != nil {
				return vaultCredentialError(err)
			}
			return printVaultProviderConfig(config, vaultOutput(cmd))
		}}
	update.Flags().String("name", "", "New organization-unique name; existing wallet bindings are preserved")
	update.Flags().String("credentials-file", "", "Read client_secret JSON from a file (use '-' for stdin)")
	update.Flags().String("publishable-key", "", "Link configurations only. Stripe publishable key sent to Link when refreshing and revoking wallet grants")
	addVaultJSONOutputFlag(update)

	delete := &cobra.Command{Use: "delete <id-or-name>", Short: "Delete an unused configuration", Args: cobra.ExactArgs(1), PreRunE: preRun,
		Long: "Deletion is blocked while any non-deleted item references the configuration, even if disconnected.\nDoes not delete the external client or revoke unrelated grants.",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getVaultProviderConfigsHandler(cmd)
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				ok, err := c.prompter.Confirm("delete configuration", "Delete unused vault provider configuration "+args[0]+"?")
				if err != nil {
					return err
				}
				if !ok {
					pterm.Info.Println("Deletion cancelled")
					return nil
				}
			}
			err := c.configs.Delete(cmd.Context(), args[0], vaultWriteRequestOptions()...)
			if err != nil {
				return vaultCredentialError(err)
			}
			pterm.Success.Println("Deleted vault provider configuration " + args[0])
			return nil
		}}
	delete.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	cmd.AddCommand(create, get, list, update, delete)
	return cmd
}

func (c VaultProviderConfigsCmd) List(ctx context.Context, limit, offset int64, output string) error {
	if limit < 1 || limit > 100 || offset < 0 {
		return fmt.Errorf("--limit must be between 1 and 100; --offset must be non-negative")
	}
	var response *http.Response
	page, err := c.configs.List(ctx, kernel.VaultProviderConfigListParams{Limit: kernel.Opt(limit), Offset: kernel.Opt(offset)}, option.WithMaxRetries(0), option.WithResponseInto(&response))
	if err != nil {
		return vaultCredentialError(err)
	}
	pagination, err := parseOffsetPagination(response, offset)
	if err != nil {
		return fmt.Errorf("invalid vault provider configuration pagination metadata")
	}
	if output == "json" {
		items, err := vaultSafeJSONSlice(page.Items, vaultProviderConfigFields)
		if err != nil {
			return err
		}
		return printVaultJSON(struct {
			Configs    []vaultJSON `json:"vault_provider_configs"`
			NextOffset int         `json:"next_offset,omitempty"`
		}{items, pagination.NextOffset})
	}
	if len(page.Items) == 0 {
		pterm.Info.Println("No vault provider configurations found")
	} else {
		rows := pterm.TableData{{"ID", "Name", "Provider", "Client ID", "Vault ID", "Environment", "Test mode"}}
		for _, config := range page.Items {
			mode := "—"
			if config.JSON.TestMode.Valid() {
				mode = fmt.Sprint(config.TestMode)
			}
			rows = append(rows, []string{config.ID, config.Name, config.Provider, config.ClientID, util.OrDash(config.VaultID), util.OrDash(config.Environment), mode})
		}
		PrintTableNoPad(rows, true)
	}
	if pagination.HasMore {
		pterm.Printf("Next: kernel vault-provider-configs list --limit %d --offset %d\n", limit, pagination.NextOffset)
	}
	return nil
}

func printVaultProviderConfig(config *kernel.VaultProviderConfigUnion, output string) error {
	if output == "json" {
		raw, err := filterVaultJSON(json.RawMessage(config.RawJSON()), vaultProviderConfigFields)
		if err != nil {
			return err
		}
		return printVaultJSON(raw)
	}
	rows := pterm.TableData{{"Property", "Value"}, {"ID", config.ID}, {"Name", config.Name}, {"Provider (immutable)", config.Provider}, {"Client ID (immutable)", config.ClientID}}
	if config.PublishableKey != "" {
		rows = append(rows, []string{"Publishable Key", config.PublishableKey})
	}
	if config.JSON.TestMode.Valid() {
		rows = append(rows, []string{"Test mode (introspected)", fmt.Sprint(config.TestMode)})
	}
	if config.JSON.VaultID.Valid() {
		rows = append(rows, []string{"VGS vault ID (immutable)", config.VaultID})
	}
	if config.JSON.Environment.Valid() {
		rows = append(rows, []string{"VGS environment (immutable)", config.Environment})
	}
	rows = append(rows, []string{"Created At", util.FormatLocal(config.CreatedAt)}, []string{"Updated At", util.FormatLocal(config.UpdatedAt)})
	PrintTableNoPad(rows, true)
	return nil
}
