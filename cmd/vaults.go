package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/shared/constant"
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
	pagination, err := parseOffsetPagination(response, offset)
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
		actions, err := effectiveVaultItemActions(&item)
		if err != nil {
			return err
		}
		rows = append(rows, []string{item.Key, item.Type, item.Spec.Provider, item.State.Status, util.OrDash(actions.RequiredAction)})
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

func (c VaultsCmd) GetItem(ctx context.Context, vault, key string, wait int64, expand []string, project, output string, open bool) error {
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
	if err := c.showItem(item, output, open); err != nil {
		return err
	}
	if output != "json" {
		return printVaultOperationHints(item, vault, key, project)
	}
	return nil
}

func (c VaultsCmd) CreateWallet(ctx context.Context, vault, key string, spec kernel.VaultItemUpsertParamsBodyWalletSpecUnion, output string, open bool) error {
	item, err := c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{IDOrName: vault, OfWallet: &kernel.VaultItemUpsertParamsBodyWallet{Spec: spec}}, option.WithMaxRetries(0))
	if err != nil {
		return vaultCredentialError(err)
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

func (c VaultsCmd) Invoke(ctx context.Context, vault, key, operation string, params *vaultFillParams, output string, open bool) error {
	if strings.TrimSpace(operation) == "" {
		return fmt.Errorf("operation must not be empty")
	}
	if operation == "fill" && (params == nil || open) {
		return fmt.Errorf("fill requires --params and does not support --open")
	}
	item, err := c.vaults.Items.Get(ctx, key, kernel.VaultItemGetParams{IDOrName: vault}, option.WithMaxRetries(0))
	if err != nil {
		if operation == "fill" {
			return fmt.Errorf("could not retrieve vault item; fill was not invoked")
		}
		return util.CleanedUpSdkError{Err: err}
	}
	if item == nil {
		return fmt.Errorf("empty vault item response; operation was not invoked")
	}
	actions, err := effectiveVaultItemActions(item)
	if err != nil {
		return fmt.Errorf("invalid vault item operations; operation was not invoked")
	}
	if actions.RecoveryRequired {
		return fmt.Errorf("recovery_required: reconcile the original operation with the provider or support; do not retry, delete, or replace it")
	}
	available := false
	for _, op := range actions.Operations {
		if op.Type == operation {
			available = true
			if output != "json" && operation != "fill" {
				pterm.Info.Println(op.Description)
			}
			break
		}
	}
	if !available {
		return fmt.Errorf("operation %q is not advertised in available_operations; inspect the item", operation)
	}
	if operation == "fill" {
		return c.fill(ctx, vault, key, params, output)
	}
	// Preserve support for other advertised parameterless operations.
	authorize := kernel.VaultItemPerformOperationParamsBodyAuthorize{Type: constant.Authorize(operation)}
	response, err := c.vaults.Items.PerformOperation(ctx, key, kernel.VaultItemPerformOperationParams{IDOrName: vault, OfAuthorize: &authorize}, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if response == nil || (response.Type != "card" && response.Type != "wallet") {
		return fmt.Errorf("unexpected vault operation response; inspect the item and do not retry")
	}
	var updated kernel.VaultItemUnion
	if err := json.Unmarshal([]byte(response.RawJSON()), &updated); err != nil {
		return fmt.Errorf("invalid vault item response; inspect the item and do not retry")
	}
	return c.showItem(&updated, output, open)
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
	actions, err := effectiveVaultItemActions(item)
	if err != nil {
		return err
	}
	if actions.RecoveryRequired {
		return nil
	}
	actionURL := actions.ActionURL
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
