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
	"id": nil, "key": nil, "type": nil, "created_at": nil, "updated_at": nil, "expires_at": nil,
	"available_operations": vaultOperationFields,
	"available_expansions": vaultOperationFields,
	"action":               vaultFieldsOf("name url"),
	"expanded":             {"payment_methods": vaultMethodFields},
	"spec": {
		"provider": nil, "wallet": nil, "user_id": nil, "payment_method_id": nil, "card_id": nil,
		"amount": nil, "currency": nil, "merchant": nil, "merchant_name": nil, "merchant_url": nil,
		"context": nil, "test": nil, "expires_at": nil,
		"authorization": {"method": nil, "client": vaultFieldsOf("type")},
		"totals":        vaultTotalFields,
		"line_items": {
			"name": nil, "quantity": nil, "unit_amount": nil, "description": nil,
			"sku": nil, "url": nil, "image_url": nil, "product_url": nil, "totals": vaultTotalFields,
		},
	},
	"state": {
		"provider": nil, "status": nil, "status_reason": nil, "user_id": nil, "domains": nil,
		"masks":         vaultFieldsOf("brand last4"),
		"aliases":       vaultFieldsOf("number cvc exp_month exp_year"),
		"authorization": vaultFieldsOf("id status psp merchant amount amount_cents currency created_at expires_at approval_url browser_id reason psp_error_code expected_cents actual_cents amount_authority amount_verified charged_amount_cents charged_currency charged_kind replay_attempted replay_status replay_delivered"),
	},
}
var vaultEventFields = vaultOutputFields{
	"id": nil, "name": nil, "created_at": nil, "browser_id": nil,
	"data": vaultFieldsOf("reason status authorization_id vault_session_id request_kind outcome_reason provider_status provider_code provider_request_id provider_payment_status provider_error_type provider_error_code provider_decline_code provider_error_param provider_http_status provider_response_bytes provider_latency_ms payment_intent_id payment_method_id checkout_session_id replay_attempted replay_delivered charged_amount_cents charged_currency charged_kind expected_cents actual_cents currency actual_currency intent_status amount_verified psp_error_code"),
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
		if value, ok := object[key]; ok {
			if key == "url" || key == "approval_url" || key == "merchant_url" || key == "image_url" || key == "product_url" {
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
	return json.Marshal(result)
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
	rows := pterm.TableData{
		{"Property", "Value"}, {"Key (immutable)", item.Key}, {"ID", item.ID},
		{"Type", item.Type}, {"Provider", item.Spec.Provider}, {"Status", item.State.Status},
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
			rows = append(rows, []string{"Test", fmt.Sprint(item.Spec.Test)}, []string{"Payment method ID", item.Spec.PaymentMethodID})
		} else {
			rows = append(rows, []string{"Mode", "Deployment-controlled (no per-item test mode)"})
		}
	}
	if item.State.JSON.Domains.Valid() {
		rows = append(rows, []string{"Permitted domains (provider-assigned)", strings.Join(item.State.Domains, ", ")})
	}
	if item.Action.Name != "" {
		rows = append(rows, []string{"Required action", item.Action.Name})
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
	PrintTableNoPad(rows, true)
	if item.Action.Name != "" && item.Action.URL != "" {
		pterm.Printf("Action URL:\n%s\n", item.Action.URL)
	}
	if item.State.JSON.Authorization.Valid() && item.State.Authorization.ApprovalURL != "" {
		pterm.Printf("Approval URL:\n%s\n", item.State.Authorization.ApprovalURL)
	}
	if item.Type == "card" {
		card := item.AsCard()
		for _, op := range card.AvailableOperations {
			pterm.Printf("Available operation: %s — %s\n", op.Type, op.Description)
		}
		for _, expansion := range card.AvailableExpansions {
			pterm.Printf("Available expansion: %s — %s\n", expansion.Type, expansion.Description)
		}
		if item.State.JSON.Aliases.Valid() {
			pterm.Info.Println("Aliases are non-secret checkout values. Use only in a browser created with this vault attached; ready does not mean paid.")
		}
		pterm.Info.Println("Inspect items events for payment outcomes. Do not retry failed, timed-out, rejected, or indeterminate payments.")
	} else {
		wallet := item.AsWallet()
		for _, expansion := range wallet.AvailableExpansions {
			pterm.Printf("Available expansion: %s — %s\n", expansion.Type, expansion.Description)
		}
	}
	if item.Expanded.JSON.PaymentMethods.Valid() {
		printVaultPaymentMethods(item.Expanded.PaymentMethods)
	}
	if item.Action.Name != "" {
		pterm.Info.Println("Complete the returned action with the provider; never pass card data or OAuth codes to the CLI. Observe with items get --wait 60.")
	}
	return nil
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
