package cmd

import (
	"context"
	"fmt"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type APIKeysService interface {
	New(ctx context.Context, body kernel.APIKeyNewParams, opts ...option.RequestOption) (*kernel.CreatedAPIKey, error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.APIKey, error)
	Update(ctx context.Context, id string, body kernel.APIKeyUpdateParams, opts ...option.RequestOption) (*kernel.APIKey, error)
	List(ctx context.Context, query kernel.APIKeyListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.APIKey], error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) error
}

type APIKeysCmd struct {
	apiKeys APIKeysService
}

type APIKeysCreateInput struct {
	Name         string
	DaysToExpire Int64Flag
	ProjectID    string
	Output       string
}

type APIKeysListInput struct {
	Limit  int
	Offset int
	Output string
}

type APIKeysGetInput struct {
	ID     string
	Output string
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

	table := pterm.TableData{{"ID", "Name", "Scope", "Project", "Masked Key", "Expires At", "Created At"}}
	for _, key := range keys {
		display := newAPIKeyDisplay(key)
		table = append(table, []string{
			display.ID,
			display.Name,
			display.Scope,
			display.Project,
			display.MaskedKey,
			display.ExpiresAt,
			display.CreatedAt,
		})
	}
	PrintTableNoPad(table, true)
	return nil
}

func (c APIKeysCmd) Get(ctx context.Context, in APIKeysGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	key, err := c.apiKeys.Get(ctx, in.ID)
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

func (c APIKeysCmd) Delete(ctx context.Context, in APIKeysDeleteInput) error {
	if !in.SkipConfirm {
		msg := fmt.Sprintf("Are you sure you want to delete API key '%s'?", in.ID)
		pterm.DefaultInteractiveConfirm.DefaultText = msg
		ok, _ := pterm.DefaultInteractiveConfirm.Show()
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

type apiKeyDisplay struct {
	ID           string
	Name         string
	PlaintextKey string
	Scope        string
	Project      string
	MaskedKey    string
	CreatedBy    string
	ExpiresAt    string
	CreatedAt    string
}

func renderCreatedAPIKey(key *kernel.CreatedAPIKey) {
	display := newCreatedAPIKeyDisplay(key)
	rows := pterm.TableData{
		{"Field", "Value"},
		{"ID", display.ID},
		{"Name", display.Name},
		{"Key", display.PlaintextKey},
		{"Scope", display.Scope},
		{"Project", display.Project},
		{"Masked Key", display.MaskedKey},
		{"Expires At", display.ExpiresAt},
	}
	PrintTableNoPad(rows, true)
}

func renderAPIKeyDetails(key *kernel.APIKey) {
	display := newAPIKeyDisplay(*key)
	rows := pterm.TableData{
		{"Field", "Value"},
		{"ID", display.ID},
		{"Name", display.Name},
		{"Scope", display.Scope},
		{"Project", display.Project},
		{"Masked Key", display.MaskedKey},
		{"Created By", display.CreatedBy},
		{"Expires At", display.ExpiresAt},
		{"Created At", display.CreatedAt},
	}
	PrintTableNoPad(rows, true)
}

func newCreatedAPIKeyDisplay(key *kernel.CreatedAPIKey) apiKeyDisplay {
	display := newAPIKeyDisplay(key.APIKey)
	display.PlaintextKey = key.Key
	return display
}

func newAPIKeyDisplay(key kernel.APIKey) apiKeyDisplay {
	return apiKeyDisplay{
		ID:        key.ID,
		Name:      key.Name,
		Scope:     apiKeyScope(key),
		Project:   apiKeyProject(key),
		MaskedKey: key.MaskedKey,
		CreatedBy: apiKeyCreator(key),
		ExpiresAt: apiKeyExpiresAt(key),
		CreatedAt: util.FormatLocal(key.CreatedAt),
	}
}

func apiKeyProject(key kernel.APIKey) string {
	if key.JSON.ProjectName.Valid() && key.ProjectName != "" {
		return key.ProjectName
	}
	if key.JSON.ProjectID.Valid() && key.ProjectID != "" {
		return key.ProjectID
	}
	return "-"
}

func apiKeyScope(key kernel.APIKey) string {
	if key.JSON.ProjectID.Valid() && key.ProjectID != "" {
		return "Project"
	}
	return "Org"
}

func apiKeyCreator(key kernel.APIKey) string {
	if key.CreatedBy.JSON.Name.Valid() && key.CreatedBy.Name != "" {
		return key.CreatedBy.Name
	}
	if key.CreatedBy.JSON.Email.Valid() && key.CreatedBy.Email != "" {
		return key.CreatedBy.Email
	}
	return "-"
}

func apiKeyExpiresAt(key kernel.APIKey) string {
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
	output, _ := cmd.Flags().GetString("output")
	return c.List(cmd.Context(), APIKeysListInput{
		Limit:  limit,
		Offset: offset,
		Output: output,
	})
}

func runAPIKeysGet(cmd *cobra.Command, args []string) error {
	c := getAPIKeysHandler(cmd)
	output, _ := cmd.Flags().GetString("output")
	return c.Get(cmd.Context(), APIKeysGetInput{ID: args[0], Output: output})
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

	addJSONOutputFlag(apiKeysGetCmd)

	addJSONOutputFlag(apiKeysUpdateCmd)
	apiKeysUpdateCmd.Flags().String("name", "", "New API key name (required)")
	_ = apiKeysUpdateCmd.MarkFlagRequired("name")

	apiKeysDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	apiKeysCmd.AddCommand(apiKeysCreateCmd)
	apiKeysCmd.AddCommand(apiKeysListCmd)
	apiKeysCmd.AddCommand(apiKeysGetCmd)
	apiKeysCmd.AddCommand(apiKeysUpdateCmd)
	apiKeysCmd.AddCommand(apiKeysDeleteCmd)

	rootCmd.AddCommand(apiKeysCmd)
}
