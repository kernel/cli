package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
)

type vaultJSON map[string]json.RawMessage
type vaultOutputFields map[string]vaultOutputFields

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
var vaultItemFields = vaultOutputFields{
	"id": nil, "key": nil, "type": nil, "description": nil, "version": nil, "created_at": nil, "updated_at": nil, "expires_at": nil,
	"available_operations": vaultOperationFields,
	"available_expansions": vaultOperationFields,
	"action":               vaultFieldsOf("name url expires_at"),
	"expanded":             {"payment_methods": vaultMethodFields},
	"spec": {
		"provider": nil, "wallet": nil, "user_id": nil, "payment_method_id": nil, "card_id": nil,
		"browser_id": nil, "page_url": nil, "amount": nil, "currency": nil, "merchant": nil, "merchant_name": nil,
		"context": nil, "expires_at": nil, "description": nil,
		"fields":          vaultFieldsOf("name label type required sensitive"),
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
		"fields":        {"*": vaultFieldsOf("has_value")},
		"masks":         vaultFieldsOf("brand last4"),
		"aliases":       vaultFieldsOf("number cvc exp_month exp_year"),
		"preparation":   vaultFieldsOf("id status browser_id merchant_origin environment psp created_at expires_at approval_url"),
		"authorization": vaultFieldsOf("id status psp merchant amount amount_cents currency created_at expires_at approval_url browser_id reason psp_error_code expected_cents actual_cents amount_authority amount_verified charged_amount_cents charged_currency charged_kind replay_attempted replay_status replay_delivered"),
	},
}
var vaultEventFields = vaultOutputFields{
	"id": nil, "name": nil, "created_at": nil, "browser_id": nil,
	"data": vaultFieldsOf("reason operation status authorization_id preparation_id vault_session_id request_kind outcome_reason provider_status provider_code provider_request_id provider_payment_status provider_error_type provider_error_code provider_decline_code provider_error_param provider_http_status provider_response_bytes provider_latency_ms payment_intent_id payment_method_id checkout_session_id replay_attempted replay_delivered charged_amount_cents charged_currency charged_kind expected_cents actual_cents currency actual_currency intent_status amount_verified psp_error_code"),
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
	for key, children := range fields {
		if key == "*" {
			for name, value := range object {
				filtered, err := filterVaultJSON(value, children)
				if err != nil {
					return nil, err
				}
				result[name] = filtered
			}
			continue
		}
		if value, ok := object[key]; ok {
			if key == "url" || key == "approval_url" || key == "merchant_origin" || key == "image_url" || key == "product_url" {
				var address string
				if json.Unmarshal(value, &address) != nil || !vaultDisplayURL(address) {
					continue
				}
			}
			filtered, err := filterVaultJSON(value, children)
			if err != nil {
				return nil, err
			}
			result[key] = filtered
		}
	}
	var itemType string
	if result["spec"] != nil && result["state"] != nil && json.Unmarshal(result["type"], &itemType) == nil {
		if itemType == "credential" {
			if err := preservePublicCredentialValues(object, result); err != nil {
				return nil, err
			}
		}
		if itemType == "card" {
			var spec struct {
				Provider string `json:"provider"`
			}
			if json.Unmarshal(result["spec"], &spec) == nil && spec.Provider == "link" {
				var state vaultJSON
				if json.Unmarshal(result["state"], &state) != nil {
					return nil, fmt.Errorf("invalid vault response shape")
				}
				delete(state, "aliases")
				filteredState, err := json.Marshal(state)
				if err != nil {
					return nil, err
				}
				result["state"] = filteredState
			}
		}
	}
	return json.Marshal(result)
}

func preservePublicCredentialValues(source, result vaultJSON) error {
	type definition struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Sensitive *bool  `json:"sensitive"`
	}
	var spec struct {
		Fields []definition `json:"fields"`
	}
	var values struct {
		Fields map[string]struct {
			HasValue bool    `json:"has_value"`
			Value    *string `json:"value,omitempty"`
		} `json:"fields"`
	}
	if json.Unmarshal(source["spec"], &spec) != nil || json.Unmarshal(source["state"], &values) != nil || values.Fields == nil {
		return nil
	}
	definitions := make(map[string]definition, len(spec.Fields))
	for _, field := range spec.Fields {
		definitions[field.Name] = field
	}
	for name, field := range values.Fields {
		definition := definitions[name]
		if definition.Sensitive == nil || *definition.Sensitive || (definition.Type != "text" && definition.Type != "email") || !field.HasValue {
			field.Value = nil
		}
		values.Fields[name] = field
	}
	var state vaultJSON
	if err := json.Unmarshal(result["state"], &state); err != nil {
		return err
	}
	fields, err := json.Marshal(values.Fields)
	if err != nil {
		return err
	}
	state["fields"] = fields
	result["state"], err = json.Marshal(state)
	return err
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

type vaultLinkCardDisplay struct {
	Spec struct {
		BrowserID       string `json:"browser_id"`
		PageURL         string `json:"page_url"`
		Wallet          string `json:"wallet"`
		PaymentMethodID string `json:"payment_method_id"`
		Amount          int64  `json:"amount"`
		Currency        string `json:"currency"`
	} `json:"spec"`
}

func vaultItemDescription(item *kernel.VaultItemUnion) string {
	var value struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal([]byte(item.RawJSON()), &value)
	return value.Description
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
		{"Property", "Value"}, {"Key (immutable)", item.Key}, {"ID", item.ID},
		{"Type", item.Type}, {"Provider", item.Spec.Provider}, {"Status", item.State.Status},
	}
	if description := vaultItemDescription(item); description != "" {
		rows = append(rows, []string{"Description", description})
	}
	if item.Type == "credential" {
		rows = append(rows, []string{"Version", fmt.Sprint(item.Version)})
		pterm.Info.Println("Use -o json for field definitions, presence, and non-sensitive values; sensitive values are omitted")
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
		rows = append(rows, []string{"Wallet key", item.Spec.Wallet}, []string{"Amount (minor units)", fmt.Sprintf("%d %s", item.Spec.Amount, item.Spec.Currency)})
		if item.Spec.Provider == "agentcard" {
			rows = append(rows, []string{"Merchant", item.Spec.Merchant})
		}
		if item.Spec.Provider == "link" {
			rows = append(rows, []string{"Merchant", item.Spec.MerchantName})
			var card vaultLinkCardDisplay
			if json.Unmarshal([]byte(item.RawJSON()), &card) != nil {
				return fmt.Errorf("invalid Link card response")
			}
			rows = append(rows,
				[]string{"Payment method ID", card.Spec.PaymentMethodID},
				[]string{"Browser session ID", card.Spec.BrowserID},
				[]string{"Checkout page", card.Spec.PageURL},
			)
		}
	}
	if item.State.JSON.Domains.Valid() {
		rows = append(rows, []string{"Permitted domains (provider-assigned)", strings.Join(item.State.Domains, ", ")})
	}
	if actions.RequiredAction != "" {
		rows = append(rows, []string{"Required action", actions.RequiredAction})
	}
	if !item.ExpiresAt.IsZero() {
		rows = append(rows, []string{"Expires At", util.FormatLocal(item.ExpiresAt)})
	}
	if item.Spec.Provider == "agentcard" && item.State.JSON.Aliases.Valid() {
		a := item.State.Aliases
		rows = append(rows, []string{"Checkout alias: number", a.Number}, []string{"Checkout alias: cvc", a.Cvc}, []string{"Checkout alias: exp_month", a.ExpMonth}, []string{"Checkout alias: exp_year", a.ExpYear})
	}
	if item.State.JSON.Preparation.Valid() {
		p := item.State.Preparation
		rows = append(rows, []string{"Preparation ID", p.ID}, []string{"Preparation status", string(p.Status)},
			[]string{"Preparation browser", p.BrowserID}, []string{"Merchant origin", p.MerchantOrigin},
			[]string{"Environment", string(p.Environment)})
		if p.Psp != "" {
			rows = append(rows, []string{"Processor", string(p.Psp)})
		}
		if !p.ExpiresAt.IsZero() {
			rows = append(rows, []string{"Submit before", util.FormatLocal(p.ExpiresAt)})
		}
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
	PrintTableNoPad(rows, true)
	printVaultItemGuidance(item, actions)
	return nil
}

func printVaultItemGuidance(item *kernel.VaultItemUnion, actions vaultItemActions) {
	if actions.RecoveryRequired {
		pterm.Warning.Println("recovery_required: the original operation is unresolved, not declined or expired. Do not retry, delete, or replace it. Reconcile with the provider or support; no reset operation exists.")
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
	for _, op := range actions.Operations {
		pterm.Printf("Available operation: %s — %s\n", op.Type, op.Description)
	}
	if item.Type == "credential" {
		if actions.RequiredAction != "" {
			pterm.Info.Println("Share the collection URL with the user to complete the credential form. Observe readiness with items get --wait 60; for edits to an already-ready item, compare versions without --wait.")
		}
		pterm.Info.Println("Do not use credential items for credit card data. Use wallet and card item types for credit cards and payment checkout instead.")
		pterm.Info.Println("Ready means required fields are populated, not that login succeeded. Fill only when advertised; fill does not submit the form.")
		return
	}
	if item.Type == "card" {
		if item.State.JSON.Preparation.Valid() {
			switch item.State.Status {
			case "preparing":
				pterm.Info.Println("Keep the approval page open and poll items get --wait 60 until ready_to_submit before submitting native Pay.")
			case "ready_to_submit":
				pterm.Info.Println("Submit native Pay before preparation.expires_at; polling does not extend the deadline.")
			}
			pterm.Info.Println("Preparations are single-use, including after failure or expiry. Preparation consumed means claimed, not payment success. Do not retry automatically.")
		}
		card := item.AsCard()
		for _, expansion := range card.AvailableExpansions {
			pterm.Printf("Available expansion: %s — %s\n", expansion.Type, expansion.Description)
		}
		if item.Spec.Provider == "agentcard" && item.State.JSON.Aliases.Valid() {
			pterm.Info.Println("Aliases are non-secret checkout values. Use only in a browser created with this vault attached; ready does not mean paid.")
		}
		pterm.Info.Println("Inspect items events for payment outcomes. Fill supplies credentials but does not submit payment. Never retry automatically; if recovery permits abandonment, delete the card only after explicit user confirmation before creating a replacement.")
	} else {
		wallet := item.AsWallet()
		for _, expansion := range wallet.AvailableExpansions {
			pterm.Printf("Available expansion: %s — %s\n", expansion.Type, expansion.Description)
		}
	}
	if item.Expanded.JSON.PaymentMethods.Valid() {
		printVaultPaymentMethods(item.Expanded.PaymentMethods)
	}
	if actions.RequiredAction != "" {
		pterm.Info.Println("Complete the returned action with the provider; never pass card data or OAuth codes to the CLI. Observe with items get --wait 60.")
	}
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
	pterm.Info.Println("Select an ID explicitly. Link cards require payment_method_id; Kernel inspects checkout and selects the execution method. AgentCard uses card_id (or omit it for cardholder selection). Capabilities are advisory; missing means unknown, not ineligible.")
}

func printVaultEvents(events []kernel.VaultItemEvent, data []vaultJSON) {
	rows := pterm.TableData{{"Event ID", "Time", "Name", "Browser ID", "Outcome data"}}
	for i, event := range events {
		rows = append(rows, []string{event.ID, util.FormatLocal(event.CreatedAt), event.Name, util.OrDash(event.BrowserID), string(data[i]["data"])})
	}
	PrintTableNoPad(rows, true)
}
