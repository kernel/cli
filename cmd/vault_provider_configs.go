package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

// VaultProviderConfigsService is the subset of the SDK's provider-configuration
// client that the CLI uses.
type VaultProviderConfigsService interface {
	New(ctx context.Context, body kernel.VaultProviderConfigNewParams, opts ...option.RequestOption) (res *kernel.VaultProviderConfigUnion, err error)
	Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *kernel.VaultProviderConfigUnion, err error)
	Update(ctx context.Context, idOrName string, body kernel.VaultProviderConfigUpdateParams, opts ...option.RequestOption) (res *kernel.VaultProviderConfigUnion, err error)
	List(ctx context.Context, query kernel.VaultProviderConfigListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.VaultProviderConfigUnion], err error)
	Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) error
}

// VaultProviderConfigsCmd handles provider-configuration operations independent
// of cobra.
type VaultProviderConfigsCmd struct {
	configs  VaultProviderConfigsService
	prompter interactive.Prompter
}

type VaultProviderConfigsCreateInput struct {
	Name         string
	Provider     string
	ClientID     string
	ClientSecret string
	Output       string
}

type VaultProviderConfigsListInput struct {
	Page    int
	PerPage int
	Output  string
}

type VaultProviderConfigsGetInput struct {
	Identifier string
	Output     string
}

type VaultProviderConfigsUpdateInput struct {
	Identifier string
	// Nil means "leave as it is". The API has no way to clear either field, so
	// neither accepts an empty value.
	Name         *string
	ClientSecret *string
	Output       string
}

type VaultProviderConfigsDeleteInput struct {
	Identifier  string
	SkipConfirm bool
}

func validateVaultProvider(provider string) error {
	if provider != "link" && provider != "agentcard" {
		return fmt.Errorf("--provider must be link or agentcard")
	}
	return nil
}

// vaultProviderConfigPreRun rejects a malformed output format or identifier
// before any request is sent, matching the rest of the vaults surface.
func vaultProviderConfigPreRun(cmd *cobra.Command, args []string) error {
	if err := validateJSONOutput(vaultOutput(cmd)); err != nil {
		return err
	}
	if len(args) > 0 {
		return validateVaultName(args[0], "provider configuration ID or name")
	}
	return nil
}

func (c VaultProviderConfigsCmd) Create(ctx context.Context, in VaultProviderConfigsCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if err := validateVaultName(in.Name, "--name"); err != nil {
		return err
	}
	if err := validateVaultProvider(in.Provider); err != nil {
		return err
	}
	if strings.TrimSpace(in.ClientID) == "" {
		return fmt.Errorf("--client-id is required")
	}
	if in.ClientSecret == "" {
		return fmt.Errorf("--client-secret or --client-secret-file is required")
	}

	params := kernel.VaultProviderConfigNewParams{}
	switch in.Provider {
	case "link":
		params.OfLink = &kernel.VaultProviderConfigNewParamsBodyLink{
			Name: in.Name,
			Credentials: kernel.VaultProviderConfigNewParamsBodyLinkCredentials{
				ClientID:     in.ClientID,
				ClientSecret: in.ClientSecret,
			},
		}
	case "agentcard":
		params.OfAgentcard = &kernel.VaultProviderConfigNewParamsBodyAgentcard{
			Name: in.Name,
			Credentials: kernel.VaultProviderConfigNewParamsBodyAgentcardCredentials{
				ClientID:     in.ClientID,
				ClientSecret: in.ClientSecret,
			},
		}
	}

	config, err := c.configs.New(ctx, params, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return printVaultProviderConfigJSON(config)
	}
	pterm.Success.Printf("Registered provider configuration: %s\n", config.ID)
	printVaultProviderConfigDetail(config)
	printVaultProviderConfigUsage(config)
	return nil
}

func (c VaultProviderConfigsCmd) List(ctx context.Context, in VaultProviderConfigsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	page := in.Page
	perPage := in.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		return fmt.Errorf("--per-page must be between 1 and 100")
	}

	params := kernel.VaultProviderConfigListParams{}
	// Request one extra item so the response itself reveals whether another page
	// exists: the SDK keeps the X-Has-More header private.
	params.Limit = kernel.Opt(int64(perPage + 1))
	params.Offset = kernel.Opt(int64((page - 1) * perPage))

	result, err := c.configs.List(ctx, params, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	var items []kernel.VaultProviderConfigUnion
	if result != nil {
		items = result.Items
	}
	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}
	itemsThisPage := len(items)

	if in.Output == "json" {
		values, err := vaultSafeJSONSlice(items, vaultProviderConfigFields)
		if err != nil {
			return err
		}
		return printVaultJSON(struct {
			ProviderConfigs []vaultJSON `json:"provider_configs"`
			Page            int         `json:"page"`
			PerPage         int         `json:"per_page"`
			HasMore         bool        `json:"has_more"`
		}{values, page, perPage, hasMore})
	}

	if len(items) == 0 {
		pterm.Info.Println("No provider configurations found")
	} else {
		rows := pterm.TableData{{"ID", "Name", "Provider", "Client ID", "Mode", "Created At", "Updated At"}}
		for _, config := range items {
			rows = append(rows, []string{
				config.ID,
				config.Name,
				config.Provider,
				config.ClientID,
				vaultProviderConfigMode(config),
				util.FormatLocal(config.CreatedAt),
				util.FormatLocal(config.UpdatedAt),
			})
		}
		PrintTableNoPad(rows, true)
	}

	pterm.Printf("\nPage: %d  Per-page: %d  Items this page: %d  Has more: %s\n", page, perPage, itemsThisPage, lo.Ternary(hasMore, "yes", "no"))
	if hasMore {
		pterm.Printf("Next: kernel vaults provider-configs list --page %d --per-page %d\n", page+1, perPage)
	}
	return nil
}

func (c VaultProviderConfigsCmd) Get(ctx context.Context, in VaultProviderConfigsGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	config, err := c.configs.Get(ctx, in.Identifier, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return printVaultProviderConfigJSON(config)
	}
	printVaultProviderConfigDetail(config)
	return nil
}

func (c VaultProviderConfigsCmd) Update(ctx context.Context, in VaultProviderConfigsUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	params := kernel.VaultProviderConfigUpdateParams{}
	if in.Name != nil {
		if err := validateVaultName(*in.Name, "--name"); err != nil {
			return err
		}
		params.Name = kernel.Opt(*in.Name)
	}
	if in.ClientSecret != nil {
		if *in.ClientSecret == "" {
			return fmt.Errorf("--client-secret must not be empty; omit it to leave the stored secret unchanged")
		}
		params.Credentials.ClientSecret = kernel.Opt(*in.ClientSecret)
	}
	if in.Name == nil && in.ClientSecret == nil {
		return fmt.Errorf("nothing to update: pass --name, --client-secret, or --client-secret-file")
	}

	config, err := c.configs.Update(ctx, in.Identifier, params, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return printVaultProviderConfigJSON(config)
	}
	pterm.Success.Printf("Updated provider configuration: %s\n", config.ID)
	printVaultProviderConfigDetail(config)
	if in.Name != nil {
		pterm.Info.Println("Renaming does not rebind existing wallets; they keep the configuration ID they were created with.")
	}
	return nil
}

func (c VaultProviderConfigsCmd) Delete(ctx context.Context, in VaultProviderConfigsDeleteInput) error {
	if !in.SkipConfirm {
		ok, err := c.prompter.Confirm(
			fmt.Sprintf("delete provider configuration '%s'", in.Identifier),
			fmt.Sprintf("Delete provider configuration '%s'? Wallets already bound to it cannot be rebound.", in.Identifier),
		)
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}
	if err := c.configs.Delete(ctx, in.Identifier, option.WithMaxRetries(0)); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Provider configuration '%s' not found\n", in.Identifier)
			return nil
		}
		// A 409 means a non-deleted vault item still references the
		// configuration; the API's message names what still holds it.
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted provider configuration: %s\n", in.Identifier)
	pterm.Info.Println("The external OAuth client still exists and unrelated grants are not revoked.")
	return nil
}

// vaultProviderConfigMode reports the provider-introspected credential mode.
// Only AgentCard configurations return test_mode, so Link rows stay blank
// rather than claiming a mode the API never reported.
func vaultProviderConfigMode(config kernel.VaultProviderConfigUnion) string {
	if !config.JSON.TestMode.Valid() {
		return "-"
	}
	return lo.Ternary(config.TestMode, "sandbox", "live")
}

// printVaultProviderConfigUsage shows how the configuration that was just
// registered gets used. Only AgentCard wallets can select one through the CLI:
// a customer-managed Link wallet is created by importing an existing grant's
// OAuth tokens, which the vaults surface never accepts.
func printVaultProviderConfigUsage(config *kernel.VaultProviderConfigUnion) {
	if config.Provider == "agentcard" {
		pterm.Info.Printf(
			"Select it when creating a wallet: kernel vaults wallets create <vault> <key> --provider agentcard --spec '{\"provider_config\": {\"name\": %q}}'\n",
			config.Name,
		)
		return
	}
	pterm.Info.Println("Link wallets on this client are created by importing a grant's OAuth tokens from your backend, not through the CLI.")
}

func printVaultProviderConfigJSON(config *kernel.VaultProviderConfigUnion) error {
	raw, err := filterVaultJSON(json.RawMessage(config.RawJSON()), vaultProviderConfigFields)
	if err != nil {
		return err
	}
	return printVaultJSON(raw)
}

func printVaultProviderConfigDetail(config *kernel.VaultProviderConfigUnion) {
	rows := pterm.TableData{
		{"Property", "Value"},
		{"ID", config.ID},
		{"Name", config.Name},
		{"Provider", config.Provider},
		{"Client ID", config.ClientID},
	}
	if config.JSON.TestMode.Valid() {
		rows = append(rows, []string{"Mode (provider-introspected)", vaultProviderConfigMode(*config)})
	}
	rows = append(rows,
		[]string{"Created At", util.FormatLocal(config.CreatedAt)},
		[]string{"Updated At", util.FormatLocal(config.UpdatedAt)},
	)
	PrintTableNoPad(rows, true)
}

// --- Cobra wiring ---

// readSecretFlag resolves a write-only credential from either its inline flag
// or a file, where "-" reads stdin. The file form keeps the secret out of shell
// history and process listings.
func readSecretFlag(cmd *cobra.Command, flagName string) (*string, error) {
	inlineChanged := cmd.Flags().Changed(flagName)
	file, _ := cmd.Flags().GetString(flagName + "-file")
	if inlineChanged && file != "" {
		return nil, fmt.Errorf("pass either --%s or --%s-file, not both", flagName, flagName)
	}
	if file != "" {
		var data []byte
		var err error
		if file == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(file)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read --%s-file: %w", flagName, err)
		}
		// Trailing newlines come from editors and `echo`, never from the secret.
		value := strings.TrimRight(string(data), "\r\n")
		return &value, nil
	}
	if inlineChanged {
		value, _ := cmd.Flags().GetString(flagName)
		return &value, nil
	}
	return nil, nil
}

func getVaultProviderConfigsHandler(cmd *cobra.Command) VaultProviderConfigsCmd {
	client := getKernelClient(cmd)
	svc := client.VaultProviderConfigs
	return VaultProviderConfigsCmd{configs: &svc, prompter: interactive.NewPrompter()}
}

func addVaultProviderConfigSecretFlags(cmd *cobra.Command, usage string) {
	cmd.Flags().String("client-secret", "", usage+". Prefer --client-secret-file to keep it out of shell history")
	cmd.Flags().String("client-secret-file", "", "Read the client secret from this file, or from stdin when set to -")
}

func newVaultProviderConfigsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "provider-configs",
		Aliases: []string{"provider-config"},
		Short:   "Register customer-owned provider credentials for wallets",
		Long: `Register and maintain the provider credentials that wallets can be created against.

A configuration is shared across the organization's projects and serves many
wallets. Names are unique within the organization. Secret credentials are
write-only: they are never returned by any command here.

These commands are organization-scoped. An organization-scoped API key or
dashboard login is required; a project-scoped key receives 403, and --project
does not apply.

Omitting a configuration when creating a wallet uses Kernel-managed credentials
instead. A wallet cannot be moved to a different configuration after creation,
and renaming a configuration does not rebind existing wallets.`,
		Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
	}

	create := &cobra.Command{
		Use:   "create --name <name> --provider <link|agentcard> --client-id <id> --client-secret <secret>",
		Short: "Register a provider configuration",
		Long: `Register a provider configuration under a name unique to the organization.
A duplicate name returns 409 and leaves the stored credentials alone.

For link, Kernel uses the client only to refresh and revoke wallet grants your
backend obtained; Kernel does not run the OAuth flow for an imported wallet.
For agentcard, Kernel obtains application access tokens with client_credentials
and introspects sandbox vs live from the credentials, so invalid credentials are
rejected at registration.`,
		Example: `  kernel vaults provider-configs create --name my-link-client --provider link \
    --client-id example-client-id --client-secret-file ./link-secret.txt

  printf '%s' "$AGENTCARD_SECRET" | kernel vaults provider-configs create \
    --name my-agentcard --provider agentcard \
    --client-id example-client-id --client-secret-file -`,
		Args:    cobra.NoArgs,
		PreRunE: vaultProviderConfigPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			secret, err := readSecretFlag(cmd, "client-secret")
			if err != nil {
				return err
			}
			name, _ := cmd.Flags().GetString("name")
			provider, _ := cmd.Flags().GetString("provider")
			clientID, _ := cmd.Flags().GetString("client-id")
			in := VaultProviderConfigsCreateInput{
				Name:     name,
				Provider: provider,
				ClientID: clientID,
				Output:   vaultOutput(cmd),
			}
			if secret != nil {
				in.ClientSecret = *secret
			}
			return getVaultProviderConfigsHandler(cmd).Create(cmd.Context(), in)
		},
	}
	create.Flags().String("name", "", "Configuration name, unique within the organization (required)")
	_ = create.MarkFlagRequired("name")
	create.Flags().String("provider", "", "Provider: link or agentcard (required)")
	_ = create.MarkFlagRequired("provider")
	create.Flags().String("client-id", "", "OAuth client identity; immutable after registration (required)")
	_ = create.MarkFlagRequired("client-id")
	addVaultProviderConfigSecretFlags(create, "OAuth client secret; stored write-only and never returned")
	addVaultJSONOutputFlag(create)

	list := &cobra.Command{
		Use:     "list",
		Short:   "List provider configurations in the organization",
		Long:    "List provider configurations. Secret credentials are never returned.",
		Args:    cobra.NoArgs,
		PreRunE: vaultProviderConfigPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			page, _ := cmd.Flags().GetInt("page")
			perPage, _ := cmd.Flags().GetInt("per-page")
			return getVaultProviderConfigsHandler(cmd).List(cmd.Context(), VaultProviderConfigsListInput{
				Page:    page,
				PerPage: perPage,
				Output:  vaultOutput(cmd),
			})
		},
	}
	list.Flags().Int("page", 1, "Page number (1-based)")
	list.Flags().Int("per-page", 20, "Items per page (1-100)")
	addVaultJSONOutputFlag(list)

	get := &cobra.Command{
		Use:     "get <id-or-name>",
		Short:   "Get a provider configuration by ID or name",
		Long:    "Get a provider configuration. Secret credentials are never returned.",
		Args:    cobra.ExactArgs(1),
		PreRunE: vaultProviderConfigPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getVaultProviderConfigsHandler(cmd).Get(cmd.Context(), VaultProviderConfigsGetInput{
				Identifier: args[0],
				Output:     vaultOutput(cmd),
			})
		},
	}
	addVaultJSONOutputFlag(get)

	update := &cobra.Command{
		Use:   "update <id-or-name>",
		Short: "Rename a provider configuration or rotate its secret",
		Long: `Update only the fields you pass; the rest are left unchanged.

The client ID is immutable, so this is the way to rotate a client secret. A
rejected rotation leaves the existing credentials in place. Names must stay
unique within the organization, and renaming does not rebind existing wallets.`,
		Example: `  kernel vaults provider-configs update my-link-client --name renamed-link-client

  kernel vaults provider-configs update my-agentcard --client-secret-file ./rotated-secret.txt`,
		Args:    cobra.ExactArgs(1),
		PreRunE: vaultProviderConfigPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			secret, err := readSecretFlag(cmd, "client-secret")
			if err != nil {
				return err
			}
			in := VaultProviderConfigsUpdateInput{
				Identifier:   args[0],
				ClientSecret: secret,
				Output:       vaultOutput(cmd),
			}
			if cmd.Flags().Changed("name") {
				name, _ := cmd.Flags().GetString("name")
				in.Name = &name
			}
			return getVaultProviderConfigsHandler(cmd).Update(cmd.Context(), in)
		},
	}
	update.Flags().String("name", "", "New configuration name, unique within the organization")
	addVaultProviderConfigSecretFlags(update, "Replacement OAuth client secret")
	addVaultJSONOutputFlag(update)

	del := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete an unused provider configuration",
		Long: `Delete a provider configuration.

The delete is refused with 409 while any non-deleted vault item still
references the configuration, regardless of connection status. Deleting does not
remove the external OAuth client or revoke unrelated grants.`,
		Args:    cobra.ExactArgs(1),
		PreRunE: vaultProviderConfigPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			return getVaultProviderConfigsHandler(cmd).Delete(cmd.Context(), VaultProviderConfigsDeleteInput{
				Identifier:  args[0],
				SkipConfirm: yes,
			})
		},
	}
	del.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	cmd.AddCommand(create, list, get, update, del)
	return cmd
}
