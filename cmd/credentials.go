package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// CredentialsService defines the subset of the Kernel SDK credential client that we use.
type CredentialsService interface {
	New(ctx context.Context, body kernel.CredentialNewParams, opts ...option.RequestOption) (res *kernel.Credential, err error)
	Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *kernel.Credential, err error)
	Update(ctx context.Context, idOrName string, body kernel.CredentialUpdateParams, opts ...option.RequestOption) (res *kernel.Credential, err error)
	List(ctx context.Context, query kernel.CredentialListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.Credential], err error)
	Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) (err error)
	TotpCode(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *kernel.CredentialTotpCodeResponse, err error)
}

// CredentialsCmd handles credential operations independent of cobra.
type CredentialsCmd struct {
	credentials CredentialsService
}

type CredentialsListInput struct {
	Domain string
	Limit  int
	Offset int
	Output string
}

type CredentialsGetInput struct {
	Identifier string
	Output     string
}

type CredentialsCreateInput struct {
	Name        string
	Domain      string
	Values      map[string]string
	SSOProvider string
	TotpSecret  string
	Output      string
}

type CredentialsUpdateInput struct {
	Identifier  string
	Name        string
	SSOProvider string
	TotpSecret  string
	Values      map[string]string
	Output      string
}

type CredentialsDeleteInput struct {
	Identifier  string
	SkipConfirm bool
}

type CredentialsTotpCodeInput struct {
	Identifier string
	Output     string
}

func (c CredentialsCmd) List(ctx context.Context, in CredentialsListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.CredentialListParams{}
	if in.Domain != "" {
		params.Domain = kernel.Opt(in.Domain)
	}
	if in.Limit > 0 {
		params.Limit = kernel.Opt(int64(in.Limit))
	}
	if in.Offset > 0 {
		params.Offset = kernel.Opt(int64(in.Offset))
	}

	page, err := c.credentials.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var credentials []kernel.Credential
	if page != nil {
		credentials = page.Items
	}

	if in.Output == "json" {
		if len(credentials) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(credentials)
	}

	if len(credentials) == 0 {
		pterm.Info.Println("No credentials found")
		return nil
	}

	tableData := pterm.TableData{{"ID", "Name", "Domain", "Has TOTP", "SSO Provider", "Created At"}}
	for _, cred := range credentials {
		ssoProvider := cred.SSOProvider
		if ssoProvider == "" {
			ssoProvider = "-"
		}
		hasTOTP := "-"
		if cred.HasTotpSecret {
			hasTOTP = "Yes"
		}
		tableData = append(tableData, []string{
			cred.ID,
			cred.Name,
			cred.Domain,
			hasTOTP,
			ssoProvider,
			util.FormatLocal(cred.CreatedAt),
		})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c CredentialsCmd) Get(ctx context.Context, in CredentialsGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	cred, err := c.credentials.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(cred)
	}

	ssoProvider := cred.SSOProvider
	if ssoProvider == "" {
		ssoProvider = "-"
	}
	hasTOTP := "No"
	if cred.HasTotpSecret {
		hasTOTP = "Yes"
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", cred.ID},
		{"Name", cred.Name},
		{"Domain", cred.Domain},
		{"Has TOTP Secret", hasTOTP},
		{"SSO Provider", ssoProvider},
		{"Created At", util.FormatLocal(cred.CreatedAt)},
		{"Updated At", util.FormatLocal(cred.UpdatedAt)},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c CredentialsCmd) Create(ctx context.Context, in CredentialsCreateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Name == "" {
		return fmt.Errorf("--name is required")
	}
	if in.Domain == "" {
		return fmt.Errorf("--domain is required")
	}
	if len(in.Values) == 0 {
		return fmt.Errorf("at least one --value is required")
	}

	params := kernel.CredentialNewParams{
		CreateCredentialRequest: kernel.CreateCredentialRequestParam{
			Name:   in.Name,
			Domain: in.Domain,
			Values: in.Values,
		},
	}
	if in.SSOProvider != "" {
		params.CreateCredentialRequest.SSOProvider = kernel.Opt(in.SSOProvider)
	}
	if in.TotpSecret != "" {
		params.CreateCredentialRequest.TotpSecret = kernel.Opt(in.TotpSecret)
	}

	if in.Output != "json" {
		pterm.Info.Printf("Creating credential '%s'...\n", in.Name)
	}

	cred, err := c.credentials.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(cred)
	}

	pterm.Success.Printf("Created credential: %s\n", cred.ID)

	ssoProvider := cred.SSOProvider
	if ssoProvider == "" {
		ssoProvider = "-"
	}
	hasTOTP := "No"
	if cred.HasTotpSecret {
		hasTOTP = "Yes"
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", cred.ID},
		{"Name", cred.Name},
		{"Domain", cred.Domain},
		{"Has TOTP Secret", hasTOTP},
		{"SSO Provider", ssoProvider},
	}

	PrintTableNoPad(tableData, true)

	// If TOTP was configured and we got a code back, show it
	if cred.TotpCode != "" {
		pterm.Info.Printf("Initial TOTP Code: %s (expires: %s)\n", cred.TotpCode, util.FormatLocal(cred.TotpCodeExpiresAt))
	}

	return nil
}

func (c CredentialsCmd) Update(ctx context.Context, in CredentialsUpdateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.CredentialUpdateParams{
		UpdateCredentialRequest: kernel.UpdateCredentialRequestParam{},
	}
	if in.Name != "" {
		params.UpdateCredentialRequest.Name = kernel.Opt(in.Name)
	}
	if in.SSOProvider != "" {
		params.UpdateCredentialRequest.SSOProvider = kernel.Opt(in.SSOProvider)
	}
	if in.TotpSecret != "" {
		params.UpdateCredentialRequest.TotpSecret = kernel.Opt(in.TotpSecret)
	}
	if len(in.Values) > 0 {
		params.UpdateCredentialRequest.Values = in.Values
	}

	if in.Output != "json" {
		pterm.Info.Printf("Updating credential '%s'...\n", in.Identifier)
	}

	cred, err := c.credentials.Update(ctx, in.Identifier, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(cred)
	}

	pterm.Success.Printf("Updated credential: %s\n", cred.ID)
	return nil
}

func (c CredentialsCmd) Delete(ctx context.Context, in CredentialsDeleteInput) error {
	if !in.SkipConfirm {
		msg := fmt.Sprintf("Are you sure you want to delete credential '%s'?", in.Identifier)
		pterm.DefaultInteractiveConfirm.DefaultText = msg
		ok, _ := pterm.DefaultInteractiveConfirm.Show()
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := c.credentials.Delete(ctx, in.Identifier); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Credential '%s' not found\n", in.Identifier)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted credential: %s\n", in.Identifier)
	return nil
}

func (c CredentialsCmd) TotpCode(ctx context.Context, in CredentialsTotpCodeInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	resp, err := c.credentials.TotpCode(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"TOTP Code", resp.Code},
		{"Expires At", util.FormatLocal(resp.ExpiresAt)},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

// --- Cobra wiring ---

var credentialsCmd = &cobra.Command{
	Use:     "credentials",
	Aliases: []string{"credential", "creds", "cred"},
	Short:   "Manage stored credentials",
	Long:    "Commands for managing stored credentials for automatic re-authentication",
}

var credentialsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List credentials",
	Args:  cobra.NoArgs,
	RunE:  runCredentialsList,
}

var credentialsGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get a credential by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialsGet,
}

var credentialsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new credential",
	Long: `Create a new credential for storing login information.

Examples:
  # Create a simple credential with username/password
  kernel credentials create --name "my-site" --domain "example.com" --value "username=myuser" --value "password=mypass"

  # Create a credential with TOTP for 2FA
  kernel credentials create --name "my-2fa-site" --domain "example.com" --value "username=myuser" --value "password=mypass" --totp-secret "JBSWY3DPEHPK3PXP"

  # Create a credential with SSO provider
  kernel credentials create --name "google-sso" --domain "example.com" --value "email=user@gmail.com" --value "password=mypass" --sso-provider google`,
	Args: cobra.NoArgs,
	RunE: runCredentialsCreate,
}

var credentialsUpdateCmd = &cobra.Command{
	Use:   "update <id-or-name>",
	Short: "Update a credential",
	Long:  `Update a credential's name, SSO provider, TOTP secret, or values.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialsUpdate,
}

var credentialsDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a credential",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialsDelete,
}

var credentialsTotpCodeCmd = &cobra.Command{
	Use:   "totp-code <id-or-name>",
	Short: "Get the current TOTP code for a credential",
	Long:  `Returns the current 6-digit TOTP code for a credential with a configured totp_secret.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialsTotpCode,
}

func init() {
	credentialsCmd.AddCommand(credentialsListCmd)
	credentialsCmd.AddCommand(credentialsGetCmd)
	credentialsCmd.AddCommand(credentialsCreateCmd)
	credentialsCmd.AddCommand(credentialsUpdateCmd)
	credentialsCmd.AddCommand(credentialsDeleteCmd)
	credentialsCmd.AddCommand(credentialsTotpCodeCmd)

	// List flags
	credentialsListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	credentialsListCmd.Flags().String("domain", "", "Filter by domain")
	credentialsListCmd.Flags().Int("limit", 0, "Maximum number of results to return")
	credentialsListCmd.Flags().Int("offset", 0, "Number of results to skip")

	// Get flags
	credentialsGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Create flags
	credentialsCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	credentialsCreateCmd.Flags().String("name", "", "Unique name for the credential (required)")
	credentialsCreateCmd.Flags().String("domain", "", "Target domain this credential is for (required)")
	credentialsCreateCmd.Flags().StringArray("value", []string{}, "Field name=value pair (repeatable, e.g., --value username=myuser --value password=mypass)")
	credentialsCreateCmd.Flags().String("sso-provider", "", "SSO provider (e.g., google, github, microsoft)")
	credentialsCreateCmd.Flags().String("totp-secret", "", "Base32-encoded TOTP secret for 2FA")
	_ = credentialsCreateCmd.MarkFlagRequired("name")
	_ = credentialsCreateCmd.MarkFlagRequired("domain")

	// Update flags
	credentialsUpdateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	credentialsUpdateCmd.Flags().String("name", "", "New name for the credential")
	credentialsUpdateCmd.Flags().String("sso-provider", "", "SSO provider (set to empty string to remove)")
	credentialsUpdateCmd.Flags().String("totp-secret", "", "Base32-encoded TOTP secret (set to empty string to remove)")
	credentialsUpdateCmd.Flags().StringArray("value", []string{}, "Field name=value pair to update (repeatable)")

	// Delete flags
	credentialsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// TOTP code flags
	credentialsTotpCodeCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
}

func runCredentialsList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	domain, _ := cmd.Flags().GetString("domain")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}
	return c.List(cmd.Context(), CredentialsListInput{
		Domain: domain,
		Limit:  limit,
		Offset: offset,
		Output: output,
	})
}

func runCredentialsGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}
	return c.Get(cmd.Context(), CredentialsGetInput{
		Identifier: args[0],
		Output:     output,
	})
}

func runCredentialsCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	domain, _ := cmd.Flags().GetString("domain")
	valuePairs, _ := cmd.Flags().GetStringArray("value")
	ssoProvider, _ := cmd.Flags().GetString("sso-provider")
	totpSecret, _ := cmd.Flags().GetString("totp-secret")

	// Parse value pairs into map
	values := make(map[string]string)
	for _, pair := range valuePairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid value format: %s (expected key=value)", pair)
		}
		values[parts[0]] = parts[1]
	}

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}
	return c.Create(cmd.Context(), CredentialsCreateInput{
		Name:        name,
		Domain:      domain,
		Values:      values,
		SSOProvider: ssoProvider,
		TotpSecret:  totpSecret,
		Output:      output,
	})
}

func runCredentialsUpdate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	ssoProvider, _ := cmd.Flags().GetString("sso-provider")
	totpSecret, _ := cmd.Flags().GetString("totp-secret")
	valuePairs, _ := cmd.Flags().GetStringArray("value")

	// Parse value pairs into map
	values := make(map[string]string)
	for _, pair := range valuePairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid value format: %s (expected key=value)", pair)
		}
		values[parts[0]] = parts[1]
	}

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}
	return c.Update(cmd.Context(), CredentialsUpdateInput{
		Identifier:  args[0],
		Name:        name,
		SSOProvider: ssoProvider,
		TotpSecret:  totpSecret,
		Values:      values,
		Output:      output,
	})
}

func runCredentialsDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	skip, _ := cmd.Flags().GetBool("yes")

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}
	return c.Delete(cmd.Context(), CredentialsDeleteInput{
		Identifier:  args[0],
		SkipConfirm: skip,
	})
}

func runCredentialsTotpCode(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}
	return c.TotpCode(cmd.Context(), CredentialsTotpCodeInput{
		Identifier: args[0],
		Output:     output,
	})
}
