package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
)

type vaultJSON map[string]json.RawMessage
type vaultOutputFields map[string]vaultOutputFields

// vaultOutputWildcard applies one schema to every property of an object whose
// keys are not known in advance, such as credential field maps.
const vaultOutputWildcard = "*"

func vaultFieldsOf(names string) vaultOutputFields {
	fields := make(vaultOutputFields)
	for _, name := range strings.Fields(names) {
		fields[name] = nil
	}
	return fields
}

var vaultFields = vaultFieldsOf("id name created_at updated_at")
var vaultOperationFields = vaultFieldsOf("type description")
var vaultTotalFields = vaultFieldsOf("type display_text amount")
var vaultMethodFields = vaultOutputFields{
	"id": nil, "provider": nil, "type": nil, "is_default": nil,
	"display":      vaultFieldsOf("label brand last4"),
	"capabilities": {"single_use_card": vaultFieldsOf("eligible reasons")},
}

// Credential field maps are keyed by caller-declared names, so their schema is
// applied to every property instead of a fixed key list.
var vaultCredentialFieldFields = vaultOutputFields{vaultOutputWildcard: vaultFieldsOf("type required sensitive")}
var vaultCredentialFieldStateFields = vaultOutputFields{vaultOutputWildcard: vaultFieldsOf("has_value value")}

var vaultItemFields = vaultOutputFields{
	"id": nil, "key": nil, "type": nil, "version": nil, "created_at": nil, "updated_at": nil, "expires_at": nil,
	"available_operations": vaultOperationFields,
	"available_expansions": vaultOperationFields,
	"action":               vaultFieldsOf("name url expires_at"),
	"expanded":             {"payment_methods": vaultMethodFields},
	"spec": {
		"provider": nil, "wallet": nil, "user_id": nil, "payment_method_id": nil, "card_id": nil,
		"amount": nil, "currency": nil, "merchant": nil, "merchant_name": nil, "merchant_url": nil,
		"context": nil, "expires_at": nil, "description": nil,
		"fields":          vaultCredentialFieldFields,
		"provider_config": vaultFieldsOf("id name"),
		"authorization":   {"method": nil, "client": {"type": nil, "provider_config": vaultFieldsOf("id name")}},
		"totals":          vaultTotalFields,
		"line_items": {
			"name": nil, "quantity": nil, "unit_amount": nil, "description": nil,
			"sku": nil, "url": nil, "image_url": nil, "product_url": nil, "totals": vaultTotalFields,
		},
	},
	"state": {
		"provider": nil, "status": nil, "status_reason": nil, "user_id": nil, "domains": nil,
		"fields":        vaultCredentialFieldStateFields,
		"masks":         vaultFieldsOf("brand last4"),
		"aliases":       vaultFieldsOf("number cvc exp_month exp_year"),
		"authorization": vaultFieldsOf("id status psp merchant amount amount_cents currency created_at expires_at approval_url browser_id reason psp_error_code expected_cents actual_cents amount_authority amount_verified charged_amount_cents charged_currency charged_kind replay_attempted replay_status replay_delivered"),
		"preparation":   vaultFieldsOf("id status browser_id merchant_origin environment approval_url created_at expires_at"),
	},
}
var vaultEventFields = vaultOutputFields{
	"id": nil, "name": nil, "created_at": nil, "browser_id": nil,
	"data": vaultFieldsOf("reason operation status authorization_id vault_session_id request_kind outcome_reason provider_status provider_code provider_request_id provider_payment_status provider_error_type provider_error_code provider_decline_code provider_error_param provider_http_status provider_response_bytes provider_latency_ms payment_intent_id payment_method_id checkout_session_id replay_attempted replay_delivered charged_amount_cents charged_currency charged_kind expected_cents actual_cents currency actual_currency intent_status amount_verified psp_error_code"),
}

// Vault output is a display-safe projection, not raw provider JSON. Keep presence
// information while dropping unknown fields and opaque event data at every level.
func filterVaultJSON(raw json.RawMessage, fields vaultOutputFields) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty vault response")
	}
	if bytes.Equal(raw, []byte("null")) {
		return raw, nil
	}
	if raw[0] == '[' {
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		for i, value := range values {
			filtered, err := filterVaultJSON(value, fields)
			if err != nil {
				return nil, err
			}
			values[i] = filtered
		}
		return json.Marshal(values)
	}
	if fields == nil {
		if raw[0] == '{' {
			return json.RawMessage("null"), nil
		}
		return raw, nil
	}
	var object vaultJSON
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("invalid vault response shape")
	}
	result := make(vaultJSON)
	if wildcard, hasWildcard := fields[vaultOutputWildcard]; hasWildcard {
		for key, value := range object {
			if err := filterVaultProperty(result, key, value, wildcard); err != nil {
				return nil, err
			}
		}
		return json.Marshal(result)
	}
	for key, children := range fields {
		if value, ok := object[key]; ok {
			if err := filterVaultProperty(result, key, value, children); err != nil {
				return nil, err
			}
		}
	}
	return json.Marshal(result)
}

// Withhold any URL that is not display-safe rather than reporting an error, so a
// credential-bearing link is dropped from output instead of being echoed back.
func filterVaultProperty(result vaultJSON, key string, value json.RawMessage, fields vaultOutputFields) error {
	if key == "url" || key == "approval_url" || key == "merchant_url" || key == "image_url" || key == "product_url" {
		var address string
		if json.Unmarshal(value, &address) != nil || !vaultDisplayURL(address) {
			return nil
		}
	}
	filtered, err := filterVaultJSON(value, fields)
	if err != nil {
		return err
	}
	result[key] = filtered
	return nil
}

func vaultSafeJSONSlice[T util.RawJSONProvider](items []T, fields vaultOutputFields) ([]vaultJSON, error) {
	result := make([]vaultJSON, 0, len(items))
	for _, item := range items {
		raw, err := filterVaultJSON(json.RawMessage(item.RawJSON()), fields)
		if err != nil {
			return nil, err
		}
		var value vaultJSON
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func printVaultJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printVault(v *kernel.Vault, output string) error {
	if output == "json" {
		raw, err := filterVaultJSON(json.RawMessage(v.RawJSON()), vaultFields)
		if err != nil {
			return err
		}
		return printVaultJSON(raw)
	}
	PrintTableNoPad(pterm.TableData{
		{"Property", "Value"}, {"ID", v.ID}, {"Name (immutable)", v.Name},
		{"Created At", util.FormatLocal(v.CreatedAt)}, {"Updated At", util.FormatLocal(v.UpdatedAt)},
	}, true)
	return nil
}

func vaultDisplayURL(address string) bool {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return false
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return false
	}
	fragment, err := url.ParseQuery(u.Fragment)
	if err != nil {
		return false
	}
	for _, values := range []url.Values{query, fragment} {
		for key := range values {
			switch strings.ToLower(key) {
			case "code", "access_token", "refresh_token", "id_token", "client_secret", "password":
				return false
			}
		}
	}
	return true
}

func vaultShellArgument(value string) string {
	if vaultNamePattern.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func printVaultOperationHints(item *kernel.VaultItemUnion, vault, key, project string) error {
	actions, err := effectiveVaultItemActions(item)
	if err != nil {
		return err
	}
	prefix := "kernel vaults items invoke"
	if project != "" {
		prefix += " --project=" + vaultShellArgument(project)
	}
	for _, op := range actions.Operations {
		command := prefix
		if op.Type == "fill" || op.Type == "prepare_checkout" {
			command += " --params '<json>'"
		}
		pterm.Printf("Invoke: %s -- %s %s %s\n", command, vaultShellArgument(vault), vaultShellArgument(key), vaultShellArgument(op.Type))
	}
	return nil
}

func printVaultItem(item *kernel.VaultItemUnion, output string) error {
	raw, err := filterVaultJSON(json.RawMessage(item.RawJSON()), vaultItemFields)
	if err != nil {
		return err
	}
	if output == "json" {
		return printVaultJSON(raw)
	}
	var safe kernel.VaultItemUnion
	if err := json.Unmarshal(raw, &safe); err != nil {
		return fmt.Errorf("invalid vault item response")
	}
	item = &safe
	actions, err := effectiveVaultItemActions(item)
	if err != nil {
		return err
	}
	rows := pterm.TableData{
		{"Property", "Value"}, {"Key (immutable)", item.Key}, {"ID", item.ID}, {"Type", item.Type},
	}
	if item.Type != "credential" {
		rows = append(rows, []string{"Provider", item.Spec.Provider})
	}
	rows = append(rows, []string{"Status", item.State.Status})
	if item.Type == "credential" {
		rows = append(rows, []string{"Version", fmt.Sprint(item.Version)})
		if item.Spec.Description != "" {
			rows = append(rows, []string{"Description", item.Spec.Description})
		}
	}
	if item.Type == "wallet" {
		configID, configName := item.Spec.ProviderConfig.ID, item.Spec.ProviderConfig.Name
		if item.Spec.Provider == "link" {
			client := item.Spec.Authorization.Client
			rows = append(rows, []string{"OAuth client type", client.Type})
			configID, configName = client.ProviderConfig.ID, client.ProviderConfig.Name
		}
		if configID != "" {
			rows = append(rows, []string{"Provider config ID (immutable)", configID})
		} else if configName != "" {
			rows = append(rows, []string{"Provider config name", configName})
		}
	}
	if item.State.StatusReason != "" {
		rows = append(rows, []string{"Status reason", item.State.StatusReason})
	}
	if item.Type == "card" {
		merchant := item.Spec.MerchantName
		if item.Spec.Provider == "agentcard" {
			merchant = item.Spec.Merchant
		}
		rows = append(rows, []string{"Wallet key", item.Spec.Wallet}, []string{"Merchant", merchant}, []string{"Amount (minor units)", fmt.Sprintf("%d %s", item.Spec.Amount, item.Spec.Currency)})
		if item.Spec.Provider == "link" {
			rows = append(rows, []string{"Payment method ID", item.Spec.PaymentMethodID})
		}
	}
	if item.State.JSON.Domains.Valid() {
		rows = append(rows, []string{"Permitted domains (provider-assigned)", strings.Join(item.State.Domains, ", ")})
	}
	if actions.RequiredAction != "" {
		// A credential form is offered whenever a session is active; on a ready
		// item it is an invitation to edit, not an outstanding requirement.
		label := "Required action"
		if item.Type == "credential" {
			label = "Collection action"
		}
		rows = append(rows, []string{label, actions.RequiredAction})
		if item.Type == "credential" && !item.Action.ExpiresAt.IsZero() {
			rows = append(rows, []string{"Collection link expires", util.FormatLocal(item.Action.ExpiresAt)})
		}
	}
	if !item.ExpiresAt.IsZero() {
		rows = append(rows, []string{"Expires At", util.FormatLocal(item.ExpiresAt)})
	}
	if item.State.JSON.Aliases.Valid() {
		a := item.State.Aliases
		rows = append(rows, []string{"Checkout alias: number", a.Number}, []string{"Checkout alias: cvc", a.Cvc}, []string{"Checkout alias: exp_month", a.ExpMonth}, []string{"Checkout alias: exp_year", a.ExpYear})
	}
	if item.State.JSON.Authorization.Valid() {
		a := item.State.Authorization
		rows = append(rows, []string{"Checkout authorization", a.ID}, []string{"Authorization status", string(a.Status)})
		if a.Reason != "" {
			rows = append(rows, []string{"Authorization reason", a.Reason})
		}
		if a.JSON.ChargedKind.Valid() {
			rows = append(rows, []string{"Charged kind", string(a.ChargedKind)}, []string{"Charged (minor units)", fmt.Sprintf("%d %s", a.ChargedAmountCents, a.ChargedCurrency)})
		}
		if a.JSON.ReplayDelivered.Valid() {
			rows = append(rows, []string{"Processor response delivered", fmt.Sprint(a.ReplayDelivered)})
		}
	}
	if item.State.JSON.Preparation.Valid() {
		p := item.State.Preparation
		rows = append(rows,
			[]string{"Checkout preparation", util.OrDash(p.ID)},
			[]string{"Preparation status", string(p.Status)},
			[]string{"Preparation environment (Square)", string(p.Environment)},
			[]string{"Merchant origin", p.MerchantOrigin},
			[]string{"Preparation browser", p.BrowserID},
		)
		if !p.ExpiresAt.IsZero() {
			rows = append(rows, []string{"Submit native Pay before", util.FormatLocal(p.ExpiresAt)})
		}
	}
	PrintTableNoPad(rows, true)
	if item.Type == "credential" {
		printVaultCredentialFields(item)
	}
	printVaultItemGuidance(item, actions)
	return nil
}

// Declared schema and per-field presence answer different questions: the schema
// says what the form collects, the state says what is stored. Sensitive values
// are never returned, so presence is all the API discloses for them.
func printVaultCredentialFields(item *kernel.VaultItemUnion) {
	if len(item.Spec.Fields) == 0 {
		return
	}
	names := make([]string, 0, len(item.Spec.Fields))
	for name := range item.Spec.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := pterm.TableData{{"Field", "Type", "Required", "Sensitive", "Has value", "Value"}}
	for _, name := range names {
		definition := item.Spec.Fields[name]
		state := item.State.Fields[name]
		value := "-"
		if definition.Sensitive {
			value = "(withheld)"
		} else if state.Value != "" {
			value = state.Value
		}
		rows = append(rows, []string{name, string(definition.Type), fmt.Sprint(definition.Required), fmt.Sprint(definition.Sensitive), fmt.Sprint(state.HasValue), value})
	}
	PrintTableNoPad(rows, true)
}

func printVaultItemGuidance(item *kernel.VaultItemUnion, actions vaultItemActions) {
	if actions.RecoveryRequired {
		if actions.Abandonable {
			pterm.Warning.Println("recovery_required: the original operation is unresolved, not declined or expired. Automatic reuse is blocked and no reset operation exists. No authorization ID was returned, so deleting this card explicitly abandons the attempt and lets you create a replacement; deletion is not proof that the payment did not occur. Deleting its wallet or vault stays blocked.")
			return
		}
		pterm.Warning.Println("recovery_required: the original operation is unresolved, not declined or expired. Do not retry, delete, or replace it. Reconcile the known authorization ID with the provider or support; no reset operation exists.")
		return
	}
	if item.Type == "wallet" && item.Spec.Provider == "link" && item.Spec.Authorization.Client.Type == "customer_managed" && item.State.Status == "degraded" {
		pterm.Warning.Println("Imported grant is degraded; there is no in-place reauthorization. Import a fresh backend OAuth grant under a new wallet key for new work only. Existing cards stay bound to the old wallet; retain them and reconcile uncertain payments before further action.")
	}
	if actions.RequiredAction != "" && actions.ActionURL != "" {
		pterm.Printf("Action URL:\n%s\n", actions.ActionURL)
	}
	if actions.ApprovalURL != "" {
		pterm.Printf("Approval URL:\n%s\n", actions.ApprovalURL)
	}
	printVaultPreparationGuidance(item)
	for _, op := range actions.Operations {
		pterm.Printf("Available operation: %s — %s\n", op.Type, op.Description)
	}
	switch item.Type {
	case "card":
		card := item.AsCard()
		for _, expansion := range card.AvailableExpansions {
			pterm.Printf("Available expansion: %s — %s\n", expansion.Type, expansion.Description)
		}
		if item.State.JSON.Aliases.Valid() {
			pterm.Info.Println("Aliases are non-secret checkout values. Use only in a browser created with this vault attached; ready does not mean paid.")
		}
		pterm.Info.Println("Inspect items events for payment outcomes. Never retry automatically; if recovery permits abandonment, delete the card only after explicit user confirmation before creating a replacement.")
	} else {
		wallet := item.AsWallet()
		for _, expansion := range wallet.AvailableExpansions {
			pterm.Printf("Available expansion: %s — %s\n", expansion.Type, expansion.Description)
		}
	}
	if item.Expanded.JSON.PaymentMethods.Valid() {
		printVaultPaymentMethods(item.Expanded.PaymentMethods)
	}
	if actions.RequiredAction != "" && item.Type != "credential" {
		pterm.Info.Println("Complete the returned action with the provider; never pass card data or OAuth codes to the CLI. Observe with items get --wait 60.")
	}
}

func printVaultCredentialGuidance(item *kernel.VaultItemUnion) {
	if item.State.Status == "pending_collection" {
		pterm.Warning.Println("pending_collection: required values are missing. Open the collection URL yourself or hand it to the person who holds the credential; treat it as a secret and keep it out of logs. Observe with items get --wait 60.")
	} else {
		pterm.Info.Println("ready: every required field has a value. This does not mean a login succeeded.")
	}
	pterm.Info.Println("Set or clear values with vaults credentials update --version, which requires the version above. Never pass credential values as shell arguments; use --values-file. Do not store card data in credential items.")
}

// Preparation state and item state answer different questions: the preparation
// says whether egress can still claim it, the item says whether the attempt has
// settled. Neither means an order or charge succeeded.
func printVaultPreparationGuidance(item *kernel.VaultItemUnion) {
	switch item.State.Status {
	case "preparing":
		pterm.Info.Println("preparing: the cardholder has not approved this device yet. Keep the approval page open and observe with items get --wait 60; do not prepare again.")
	case "ready_to_submit":
		pterm.Warning.Println("ready_to_submit: device readiness lasts at most 30 seconds. Submit native Square Pay before the preparation deadline; polling never extends it. An expired readiness window cannot be reused.")
	case "consumed":
		pterm.Warning.Println("consumed: the prepared attempt has settled. This does not mean an order or charge succeeded. Inspect items events and reconcile with the merchant; preparations are single-use and this one cannot be reused.")
	case "stopped":
		pterm.Warning.Println("stopped: this preparation cannot be reused. Do not retry it; create a replacement card only after confirming with the merchant that no payment occurred.")
	case "outcome_unknown":
		pterm.Warning.Println("outcome_unknown: the checkout outcome is unresolved and new requests are blocked. Reconcile with the merchant; do not retry, delete, or replace the card.")
	}
	if !item.State.JSON.Preparation.Valid() {
		return
	}
	if item.State.Preparation.Status == kernel.AgentcardCheckoutPreparationStatusConsumed {
		pterm.Info.Println("Preparation consumed means egress claimed it and it cannot be reused. Use the item status as the lifecycle indicator.")
	}
	pterm.Info.Println("The preparation amount is display-only and does not constrain the merchant's eventual charge.")
}

func printVaultPaymentMethods(methods []kernel.VaultPaymentMethod) {
	if len(methods) == 0 {
		pterm.Info.Println("No payment methods returned")
		return
	}
	rows := pterm.TableData{{"Payment method ID", "Provider", "Type", "Label", "Brand", "Last4", "Default", "Single-use eligible", "Reasons"}}
	for _, m := range methods {
		capability := m.Capabilities.SingleUseCard
		eligible := "unknown"
		if capability.JSON.Eligible.Valid() {
			eligible = fmt.Sprint(capability.Eligible)
		}
		rows = append(rows, []string{m.ID, m.Provider, m.Type, m.Display.Label, m.Display.Brand, m.Display.Last4, fmt.Sprint(m.IsDefault), eligible, strings.Join(capability.Reasons, ", ")})
	}
	PrintTableNoPad(rows, true)
	pterm.Info.Println("Select an ID explicitly in the card --spec JSON: Link uses payment_method_id; AgentCard uses card_id (or omit it for cardholder selection). Capabilities are advisory; missing means unknown, not ineligible.")
}

func printVaultEvents(events []kernel.VaultItemEvent, data []vaultJSON) {
	rows := pterm.TableData{{"Event ID", "Time", "Name", "Browser ID", "Outcome data"}}
	for i, event := range events {
		rows = append(rows, []string{event.ID, util.FormatLocal(event.CreatedAt), event.Name, util.OrDash(event.BrowserID), string(data[i]["data"])})
	}
	PrintTableNoPad(rows, true)
}
