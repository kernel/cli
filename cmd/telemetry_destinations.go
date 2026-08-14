package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

// TelemetryDestinationsService defines the subset of the Kernel SDK OTLP
// destination client that we use.
type TelemetryDestinationsService interface {
	New(ctx context.Context, body kernel.TelemetryDestinationNewParams, opts ...option.RequestOption) (res *kernel.OtlpDestination, err error)
	Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *kernel.OtlpDestination, err error)
	Update(ctx context.Context, idOrName string, body kernel.TelemetryDestinationUpdateParams, opts ...option.RequestOption) (res *kernel.OtlpDestination, err error)
	List(ctx context.Context, query kernel.TelemetryDestinationListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.OtlpDestination], err error)
	Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) (err error)
}

// TelemetryDestinationsCmd handles OTLP destination operations independent of cobra.
type TelemetryDestinationsCmd struct {
	destinations TelemetryDestinationsService
	prompter     interactive.Prompter
}

type TelemetryDestinationsListInput struct {
	Page    int
	PerPage int
	Name    string
	Query   string
	Output  string
}

type TelemetryDestinationsGetInput struct {
	Identifier string
	Output     string
}

type TelemetryDestinationsCreateInput struct {
	Name        string
	Endpoint    string
	Description string
	Headers     map[string]string
	Output      string
}

type TelemetryDestinationsUpdateInput struct {
	Identifier string
	// Nil means "leave as it is"; a non-nil empty Description clears it.
	Name        *string
	Endpoint    *string
	Description *string
	// Headers adds or replaces individual headers; RemoveHeaders deletes them.
	// Both edit the stored map key by key rather than replacing it.
	Headers       map[string]string
	RemoveHeaders []string
	Output        string
}

type TelemetryDestinationsDeleteInput struct {
	Identifier  string
	SkipConfirm bool
}

func (c TelemetryDestinationsCmd) List(ctx context.Context, in TelemetryDestinationsListInput) error {
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

	if in.Output != "json" {
		pterm.Info.Println("Fetching OTLP destinations...")
	}

	params := kernel.TelemetryDestinationListParams{}
	if in.Name != "" {
		params.Name = kernel.Opt(in.Name)
	}
	if in.Query != "" {
		params.Query = kernel.Opt(in.Query)
	}
	// Request one extra item so the response itself reveals whether another page
	// exists: the SDK keeps the X-Has-More header private.
	params.Limit = kernel.Opt(int64(perPage + 1))
	params.Offset = kernel.Opt(int64((page - 1) * perPage))

	result, err := c.destinations.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var items []kernel.OtlpDestination
	if result != nil {
		items = result.Items
	}

	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}
	itemsThisPage := len(items)

	if in.Output == "json" {
		if len(items) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(items)
	}

	if len(items) == 0 {
		pterm.Info.Println("No OTLP destinations found")
		return nil
	}

	rows := pterm.TableData{{"ID", "Name", "Endpoint", "Description", "Headers", "Created At"}}
	for _, d := range items {
		rows = append(rows, []string{
			d.ID,
			d.Name,
			d.Endpoint,
			util.OrDash(d.Description),
			util.OrDash(formatOtlpDestinationHeaders(d.Headers)),
			util.FormatLocal(d.CreatedAt),
		})
	}
	PrintTableNoPad(rows, true)

	pterm.Printf("\nPage: %d  Per-page: %d  Items this page: %d  Has more: %s\n", page, perPage, itemsThisPage, lo.Ternary(hasMore, "yes", "no"))
	if hasMore {
		nextCmd := fmt.Sprintf("kernel telemetry destinations list --page %d --per-page %d", page+1, perPage)
		if in.Name != "" {
			nextCmd += fmt.Sprintf(" --name \"%s\"", in.Name)
		}
		if in.Query != "" {
			nextCmd += fmt.Sprintf(" --query \"%s\"", in.Query)
		}
		pterm.Printf("Next: %s\n", nextCmd)
	}
	return nil
}

func (c TelemetryDestinationsCmd) Get(ctx context.Context, in TelemetryDestinationsGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	dest, err := c.destinations.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if dest == nil || dest.ID == "" {
		if in.Output == "json" {
			fmt.Println("null")
			return nil
		}
		pterm.Error.Printf("OTLP destination '%s' not found\n", in.Identifier)
		return nil
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(dest)
	}

	printOtlpDestinationDetail(dest)
	return nil
}

func (c TelemetryDestinationsCmd) Create(ctx context.Context, in TelemetryDestinationsCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("--name is required")
	}
	if strings.TrimSpace(in.Endpoint) == "" {
		return fmt.Errorf("--endpoint is required")
	}

	params := kernel.TelemetryDestinationNewParams{
		Name:     in.Name,
		Endpoint: in.Endpoint,
	}
	if in.Description != "" {
		params.Description = kernel.Opt(in.Description)
	}
	if len(in.Headers) > 0 {
		params.Headers = in.Headers
	}

	dest, err := c.destinations.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(dest)
	}

	pterm.Success.Printf("Created OTLP destination: %s\n", dest.ID)
	printOtlpDestinationDetail(dest)
	pterm.Info.Printf("Export a session to it with: kernel browsers create --telemetry-export-otlp %s\n", dest.Name)
	return nil
}

func (c TelemetryDestinationsCmd) Update(ctx context.Context, in TelemetryDestinationsUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.TelemetryDestinationUpdateParams{}
	if in.Name != nil {
		params.Name = kernel.Opt(*in.Name)
	}
	if in.Endpoint != nil {
		params.Endpoint = kernel.Opt(*in.Endpoint)
	}
	if in.Description != nil {
		params.Description = kernel.Opt(*in.Description)
	}

	// Header edits are key by key: a value sets or replaces that header and null
	// deletes it. A nil value cannot be expressed through the typed
	// map[string]string field, so the whole map goes through the SDK's
	// extra-fields escape hatch whenever a removal is requested.
	if len(in.RemoveHeaders) > 0 {
		headers := make(map[string]any, len(in.RemoveHeaders)+len(in.Headers))
		for _, name := range in.RemoveHeaders {
			headers[name] = nil
		}
		// Applied after the removals, so a header named by both flags keeps its
		// new value rather than being deleted.
		for name, value := range in.Headers {
			headers[name] = value
		}
		params.SetExtraFields(map[string]any{"headers": headers})
	} else if len(in.Headers) > 0 {
		params.Headers = in.Headers
	}

	if !params.Name.Valid() && !params.Endpoint.Valid() && !params.Description.Valid() && len(in.Headers) == 0 && len(in.RemoveHeaders) == 0 {
		return fmt.Errorf("nothing to update: pass at least one of --name, --endpoint, --description, --header, or --remove-header")
	}

	dest, err := c.destinations.Update(ctx, in.Identifier, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(dest)
	}

	pterm.Success.Printf("Updated OTLP destination: %s\n", dest.ID)
	printOtlpDestinationDetail(dest)
	return nil
}

func (c TelemetryDestinationsCmd) Delete(ctx context.Context, in TelemetryDestinationsDeleteInput) error {
	if !in.SkipConfirm {
		ok, err := c.prompter.Confirm(
			fmt.Sprintf("delete OTLP destination '%s'", in.Identifier),
			fmt.Sprintf("Are you sure you want to delete OTLP destination '%s'?", in.Identifier),
		)
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := c.destinations.Delete(ctx, in.Identifier); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("OTLP destination '%s' not found\n", in.Identifier)
			return nil
		}
		// A 409 here means the destination is still referenced; the API's own
		// message names what still holds it, so it is surfaced as-is.
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted OTLP destination: %s\n", in.Identifier)
	return nil
}

// formatOtlpDestinationHeaders renders a destination's header names as a
// deterministic comma-separated list. Values are always returned redacted by the
// API, so only the names are shown.
func formatOtlpDestinationHeaders(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func printOtlpDestinationDetail(d *kernel.OtlpDestination) {
	rows := pterm.TableData{
		{"Property", "Value"},
		{"ID", d.ID},
		{"Name", d.Name},
		{"Endpoint", d.Endpoint},
		{"Description", util.OrDash(d.Description)},
		{"Headers", util.OrDash(formatOtlpDestinationHeaders(d.Headers))},
		{"Created At", util.FormatLocal(d.CreatedAt)},
		{"Updated At", util.FormatLocal(d.UpdatedAt)},
	}
	PrintTableNoPad(rows, true)
}

// --- Cobra wiring ---

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Manage browser telemetry export",
	Long:  "Commands for managing where browser sessions export their captured telemetry.",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var telemetryDestinationsCmd = &cobra.Command{
	Use:     "destinations",
	Aliases: []string{"destination", "dest"},
	Short:   "Manage OTLP export destinations",
	Long: "Manage the OTLP/HTTP endpoints that browser sessions export captured telemetry to.\n\n" +
		"Reference a destination by ID or name from 'kernel browsers create --telemetry-export-otlp' or " +
		"'kernel auth connections create --telemetry-export-otlp'.",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var telemetryDestinationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List OTLP destinations",
	Args:  cobra.NoArgs,
	RunE:  runTelemetryDestinationsList,
}

var telemetryDestinationsGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get an OTLP destination by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE:  runTelemetryDestinationsGet,
}

var telemetryDestinationsCreateCmd = &cobra.Command{
	Use:   "create --name <name> --endpoint <url>",
	Short: "Create an OTLP destination",
	Long: "Create an OTLP export destination. The name must be unique within the project.\n\n" +
		"--endpoint takes the collector's base endpoint without a signal path: pass https://api.honeycomb.io " +
		"rather than https://api.honeycomb.io/v1/logs, since Kernel appends the signal path itself. " +
		"Header values are encrypted at rest and always returned redacted.",
	Args: cobra.NoArgs,
	RunE: runTelemetryDestinationsCreate,
}

var telemetryDestinationsUpdateCmd = &cobra.Command{
	Use:   "update <id-or-name>",
	Short: "Update an OTLP destination",
	Long: "Update an OTLP destination. Sessions already exporting to it pick up the new values without " +
		"restarting, which makes this the way to rotate credentials without interrupting export.\n\n" +
		"--header and --remove-header edit the stored headers key by key rather than replacing the whole map, " +
		"so headers you do not name are left as they are.",
	Args: cobra.ExactArgs(1),
	RunE: runTelemetryDestinationsUpdate,
}

var telemetryDestinationsDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete an OTLP destination by ID or name",
	Long: "Delete an OTLP destination. The delete is refused while sessions are still exporting to it, and " +
		"while a managed auth connection still selects it.",
	Args: cobra.ExactArgs(1),
	RunE: runTelemetryDestinationsDelete,
}

func init() {
	addJSONOutputFlag(telemetryDestinationsListCmd)
	telemetryDestinationsListCmd.Flags().Int("page", 1, "Page number (1-based)")
	telemetryDestinationsListCmd.Flags().Int("per-page", 20, "Items per page (default 20)")
	telemetryDestinationsListCmd.Flags().String("name", "", "Filter by exact destination name")
	telemetryDestinationsListCmd.Flags().String("query", "", "Search destinations by name or endpoint substring; IDs match by exact value")

	addJSONOutputFlag(telemetryDestinationsGetCmd)

	addJSONOutputFlag(telemetryDestinationsCreateCmd)
	telemetryDestinationsCreateCmd.Flags().String("name", "", "Destination name, unique within the project (required)")
	_ = telemetryDestinationsCreateCmd.MarkFlagRequired("name")
	telemetryDestinationsCreateCmd.Flags().String("endpoint", "", "Base OTLP/HTTP endpoint of the collector, without a signal path (required)")
	_ = telemetryDestinationsCreateCmd.MarkFlagRequired("endpoint")
	telemetryDestinationsCreateCmd.Flags().String("description", "", "Optional description of the destination")
	telemetryDestinationsCreateCmd.Flags().StringArray("header", nil, "Header sent with each export request as NAME=VALUE, typically an ingestion key (repeatable)")

	addJSONOutputFlag(telemetryDestinationsUpdateCmd)
	telemetryDestinationsUpdateCmd.Flags().String("name", "", "New destination name, unique within the project")
	telemetryDestinationsUpdateCmd.Flags().String("endpoint", "", "New base OTLP/HTTP endpoint, without a signal path")
	telemetryDestinationsUpdateCmd.Flags().String("description", "", "New description; pass an empty string to clear it")
	telemetryDestinationsUpdateCmd.Flags().StringArray("header", nil, "Header to add or replace as NAME=VALUE (repeatable). Other stored headers are left as they are")
	telemetryDestinationsUpdateCmd.Flags().StringArray("remove-header", nil, "Header name to delete (repeatable). Removals are applied before --header is merged, so a header given to both keeps its new value")

	telemetryDestinationsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	telemetryDestinationsCmd.AddCommand(telemetryDestinationsListCmd)
	telemetryDestinationsCmd.AddCommand(telemetryDestinationsGetCmd)
	telemetryDestinationsCmd.AddCommand(telemetryDestinationsCreateCmd)
	telemetryDestinationsCmd.AddCommand(telemetryDestinationsUpdateCmd)
	telemetryDestinationsCmd.AddCommand(telemetryDestinationsDeleteCmd)

	telemetryCmd.AddCommand(telemetryDestinationsCmd)
	rootCmd.AddCommand(telemetryCmd)
}

func getTelemetryDestinationsHandler(cmd *cobra.Command) TelemetryDestinationsCmd {
	client := getKernelClient(cmd)
	svc := client.Telemetry.Destinations
	return TelemetryDestinationsCmd{destinations: &svc, prompter: interactive.NewPrompter()}
}

// headersFromFlag reads a repeated NAME=VALUE flag into a map, rejecting
// malformed entries rather than dropping them: a silently ignored header would
// leave the destination exporting without its ingestion key.
func headersFromFlag(cmd *cobra.Command, flagName string) (map[string]string, error) {
	specs, _ := cmd.Flags().GetStringArray(flagName)
	headers, malformed := parseKeyValueSpecs(specs)
	if len(malformed) > 0 {
		return nil, fmt.Errorf("malformed --%s value %q: expected NAME=VALUE", flagName, malformed[0])
	}
	return headers, nil
}

func runTelemetryDestinationsList(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	name, _ := cmd.Flags().GetString("name")
	query, _ := cmd.Flags().GetString("query")
	c := getTelemetryDestinationsHandler(cmd)
	return c.List(cmd.Context(), TelemetryDestinationsListInput{
		Page:    page,
		PerPage: perPage,
		Name:    name,
		Query:   query,
		Output:  output,
	})
}

func runTelemetryDestinationsGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	c := getTelemetryDestinationsHandler(cmd)
	return c.Get(cmd.Context(), TelemetryDestinationsGetInput{Identifier: args[0], Output: output})
}

func runTelemetryDestinationsCreate(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	endpoint, _ := cmd.Flags().GetString("endpoint")
	description, _ := cmd.Flags().GetString("description")
	headers, err := headersFromFlag(cmd, "header")
	if err != nil {
		return err
	}
	c := getTelemetryDestinationsHandler(cmd)
	return c.Create(cmd.Context(), TelemetryDestinationsCreateInput{
		Name:        name,
		Endpoint:    endpoint,
		Description: description,
		Headers:     headers,
		Output:      output,
	})
}

func runTelemetryDestinationsUpdate(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	headers, err := headersFromFlag(cmd, "header")
	if err != nil {
		return err
	}
	removeHeaders, _ := cmd.Flags().GetStringArray("remove-header")
	in := TelemetryDestinationsUpdateInput{
		Identifier:    args[0],
		Headers:       headers,
		RemoveHeaders: removeHeaders,
		Output:        output,
	}
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		in.Name = &name
	}
	if cmd.Flags().Changed("endpoint") {
		endpoint, _ := cmd.Flags().GetString("endpoint")
		in.Endpoint = &endpoint
	}
	if cmd.Flags().Changed("description") {
		description, _ := cmd.Flags().GetString("description")
		in.Description = &description
	}
	c := getTelemetryDestinationsHandler(cmd)
	return c.Update(cmd.Context(), in)
}

func runTelemetryDestinationsDelete(cmd *cobra.Command, args []string) error {
	skip, _ := cmd.Flags().GetBool("yes")
	c := getTelemetryDestinationsHandler(cmd)
	return c.Delete(cmd.Context(), TelemetryDestinationsDeleteInput{Identifier: args[0], SkipConfirm: skip})
}
