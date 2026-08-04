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

type APIKeysService interface {
	New(ctx context.Context, body kernel.APIKeyNewParams, opts ...option.RequestOption) (*kernel.CreatedAPIKey, error)
	Get(ctx context.Context, id string, query kernel.APIKeyGetParams, opts ...option.RequestOption) (*kernel.APIKey, error)
	Update(ctx context.Context, id string, body kernel.APIKeyUpdateParams, opts ...option.RequestOption) (*kernel.APIKey, error)
	List(ctx context.Context, query kernel.APIKeyListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.APIKey], error)
	Rotate(ctx context.Context, id string, body kernel.APIKeyRotateParams, opts ...option.RequestOption) (*kernel.CreatedAPIKey, error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) error
}

type APIKeysCmd struct {
	apiKeys  APIKeysService
	prompter interactive.Prompter
}

type APIKeysCreateInput struct {
	Name         string
	DaysToExpire Int64Flag
	ProjectID    string
	Output       string
}

type APIKeysListInput struct {
	Limit          int
	Offset         int
	Name           string
	Query          string
	Status         string
	IncludeDeleted bool
	SortBy         string
	SortDirection  string
	Output         string
}

type APIKeysGetInput struct {
	ID             string
	IncludeDeleted bool
	Output         string
}

type APIKeysRotateInput struct {
	ID           string
	DaysToExpire Int64Flag
	ExpireInDays Int64Flag
	SkipConfirm  bool
	Output       string
}

type APIKeysUpdateInput struct {
	ID     string
	Name   string
	Output string
}

type APIKeysDeleteInput struct {
	ID          string
	SkipConfirm bool
}

func (c APIKeysCmd) Create(ctx context.Context, in APIKeysCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Name == "" {
		return fmt.Errorf("--name is required")
	}

	params := kernel.APIKeyNewParams{Name: in.Name}
	if in.DaysToExpire.Set {
		if in.DaysToExpire.Value < 1 || in.DaysToExpire.Value > 3650 {
			return fmt.Errorf("--days-to-expire must be between 1 and 3650")
		}
		params.DaysToExpire = kernel.Int(in.DaysToExpire.Value)
	}
	if in.ProjectID != "" {
		params.ProjectID = kernel.String(in.ProjectID)
	}

	key, err := c.apiKeys.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(key)
	}

	pterm.Success.Printf("Created API key: %s\n", key.ID)
	renderCreatedAPIKey(key)
	return nil
}

func (c APIKeysCmd) List(ctx context.Context, in APIKeysListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Limit < 0 {
		return fmt.Errorf("--limit must be non-negative")
	}
	if in.Offset < 0 {
		return fmt.Errorf("--offset must be non-negative")
	}

	params := kernel.APIKeyListParams{}
	if in.Limit > 0 {
		params.Limit = kernel.Int(int64(in.Limit))
	}
	if in.Offset > 0 {
		params.Offset = kernel.Int(int64(in.Offset))
	}
	if in.Name != "" {
		params.Name = kernel.String(in.Name)
	}
	if in.Query != "" {
		params.Query = kernel.String(in.Query)
	}
	// Prefer the newer --status filter; fall back to the deprecated
	// --include-deleted so existing scripts keep working.
	if in.Status != "" {
		switch in.Status {
		case "active":
			params.Status = kernel.APIKeyListParamsStatusActive
		case "deleted":
			params.Status = kernel.APIKeyListParamsStatusDeleted
		case "all":
			params.Status = kernel.APIKeyListParamsStatusAll
		default:
			return fmt.Errorf("invalid --status value: %s (must be 'active', 'deleted', or 'all')", in.Status)
		}
	} else if in.IncludeDeleted {
		params.IncludeDeleted = kernel.Opt(true)
	}
	if in.SortBy != "" {
		switch in.SortBy {
		case "created_at":
			params.SortBy = kernel.APIKeyListParamsSortByCreatedAt
		case "name":
			params.SortBy = kernel.APIKeyListParamsSortByName
		case "expires_at":
			params.SortBy = kernel.APIKeyListParamsSortByExpiresAt
		default:
			return fmt.Errorf("invalid --sort-by value: %s (must be 'created_at', 'name', or 'expires_at')", in.SortBy)
		}
	}
	if in.SortDirection != "" {
		switch in.SortDirection {
		case "asc":
			params.SortDirection = kernel.APIKeyListParamsSortDirectionAsc
		case "desc":
			params.SortDirection = kernel.APIKeyListParamsSortDirectionDesc
		default:
			return fmt.Errorf("invalid --sort-direction value: %s (must be 'asc' or 'desc')", in.SortDirection)
		}
	}

	page, err := c.apiKeys.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var keys []kernel.APIKey
	if page != nil {
		keys = page.Items
	}

	if in.Output == "json" {
		return util.PrintPrettyJSONSlice(keys)
	}

	if len(keys) == 0 {
		pterm.Info.Println("No API keys found")
		return nil
	}

	// Only surface Deleted At when the filter can actually return deleted keys.
	showDeletedAt := in.IncludeDeleted || in.Status == "deleted" || in.Status == "all"
	header := []string{"ID", "Name", "Scope", "Project", "Masked Key", "Expires At", "Created At"}
	if showDeletedAt {
		header = append(header, "Deleted At")
	}
	table := pterm.TableData{header}
	for _, key := range keys {
		row := []string{
			key.ID,
			key.Name,
			formatAPIKeyScope(key),
			formatAPIKeyProject(key),
			key.MaskedKey,
			formatAPIKeyExpiresAt(key),
			util.FormatLocal(key.CreatedAt),
		}
		if showDeletedAt {
			row = append(row, util.FormatLocal(key.DeletedAt))
		}
		table = append(table, row)
	}
	PrintTableNoPad(table, true)
	return nil
}

func (c APIKeysCmd) Get(ctx context.Context, in APIKeysGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.APIKeyGetParams{}
	if in.IncludeDeleted {
		params.IncludeDeleted = kernel.Bool(true)
	}

	key, err := c.apiKeys.Get(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(key)
	}

	renderAPIKeyDetails(key)
	return nil
}

func (c APIKeysCmd) Update(ctx context.Context, in APIKeysUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Name == "" {
		return fmt.Errorf("--name is required")
	}

	key, err := c.apiKeys.Update(ctx, in.ID, kernel.APIKeyUpdateParams{Name: in.Name})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(key)
	}

	pterm.Success.Printf("Updated API key: %s\n", key.ID)
	return nil
}

// Rotate issues a replacement API key. The rotated key keeps working until its
// grace period elapses, so this is disruptive but not immediately breaking --
// it still prompts, since the old key does eventually stop working.
func (c APIKeysCmd) Rotate(ctx context.Context, in APIKeysRotateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.APIKeyRotateParams{}
	if in.DaysToExpire.Set {
		if in.DaysToExpire.Value < 1 || in.DaysToExpire.Value > 3650 {
			return fmt.Errorf("--days-to-expire must be between 1 and 3650")
		}
		params.DaysToExpire = kernel.Int(in.DaysToExpire.Value)
	}
	if in.ExpireInDays.Set {
		if in.ExpireInDays.Value < 0 {
			return fmt.Errorf("--expire-in-days must be non-negative; use 0 to expire the rotated key immediately")
		}
		params.ExpireInDays = kernel.Int(in.ExpireInDays.Value)
	}

	if !in.SkipConfirm {
		ok, err := c.prompter.Confirm(
			fmt.Sprintf("rotate API key '%s'", in.ID),
			fmt.Sprintf("Are you sure you want to rotate API key '%s'? The current key stops working after its grace period.", in.ID),
		)
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Rotation cancelled")
			return nil
		}
	}

	key, err := c.apiKeys.Rotate(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(key)
	}

	pterm.Success.Printf("Rotated API key %s into new key: %s\n", in.ID, key.ID)
	renderCreatedAPIKey(key)
	return nil
}

func (c APIKeysCmd) Delete(ctx context.Context, in APIKeysDeleteInput) error {
	if !in.SkipConfirm {
		ok, err := c.prompter.Confirm(
			fmt.Sprintf("delete API key '%s'", in.ID),
			fmt.Sprintf("Are you sure you want to delete API key '%s'?", in.ID),
		)
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := c.apiKeys.Delete(ctx, in.ID); err != nil {
		if util.IsNotFound(err) {
			return fmt.Errorf("API key %q not found", in.ID)
		}
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Success.Printf("Deleted API key: %s\n", in.ID)
	return nil
}

func renderCreatedAPIKey(key *kernel.CreatedAPIKey) {
	rows := pterm.TableData{
		{"Field", "Value"},
		{"ID", key.ID},
		{"Name", key.Name},
		{"Key", key.Key},
		{"Scope", formatAPIKeyScope(key.APIKey)},
		{"Project", formatAPIKeyProject(key.APIKey)},
		{"Masked Key", key.MaskedKey},
		{"Expires At", formatAPIKeyExpiresAt(key.APIKey)},
	}
	PrintTableNoPad(rows, true)
}

func renderAPIKeyDetails(key *kernel.APIKey) {
	rows := pterm.TableData{
		{"Field", "Value"},
		{"ID", key.ID},
		{"Name", key.Name},
		{"Scope", formatAPIKeyScope(*key)},
		{"Project", formatAPIKeyProject(*key)},
		{"Masked Key", key.MaskedKey},
		{"Created By", formatAPIKeyCreator(*key)},
		{"Expires At", formatAPIKeyExpiresAt(*key)},
		{"Created At", util.FormatLocal(key.CreatedAt)},
	}
	PrintTableNoPad(rows, true)
}

func formatAPIKeyProject(key kernel.APIKey) string {
	if key.JSON.ProjectName.Valid() && key.ProjectName != "" {
		return key.ProjectName
	}
	if key.JSON.ProjectID.Valid() && key.ProjectID != "" {
		return key.ProjectID
	}
	return "-"
}

func formatAPIKeyScope(key kernel.APIKey) string {
	if key.JSON.ProjectID.Valid() && key.ProjectID != "" {
		return "Project"
	}
	return "Org"
}

func formatAPIKeyCreator(key kernel.APIKey) string {
	if key.CreatedBy.JSON.Name.Valid() && key.CreatedBy.Name != "" {
		return key.CreatedBy.Name
	}
	if key.CreatedBy.JSON.Email.Valid() && key.CreatedBy.Email != "" {
		return key.CreatedBy.Email
	}
	return "-"
}

func formatAPIKeyExpiresAt(key kernel.APIKey) string {
	if !key.JSON.ExpiresAt.Valid() {
		return "Never"
	}
	return util.FormatLocal(key.ExpiresAt)
}

func getAPIKeysHandler(cmd *cobra.Command) APIKeysCmd {
	client := getKernelClient(cmd)
	return APIKeysCmd{apiKeys: &client.APIKeys}
}

func runAPIKeysCreate(cmd *cobra.Command, args []string) error {
	c := getAPIKeysHandler(cmd)
	name, _ := cmd.Flags().GetString("name")
	daysToExpire, _ := cmd.Flags().GetInt64("days-to-expire")
	projectID, _ := cmd.Flags().GetString("project-id")
	output, _ := cmd.Flags().GetString("output")

	return c.Create(cmd.Context(), APIKeysCreateInput{
		Name: name,
		DaysToExpire: Int64Flag{
			Set:   cmd.Flags().Changed("days-to-expire"),
			Value: daysToExpire,
		},
		ProjectID: projectID,
		Output:    output,
	})
}

func runAPIKeysList(cmd *cobra.Command, args []string) error {
	c := getAPIKeysHandler(cmd)
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	name, _ := cmd.Flags().GetString("name")
	query, _ := cmd.Flags().GetString("query")
	status, _ := cmd.Flags().GetString("status")
	includeDeleted, _ := cmd.Flags().GetBool("include-deleted")
	sortBy, _ := cmd.Flags().GetString("sort-by")
	sortDirection, _ := cmd.Flags().GetString("sort-direction")
	output, _ := cmd.Flags().GetString("output")
	return c.List(cmd.Context(), APIKeysListInput{
		Limit:          limit,
		Offset:         offset,
		Name:           name,
		Query:          query,
		Status:         status,
		IncludeDeleted: includeDeleted,
		SortBy:         sortBy,
		SortDirection:  sortDirection,
		Output:         output,
	})
}

func runAPIKeysGet(cmd *cobra.Command, args []string) error {
	c := getAPIKeysHandler(cmd)
	output, _ := cmd.Flags().GetString("output")
	includeDeleted, _ := cmd.Flags().GetBool("include-deleted")
	return c.Get(cmd.Context(), APIKeysGetInput{ID: args[0], IncludeDeleted: includeDeleted, Output: output})
}

func runAPIKeysRotate(cmd *cobra.Command, args []string) error {
	c := getAPIKeysHandler(cmd)
	daysToExpire, _ := cmd.Flags().GetInt64("days-to-expire")
	expireInDays, _ := cmd.Flags().GetInt64("expire-in-days")
	skip, _ := cmd.Flags().GetBool("yes")
	output, _ := cmd.Flags().GetString("output")
	return c.Rotate(cmd.Context(), APIKeysRotateInput{
		ID:           args[0],
		DaysToExpire: Int64Flag{Set: cmd.Flags().Changed("days-to-expire"), Value: daysToExpire},
		ExpireInDays: Int64Flag{Set: cmd.Flags().Changed("expire-in-days"), Value: expireInDays},
		SkipConfirm:  skip,
		Output:       output,
	})
}

func runAPIKeysUpdate(cmd *cobra.Command, args []string) error {
	c := getAPIKeysHandler(cmd)
	name, _ := cmd.Flags().GetString("name")
	output, _ := cmd.Flags().GetString("output")
	return c.Update(cmd.Context(), APIKeysUpdateInput{ID: args[0], Name: name, Output: output})
}

func runAPIKeysDelete(cmd *cobra.Command, args []string) error {
	c := getAPIKeysHandler(cmd)
	skip, _ := cmd.Flags().GetBool("yes")
	return c.Delete(cmd.Context(), APIKeysDeleteInput{ID: args[0], SkipConfirm: skip})
}

var apiKeysCmd = &cobra.Command{
	Use:     "api-keys",
	Aliases: []string{"api-key", "apikeys", "apikey"},
	Short:   "Manage API keys",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var apiKeysCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an API key",
	Long:  "Create an API key.\n\nBy default the new key is org-wide. Use --project-id to create a key whose own access is scoped to that project. The global --project flag only scopes this CLI request.",
	Args:  cobra.NoArgs,
	RunE:  runAPIKeysCreate,
}

var apiKeysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API keys",
	Args:  cobra.NoArgs,
	RunE:  runAPIKeysList,
}

var apiKeysGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get an API key",
	Args:  cobra.ExactArgs(1),
	RunE:  runAPIKeysGet,
}

var apiKeysUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update an API key",
	Args:  cobra.ExactArgs(1),
	RunE:  runAPIKeysUpdate,
}

var apiKeysRotateCmd = &cobra.Command{
	Use:   "rotate <id>",
	Short: "Rotate an API key",
	Long:  "Issue a replacement API key. The rotated key keeps working for a grace period (7 days by default) so callers can migrate; use --expire-in-days 0 to revoke it immediately.",
	Args:  cobra.ExactArgs(1),
	RunE:  runAPIKeysRotate,
}

var apiKeysDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete an API key",
	Args:  cobra.ExactArgs(1),
	RunE:  runAPIKeysDelete,
}

func init() {
	addJSONOutputFlag(apiKeysCreateCmd)
	apiKeysCreateCmd.Flags().String("name", "", "API key name (required)")
	apiKeysCreateCmd.Flags().Int64("days-to-expire", 0, "Number of days until expiry (1-3650); omit for never")
	apiKeysCreateCmd.Flags().String("project-id", "", "Create a project-scoped API key for this project ID; omit for org-wide")
	_ = apiKeysCreateCmd.MarkFlagRequired("name")

	addJSONOutputFlag(apiKeysListCmd)
	apiKeysListCmd.Flags().Int("limit", 0, "Maximum number of results to return")
	apiKeysListCmd.Flags().Int("offset", 0, "Number of results to skip")
	apiKeysListCmd.Flags().String("name", "", "Exact-match filter on API key name (names are not unique, so several keys may match)")
	apiKeysListCmd.Flags().String("query", "", "Search API keys by name, creator, or project (identifiers and masked keys match by exact value or prefix)")
	apiKeysListCmd.Flags().String("status", "", "Filter by status: 'active' (default), 'deleted', or 'all'")
	apiKeysListCmd.Flags().Bool("include-deleted", false, "Deprecated: Use --status all instead. Include soft-deleted API keys in the results")
	apiKeysListCmd.Flags().String("sort-by", "", "Sort by: created_at, name, or expires_at")
	apiKeysListCmd.Flags().String("sort-direction", "", "Sort direction: asc or desc")

	addJSONOutputFlag(apiKeysGetCmd)
	apiKeysGetCmd.Flags().Bool("include-deleted", false, "Include soft-deleted API keys in the lookup")

	addJSONOutputFlag(apiKeysUpdateCmd)
	apiKeysUpdateCmd.Flags().String("name", "", "New API key name (required)")
	_ = apiKeysUpdateCmd.MarkFlagRequired("name")

	addJSONOutputFlag(apiKeysRotateCmd)
	apiKeysRotateCmd.Flags().Int64("days-to-expire", 0, "Lifetime in days for the new key (1-3650); omit to reuse the rotated key's lifetime")
	apiKeysRotateCmd.Flags().Int64("expire-in-days", 0, "Grace period in days before the rotated key expires; 0 expires it immediately, omit for the default 7 days")
	apiKeysRotateCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	apiKeysDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	apiKeysCmd.AddCommand(apiKeysCreateCmd)
	apiKeysCmd.AddCommand(apiKeysListCmd)
	apiKeysCmd.AddCommand(apiKeysGetCmd)
	apiKeysCmd.AddCommand(apiKeysUpdateCmd)
	apiKeysCmd.AddCommand(apiKeysRotateCmd)
	apiKeysCmd.AddCommand(apiKeysDeleteCmd)

	rootCmd.AddCommand(apiKeysCmd)
}
