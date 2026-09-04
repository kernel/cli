package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
)

var vaultNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,255}$`)

type VaultsCmd struct {
	vaults   *kernel.VaultService
	prompter interactive.Prompter
	openURL  func(string) error
}

func validateVaultName(value, label string) error {
	if !vaultNamePattern.MatchString(value) || value == "." || value == ".." {
		return fmt.Errorf("%s must contain 1-255 letters, digits, dots, underscores, or hyphens (not . or ..)", label)
	}
	return nil
}

func (c VaultsCmd) Create(ctx context.Context, name, output string) error {
	if err := validateVaultName(name, "--name"); err != nil {
		return err
	}
	v, err := c.vaults.Upsert(ctx, kernel.VaultUpsertParams{Name: name}, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return printVault(v, output)
}

func (c VaultsCmd) Get(ctx context.Context, vault, output string) error {
	v, err := c.vaults.Get(ctx, vault, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return printVault(v, output)
}

func (c VaultsCmd) List(ctx context.Context, limit, offset int64, project, output string) error {
	if limit < 1 || limit > 100 || offset < 0 {
		return fmt.Errorf("--limit must be between 1 and 100; --offset must be non-negative")
	}
	var response *http.Response
	page, err := c.vaults.List(ctx, kernel.VaultListParams{Limit: kernel.Opt(limit), Offset: kernel.Opt(offset)}, option.WithMaxRetries(0), option.WithResponseInto(&response))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pagination, err := parseProjectListPagination(response)
	if err != nil {
		return fmt.Errorf("invalid vault pagination metadata")
	}
	if output == "json" {
		items, err := vaultSafeJSONSlice(page.Items, vaultFields)
		if err != nil {
			return err
		}
		return printVaultJSON(struct {
			Vaults     []vaultJSON `json:"vaults"`
			NextOffset int         `json:"next_offset,omitempty"`
		}{items, pagination.NextOffset})
	}
	if len(page.Items) == 0 {
		pterm.Info.Println("No vaults found")
	} else {
		rows := pterm.TableData{{"ID", "Name", "Created At"}}
		for _, v := range page.Items {
			rows = append(rows, []string{v.ID, v.Name, util.FormatLocal(v.CreatedAt)})
		}
		PrintTableNoPad(rows, true)
	}
	if pagination.HasMore {
		projectFlag := ""
		if project != "" {
			projectFlag = fmt.Sprintf(" --project %q", project)
		}
		pterm.Printf("Next: kernel%s vaults list --limit %d --offset %d\n", projectFlag, limit, pagination.NextOffset)
	}
	return nil
}

func (c VaultsCmd) Delete(ctx context.Context, vault, key string, yes bool) error {
	label := "vault " + vault + " and all its items"
	if key != "" {
		label = "vault item " + vault + "/" + key
	}
	if !yes {
		ok, err := c.prompter.Confirm("delete "+label, "Delete "+label+" and invalidate its credentials?")
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}
	var err error
	if key == "" {
		err = c.vaults.Delete(ctx, vault, option.WithMaxRetries(0))
	} else {
		err = c.vaults.Items.Delete(ctx, key, kernel.VaultItemDeleteParams{IDOrName: vault}, option.WithMaxRetries(0))
	}
	if err != nil && !util.IsNotFound(err) {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Println("Deleted or not found: " + label)
	return nil
}

func (c VaultsCmd) ListItems(ctx context.Context, vault, output string) error {
	items, err := c.vaults.Items.List(ctx, vault, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if output == "json" {
		data, err := vaultSafeJSONSlice(*items, vaultItemFields)
		if err != nil {
			return err
		}
		return printVaultJSON(data)
	}
	if len(*items) == 0 {
		pterm.Info.Println("No vault items found")
		return nil
	}
	rows := pterm.TableData{{"Key", "Type", "Provider", "Status", "Action"}}
	for _, item := range *items {
		rows = append(rows, []string{item.Key, item.Type, item.Spec.Provider, item.State.Status, util.OrDash(item.Action.Name)})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func validateVaultWait(wait int64) error {
	if wait < 0 || wait > 60 {
		return fmt.Errorf("--wait must be between 0 and 60 seconds")
	}
	return nil
}

func (c VaultsCmd) GetItem(ctx context.Context, vault, key string, wait int64, expand []string, output string, open bool) error {
	if err := validateVaultWait(wait); err != nil {
		return err
	}
	for _, field := range expand {
		if field != "payment_methods" {
			return fmt.Errorf("--expand only supports payment_methods")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second+30*time.Second)
	defer cancel()
	item, err := c.vaults.Items.Get(ctx, key, kernel.VaultItemGetParams{IDOrName: vault, Wait: kernel.Opt(wait), Expand: expand}, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return c.showItem(item, output, open)
}

func (c VaultsCmd) CreateWallet(ctx context.Context, vault, key, provider, userID, output string, open bool) error {
	var spec kernel.WalletVaultItemSpecUnionParam
	switch provider {
	case "link":
		if userID != "" {
			return fmt.Errorf("--user-id is only supported by agentcard")
		}
		spec = kernel.WalletVaultItemSpecParamOfLink(kernel.WalletVaultItemSpecLinkAuthorizationParam{
			Method: "oauth", Client: kernel.WalletVaultItemSpecLinkAuthorizationClientParam{Type: "kernel_managed"},
		})
	case "agentcard":
		spec.OfAgentcard = &kernel.WalletVaultItemSpecAgentcardParam{}
		if userID != "" {
			if !regexp.MustCompile(`^usr_[A-Za-z0-9_]+$`).MatchString(userID) {
				return fmt.Errorf("--user-id must be an enrolled AgentCard user ID (usr_...)")
			}
			spec.OfAgentcard.UserID = kernel.Opt(userID)
		}
	default:
		return fmt.Errorf("--provider must be link or agentcard")
	}
	item, err := c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{IDOrName: vault, OfWallet: &kernel.VaultItemUpsertParamsBodyWallet{Spec: spec}}, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return c.showItem(item, output, open)
}

func (c VaultsCmd) SaveCard(ctx context.Context, vault, key string, spec kernel.CardVaultItemSpecUnionParam, update bool, output string) error {
	var item *kernel.VaultItemUnion
	var err error
	if update {
		item, err = c.vaults.Items.Update(ctx, key, kernel.VaultItemUpdateParams{IDOrName: vault, Spec: spec}, option.WithMaxRetries(0))
	} else {
		item, err = c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{IDOrName: vault, OfCard: &kernel.VaultItemUpsertParamsBodyCard{Spec: spec}}, option.WithMaxRetries(0))
	}
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return c.showItem(item, output, false)
}

func (c VaultsCmd) Authorize(ctx context.Context, vault, key, output string, open bool) error {
	item, err := c.vaults.Items.Get(ctx, key, kernel.VaultItemGetParams{IDOrName: vault}, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if item.Type != "card" || item.Spec.Provider != "link" || item.State.Status != "requested" {
		return fmt.Errorf("authorization requires a requested Link card; inspect item state and events; do not retry payments or resume indeterminate authorizations")
	}
	available := false
	for _, operation := range item.AsCard().AvailableOperations {
		if operation.Type == "authorize" {
			available = true
			if output != "json" {
				pterm.Info.Println(operation.Description)
			}
		}
	}
	if !available {
		return fmt.Errorf("authorize is not advertised in available_operations; inspect the item and its wallet")
	}
	item, err = c.vaults.Items.PerformOperation(ctx, key, kernel.VaultItemPerformOperationParams{IDOrName: vault, Type: kernel.VaultItemPerformOperationParamsTypeAuthorize}, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return c.showItem(item, output, open)
}

func (c VaultsCmd) Events(ctx context.Context, vault, key, after string, wait int64, output string) error {
	if err := validateVaultWait(wait); err != nil {
		return err
	}
	params := kernel.VaultItemEventsParams{IDOrName: vault, Wait: kernel.Opt(wait)}
	if after != "" {
		params.After = kernel.Opt(after)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second+30*time.Second)
	defer cancel()
	events, err := c.vaults.Items.Events(ctx, key, params, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	data, err := vaultSafeJSONSlice(*events, vaultEventFields)
	if err != nil {
		return err
	}
	if output == "json" {
		return printVaultJSON(data)
	}
	if len(*events) == 0 {
		pterm.Info.Println("No new vault item events")
		return nil
	}
	printVaultEvents(*events, data)
	pterm.Printf("For later events, pass --after %s to items events. Observing events does not retry a payment.\n", (*events)[len(*events)-1].ID)
	return nil
}

func (c VaultsCmd) showItem(item *kernel.VaultItemUnion, output string, open bool) error {
	if err := printVaultItem(item, output); err != nil {
		return err
	}
	if !open {
		return nil
	}
	actionURL := item.Action.URL
	if actionURL == "" {
		if output != "json" {
			pterm.Info.Println("No action URL returned; no browser opened")
		}
		return nil
	}
	u, err := url.Parse(actionURL)
	if err != nil || u.Scheme != "https" || !vaultDisplayURL(actionURL) {
		return fmt.Errorf("action URL is not a display-safe HTTPS URL; no browser opened")
	}
	if err := c.openURL(actionURL); err != nil {
		return fmt.Errorf("could not open the browser; open the returned action URL manually")
	}
	return nil
}
