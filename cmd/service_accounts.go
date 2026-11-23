package cmd

import (
	"context"
	"fmt"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// ServiceAccountsService is the subset of the SDK service used by the CLI.
type ServiceAccountsService interface {
	New(ctx context.Context, body kernel.ServiceAccountNewParams, opts ...option.RequestOption) (*kernel.ServiceAccount, error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ServiceAccount, error)
	Update(ctx context.Context, id string, body kernel.ServiceAccountUpdateParams, opts ...option.RequestOption) (*kernel.ServiceAccount, error)
	List(ctx context.Context, query kernel.ServiceAccountListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ServiceAccount], error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) error
}

type ServiceAccountsCmd struct {
	serviceAccounts ServiceAccountsService
	prompter        interactive.Prompter
}

type ServiceAccountsListInput struct {
	Page    int
	PerPage int
	Query   string
	Status  string
	Output  string
}

type ServiceAccountsGetInput struct {
	ID     string
	Output string
}

type ServiceAccountsCreateInput struct {
	Name        string
	Description string
	Output      string
}

type ServiceAccountsUpdateInput struct {
	ID              string
	ExpectedVersion int64
	Name            *string
	Description     *string
	Output          string
}

type ServiceAccountsDeleteInput struct {
	ID          string
	SkipConfirm bool
}

func (c ServiceAccountsCmd) List(ctx context.Context, in ServiceAccountsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	page, perPage, err := normalizePageInput(in.Page, in.PerPage)
	if err != nil {
		return err
	}
	status, err := validateWorkloadIdentityListStatus(in.Status)
	if err != nil {
		return err
	}
	if len(in.Query) > 256 {
		return fmt.Errorf("--query must be at most 256 characters")
	}

	params := kernel.ServiceAccountListParams{
		Limit:  kernel.Opt(int64(perPage + 1)),
		Offset: kernel.Opt(int64((page - 1) * perPage)),
	}
	if in.Query != "" {
		params.Query = kernel.Opt(in.Query)
	}
	if status != "" {
		params.Status = kernel.ServiceAccountListParamsStatus(status)
	}

	result, err := c.serviceAccounts.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	items := []kernel.ServiceAccount{}
	if result != nil {
		items = result.Items
	}
	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}

	if in.Output == "json" {
		return util.PrintPageJSON(items, page, perPage, hasMore)
	}
	if len(items) == 0 {
		pterm.Info.Println("No service accounts found")
	} else {
		rows := pterm.TableData{{"ID", "Name", "Description", "Status", "Security Version", "Last Used At", "Created At"}}
		for _, account := range items {
			rows = append(rows, []string{
				account.ID,
				account.Name,
				util.OrDash(account.Description),
				workloadIdentityResourceStatus(account.JSON.ArchivedAt.Valid()),
				fmt.Sprintf("%d", account.SecurityVersion),
				util.FormatLocal(account.LastUsedAt),
				util.FormatLocal(account.CreatedAt),
			})
		}
		PrintTableNoPad(rows, true)
	}

	filters := ""
	if in.Query != "" {
		filters += fmt.Sprintf(" --query %q", in.Query)
	}
	if status != "" {
		filters += fmt.Sprintf(" --status %s", status)
	}
	nextCommand := fmt.Sprintf("kernel service-accounts list --page %d --per-page %d%s", page+1, perPage, filters)
	printListPaginationFooter(page, perPage, len(items), hasMore, nextCommand)
	return nil
}

func (c ServiceAccountsCmd) Get(ctx context.Context, in ServiceAccountsGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	account, err := c.serviceAccounts.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(account)
	}
	printServiceAccountDetail(account)
	return nil
}

func (c ServiceAccountsCmd) Create(ctx context.Context, in ServiceAccountsCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	name, err := validateWorkloadIdentityName(in.Name, "name")
	if err != nil {
		return err
	}
	if err := validateWorkloadIdentityDescription(in.Description); err != nil {
		return err
	}

	params := kernel.ServiceAccountNewParams{ServiceAccountConfig: kernel.ServiceAccountConfigParam{Name: name}}
	if in.Description != "" {
		params.ServiceAccountConfig.Description = kernel.Opt(in.Description)
	}
	account, err := c.serviceAccounts.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(account)
	}
	pterm.Success.Printf("Created service account: %s\n", account.ID)
	printServiceAccountDetail(account)
	return nil
}

func (c ServiceAccountsCmd) Update(ctx context.Context, in ServiceAccountsUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.ExpectedVersion < 1 {
		return fmt.Errorf("--expected-version must be at least 1")
	}

	params := kernel.ServiceAccountUpdateParams{ExpectedVersion: in.ExpectedVersion}
	if in.Name != nil {
		name, err := validateWorkloadIdentityName(*in.Name, "name")
		if err != nil {
			return err
		}
		params.Name = kernel.Opt(name)
	}
	if in.Description != nil {
		if err := validateWorkloadIdentityDescription(*in.Description); err != nil {
			return err
		}
		params.Description = kernel.Opt(*in.Description)
	}
	if in.Name == nil && in.Description == nil {
		return fmt.Errorf("nothing to update: pass --name or --description")
	}

	account, err := c.serviceAccounts.Update(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(account)
	}
	pterm.Success.Printf("Updated service account: %s\n", account.ID)
	printServiceAccountDetail(account)
	return nil
}

func (c ServiceAccountsCmd) Delete(ctx context.Context, in ServiceAccountsDeleteInput) error {
	if !in.SkipConfirm {
		ok, err := c.prompter.Confirm(
			fmt.Sprintf("archive service account '%s'", in.ID),
			fmt.Sprintf("Archive service account '%s'? Existing history is retained, but new tokens will no longer be issued for it.", in.ID),
		)
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Archive cancelled")
			return nil
		}
	}
	if err := c.serviceAccounts.Delete(ctx, in.ID); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Archived service account: %s\n", in.ID)
	return nil
}

func printServiceAccountDetail(account *kernel.ServiceAccount) {
	if account == nil {
		pterm.Info.Println("Service account not found")
		return
	}
	rows := pterm.TableData{
		{"Property", "Value"},
		{"ID", account.ID},
		{"Name", account.Name},
		{"Description", util.OrDash(account.Description)},
		{"Status", workloadIdentityResourceStatus(account.JSON.ArchivedAt.Valid())},
		{"Security Version", fmt.Sprintf("%d", account.SecurityVersion)},
		{"Created By User ID", util.OrDash(account.CreatedByUserID)},
		{"Last Used At", util.FormatLocal(account.LastUsedAt)},
		{"Archived At", util.FormatLocal(account.ArchivedAt)},
		{"Created At", util.FormatLocal(account.CreatedAt)},
		{"Updated At", util.FormatLocal(account.UpdatedAt)},
	}
	PrintTableNoPad(rows, true)
}

var serviceAccountsCmd = &cobra.Command{
	Use:     "service-accounts",
	Aliases: []string{"service-account"},
	Short:   "Manage workload service accounts",
	Long: "Manage organization-scoped service accounts used as workload identity federation targets. " +
		"Reading requires a signed-in organization member; creating, updating, and archiving require credential administrator access. " +
		"Kernel API keys cannot manage service accounts.",
	Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var serviceAccountsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List service account records",
	Long: "List service accounts in the current organization. Search matches name or description case-insensitively and matches an exact service account ID. " +
		"Any signed-in organization member can read this; Kernel API keys cannot.",
	Args: cobra.NoArgs,
	RunE: runServiceAccountsList,
}

var serviceAccountsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a service account",
	Long:  "Get a service account by its stable resource ID. Any signed-in organization member can read this; Kernel API keys cannot.",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceAccountsGet,
}

var serviceAccountsCreateCmd = &cobra.Command{
	Use:   "create --name <name>",
	Short: "Create a service account",
	Long:  "Create an organization-scoped service account. Organization credential administrator access is required; no external namespace-ownership or staff approval is required.",
	Args:  cobra.NoArgs,
	RunE:  runServiceAccountsCreate,
}

var serviceAccountsUpdateCmd = &cobra.Command{
	Use:   "update <id> --expected-version <version>",
	Short: "Update a service account",
	Long:  "Update a service account using optimistic concurrency. Pass the current security_version from get or list as --expected-version.",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceAccountsUpdate,
}

var serviceAccountsDeleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Aliases: []string{"archive"},
	Short:   "Archive a service account",
	Long:    "Archive a service account. This retains history and invalidates associated authority within the documented status bound; it does not delete browser resources the account created.",
	Args:    cobra.ExactArgs(1),
	RunE:    runServiceAccountsDelete,
}

func init() {
	addJSONOutputFlag(serviceAccountsListCmd)
	serviceAccountsListCmd.Flags().Int("page", 1, "Page number (1-based)")
	serviceAccountsListCmd.Flags().Int("per-page", 20, "Items per page (1-99; default 20)")
	serviceAccountsListCmd.Flags().String("query", "", "Case-insensitive search by name or description; an exact service account ID also matches (maximum 256 characters)")
	serviceAccountsListCmd.Flags().String("status", "", "Filter by lifecycle status: active, archived, or all")

	addJSONOutputFlag(serviceAccountsGetCmd)

	addJSONOutputFlag(serviceAccountsCreateCmd)
	serviceAccountsCreateCmd.Flags().String("name", "", "Service account name; starts with a letter or number and contains only letters, numbers, and hyphens (required)")
	serviceAccountsCreateCmd.Flags().String("description", "", "Optional description (maximum 4096 characters)")
	_ = serviceAccountsCreateCmd.MarkFlagRequired("name")

	addJSONOutputFlag(serviceAccountsUpdateCmd)
	serviceAccountsUpdateCmd.Flags().Int64("expected-version", 0, "Current security_version used for optimistic concurrency (required)")
	serviceAccountsUpdateCmd.Flags().String("name", "", "New service account name")
	serviceAccountsUpdateCmd.Flags().String("description", "", "New description; pass an empty string to clear it")
	_ = serviceAccountsUpdateCmd.MarkFlagRequired("expected-version")

	serviceAccountsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	serviceAccountsCmd.AddCommand(serviceAccountsListCmd, serviceAccountsGetCmd, serviceAccountsCreateCmd, serviceAccountsUpdateCmd, serviceAccountsDeleteCmd)
	rootCmd.AddCommand(serviceAccountsCmd)
}

func getServiceAccountsHandler(cmd *cobra.Command) ServiceAccountsCmd {
	client := getKernelClient(cmd)
	return ServiceAccountsCmd{serviceAccounts: &client.ServiceAccounts, prompter: interactive.NewPrompter()}
}

func runServiceAccountsList(cmd *cobra.Command, args []string) error {
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	query, _ := cmd.Flags().GetString("query")
	status, _ := cmd.Flags().GetString("status")
	output, _ := cmd.Flags().GetString("output")
	return getServiceAccountsHandler(cmd).List(cmd.Context(), ServiceAccountsListInput{Page: page, PerPage: perPage, Query: query, Status: status, Output: output})
}

func runServiceAccountsGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	return getServiceAccountsHandler(cmd).Get(cmd.Context(), ServiceAccountsGetInput{ID: args[0], Output: output})
}

func runServiceAccountsCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	output, _ := cmd.Flags().GetString("output")
	return getServiceAccountsHandler(cmd).Create(cmd.Context(), ServiceAccountsCreateInput{Name: name, Description: description, Output: output})
}

func runServiceAccountsUpdate(cmd *cobra.Command, args []string) error {
	expectedVersion, _ := cmd.Flags().GetInt64("expected-version")
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	output, _ := cmd.Flags().GetString("output")
	var namePtr, descriptionPtr *string
	if cmd.Flags().Changed("name") {
		namePtr = &name
	}
	if cmd.Flags().Changed("description") {
		descriptionPtr = &description
	}
	return getServiceAccountsHandler(cmd).Update(cmd.Context(), ServiceAccountsUpdateInput{
		ID: args[0], ExpectedVersion: expectedVersion, Name: namePtr, Description: descriptionPtr, Output: output,
	})
}

func runServiceAccountsDelete(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	return getServiceAccountsHandler(cmd).Delete(cmd.Context(), ServiceAccountsDeleteInput{ID: args[0], SkipConfirm: yes})
}
