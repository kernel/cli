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
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

// VaultsService defines the subset of the Kernel SDK vault client that we use.
type VaultsService interface {
	Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *kernel.Vault, err error)
	List(ctx context.Context, query kernel.VaultListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.Vault], err error)
	Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) (err error)
	Upsert(ctx context.Context, body kernel.VaultUpsertParams, opts ...option.RequestOption) (res *kernel.Vault, err error)
}

// VaultItemsService defines the subset of the Kernel SDK vault item client that we use.
type VaultItemsService interface {
	Get(ctx context.Context, key string, params kernel.VaultItemGetParams, opts ...option.RequestOption) (res *kernel.VaultItemUnion, err error)
	Update(ctx context.Context, key string, params kernel.VaultItemUpdateParams, opts ...option.RequestOption) (res *kernel.VaultItemUnion, err error)
	List(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *[]kernel.VaultItemUnion, err error)
	Delete(ctx context.Context, key string, body kernel.VaultItemDeleteParams, opts ...option.RequestOption) (err error)
	Events(ctx context.Context, key string, params kernel.VaultItemEventsParams, opts ...option.RequestOption) (res *[]kernel.VaultItemEvent, err error)
	PerformOperation(ctx context.Context, key string, params kernel.VaultItemPerformOperationParams, opts ...option.RequestOption) (res *kernel.VaultItemUnion, err error)
	Upsert(ctx context.Context, key string, params kernel.VaultItemUpsertParams, opts ...option.RequestOption) (res *kernel.VaultItemUnion, err error)
}

// VaultsCmd handles vault and vault item operations independent of cobra.
type VaultsCmd struct {
	vaults   VaultsService
	items    VaultItemsService
	prompter interactive.Prompter
}

type VaultsListInput struct {
	Page    int
	PerPage int
	Output  string
}

type VaultsGetInput struct {
	Identifier string
	Output     string
}

type VaultsCreateInput struct {
	Name   string
	Output string
}

type VaultsDeleteInput struct {
	Identifier  string
	SkipConfirm bool
}

type VaultItemsListInput struct {
	Vault  string
	Output string
}

type VaultItemsGetInput struct {
	Vault  string
	Key    string
	Wait   int
	Expand []string
	Output string
}

type VaultItemsCreateInput struct {
	Vault    string
	Key      string
	Type     string
	Spec     string
	SpecFile string
	Output   string
}

type VaultItemsUpdateInput struct {
	Vault    string
	Key      string
	Spec     string
	SpecFile string
	Output   string
}

type VaultItemsDeleteInput struct {
	Vault       string
	Key         string
	SkipConfirm bool
}

type VaultItemsEventsInput struct {
	Vault  string
	Key    string
	After  string
	Wait   int
	Output string
}

type VaultItemsPerformOperationInput struct {
	Vault  string
	Key    string
	Type   string
	Output string
}

// --- Vaults ---

func (v VaultsCmd) List(ctx context.Context, in VaultsListInput) error {
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
		pterm.Info.Println("Fetching vaults...")
	}

	params := kernel.VaultListParams{}
	params.Limit = kernel.Opt(int64(perPage + 1))
	params.Offset = kernel.Opt(int64((page - 1) * perPage))

	result, err := v.vaults.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var items []kernel.Vault
	if result != nil {
		items = result.Items
	}

	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}
	itemsThisPage := len(items)

	if in.Output == "json" {
		return util.PrintPrettyJSONSlice(items)
	}

	if len(items) == 0 {
		pterm.Info.Println("No vaults found")
		return nil
	}

	rows := pterm.TableData{{"Vault ID", "Name", "Created At", "Updated At"}}
	for _, vault := range items {
		rows = append(rows, []string{
			vault.ID,
			vault.Name,
			util.FormatLocal(vault.CreatedAt),
			util.FormatLocal(vault.UpdatedAt),
		})
	}
	PrintTableNoPad(rows, true)

	pterm.Printf("\nPage: %d  Per-page: %d  Items this page: %d  Has more: %s\n", page, perPage, itemsThisPage, lo.Ternary(hasMore, "yes", "no"))
	if hasMore {
		pterm.Printf("Next: %s\n", fmt.Sprintf("kernel vaults list --page %d --per-page %d", page+1, perPage))
	}

	return nil
}

func (v VaultsCmd) Get(ctx context.Context, in VaultsGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	vault, err := v.vaults.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if vault == nil || vault.ID == "" {
		if in.Output == "json" {
			fmt.Println("null")
			return nil
		}
		pterm.Error.Printf("Vault '%s' not found\n", in.Identifier)
		return nil
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(vault)
	}

	printVaultTable(vault)
	return nil
}

func (v VaultsCmd) Create(ctx context.Context, in VaultsCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("--name is required")
	}

	vault, err := v.vaults.Upsert(ctx, kernel.VaultUpsertParams{Name: in.Name})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(vault)
	}

	printVaultTable(vault)
	return nil
}

func (v VaultsCmd) Delete(ctx context.Context, in VaultsDeleteInput) error {
	if !in.SkipConfirm {
		ok, err := v.prompter.Confirm(
			fmt.Sprintf("delete vault '%s'", in.Identifier),
			fmt.Sprintf("Are you sure you want to delete vault '%s'? Its items are invalidated.", in.Identifier),
		)
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := v.vaults.Delete(ctx, in.Identifier); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Vault '%s' not found\n", in.Identifier)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted vault: %s\n", in.Identifier)
	return nil
}

// --- Vault items ---

func (v VaultsCmd) ItemsList(ctx context.Context, in VaultItemsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	if in.Output != "json" {
		pterm.Info.Printf("Fetching items in vault '%s'...\n", in.Vault)
	}

	res, err := v.items.List(ctx, in.Vault)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var items []kernel.VaultItemUnion
	if res != nil {
		items = *res
	}

	if in.Output == "json" {
		return util.PrintPrettyJSONSlice(items)
	}

	if len(items) == 0 {
		pterm.Info.Printf("No items found in vault '%s'\n", in.Vault)
		return nil
	}

	rows := pterm.TableData{{"Key", "Type", "Provider", "Status", "Created At", "Updated At"}}
	for _, item := range items {
		rows = append(rows, []string{
			item.Key,
			orDash(item.Type),
			orDash(item.State.Provider),
			orDash(item.State.Status),
			util.FormatLocal(item.CreatedAt),
			util.FormatLocal(item.UpdatedAt),
		})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func (v VaultsCmd) ItemsGet(ctx context.Context, in VaultItemsGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.VaultItemGetParams{IDOrName: in.Vault}
	if in.Wait > 0 {
		params.Wait = kernel.Opt(int64(in.Wait))
	}
	if expand := normalizeList(in.Expand); len(expand) > 0 {
		params.Expand = expand
	}

	item, err := v.items.Get(ctx, in.Key, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if item == nil || item.ID == "" {
		if in.Output == "json" {
			fmt.Println("null")
			return nil
		}
		pterm.Error.Printf("Vault item '%s' not found in vault '%s'\n", in.Key, in.Vault)
		return nil
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(item)
	}

	printVaultItemTable(item)
	return nil
}

func (v VaultsCmd) ItemsCreate(ctx context.Context, in VaultItemsCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	itemType := strings.ToLower(strings.TrimSpace(in.Type))
	if itemType != "wallet" && itemType != "card" {
		return fmt.Errorf("invalid --type %q: must be one of wallet, card", in.Type)
	}

	raw, err := readVaultItemSpec(in.Spec, in.SpecFile)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return fmt.Errorf("must specify one of --spec or --spec-file")
	}

	params := kernel.VaultItemUpsertParams{IDOrName: in.Vault}
	switch itemType {
	case "wallet":
		var spec kernel.WalletVaultItemSpecUnionParam
		if err := json.Unmarshal(raw, &spec); err != nil {
			return fmt.Errorf("invalid JSON in wallet spec: %w", err)
		}
		params.OfWallet = &kernel.VaultItemUpsertParamsBodyWallet{Spec: spec}
	case "card":
		var spec kernel.CardVaultItemSpecUnionParam
		if err := json.Unmarshal(raw, &spec); err != nil {
			return fmt.Errorf("invalid JSON in card spec: %w", err)
		}
		params.OfCard = &kernel.VaultItemUpsertParamsBodyCard{Spec: spec}
	}

	item, err := v.items.Upsert(ctx, in.Key, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(item)
	}

	printVaultItemTable(item)
	return nil
}

func (v VaultsCmd) ItemsUpdate(ctx context.Context, in VaultItemsUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	raw, err := readVaultItemSpec(in.Spec, in.SpecFile)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return fmt.Errorf("must specify one of --spec or --spec-file")
	}

	var spec kernel.CardVaultItemSpecUnionParam
	if err := json.Unmarshal(raw, &spec); err != nil {
		return fmt.Errorf("invalid JSON in card spec: %w", err)
	}

	item, err := v.items.Update(ctx, in.Key, kernel.VaultItemUpdateParams{
		IDOrName: in.Vault,
		Spec:     spec,
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(item)
	}

	printVaultItemTable(item)
	return nil
}

func (v VaultsCmd) ItemsDelete(ctx context.Context, in VaultItemsDeleteInput) error {
	if !in.SkipConfirm {
		ok, err := v.prompter.Confirm(
			fmt.Sprintf("delete vault item '%s'", in.Key),
			fmt.Sprintf("Are you sure you want to delete item '%s' from vault '%s'? Its secret value is invalidated.", in.Key, in.Vault),
		)
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := v.items.Delete(ctx, in.Key, kernel.VaultItemDeleteParams{IDOrName: in.Vault}); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Vault item '%s' not found in vault '%s'\n", in.Key, in.Vault)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted vault item: %s\n", in.Key)
	return nil
}

func (v VaultsCmd) ItemsEvents(ctx context.Context, in VaultItemsEventsInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.VaultItemEventsParams{IDOrName: in.Vault}
	if in.After != "" {
		params.After = kernel.Opt(in.After)
	}
	if in.Wait > 0 {
		params.Wait = kernel.Opt(int64(in.Wait))
	}

	res, err := v.items.Events(ctx, in.Key, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var events []kernel.VaultItemEvent
	if res != nil {
		events = *res
	}

	if in.Output == "json" {
		return util.PrintPrettyJSONSlice(events)
	}

	if len(events) == 0 {
		pterm.Info.Printf("No events found for item '%s'\n", in.Key)
		return nil
	}

	rows := pterm.TableData{{"Event ID", "Name", "Browser ID", "Created At"}}
	for _, event := range events {
		rows = append(rows, []string{
			event.ID,
			event.Name,
			orDash(event.BrowserID),
			util.FormatLocal(event.CreatedAt),
		})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func (v VaultsCmd) ItemsPerformOperation(ctx context.Context, in VaultItemsPerformOperationInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	opType := strings.ToLower(strings.TrimSpace(in.Type))
	if opType == "" {
		return fmt.Errorf("--type is required")
	}

	item, err := v.items.PerformOperation(ctx, in.Key, kernel.VaultItemPerformOperationParams{
		IDOrName: in.Vault,
		Type:     kernel.VaultItemPerformOperationParamsType(opType),
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(item)
	}

	printVaultItemTable(item)
	return nil
}

// --- Display helpers ---

func printVaultTable(vault *kernel.Vault) {
	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", vault.ID})
	rows = append(rows, []string{"Name", vault.Name})
	rows = append(rows, []string{"Created At", util.FormatLocal(vault.CreatedAt)})
	rows = append(rows, []string{"Updated At", util.FormatLocal(vault.UpdatedAt)})
	PrintTableNoPad(rows, true)
}

func printVaultItemTable(item *kernel.VaultItemUnion) {
	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", item.ID})
	rows = append(rows, []string{"Key", item.Key})
	rows = append(rows, []string{"Type", orDash(item.Type)})
	rows = append(rows, []string{"Provider", orDash(item.State.Provider)})
	rows = append(rows, []string{"Status", orDash(item.State.Status)})
	if item.State.StatusReason != "" {
		rows = append(rows, []string{"Status Reason", item.State.StatusReason})
	}
	if item.Action.Name != "" {
		rows = append(rows, []string{"Action", item.Action.Name})
	}
	if item.Action.URL != "" {
		rows = append(rows, []string{"Action URL", item.Action.URL})
	}
	if ops := vaultItemOperationTypes(item); len(ops) > 0 {
		rows = append(rows, []string{"Available Operations", strings.Join(ops, ", ")})
	}
	if exp := vaultItemExpansionTypes(item); len(exp) > 0 {
		rows = append(rows, []string{"Available Expansions", strings.Join(exp, ", ")})
	}
	rows = append(rows, []string{"Created At", util.FormatLocal(item.CreatedAt)})
	rows = append(rows, []string{"Updated At", util.FormatLocal(item.UpdatedAt)})
	if !item.ExpiresAt.IsZero() {
		rows = append(rows, []string{"Expires At", util.FormatLocal(item.ExpiresAt)})
	}
	PrintTableNoPad(rows, true)

	// The operation descriptions tell the caller which operation to invoke next,
	// so surface them rather than only the bare type names.
	if descriptions := vaultItemOperationDescriptions(item); len(descriptions) > 0 {
		pterm.Println()
		pterm.DefaultSection.Println("Available operations")
		for _, d := range descriptions {
			pterm.Printf("  %s: %s\n", d[0], d[1])
		}
	}
}

// vaultItemOperationTypes flattens the wallet/card variants of available_operations
// into their type names.
func vaultItemOperationTypes(item *kernel.VaultItemUnion) []string {
	var out []string
	for _, op := range item.AvailableOperations.OfVaultItemWalletAvailableOperations {
		out = append(out, op.Type)
	}
	for _, op := range item.AvailableOperations.OfVaultItemCardAvailableOperations {
		out = append(out, op.Type)
	}
	return out
}

func vaultItemOperationDescriptions(item *kernel.VaultItemUnion) [][2]string {
	var out [][2]string
	for _, op := range item.AvailableOperations.OfVaultItemWalletAvailableOperations {
		out = append(out, [2]string{op.Type, op.Description})
	}
	for _, op := range item.AvailableOperations.OfVaultItemCardAvailableOperations {
		out = append(out, [2]string{op.Type, op.Description})
	}
	return out
}

func vaultItemExpansionTypes(item *kernel.VaultItemUnion) []string {
	var out []string
	for _, e := range item.AvailableExpansions.OfVaultItemWalletAvailableExpansions {
		out = append(out, e.Type)
	}
	for _, e := range item.AvailableExpansions.OfVaultItemCardAvailableExpansions {
		out = append(out, e.Type)
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// normalizeList splits comma-separated entries out of a repeatable string flag
// and drops blanks, so --expand a,b and --expand a --expand b behave the same.
func normalizeList(values []string) []string {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

// readVaultItemSpec resolves the --spec / --spec-file inputs into raw JSON. The two
// inputs are mutually exclusive (enforced by cobra); a file path of "-" reads stdin.
// It returns nil when neither input is set.
func readVaultItemSpec(inline, file string) ([]byte, error) {
	data := strings.TrimSpace(inline)
	if file != "" {
		var b []byte
		var err error
		if file == "-" {
			b, err = io.ReadAll(os.Stdin)
		} else {
			b, err = os.ReadFile(file)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read spec file: %w", err)
		}
		data = strings.TrimSpace(string(b))
	}

	if data == "" {
		return nil, nil
	}
	if !json.Valid([]byte(data)) {
		return nil, fmt.Errorf("invalid JSON in spec (must be a JSON object)")
	}
	return []byte(data), nil
}

// --- Cobra wiring ---

var vaultsCmd = &cobra.Command{
	Use:     "vaults",
	Aliases: []string{"vault"},
	Short:   "Manage vaults",
	Long: "Manage project-scoped vaults and the items they hold.\n\n" +
		"A vault is a named container for payment items. Link vaults to a browser session with " +
		"'kernel browsers create --vault' so the session can use their items.",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var vaultsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List vaults in the current project",
	Args:  cobra.NoArgs,
	RunE:  runVaultsList,
}

var vaultsGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get a vault by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultsGet,
}

var vaultsCreateCmd = &cobra.Command{
	Use:   "create --name <name>",
	Short: "Create or retrieve a vault by name",
	Long: "Create a vault with the given name. The name is immutable, and creating with a name that " +
		"already exists returns the existing vault rather than failing.",
	Args: cobra.NoArgs,
	RunE: runVaultsCreate,
}

var vaultsDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a vault by ID or name",
	Long:  "Delete a vault. Every item it holds is invalidated along with it.",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultsDelete,
}

var vaultItemsCmd = &cobra.Command{
	Use:     "items",
	Aliases: []string{"item"},
	Short:   "Manage vault items",
	Long: "Manage the items held in a vault.\n\n" +
		"An item is either a wallet (an authorized funding source) or a card (a payment credential minted " +
		"from a wallet). Items advertise the operations valid in their current state; run 'kernel vaults items get' " +
		"and read Available Operations before invoking one with 'kernel vaults items perform-operation'.",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var vaultItemsListCmd = &cobra.Command{
	Use:   "list <vault-id-or-name>",
	Short: "List items in a vault",
	Long:  "List the items in a vault. Secret values are never returned.",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultItemsList,
}

var vaultItemsGetCmd = &cobra.Command{
	Use:   "get <vault-id-or-name> <key>",
	Short: "Get a vault item",
	Long: "Get a vault item along with the operations currently valid for it.\n\n" +
		"--wait holds for up to that many seconds while the item is pending authorization or approval. " +
		"--expand requests live provider data listed under Available Expansions; expanded data is fetched " +
		"from the provider and is not persisted in the item.",
	Args: cobra.ExactArgs(2),
	RunE: runVaultItemsGet,
}

var vaultItemsCreateCmd = &cobra.Command{
	Use:   "create <vault-id-or-name> <key> --type <wallet|card> --spec <json>",
	Short: "Create or retrieve a vault item by key",
	Long: "Create a vault item under the given key. The key is immutable, and creating with a key that " +
		"already exists returns the existing item rather than failing.\n\n" +
		"--spec takes the provider-specific spec as a JSON object, discriminated by its \"provider\" field.\n\n" +
		"Wallet examples:\n" +
		"  --type wallet --spec '{\"provider\":\"agentcard\",\"user_id\":\"usr_123\"}'\n" +
		"  --type wallet --spec '{\"provider\":\"link\",\"authorization\":{\"method\":\"oauth\",\"client\":{\"type\":\"kernel_managed\"}}}'\n\n" +
		"Card example:\n" +
		"  --type card --spec '{\"provider\":\"agentcard\",\"wallet\":\"my-wallet\",\"merchant\":\"Acme\",\"amount\":1250,\"currency\":\"USD\"}'\n\n" +
		"Amounts are integers in minor currency units (1250 = $12.50).",
	Args: cobra.ExactArgs(2),
	RunE: runVaultItemsCreate,
}

var vaultItemsUpdateCmd = &cobra.Command{
	Use:   "update <vault-id-or-name> <key> --spec <json>",
	Short: "Update a card item's specification",
	Long: "Update a card item's specification before or between authorizations. Only card items can be " +
		"updated; --spec takes the full replacement card spec as a JSON object.",
	Args: cobra.ExactArgs(2),
	RunE: runVaultItemsUpdate,
}

var vaultItemsDeleteCmd = &cobra.Command{
	Use:   "delete <vault-id-or-name> <key>",
	Short: "Delete a vault item",
	Long:  "Delete a vault item. Its secret value is invalidated.",
	Args:  cobra.ExactArgs(2),
	RunE:  runVaultItemsDelete,
}

var vaultItemsEventsCmd = &cobra.Command{
	Use:   "events <vault-id-or-name> <key>",
	Short: "List audit events for a vault item",
	Long: "List the immutable audit events recorded for a vault item, oldest first.\n\n" +
		"--after returns only events after the given event ID, and --wait long-polls for up to that many " +
		"seconds when there is nothing new yet, so the two together follow an item's progress.",
	Args: cobra.ExactArgs(2),
	RunE: runVaultItemsEvents,
}

var vaultItemsPerformOperationCmd = &cobra.Command{
	Use:   "perform-operation <vault-id-or-name> <key> --type <type>",
	Short: "Perform an operation advertised by a vault item",
	Long: "Perform one of the operations the item currently advertises. Run 'kernel vaults items get' first " +
		"and invoke only an operation listed under Available Operations, following its description. " +
		"Operations may call an external provider and return the item's updated state.",
	Args: cobra.ExactArgs(2),
	RunE: runVaultItemsPerformOperation,
}

func init() {
	vaultsCmd.AddCommand(vaultsListCmd)
	vaultsCmd.AddCommand(vaultsGetCmd)
	vaultsCmd.AddCommand(vaultsCreateCmd)
	vaultsCmd.AddCommand(vaultsDeleteCmd)
	vaultsCmd.AddCommand(vaultItemsCmd)

	vaultItemsCmd.AddCommand(vaultItemsListCmd)
	vaultItemsCmd.AddCommand(vaultItemsGetCmd)
	vaultItemsCmd.AddCommand(vaultItemsCreateCmd)
	vaultItemsCmd.AddCommand(vaultItemsUpdateCmd)
	vaultItemsCmd.AddCommand(vaultItemsDeleteCmd)
	vaultItemsCmd.AddCommand(vaultItemsEventsCmd)
	vaultItemsCmd.AddCommand(vaultItemsPerformOperationCmd)

	addJSONOutputFlag(vaultsListCmd)
	vaultsListCmd.Flags().Int("page", 1, "Page number (1-based)")
	vaultsListCmd.Flags().Int("per-page", 20, "Items per page (default 20)")

	addJSONOutputFlag(vaultsGetCmd)

	addJSONOutputFlag(vaultsCreateCmd)
	vaultsCreateCmd.Flags().String("name", "", "Immutable name used to create or retrieve the vault (required)")
	_ = vaultsCreateCmd.MarkFlagRequired("name")

	vaultsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	addJSONOutputFlag(vaultItemsListCmd)

	addJSONOutputFlag(vaultItemsGetCmd)
	vaultItemsGetCmd.Flags().Int("wait", 0, "Hold for up to this many seconds while the item is pending authorization or approval (max 60)")
	vaultItemsGetCmd.Flags().StringArray("expand", nil, "Live fields to include, from the item's available expansions (repeatable or comma-separated; e.g. payment_methods)")

	addJSONOutputFlag(vaultItemsCreateCmd)
	vaultItemsCreateCmd.Flags().String("type", "", "Item type: wallet or card (required)")
	vaultItemsCreateCmd.Flags().String("spec", "", "Provider-specific item spec as a JSON object")
	vaultItemsCreateCmd.Flags().String("spec-file", "", "Read the item spec (JSON object) from a file (use '-' for stdin)")
	vaultItemsCreateCmd.MarkFlagsMutuallyExclusive("spec", "spec-file")
	_ = vaultItemsCreateCmd.MarkFlagRequired("type")

	addJSONOutputFlag(vaultItemsUpdateCmd)
	vaultItemsUpdateCmd.Flags().String("spec", "", "Replacement card spec as a JSON object")
	vaultItemsUpdateCmd.Flags().String("spec-file", "", "Read the card spec (JSON object) from a file (use '-' for stdin)")
	vaultItemsUpdateCmd.MarkFlagsMutuallyExclusive("spec", "spec-file")

	vaultItemsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	addJSONOutputFlag(vaultItemsEventsCmd)
	vaultItemsEventsCmd.Flags().String("after", "", "Return only events after this event ID")
	vaultItemsEventsCmd.Flags().Int("wait", 0, "Long-poll for new events for up to this many seconds (max 60)")

	addJSONOutputFlag(vaultItemsPerformOperationCmd)
	vaultItemsPerformOperationCmd.Flags().String("type", "authorize", "Operation to perform, from the item's available operations")

	rootCmd.AddCommand(vaultsCmd)
}

func getVaultsHandler(cmd *cobra.Command) VaultsCmd {
	client := getKernelClient(cmd)
	svc := client.Vaults
	items := client.Vaults.Items
	return VaultsCmd{vaults: &svc, items: &items, prompter: interactive.NewPrompter()}
}

func runVaultsList(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	return getVaultsHandler(cmd).List(cmd.Context(), VaultsListInput{
		Page:    page,
		PerPage: perPage,
		Output:  output,
	})
}

func runVaultsGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	return getVaultsHandler(cmd).Get(cmd.Context(), VaultsGetInput{Identifier: args[0], Output: output})
}

func runVaultsCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	output, _ := cmd.Flags().GetString("output")
	return getVaultsHandler(cmd).Create(cmd.Context(), VaultsCreateInput{Name: name, Output: output})
}

func runVaultsDelete(cmd *cobra.Command, args []string) error {
	skip, _ := cmd.Flags().GetBool("yes")
	return getVaultsHandler(cmd).Delete(cmd.Context(), VaultsDeleteInput{Identifier: args[0], SkipConfirm: skip})
}

func runVaultItemsList(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	return getVaultsHandler(cmd).ItemsList(cmd.Context(), VaultItemsListInput{Vault: args[0], Output: output})
}

func runVaultItemsGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	wait, _ := cmd.Flags().GetInt("wait")
	expand, _ := cmd.Flags().GetStringArray("expand")
	return getVaultsHandler(cmd).ItemsGet(cmd.Context(), VaultItemsGetInput{
		Vault:  args[0],
		Key:    args[1],
		Wait:   wait,
		Expand: expand,
		Output: output,
	})
}

func runVaultItemsCreate(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	itemType, _ := cmd.Flags().GetString("type")
	spec, _ := cmd.Flags().GetString("spec")
	specFile, _ := cmd.Flags().GetString("spec-file")
	return getVaultsHandler(cmd).ItemsCreate(cmd.Context(), VaultItemsCreateInput{
		Vault:    args[0],
		Key:      args[1],
		Type:     itemType,
		Spec:     spec,
		SpecFile: specFile,
		Output:   output,
	})
}

func runVaultItemsUpdate(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	spec, _ := cmd.Flags().GetString("spec")
	specFile, _ := cmd.Flags().GetString("spec-file")
	return getVaultsHandler(cmd).ItemsUpdate(cmd.Context(), VaultItemsUpdateInput{
		Vault:    args[0],
		Key:      args[1],
		Spec:     spec,
		SpecFile: specFile,
		Output:   output,
	})
}

func runVaultItemsDelete(cmd *cobra.Command, args []string) error {
	skip, _ := cmd.Flags().GetBool("yes")
	return getVaultsHandler(cmd).ItemsDelete(cmd.Context(), VaultItemsDeleteInput{
		Vault:       args[0],
		Key:         args[1],
		SkipConfirm: skip,
	})
}

func runVaultItemsEvents(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	after, _ := cmd.Flags().GetString("after")
	wait, _ := cmd.Flags().GetInt("wait")
	return getVaultsHandler(cmd).ItemsEvents(cmd.Context(), VaultItemsEventsInput{
		Vault:  args[0],
		Key:    args[1],
		After:  after,
		Wait:   wait,
		Output: output,
	})
}

func runVaultItemsPerformOperation(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	opType, _ := cmd.Flags().GetString("type")
	return getVaultsHandler(cmd).ItemsPerformOperation(cmd.Context(), VaultItemsPerformOperationInput{
		Vault:  args[0],
		Key:    args[1],
		Type:   opType,
		Output: output,
	})
}
