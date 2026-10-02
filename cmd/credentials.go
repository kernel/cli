package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/interactive"
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
	prompter    interactive.Prompter
}

type CredentialsListInput struct {
	Domain string
	Query  string
	Limit  int
	Offset int
	Output string
}

type CredentialsGetInput struct {
	Identifier string
	Output     string
}

type CredentialsCreateInput struct {
	Name          string
	Domain        string
	Values        map[string]string
	SSOProvider   string
	TotpSecret    string
	TotpAlgorithm string
	TotpDigits    *int
	TotpPeriod    *int
	Output        string
}

type CredentialsUpdateInput struct {
	Identifier      string
	Name            string
	SSOProvider     string
	TotpSecret      string
	TotpAlgorithm   string
	TotpDigits      *int
	TotpPeriod      *int
	Values          map[string]string
	RemoveValueKeys []string
	Output          string
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
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.CredentialListParams{}
	if in.Domain != "" {
		params.Domain = kernel.Opt(in.Domain)
	}
	if in.Query != "" {
		params.Query = kernel.Opt(in.Query)
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
	if err := validateJSONOutput(in.Output); err != nil {
		return err
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
	}
	tableData = append(tableData, credentialTotpRows(cred)...)
	tableData = append(tableData, pterm.TableData{
		{"SSO Provider", ssoProvider},
		{"Created At", util.FormatLocal(cred.CreatedAt)},
		{"Updated At", util.FormatLocal(cred.UpdatedAt)},
	}...)

	PrintTableNoPad(tableData, true)
	return nil
}

func validateTotpSettings(secret, algorithm string, digits, period *int) error {
	if algorithm == "" && digits == nil && period == nil {
		return nil
	}
	if secret == "" {
		return fmt.Errorf("TOTP settings require --totp-secret")
	}
	if algorithm != "" {
		switch strings.ToUpper(algorithm) {
		case "SHA1", "SHA256", "SHA512":
		default:
			return fmt.Errorf("--totp-algorithm must be SHA1, SHA256, or SHA512")
		}
	}
	if digits != nil && (*digits < 6 || *digits > 9) {
		return fmt.Errorf("--totp-digits must be between 6 and 9")
	}
	if period != nil && (*period < 15 || *period > 300) {
		return fmt.Errorf("--totp-period must be between 15 and 300")
	}
	return nil
}

func (c CredentialsCmd) Create(ctx context.Context, in CredentialsCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
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

	if err := validateTotpSettings(in.TotpSecret, in.TotpAlgorithm, in.TotpDigits, in.TotpPeriod); err != nil {
		return err
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
	if in.TotpAlgorithm != "" {
		params.CreateCredentialRequest.TotpAlgorithm = kernel.CreateCredentialRequestTotpAlgorithm(strings.ToUpper(in.TotpAlgorithm))
	}
	if in.TotpDigits != nil {
		params.CreateCredentialRequest.TotpDigits = kernel.Int(int64(*in.TotpDigits))
	}
	if in.TotpPeriod != nil {
		params.CreateCredentialRequest.TotpPeriod = kernel.Int(int64(*in.TotpPeriod))
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
	}
	tableData = append(tableData, credentialTotpRows(cred)...)
	tableData = append(tableData, []string{"SSO Provider", ssoProvider})

	PrintTableNoPad(tableData, true)

	// If TOTP was configured and we got a code back, show it
	if cred.TotpCode != "" {
		pterm.Info.Printf("Initial TOTP Code: %s (expires: %s)\n", cred.TotpCode, util.FormatLocal(cred.TotpCodeExpiresAt))
	}

	return nil
}

// normalizeTotpAlgorithm validates a TOTP HMAC algorithm and returns its
// canonical upper-case form (SHA1, SHA256, or SHA512).
func normalizeTotpAlgorithm(algorithm string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(algorithm))
	switch normalized {
	case "SHA1", "SHA256", "SHA512":
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid --totp-algorithm %q (must be one of SHA1, SHA256, SHA512)", algorithm)
	}
}

// credentialTotpRows returns TOTP metadata rows for credentials with a TOTP secret.
func credentialTotpRows(cred *kernel.Credential) pterm.TableData {
	if !cred.HasTotpSecret {
		return nil
	}
	rows := pterm.TableData{}
	if cred.TotpAlgorithm != "" {
		rows = append(rows, []string{"TOTP Algorithm", string(cred.TotpAlgorithm)})
	}
	if cred.TotpDigits > 0 {
		rows = append(rows, []string{"TOTP Digits", fmt.Sprintf("%d", cred.TotpDigits)})
	}
	if cred.TotpPeriod > 0 {
		rows = append(rows, []string{"TOTP Period", fmt.Sprintf("%ds", cred.TotpPeriod)})
	}
	return rows
}

func (c CredentialsCmd) Update(ctx context.Context, in CredentialsUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if err := validateTotpSettings(in.TotpSecret, in.TotpAlgorithm, in.TotpDigits, in.TotpPeriod); err != nil {
		return err
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
	if in.TotpAlgorithm != "" {
		params.UpdateCredentialRequest.TotpAlgorithm = kernel.UpdateCredentialRequestTotpAlgorithm(strings.ToUpper(in.TotpAlgorithm))
	}
	if in.TotpDigits != nil {
		params.UpdateCredentialRequest.TotpDigits = kernel.Int(int64(*in.TotpDigits))
	}
	if in.TotpPeriod != nil {
		params.UpdateCredentialRequest.TotpPeriod = kernel.Int(int64(*in.TotpPeriod))
	}
	if len(in.Values) > 0 {
		params.UpdateCredentialRequest.Values = in.Values
	}
	if len(in.RemoveValueKeys) > 0 {
		params.UpdateCredentialRequest.RemoveValueKeys = in.RemoveValueKeys
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
		ok, err := c.prompter.Confirm(
			fmt.Sprintf("delete credential '%s'", in.Identifier),
			fmt.Sprintf("Are you sure you want to delete credential '%s'?", in.Identifier),
		)
		if err != nil {
			return err
		}
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
	if err := validateJSONOutput(in.Output); err != nil {
		return err
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

  # Create a credential with custom TOTP parameters
  kernel credentials create --name "my-8digit-site" --domain "example.com" --value "username=myuser" --totp-secret "JBSWY3DPEHPK3PXP" --totp-algorithm SHA256 --totp-digits 8 --totp-period 60

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
	Long:  `Returns the current TOTP code for a credential with a configured totp_secret.`,
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
	addJSONOutputFlag(credentialsListCmd)
	credentialsListCmd.Flags().String("domain", "", "Filter by domain")
	credentialsListCmd.Flags().String("query", "", "Search credentials by name or domain (IDs match by exact value)")
	credentialsListCmd.Flags().Int("limit", 0, "Maximum number of results to return")
	credentialsListCmd.Flags().Int("offset", 0, "Number of results to skip")

	// Get flags
	addJSONOutputFlag(credentialsGetCmd)

	// Create flags
	addJSONOutputFlag(credentialsCreateCmd)
	credentialsCreateCmd.Flags().String("name", "", "Unique name for the credential (required)")
	credentialsCreateCmd.Flags().String("domain", "", "Target domain this credential is for (required)")
	credentialsCreateCmd.Flags().StringArray("value", []string{}, "Field name=value pair (repeatable, e.g., --value username=myuser --value password=mypass)")
	credentialsCreateCmd.Flags().String("sso-provider", "", "SSO provider (e.g., google, github, microsoft)")
	credentialsCreateCmd.Flags().String("totp-secret", "", "Base32 secret (16-128 characters) or otpauth:// URI for 2FA")
	credentialsCreateCmd.Flags().String("totp-algorithm", "", "TOTP algorithm: SHA1, SHA256, or SHA512 (default SHA1)")
	credentialsCreateCmd.Flags().Int("totp-digits", 6, "TOTP code digits: 6-9 (default 6)")
	credentialsCreateCmd.Flags().Int("totp-period", 30, "TOTP period in seconds: 15-300 (default 30)")
	_ = credentialsCreateCmd.MarkFlagRequired("name")
	_ = credentialsCreateCmd.MarkFlagRequired("domain")

	// Update flags
	addJSONOutputFlag(credentialsUpdateCmd)
	credentialsUpdateCmd.Flags().String("name", "", "New name for the credential")
	credentialsUpdateCmd.Flags().String("sso-provider", "", "SSO provider (set to empty string to remove)")
	credentialsUpdateCmd.Flags().String("totp-secret", "", "Base32 secret (16-128 characters) or otpauth:// URI")
	credentialsUpdateCmd.Flags().String("totp-algorithm", "", "TOTP algorithm: SHA1, SHA256, or SHA512")
	credentialsUpdateCmd.Flags().Int("totp-digits", 6, "TOTP code digits: 6-9")
	credentialsUpdateCmd.Flags().Int("totp-period", 30, "TOTP period in seconds: 15-300")
	credentialsUpdateCmd.Flags().StringArray("value", []string{}, "Field name=value pair to update (repeatable)")
	credentialsUpdateCmd.Flags().StringArray("remove-value-key", []string{}, "Field name to remove from the credential's stored values (repeatable). Removals are applied before --value is merged, so a key given to both keeps its new value")

	// Delete flags
	credentialsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// TOTP code flags
	addJSONOutputFlag(credentialsTotpCodeCmd)
}

func runCredentialsList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	domain, _ := cmd.Flags().GetString("domain")
	query, _ := cmd.Flags().GetString("query")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}
	return c.List(cmd.Context(), CredentialsListInput{
		Domain: domain,
		Query:  query,
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

func optionalIntFlag(cmd *cobra.Command, name string, value int) *int {
	if cmd.Flags().Changed(name) {
		return &value
	}
	return nil
}

func runCredentialsCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	domain, _ := cmd.Flags().GetString("domain")
	valuePairs, _ := cmd.Flags().GetStringArray("value")
	ssoProvider, _ := cmd.Flags().GetString("sso-provider")
	totpSecret, _ := cmd.Flags().GetString("totp-secret")
	algorithm, _ := cmd.Flags().GetString("totp-algorithm")
	digits, _ := cmd.Flags().GetInt("totp-digits")
	period, _ := cmd.Flags().GetInt("totp-period")

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
		Name:          name,
		Domain:        domain,
		Values:        values,
		SSOProvider:   ssoProvider,
		TotpSecret:    totpSecret,
		TotpAlgorithm: algorithm,
		TotpDigits:    optionalIntFlag(cmd, "totp-digits", digits),
		TotpPeriod:    optionalIntFlag(cmd, "totp-period", period),
		Output:        output,
	})
}

func runCredentialsUpdate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	ssoProvider, _ := cmd.Flags().GetString("sso-provider")
	totpSecret, _ := cmd.Flags().GetString("totp-secret")
	algorithm, _ := cmd.Flags().GetString("totp-algorithm")
	digits, _ := cmd.Flags().GetInt("totp-digits")
	period, _ := cmd.Flags().GetInt("totp-period")
	valuePairs, _ := cmd.Flags().GetStringArray("value")
	removeValueKeys, _ := cmd.Flags().GetStringArray("remove-value-key")

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
		Identifier:      args[0],
		Name:            name,
		SSOProvider:     ssoProvider,
		TotpSecret:      totpSecret,
		TotpAlgorithm:   algorithm,
		TotpDigits:      optionalIntFlag(cmd, "totp-digits", digits),
		TotpPeriod:      optionalIntFlag(cmd, "totp-period", period),
		Values:          values,
		RemoveValueKeys: removeValueKeys,
		Output:          output,
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
