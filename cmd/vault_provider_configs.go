package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var vaultProviderConfigFields = vaultFieldsOf("id name provider client_id test_mode created_at updated_at")

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
		Long: `Manage Link OAuth clients and AgentCard application credentials, shared across projects.
Config credentials are not wallet grants. Link grants are imported separately when creating a wallet.
Writes require organization-scoped authentication; --project does not elevate a project API key.
Provider and client ID are immutable. Rename or rotate a secret without changing wallet bindings.
Rotation affects all bound wallets and must preserve client identity and AgentCard mode.
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
	create := &cobra.Command{Use: "create --name <name> --provider <link|agentcard> --credentials-file <path|->", Short: "Register a provider configuration", Args: cobra.NoArgs, PreRunE: preRun,
		Long: "Register a configuration; duplicate names return a conflict without replacing credentials.\n--credentials-file must contain a JSON object with client_id and client_secret strings.\nUse a protected file or pipe from a secret manager; never put credentials in shell arguments.",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			if err := validateVaultName(name, "--name"); err != nil {
				return err
			}
			provider, _ := cmd.Flags().GetString("provider")
			if provider != "link" && provider != "agentcard" {
				return fmt.Errorf("--provider must be link or agentcard")
			}
			credentials, err := readVaultSecrets(cmd, "credentials-file", "client_id", "client_secret")
			if err != nil {
				return err
			}
			params := kernel.VaultProviderConfigNewParams{}
			if provider == "link" {
				params.OfLink = &kernel.VaultProviderConfigNewParamsBodyLink{Name: name, Credentials: kernel.VaultProviderConfigNewParamsBodyLinkCredentials{ClientID: credentials["client_id"], ClientSecret: credentials["client_secret"]}}
			} else {
				params.OfAgentcard = &kernel.VaultProviderConfigNewParamsBodyAgentcard{Name: name, Credentials: kernel.VaultProviderConfigNewParamsBodyAgentcardCredentials{ClientID: credentials["client_id"], ClientSecret: credentials["client_secret"]}}
			}
			c := getVaultProviderConfigsHandler(cmd)
			config, err := c.configs.New(cmd.Context(), params, option.WithMaxRetries(0))
			if err != nil {
				return vaultCredentialError(err)
			}
			return printVaultProviderConfig(config, vaultOutput(cmd))
		}}
	create.Flags().String("name", "", "Organization-unique configuration name (required)")
	create.Flags().String("provider", "", "Provider: link or agentcard (required)")
	create.Flags().String("credentials-file", "", "Read client_id and client_secret JSON from a file (use '-' for stdin)")
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
		Long: "Omitted fields remain unchanged. --credentials-file accepts only a client_secret JSON string field.\nProvider, client ID, and mode cannot change. Rotation affects all wallets using this configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("credentials-file") {
				return fmt.Errorf("provide --name or --credentials-file")
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
			c := getVaultProviderConfigsHandler(cmd)
			config, err := c.configs.Update(cmd.Context(), args[0], params, option.WithMaxRetries(0))
			if err != nil {
				return vaultCredentialError(err)
			}
			return printVaultProviderConfig(config, vaultOutput(cmd))
		}}
	update.Flags().String("name", "", "New organization-unique name; existing wallet bindings are preserved")
	update.Flags().String("credentials-file", "", "Read client_secret JSON from a file (use '-' for stdin)")
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
			err := c.configs.Delete(cmd.Context(), args[0], option.WithMaxRetries(0))
			if err != nil && !util.IsNotFound(err) {
				return vaultCredentialError(err)
			}
			pterm.Success.Println("Deleted or not found: vault provider configuration " + args[0])
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
	hasMore, err := strconv.ParseBool(response.Header.Get("X-Has-More"))
	if err != nil {
		return fmt.Errorf("invalid vault provider configuration pagination metadata")
	}
	nextOffset := 0
	if value := response.Header.Get("X-Next-Offset"); value != "" {
		nextOffset, err = strconv.Atoi(value)
	}
	if err != nil || nextOffset < 0 || (hasMore && int64(nextOffset) <= offset) || (!hasMore && nextOffset != 0) {
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
		}{items, nextOffset})
	}
	if len(page.Items) == 0 {
		pterm.Info.Println("No vault provider configurations found")
	} else {
		rows := pterm.TableData{{"ID", "Name", "Provider", "Client ID", "Test mode"}}
		for _, config := range page.Items {
			mode := "—"
			if config.JSON.TestMode.Valid() {
				mode = fmt.Sprint(config.TestMode)
			}
			rows = append(rows, []string{config.ID, config.Name, config.Provider, config.ClientID, mode})
		}
		PrintTableNoPad(rows, true)
	}
	if hasMore {
		pterm.Printf("Next: kernel vault-provider-configs list --limit %d --offset %d\n", limit, nextOffset)
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
	if config.JSON.TestMode.Valid() {
		rows = append(rows, []string{"Test mode (introspected)", fmt.Sprint(config.TestMode)})
	}
	rows = append(rows, []string{"Created At", util.FormatLocal(config.CreatedAt)}, []string{"Updated At", util.FormatLocal(config.UpdatedAt)})
	PrintTableNoPad(rows, true)
	return nil
}
