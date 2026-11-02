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
	"github.com/kernel/kernel-go-sdk/packages/param"
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

func (c VaultsCmd) GetEncryptionKey(ctx context.Context, vault, output string) error {
	k, err := c.vaults.GetEncryptionKey(ctx, vault, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return printVaultEncryptionKey(k, output)
}

func (c VaultsCmd) List(ctx context.Context, limit, offset int64, query, project, output string) error {
	if limit < 1 || limit > 100 || offset < 0 {
		return fmt.Errorf("--limit must be between 1 and 100; --offset must be non-negative")
	}
	var response *http.Response
	params := kernel.VaultListParams{Limit: kernel.Opt(limit), Offset: kernel.Opt(offset)}
	if query != "" {
		params.Query = kernel.Opt(query)
	}
	page, err := c.vaults.List(ctx, params, option.WithMaxRetries(0), option.WithResponseInto(&response))
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
		queryFlag := ""
		if query != "" {
			queryFlag = fmt.Sprintf(" --query %q", query)
		}
		pterm.Printf("Next: kernel%s vaults list --limit %d --offset %d%s\n", projectFlag, limit, pagination.NextOffset, queryFlag)
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
	filtered, err := vaultSafeJSONSlice(*items, vaultItemFields)
	if err != nil {
		return err
	}
	data, err := json.Marshal(filtered)
	var safe []kernel.VaultItemUnion
	if err != nil || json.Unmarshal(data, &safe) != nil {
		return fmt.Errorf("invalid vault item response")
	}
	rows := pterm.TableData{{"Key", "Type", "Provider", "Status", "Site", "Action"}}
	for _, item := range safe {
		actions, err := effectiveVaultItemActions(&item)
		if err != nil {
			return err
		}
		rows = append(rows, []string{item.Key, item.Type, item.Spec.Provider, item.State.Status, util.OrDash(vaultItemSite(item)), util.OrDash(actions.RequiredAction)})
	}
	PrintTableNoPad(rows, true)
	return nil
}

// isVaultPurchase reports whether an item is a single purchase: a card or a payment_request.
func isVaultPurchase(item *kernel.VaultItemUnion) bool {
	return item.Type == "card" || item.Type == "payment_request"
}

// vaultItemSite names the site a credential signs in to so agents can reuse it.
func vaultItemSite(item kernel.VaultItemUnion) string {
	if item.Type != "credential" {
		return ""
	}
	if item.Spec.Provider != "1password" {
		return item.Spec.Description
	}
	websites := make([]string, 0, len(item.Spec.Requests.Entries))
	for _, entry := range item.Spec.Requests.Entries {
		websites = append(websites, entry.Parameters.Website)
	}
	return strings.Join(websites, ", ")
}

func validateVaultWait(wait int64) error {
	if wait < 0 || wait > 60 {
		return fmt.Errorf("--wait must be between 0 and 60 seconds")
	}
	return nil
}

var vaultItemExpansionTypes = []string{"payment_methods", "link_user_info", "link_shipping_addresses"}

func validateVaultItemExpansions(expand []string) error {
	allowed := make(map[string]bool, len(vaultItemExpansionTypes))
	for _, field := range vaultItemExpansionTypes {
		allowed[field] = true
	}
	for _, field := range expand {
		if !allowed[field] {
			return fmt.Errorf("--expand must contain only: %s", strings.Join(vaultItemExpansionTypes, ", "))
		}
	}
	return nil
}

func (c VaultsCmd) GetItem(ctx context.Context, vault, key string, wait int64, expand []string, project, output string, open bool) error {
	if err := validateVaultWait(wait); err != nil {
		return err
	}
	if err := validateVaultItemExpansions(expand); err != nil {
		return err
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
		if vaultSpecProvider(spec) == "kernel" {
			return kernelVaultRequestError(err, "create")
		}
		return vaultCredentialError(err)
	}
	return c.showItem(item, output, open)
}

// SaveCard creates or updates a card or payment_request item; both take the same provider specs.
func (c VaultsCmd) SaveCard(ctx context.Context, vault, key, itemType string, spec map[string]json.RawMessage, update bool, output string) error {
	var item *kernel.VaultItemUnion
	var err error
	if itemType == "payment_request" {
		requestSpec := param.Override[kernel.PaymentRequestVaultItemSpecUnionParam](spec)
		if update {
			item, err = c.vaults.Items.Update(ctx, key, kernel.VaultItemUpdateParams{IDOrName: vault, OfPaymentRequestVaultItemUpdateRequest: &kernel.VaultItemUpdateParamsBodyPaymentRequestVaultItemUpdateRequest{Type: "payment_request", Spec: requestSpec}}, option.WithMaxRetries(0))
		} else {
			item, err = c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{IDOrName: vault, OfPaymentRequest: &kernel.VaultItemUpsertParamsBodyPaymentRequest{Spec: requestSpec}}, option.WithMaxRetries(0))
		}
	} else {
		cardSpec := param.Override[kernel.CardVaultItemSpecUnionParam](spec)
		if update {
			item, err = c.vaults.Items.Update(ctx, key, kernel.VaultItemUpdateParams{IDOrName: vault, OfCardVaultItemUpdateRequest: &kernel.VaultItemUpdateParamsBodyCardVaultItemUpdateRequest{Type: "card", Spec: cardSpec}}, option.WithMaxRetries(0))
		} else {
			item, err = c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{IDOrName: vault, OfCard: &kernel.VaultItemUpsertParamsBodyCard{Spec: cardSpec}}, option.WithMaxRetries(0))
		}
	}
	if err != nil {
		if string(spec["provider"]) == `"kernel"` {
			return kernelVaultRequestError(err, "create")
		}
		return util.CleanedUpSdkError{Err: err}
	}
	return c.showItem(item, output, false)
}

func (c VaultsCmd) Invoke(ctx context.Context, vault, key, operation string, params *vaultOperationParams, output string, open bool) error {
	if strings.TrimSpace(operation) == "" {
		return fmt.Errorf("operation must not be empty")
	}
	if operation == "fill" && (params == nil || params.Fill == nil || open) {
		return fmt.Errorf("fill requires --params and does not support --open")
	}
	if operation == "webmcp_invoke" && (params == nil || params.WebMCP == nil || open) {
		return fmt.Errorf("webmcp_invoke requires --params and does not support --open")
	}
	if operation == "prepare_checkout" && (params == nil || params.Checkout == nil) {
		return fmt.Errorf("prepare_checkout requires checkout parameters")
	}
	if operation == "confirm_transaction" && (params == nil || params.ConfirmTransaction == nil || open) {
		return fmt.Errorf("confirm_transaction requires --params and does not support --open")
	}
	if operation == "remove_payment_method" && (params == nil || params.RemovePaymentMethod == nil || open) {
		return fmt.Errorf("remove_payment_method requires payment_method_id and does not support --open")
	}
	if isPasskeyOperation(operation) && (params == nil || params.Passkey == nil || open) {
		return fmt.Errorf("%s requires --browser-id and does not support --open", operation)
	}
	if isOnePasswordOperation(operation) && (params == nil || params.OnePassword == nil) {
		return fmt.Errorf("%s requires its documented parameters", operation)
	}
	item, err := c.vaults.Items.Get(ctx, key, kernel.VaultItemGetParams{IDOrName: vault}, option.WithMaxRetries(0))
	if err != nil {
		if operation == "fill" || operation == "1pw_fill" || operation == "webmcp_invoke" {
			return vaultFillLookupError(err, operation)
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
		if actions.RecoveryRequired {
			message := "recovery_required: " + kernelSupportGuidance
			if actions.advertises("restart_verification") {
				message = "recovery_required: " + vaultRecoveryHelp
			}
			return fmt.Errorf("%s", message)
		}
		if isOnePasswordOperation(operation) {
			return fmt.Errorf("operation %q is not advertised in available_operations; inspect the item with items get -o json. For 1Password credentials, check that the linked credential_account is connected and the credential state; do not delete or recreate the item to reset it", operation)
		}
		if operation == "add_payment_method" || operation == "remove_payment_method" {
			return fmt.Errorf("operation %q is not advertised in available_operations; it is offered only on connected kernel wallets where payment methods are enabled, and scoped api keys also need vaults:write on the vault. a wallet holds at most 10 payment methods and 3 open enrollment links. inspect the wallet with wallets payment-methods", operation)
		}
		return fmt.Errorf("operation %q is not advertised in available_operations; inspect the item", operation)
	}
	if operation == "confirm_transaction" && item.Spec.Provider == "kernel" {
		if params.ConfirmTransaction.Currency != strings.ToUpper(item.Spec.Currency) || params.ConfirmTransaction.Amount > item.Spec.Amount {
			return fmt.Errorf("confirm_transaction currency must match the approved purchase currency and amount must not exceed the approved purchase amount; operation was not invoked")
		}
	}
	linkCard := isVaultPurchase(item) && item.Spec.Provider == "link"
	if params != nil && params.Authorize != nil && !linkCard {
		return fmt.Errorf("--browser-id and --page-url only apply to Link cards; authorize was not invoked")
	}
	if operation == "fill" {
		if !linkCard && len(params.Fill.Fields) == 0 {
			return fmt.Errorf("fill for this item requires 1-32 fields binding field names to selectors; only a Link card's fill may omit them. fill was not invoked")
		}
		return c.fill(ctx, vault, key, params.Fill, output, item.Spec.Provider)
	}
	if operation == "1pw_fill" {
		return c.onePasswordFill(ctx, vault, key, params.OnePassword, output)
	}
	if operation == "webmcp_invoke" {
		return c.webMCPInvoke(ctx, vault, key, params.WebMCP, output)
	}
	request := kernel.VaultItemPerformOperationParams{IDOrName: vault}
	opts := []option.RequestOption{option.WithMaxRetries(0)}
	if operation == "confirm_transaction" {
		// Replace with a typed SDK parameter once the operation is generated.
		body, err := json.Marshal(params.ConfirmTransaction)
		if err != nil {
			return err
		}
		opts = append(opts, option.WithRequestBody("application/json", body))
	} else if params != nil && params.Passkey != nil {
		request = *params.Passkey
		request.IDOrName = vault
	} else if params != nil && params.OnePassword != nil {
		request = *params.OnePassword
		request.IDOrName = vault
	} else if operation == "prepare_checkout" {
		if item.Type != "card" || item.Spec.Provider != "agentcard" {
			return fmt.Errorf("prepare_checkout requires an AgentCard card")
		}
		request.OfPrepareCheckout = &kernel.PrepareCheckoutVaultItemOperationRequestParam{Type: "prepare_checkout", Checkout: *params.Checkout}
	} else if operation == "collect" {
		request.OfCollect = &kernel.CollectVaultItemOperationRequestParam{Type: "collect"}
	} else if operation == "add_payment_method" {
		request.OfAddPaymentMethod = &kernel.AddPaymentMethodVaultItemOperationRequestParam{Type: kernel.AddPaymentMethodVaultItemOperationRequestTypeAddPaymentMethod}
	} else if params != nil && params.RemovePaymentMethod != nil {
		request.OfRemovePaymentMethod = params.RemovePaymentMethod
	} else if params != nil && params.Authorize != nil {
		request.OfAuthorize = params.Authorize
	} else {
		// Preserve support for other advertised parameterless operations.
		request.OfAuthorize = &kernel.AuthorizeVaultItemOperationRequestParam{Type: kernel.AuthorizeVaultItemOperationRequestType(operation)}
	}
	response, err := c.vaults.Items.PerformOperation(ctx, key, request, opts...)
	if err != nil {
		if isOnePasswordOperation(operation) {
			return onePasswordOperationError(err, operation)
		}
		if item.Type == "credential" || item.Type == "credential_account" {
			return vaultCredentialError(err)
		}
		if item.Spec.Provider == "kernel" {
			return kernelVaultRequestError(err, operation)
		}
		if operation == "authorize" && linkCard {
			return linkAuthorizeError(err, params != nil && params.Authorize != nil)
		}
		return util.CleanedUpSdkError{Err: err}
	}
	if isPasskeyOperation(operation) {
		return printVaultPasskeyOperationResult(response, operation, output)
	}
	if operation == "add_payment_method" || operation == "remove_payment_method" {
		actionURL, err := printVaultPaymentMethodOperationResult(response, operation, output)
		if err != nil || !open {
			return err
		}
		return c.openActionURL(actionURL, output)
	}
	if response == nil || (response.Type != "card" && response.Type != "payment_request" && response.Type != "wallet" && response.Type != "credential" && response.Type != "credential_account") {
		if item.Spec.Provider == "kernel" {
			return fmt.Errorf("unexpected vault operation response; %s", kernelVaultRetryGuidance(operation))
		}
		return fmt.Errorf("unexpected vault operation response; inspect the item and do not retry")
	}
	var updated kernel.VaultItemUnion
	if err := json.Unmarshal([]byte(response.RawJSON()), &updated); err != nil {
		if item.Spec.Provider == "kernel" {
			return fmt.Errorf("invalid vault item response; %s", kernelVaultRetryGuidance(operation))
		}
		return fmt.Errorf("invalid vault item response; inspect the item and do not retry")
	}
	return c.showItem(&updated, output, open)
}

func (c VaultsCmd) RemovePaymentMethod(ctx context.Context, vault, key, paymentMethodID string, yes bool, output string) error {
	raw, err := json.Marshal(map[string]string{"payment_method_id": paymentMethodID})
	if err != nil {
		return err
	}
	params, err := parseVaultOperationParams("remove_payment_method", string(raw), true, false)
	if err != nil {
		return err
	}
	if !yes {
		label := "payment method " + paymentMethodID + " from wallet " + vault + "/" + key
		ok, err := c.prompter.Confirm("remove "+label, "Remove "+label+" and delete its stored card?")
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Removal cancelled")
			return nil
		}
	}
	return c.Invoke(ctx, vault, key, "remove_payment_method", params, output, false)
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
	return c.openActionURL(actions.ActionURL, output)
}

func (c VaultsCmd) openActionURL(actionURL, output string) error {
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
