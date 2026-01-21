package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// CredentialProvidersService defines the subset of the Kernel SDK credential provider client that we use.
type CredentialProvidersService interface {
	New(ctx context.Context, body kernel.CredentialProviderNewParams, opts ...option.RequestOption) (res *kernel.CredentialProvider, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.CredentialProvider, err error)
	Update(ctx context.Context, id string, body kernel.CredentialProviderUpdateParams, opts ...option.RequestOption) (res *kernel.CredentialProvider, err error)
	List(ctx context.Context, opts ...option.RequestOption) (res *[]kernel.CredentialProvider, err error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error)
	Test(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.CredentialProviderTestResult, err error)
}

// CredentialProvidersCmd handles credential provider operations independent of cobra.
type CredentialProvidersCmd struct {
	providers CredentialProvidersService
}

type CredentialProvidersListInput struct {
	Output string
}

type CredentialProvidersGetInput struct {
	ID     string
	Output string
}

type CredentialProvidersCreateInput struct {
	ProviderType    string
	Token           string
	CacheTtlSeconds int64
	Output          string
}

type CredentialProvidersUpdateInput struct {
	ID              string
	Token           string
	CacheTtlSeconds int64
	Enabled         BoolFlag
	Priority        Int64Flag
	Output          string
}

type CredentialProvidersDeleteInput struct {
	ID          string
	SkipConfirm bool
}

type CredentialProvidersTestInput struct {
	ID     string
	Output string
}

func (c CredentialProvidersCmd) List(ctx context.Context, in CredentialProvidersListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	providers, err := c.providers.List(ctx)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if providers == nil || len(*providers) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(*providers)
	}

	if providers == nil || len(*providers) == 0 {
		pterm.Info.Println("No credential providers found")
		return nil
	}

	tableData := pterm.TableData{{"ID", "Provider Type", "Enabled", "Priority", "Created At"}}
	for _, p := range *providers {
		tableData = append(tableData, []string{
			p.ID,
			string(p.ProviderType),
			fmt.Sprintf("%t", p.Enabled),
			fmt.Sprintf("%d", p.Priority),
			util.FormatLocal(p.CreatedAt),
		})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c CredentialProvidersCmd) Get(ctx context.Context, in CredentialProvidersGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	provider, err := c.providers.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(provider)
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", provider.ID},
		{"Provider Type", string(provider.ProviderType)},
		{"Enabled", fmt.Sprintf("%t", provider.Enabled)},
		{"Priority", fmt.Sprintf("%d", provider.Priority)},
		{"Created At", util.FormatLocal(provider.CreatedAt)},
		{"Updated At", util.FormatLocal(provider.UpdatedAt)},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c CredentialProvidersCmd) Create(ctx context.Context, in CredentialProvidersCreateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.ProviderType == "" {
		return fmt.Errorf("--provider-type is required")
	}
	if in.Token == "" {
		return fmt.Errorf("--token is required")
	}

	// Validate provider type
	providerType := strings.ToLower(in.ProviderType)
	if providerType != "onepassword" {
		return fmt.Errorf("invalid provider type: %s (must be 'onepassword')", in.ProviderType)
	}

	params := kernel.CredentialProviderNewParams{
		CreateCredentialProviderRequest: kernel.CreateCredentialProviderRequestParam{
			Token:        in.Token,
			ProviderType: kernel.CreateCredentialProviderRequestProviderTypeOnepassword,
		},
	}
	if in.CacheTtlSeconds > 0 {
		params.CreateCredentialProviderRequest.CacheTtlSeconds = kernel.Opt(in.CacheTtlSeconds)
	}

	if in.Output != "json" {
		pterm.Info.Printf("Creating credential provider (%s)...\n", providerType)
	}

	provider, err := c.providers.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(provider)
	}

	pterm.Success.Printf("Created credential provider: %s\n", provider.ID)

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", provider.ID},
		{"Provider Type", string(provider.ProviderType)},
		{"Enabled", fmt.Sprintf("%t", provider.Enabled)},
		{"Priority", fmt.Sprintf("%d", provider.Priority)},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c CredentialProvidersCmd) Update(ctx context.Context, in CredentialProvidersUpdateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.CredentialProviderUpdateParams{
		UpdateCredentialProviderRequest: kernel.UpdateCredentialProviderRequestParam{},
	}
	if in.Token != "" {
		params.UpdateCredentialProviderRequest.Token = kernel.Opt(in.Token)
	}
	if in.CacheTtlSeconds > 0 {
		params.UpdateCredentialProviderRequest.CacheTtlSeconds = kernel.Opt(in.CacheTtlSeconds)
	}
	if in.Enabled.Set {
		params.UpdateCredentialProviderRequest.Enabled = kernel.Opt(in.Enabled.Value)
	}
	if in.Priority.Set {
		params.UpdateCredentialProviderRequest.Priority = kernel.Opt(in.Priority.Value)
	}

	if in.Output != "json" {
		pterm.Info.Printf("Updating credential provider '%s'...\n", in.ID)
	}

	provider, err := c.providers.Update(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(provider)
	}

	pterm.Success.Printf("Updated credential provider: %s\n", provider.ID)
	return nil
}

func (c CredentialProvidersCmd) Delete(ctx context.Context, in CredentialProvidersDeleteInput) error {
	if !in.SkipConfirm {
		msg := fmt.Sprintf("Are you sure you want to delete credential provider '%s'?", in.ID)
		pterm.DefaultInteractiveConfirm.DefaultText = msg
		ok, _ := pterm.DefaultInteractiveConfirm.Show()
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := c.providers.Delete(ctx, in.ID); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Credential provider '%s' not found\n", in.ID)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted credential provider: %s\n", in.ID)
	return nil
}

func (c CredentialProvidersCmd) Test(ctx context.Context, in CredentialProvidersTestInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Output != "json" {
		pterm.Info.Printf("Testing credential provider '%s'...\n", in.ID)
	}

	result, err := c.providers.Test(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(result)
	}

	if result.Success {
		pterm.Success.Println("Connection test successful")
	} else {
		pterm.Error.Printf("Connection test failed: %s\n", result.Error)
	}

	if len(result.Vaults) > 0 {
		pterm.Info.Println("Accessible vaults:")
		tableData := pterm.TableData{{"Vault ID", "Vault Name"}}
		for _, v := range result.Vaults {
			tableData = append(tableData, []string{v.ID, v.Name})
		}
		PrintTableNoPad(tableData, true)
	} else {
		pterm.Info.Println("No vaults accessible")
	}

	return nil
}

// --- Cobra wiring ---

var credentialProvidersCmd = &cobra.Command{
	Use:     "credential-providers",
	Aliases: []string{"credential-provider", "cred-providers", "cred-provider"},
	Short:   "Manage external credential providers",
	Long:    "Commands for managing external credential providers (e.g., 1Password) for automatic credential lookup",
}

var credentialProvidersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List credential providers",
	Args:  cobra.NoArgs,
	RunE:  runCredentialProvidersList,
}

var credentialProvidersGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a credential provider by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialProvidersGet,
}

var credentialProvidersCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new credential provider",
	Long: `Create a new external credential provider for automatic credential lookup.

Currently supported provider types:
  - onepassword: 1Password service account integration

Examples:
  # Create a 1Password credential provider
  kernel credential-providers create --provider-type onepassword --token "ops_xxx..."

  # Create with custom cache TTL
  kernel credential-providers create --provider-type onepassword --token "ops_xxx..." --cache-ttl 600`,
	Args: cobra.NoArgs,
	RunE: runCredentialProvidersCreate,
}

var credentialProvidersUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a credential provider",
	Long:  `Update a credential provider's configuration (token, cache TTL, enabled status, or priority).`,
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialProvidersUpdate,
}

var credentialProvidersDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a credential provider",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialProvidersDelete,
}

var credentialProvidersTestCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Test a credential provider connection",
	Long:  `Validate the credential provider's token and list accessible vaults.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialProvidersTest,
}

func init() {
	credentialProvidersCmd.AddCommand(credentialProvidersListCmd)
	credentialProvidersCmd.AddCommand(credentialProvidersGetCmd)
	credentialProvidersCmd.AddCommand(credentialProvidersCreateCmd)
	credentialProvidersCmd.AddCommand(credentialProvidersUpdateCmd)
	credentialProvidersCmd.AddCommand(credentialProvidersDeleteCmd)
	credentialProvidersCmd.AddCommand(credentialProvidersTestCmd)

	// List flags
	credentialProvidersListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Get flags
	credentialProvidersGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Create flags
	credentialProvidersCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	credentialProvidersCreateCmd.Flags().String("provider-type", "", "Provider type (e.g., onepassword)")
	credentialProvidersCreateCmd.Flags().String("token", "", "Service account token for the provider")
	credentialProvidersCreateCmd.Flags().Int64("cache-ttl", 0, "How long to cache credential lists in seconds (default 300)")
	_ = credentialProvidersCreateCmd.MarkFlagRequired("provider-type")
	_ = credentialProvidersCreateCmd.MarkFlagRequired("token")

	// Update flags
	credentialProvidersUpdateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	credentialProvidersUpdateCmd.Flags().String("token", "", "New service account token (to rotate credentials)")
	credentialProvidersUpdateCmd.Flags().Int64("cache-ttl", 0, "How long to cache credential lists in seconds")
	credentialProvidersUpdateCmd.Flags().Bool("enabled", true, "Whether the provider is enabled for credential lookups")
	credentialProvidersUpdateCmd.Flags().Int64("priority", 0, "Priority order for credential lookups (lower numbers are checked first)")

	// Delete flags
	credentialProvidersDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Test flags
	credentialProvidersTestCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
}

func runCredentialProvidersList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.CredentialProviders
	c := CredentialProvidersCmd{providers: &svc}
	return c.List(cmd.Context(), CredentialProvidersListInput{
		Output: output,
	})
}

func runCredentialProvidersGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.CredentialProviders
	c := CredentialProvidersCmd{providers: &svc}
	return c.Get(cmd.Context(), CredentialProvidersGetInput{
		ID:     args[0],
		Output: output,
	})
}

func runCredentialProvidersCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	providerType, _ := cmd.Flags().GetString("provider-type")
	token, _ := cmd.Flags().GetString("token")
	cacheTtl, _ := cmd.Flags().GetInt64("cache-ttl")

	svc := client.CredentialProviders
	c := CredentialProvidersCmd{providers: &svc}
	return c.Create(cmd.Context(), CredentialProvidersCreateInput{
		ProviderType:    providerType,
		Token:           token,
		CacheTtlSeconds: cacheTtl,
		Output:          output,
	})
}

func runCredentialProvidersUpdate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	token, _ := cmd.Flags().GetString("token")
	cacheTtl, _ := cmd.Flags().GetInt64("cache-ttl")
	enabled, _ := cmd.Flags().GetBool("enabled")
	priority, _ := cmd.Flags().GetInt64("priority")

	svc := client.CredentialProviders
	c := CredentialProvidersCmd{providers: &svc}
	return c.Update(cmd.Context(), CredentialProvidersUpdateInput{
		ID:              args[0],
		Token:           token,
		CacheTtlSeconds: cacheTtl,
		Enabled:         BoolFlag{Set: cmd.Flags().Changed("enabled"), Value: enabled},
		Priority:        Int64Flag{Set: cmd.Flags().Changed("priority"), Value: priority},
		Output:          output,
	})
}

func runCredentialProvidersDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	skip, _ := cmd.Flags().GetBool("yes")

	svc := client.CredentialProviders
	c := CredentialProvidersCmd{providers: &svc}
	return c.Delete(cmd.Context(), CredentialProvidersDeleteInput{
		ID:          args[0],
		SkipConfirm: skip,
	})
}

func runCredentialProvidersTest(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.CredentialProviders
	c := CredentialProvidersCmd{providers: &svc}
	return c.Test(cmd.Context(), CredentialProvidersTestInput{
		ID:     args[0],
		Output: output,
	})
}
