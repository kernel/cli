package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// CredentialService defines the subset of the Kernel SDK credential client that we use.
type CredentialService interface {
	New(ctx context.Context, body kernel.CredentialNewParams, opts ...option.RequestOption) (*kernel.Credential, error)
	Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Credential, error)
	Update(ctx context.Context, idOrName string, body kernel.CredentialUpdateParams, opts ...option.RequestOption) (*kernel.Credential, error)
	List(ctx context.Context, query kernel.CredentialListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Credential], error)
	Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) error
	TotpCode(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.CredentialTotpCodeResponse, error)
}

// CredentialsCmd handles credential operations.
type CredentialsCmd struct {
	credentials CredentialService
}

// CreateCredentialInput holds input for creating a credential.
type CreateCredentialInput struct {
	Name        string
	Domain      string
	Values      map[string]string
	SSOProvider string
	TotpSecret  string
}

// Create creates a new credential.
func (c CredentialsCmd) Create(ctx context.Context, in CreateCredentialInput) error {
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

	cred, err := c.credentials.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", cred.ID})
	rows = append(rows, []string{"Name", cred.Name})
	rows = append(rows, []string{"Domain", cred.Domain})
	rows = append(rows, []string{"Created", cred.CreatedAt.Format(time.RFC3339)})
	if cred.SSOProvider != "" {
		rows = append(rows, []string{"SSO Provider", cred.SSOProvider})
	}
	rows = append(rows, []string{"Has TOTP", fmt.Sprintf("%t", cred.HasTotpSecret)})
	if cred.TotpCode != "" {
		rows = append(rows, []string{"TOTP Code", cred.TotpCode})
		rows = append(rows, []string{"TOTP Expires", cred.TotpCodeExpiresAt.Format(time.RFC3339)})
	}

	PrintTableNoPad(rows, true)
	return nil
}

// GetCredentialInput holds input for getting a credential.
type GetCredentialInput struct {
	IDOrName string
}

// Get retrieves a credential by ID or name.
func (c CredentialsCmd) Get(ctx context.Context, in GetCredentialInput) error {
	cred, err := c.credentials.Get(ctx, in.IDOrName)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", cred.ID})
	rows = append(rows, []string{"Name", cred.Name})
	rows = append(rows, []string{"Domain", cred.Domain})
	rows = append(rows, []string{"Created", cred.CreatedAt.Format(time.RFC3339)})
	rows = append(rows, []string{"Updated", cred.UpdatedAt.Format(time.RFC3339)})
	if cred.SSOProvider != "" {
		rows = append(rows, []string{"SSO Provider", cred.SSOProvider})
	}
	rows = append(rows, []string{"Has TOTP", fmt.Sprintf("%t", cred.HasTotpSecret)})

	PrintTableNoPad(rows, true)
	return nil
}

// ListCredentialsInput holds input for listing credentials.
type ListCredentialsInput struct {
	Domain string
	Limit  int64
	Offset int64
}

// List lists credentials.
func (c CredentialsCmd) List(ctx context.Context, in ListCredentialsInput) error {
	params := kernel.CredentialListParams{}
	if in.Domain != "" {
		params.Domain = kernel.Opt(in.Domain)
	}
	if in.Limit > 0 {
		params.Limit = kernel.Opt(in.Limit)
	}
	if in.Offset > 0 {
		params.Offset = kernel.Opt(in.Offset)
	}

	page, err := c.credentials.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	creds := page.Items
	if len(creds) == 0 {
		pterm.Info.Println("No credentials found")
		return nil
	}

	rows := pterm.TableData{{"ID", "Name", "Domain", "SSO", "TOTP"}}
	for _, cred := range creds {
		sso := "-"
		if cred.SSOProvider != "" {
			sso = cred.SSOProvider
		}
		rows = append(rows, []string{
			cred.ID,
			cred.Name,
			cred.Domain,
			sso,
			fmt.Sprintf("%t", cred.HasTotpSecret),
		})
	}

	PrintTableNoPad(rows, true)
	return nil
}

// UpdateCredentialInput holds input for updating a credential.
type UpdateCredentialInput struct {
	IDOrName    string
	Name        string
	Values      map[string]string
	SSOProvider string
	TotpSecret  string
}

// Update updates a credential.
func (c CredentialsCmd) Update(ctx context.Context, in UpdateCredentialInput) error {
	params := kernel.CredentialUpdateParams{
		UpdateCredentialRequest: kernel.UpdateCredentialRequestParam{},
	}

	if in.Name != "" {
		params.UpdateCredentialRequest.Name = kernel.Opt(in.Name)
	}
	if len(in.Values) > 0 {
		params.UpdateCredentialRequest.Values = in.Values
	}
	if in.SSOProvider != "" {
		params.UpdateCredentialRequest.SSOProvider = kernel.Opt(in.SSOProvider)
	}
	if in.TotpSecret != "" {
		params.UpdateCredentialRequest.TotpSecret = kernel.Opt(in.TotpSecret)
	}

	cred, err := c.credentials.Update(ctx, in.IDOrName, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", cred.ID})
	rows = append(rows, []string{"Name", cred.Name})
	rows = append(rows, []string{"Domain", cred.Domain})
	rows = append(rows, []string{"Updated", cred.UpdatedAt.Format(time.RFC3339)})
	if cred.SSOProvider != "" {
		rows = append(rows, []string{"SSO Provider", cred.SSOProvider})
	}
	rows = append(rows, []string{"Has TOTP", fmt.Sprintf("%t", cred.HasTotpSecret)})
	if cred.TotpCode != "" {
		rows = append(rows, []string{"TOTP Code", cred.TotpCode})
		rows = append(rows, []string{"TOTP Expires", cred.TotpCodeExpiresAt.Format(time.RFC3339)})
	}

	pterm.Success.Println("Credential updated")
	PrintTableNoPad(rows, true)
	return nil
}

// DeleteCredentialInput holds input for deleting a credential.
type DeleteCredentialInput struct {
	IDOrName string
}

// Delete removes a credential.
func (c CredentialsCmd) Delete(ctx context.Context, in DeleteCredentialInput) error {
	if err := c.credentials.Delete(ctx, in.IDOrName); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Success.Printf("Credential %s deleted\n", in.IDOrName)
	return nil
}

// TotpCodeInput holds input for getting a TOTP code.
type TotpCodeInput struct {
	IDOrName string
}

// TotpCode gets the current TOTP code for a credential.
func (c CredentialsCmd) TotpCode(ctx context.Context, in TotpCodeInput) error {
	resp, err := c.credentials.TotpCode(ctx, in.IDOrName)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Info.Printf("TOTP Code: %s\n", resp.Code)
	pterm.Info.Printf("Expires: %s\n", resp.ExpiresAt.Format(time.RFC3339))
	return nil
}

// --- Cobra wiring ---

var credentialsCmd = &cobra.Command{
	Use:   "credentials",
	Short: "Manage credentials",
	Long:  "Commands for managing stored credentials for automatic authentication",
}

var credentialsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a credential",
	Long:  "Create a new credential for storing login information",
	Args:  cobra.NoArgs,
	RunE:  runCredentialsCreate,
}

var credentialsGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get credential details",
	Long:  "Retrieve a credential by its ID or name",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialsGet,
}

var credentialsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List credentials",
	Long:  "List credentials with optional filters",
	Args:  cobra.NoArgs,
	RunE:  runCredentialsList,
}

var credentialsUpdateCmd = &cobra.Command{
	Use:   "update <id-or-name>",
	Short: "Update a credential",
	Long:  "Update an existing credential's name or values",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialsUpdate,
}

var credentialsDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a credential",
	Long:  "Delete a credential by its ID or name",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialsDelete,
}

var credentialsTotpCodeCmd = &cobra.Command{
	Use:   "totp-code <id-or-name>",
	Short: "Get TOTP code",
	Long:  "Get the current 6-digit TOTP code for a credential with a configured totp_secret",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredentialsTotpCode,
}

func init() {
	credentialsCmd.AddCommand(credentialsCreateCmd)
	credentialsCmd.AddCommand(credentialsGetCmd)
	credentialsCmd.AddCommand(credentialsListCmd)
	credentialsCmd.AddCommand(credentialsUpdateCmd)
	credentialsCmd.AddCommand(credentialsDeleteCmd)
	credentialsCmd.AddCommand(credentialsTotpCodeCmd)

	// create flags
	credentialsCreateCmd.Flags().String("name", "", "Unique name for the credential (required)")
	credentialsCreateCmd.Flags().String("domain", "", "Target domain this credential is for (required)")
	credentialsCreateCmd.Flags().StringToString("value", nil, "Field values as key=value pairs (use multiple --value flags for multiple fields, e.g. --value username=user --value password=pass)")
	credentialsCreateCmd.Flags().String("sso-provider", "", "SSO provider (e.g., google, github, microsoft)")
	credentialsCreateCmd.Flags().String("totp-secret", "", "Base32-encoded TOTP secret for 2FA")
	_ = credentialsCreateCmd.MarkFlagRequired("name")
	_ = credentialsCreateCmd.MarkFlagRequired("domain")

	// list flags
	credentialsListCmd.Flags().String("domain", "", "Filter by domain")
	credentialsListCmd.Flags().Int64("limit", 0, "Maximum number of results")
	credentialsListCmd.Flags().Int64("offset", 0, "Number of results to skip")

	// update flags
	credentialsUpdateCmd.Flags().String("name", "", "New name for the credential")
	credentialsUpdateCmd.Flags().StringToString("value", nil, "Field values to update as key=value pairs (use multiple --value flags for multiple fields)")
	credentialsUpdateCmd.Flags().String("sso-provider", "", "SSO provider (e.g., google, github, microsoft)")
	credentialsUpdateCmd.Flags().String("totp-secret", "", "Base32-encoded TOTP secret for 2FA")
}

func runCredentialsCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	name, _ := cmd.Flags().GetString("name")
	domain, _ := cmd.Flags().GetString("domain")
	values, _ := cmd.Flags().GetStringToString("value")
	ssoProvider, _ := cmd.Flags().GetString("sso-provider")
	totpSecret, _ := cmd.Flags().GetString("totp-secret")

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}

	return c.Create(cmd.Context(), CreateCredentialInput{
		Name:        name,
		Domain:      domain,
		Values:      values,
		SSOProvider: ssoProvider,
		TotpSecret:  totpSecret,
	})
}

func runCredentialsGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}

	return c.Get(cmd.Context(), GetCredentialInput{IDOrName: args[0]})
}

func runCredentialsList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	domain, _ := cmd.Flags().GetString("domain")
	limit, _ := cmd.Flags().GetInt64("limit")
	offset, _ := cmd.Flags().GetInt64("offset")

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}

	return c.List(cmd.Context(), ListCredentialsInput{
		Domain: domain,
		Limit:  limit,
		Offset: offset,
	})
}

func runCredentialsUpdate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	name, _ := cmd.Flags().GetString("name")
	values, _ := cmd.Flags().GetStringToString("value")
	ssoProvider, _ := cmd.Flags().GetString("sso-provider")
	totpSecret, _ := cmd.Flags().GetString("totp-secret")

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}

	return c.Update(cmd.Context(), UpdateCredentialInput{
		IDOrName:    args[0],
		Name:        name,
		Values:      values,
		SSOProvider: ssoProvider,
		TotpSecret:  totpSecret,
	})
}

func runCredentialsDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}

	return c.Delete(cmd.Context(), DeleteCredentialInput{IDOrName: args[0]})
}

func runCredentialsTotpCode(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	svc := client.Credentials
	c := CredentialsCmd{credentials: &svc}

	return c.TotpCode(cmd.Context(), TotpCodeInput{IDOrName: args[0]})
}
