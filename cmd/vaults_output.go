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
var onePasswordRequestEntryFields = vaultOutputFields{
	"id": nil, "type": nil, "reason": nil, "keywords": nil,
	"parameters": vaultFieldsOf("website"),
}
var onePasswordRequestFields = vaultOutputFields{"version": nil, "goal": nil, "entries": onePasswordRequestEntryFields}
var vaultMethodFields = vaultOutputFields{
	"id": nil, "provider": nil, "type": nil, "is_default": nil,
	"display":      vaultFieldsOf("label brand last4"),
	"capabilities": {"single_use_card": vaultFieldsOf("eligible reasons")},
}
var vaultItemFields = vaultOutputFields{
	"id": nil, "key": nil, "type": nil, "version": nil, "created_at": nil, "updated_at": nil, "expires_at": nil, "description": nil,
	"available_operations": vaultOperationFields,
	"available_expansions": vaultOperationFields,
	"action":               vaultFieldsOf("name url expires_at instructions"),
	"expanded":             {"payment_methods": vaultMethodFields},
	"spec": {
		"provider": nil, "wallet": nil, "user_id": nil, "payment_method_id": nil, "card_id": nil, "checkout_origin": nil,
		"amount": nil, "currency": nil, "merchant": nil, "merchant_name": nil, "merchant_url": nil, "merchant_country": nil, "merchant_category": nil, "merchant_category_code": nil,
		"context": nil, "expires_at": nil, "description": nil, "account": nil, "connection_id": nil,
		"requests":        onePasswordRequestFields,
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
		"provider": nil, "status": nil, "status_reason": nil, "user_id": nil, "domains": nil, "access_request_id": nil,
		"access_request": {
			"id": nil, "state": nil, "goal": nil, "createdAt": nil, "has_autofill_token": nil, "granted_count": nil,
			"request": onePasswordRequestFields, "entries": onePasswordRequestEntryFields,
		},
		"fields":        {"*": vaultFieldsOf("has_value type")},
		"masks":         vaultFieldsOf("brand last4 token_last4"),
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
			if key == "url" || key == "approval_url" || key == "merchant_url" || key == "merchant_origin" || key == "image_url" || key == "product_url" || key == "website" {
				var address string
				if json.Unmarshal(value, &address) != nil {
					continue
				}
				var name string
				approval := key == "url" && json.Unmarshal(object["name"], &name) == nil && name == "1password_access_approval"
				if !vaultDisplayURL(address) && !(approval && onePasswordApprovalURL(address)) {
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
	if _, item := fields["available_operations"]; item {
		var state struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(result["state"], &state) == nil && state.Status == "recovery_required" {
			var operations []vaultItemOperation
			if err := json.Unmarshal(result["available_operations"], &operations); err != nil {
				return nil, err
			}
			allowed := make([]vaultItemOperation, 0)
			for _, op := range operations {
				if op.Type == "restart_verification" {
					allowed = append(allowed, op)
				}
			}
			result["available_operations"], _ = json.Marshal(allowed)
			delete(result, "action")
			var stateFields vaultJSON
			if err := json.Unmarshal(result["state"], &stateFields); err != nil {
				return nil, err
			}
			for _, field := range []string{"authorization", "preparation"} {
				if len(stateFields[field]) == 0 || string(stateFields[field]) == "null" {
					continue
				}
				var detail vaultJSON
				if err := json.Unmarshal(stateFields[field], &detail); err != nil {
					return nil, err
				}
				delete(detail, "approval_url")
				stateFields[field], _ = json.Marshal(detail)
			}
			result["state"], _ = json.Marshal(stateFields)
		}
	}
	var itemType string
	if result["spec"] != nil && result["state"] != nil && json.Unmarshal(result["type"], &itemType) == nil && itemType == "credential" {
		if err := preservePublicCredentialValues(object, result); err != nil {
			return nil, err
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
		Provider string       `json:"provider"`
		Fields   []definition `json:"fields"`
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
	// Managed auth state lists binding names and types only; values are never returned.
	if spec.Provider == "managed_auth" {
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
		{"property", "value"}, {"id", v.ID}, {"name (immutable)", v.Name},
		{"created at", util.FormatLocal(v.CreatedAt)}, {"updated at", util.FormatLocal(v.UpdatedAt)},
	}, true)
	return nil
}

// The encryption key is public, so JSON output passes the API response through unchanged.
func printVaultEncryptionKey(k *kernel.VaultEncryptionKey, output string) error {
	if output == "json" {
		return printVaultJSON(json.RawMessage(k.RawJSON()))
	}
	PrintTableNoPad(pterm.TableData{
		{"property", "value"}, {"key id (kid)", k.Kid}, {"algorithm (alg)", string(k.Alg)}, {"encryption (enc)", string(k.Enc)},
		{"jwk kty", k.Jwk.Kty}, {"jwk crv", k.Jwk.Crv}, {"jwk x", k.Jwk.X}, {"jwk y", k.Jwk.Y},
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

// The native 1Password approval link is handed to the account owner unchanged; it
// grants nothing until they approve in their own 1Password app.
func onePasswordApprovalURL(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "onepassword" || u.Host != "grant-brokered-access" || u.User != nil || u.Fragment != "" {
		return false
	}
	query, err := url.ParseQuery(u.RawQuery)
	return err == nil && len(query) == 1 && len(query["access_request_reference"]) == 1 && query.Get("access_request_reference") != ""
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
		if vaultOperationTakesParams(op.Type) {
			command += " --params '<json>'"
		}
		if op.Type == "authorize" && item.Type == "card" && item.Spec.Provider == "link" && item.State.Status == "requested" {
			command += " --browser-id '<browser-session-id>' --page-url '<checkout-page-url>'"
		}
		pterm.Printf("invoke: %s -- %s %s %s\n", command, vaultShellArgument(vault), vaultShellArgument(key), vaultShellArgument(op.Type))
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
		{"property", "value"}, {"key (immutable)", item.Key}, {"id", item.ID},
		{"type", item.Type}, {"provider", item.Spec.Provider}, {"status", item.State.Status},
	}
	if item.Type == "credential" {
		rows = append(rows, []string{"version", fmt.Sprint(item.Version)})
		if item.Spec.Provider == "1password" {
			if item.Spec.Account != "" {
				rows = append(rows, []string{"1Password account (immutable)", item.Spec.Account})
			} else {
				rows = append(rows, []string{"1Password account", "stored token (developer-supplied)"})
			}
			for _, entry := range item.Spec.Requests.Entries {
				rows = append(rows, []string{"requested login", entry.Parameters.Website})
			}
			if item.State.JSON.AccessRequest.Valid() {
				rows = append(rows, []string{"access request state", item.State.AccessRequest.State})
				entryRow := func(id, website string) {
					if id != "" {
						rows = append(rows, []string{"entry id", id + " " + website})
					}
				}
				for _, entry := range item.State.AccessRequest.Entries {
					entryRow(entry.ID, entry.Parameters.Website)
				}
				if len(item.State.AccessRequest.Entries) == 0 {
					for _, entry := range item.State.AccessRequest.Request.Entries {
						entryRow(entry.ID, entry.Parameters.Website)
					}
				}
			}
		} else if item.Spec.Provider == "managed_auth" {
			rows = append(rows, []string{"managed auth connection (immutable)", item.Spec.ConnectionID})
		} else {
			pterm.Info.Println("use -o json for field definitions, presence, and non-sensitive values; sensitive values are omitted")
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
			rows = append(rows, []string{"provider config id (immutable)", configID})
		} else if configName != "" {
			rows = append(rows, []string{"provider config name", configName})
		}
	}
	if item.State.StatusReason != "" {
		rows = append(rows, []string{"status reason", item.State.StatusReason})
	}
	if item.Type == "card" {
		merchant := item.Spec.MerchantName
		if item.Spec.Provider == "agentcard" {
			merchant = item.Spec.Merchant
		}
		rows = append(rows, []string{"wallet key", item.Spec.Wallet}, []string{"merchant", merchant}, []string{"amount (minor units)", fmt.Sprintf("%d %s", item.Spec.Amount, item.Spec.Currency)})
		if item.Spec.Provider == "link" {
			rows = append(rows, []string{"payment method id", item.Spec.PaymentMethodID})
		}
		if item.Spec.Provider == "agentcard" && item.Spec.CheckoutOrigin != "" {
			rows = append(rows, []string{"checkout origin", item.Spec.CheckoutOrigin})
		}
		if item.Spec.Provider == "kernel" && item.Spec.MerchantURL != "" {
			rows = append(rows, []string{"merchant url", item.Spec.MerchantURL})
		}
		if item.Spec.Provider == "kernel" && item.Spec.MerchantCountry != "" {
			rows = append(rows, []string{"merchant country", item.Spec.MerchantCountry})
		}
		if masks := item.State.Masks; masks.Last4 != "" || masks.TokenLast4 != "" {
			rows = append(rows, []string{"card last4", util.OrDash(masks.Last4)})
			if masks.TokenLast4 != "" {
				rows = append(rows, []string{"network token last4", masks.TokenLast4})
			}
		}
	}
	if item.State.JSON.Domains.Valid() {
		rows = append(rows, []string{"permitted domains (provider-assigned)", strings.Join(item.State.Domains, ", ")})
	}
	if actions.RequiredAction != "" {
		rows = append(rows, []string{"required action", actions.RequiredAction})
	}
	if !item.ExpiresAt.IsZero() {
		rows = append(rows, []string{"expires at", util.FormatLocal(item.ExpiresAt)})
	}
	if item.State.JSON.Aliases.Valid() {
		a := item.State.Aliases
		rows = append(rows, []string{"checkout alias: number", a.Number}, []string{"checkout alias: cvc", a.Cvc}, []string{"checkout alias: exp_month", a.ExpMonth}, []string{"checkout alias: exp_year", a.ExpYear})
	}
	if item.State.JSON.Preparation.Valid() {
		p := item.State.Preparation
		rows = append(rows, []string{"preparation id", p.ID}, []string{"preparation status", string(p.Status)},
			[]string{"preparation browser", p.BrowserID}, []string{"merchant origin", p.MerchantOrigin},
			[]string{"environment", string(p.Environment)})
		if p.Psp != "" {
			rows = append(rows, []string{"processor", string(p.Psp)})
		}
		if !p.ExpiresAt.IsZero() {
			rows = append(rows, []string{"submit before", util.FormatLocal(p.ExpiresAt)})
		}
	}
	if item.State.JSON.Authorization.Valid() {
		a := item.State.Authorization
		rows = append(rows, []string{"checkout authorization", a.ID}, []string{"authorization status", string(a.Status)})
		if a.Reason != "" {
			rows = append(rows, []string{"authorization reason", a.Reason})
		}
		if a.JSON.ChargedKind.Valid() {
			rows = append(rows, []string{"charged kind", string(a.ChargedKind)}, []string{"charged (minor units)", fmt.Sprintf("%d %s", a.ChargedAmountCents, a.ChargedCurrency)})
		}
		if a.JSON.ReplayDelivered.Valid() {
			rows = append(rows, []string{"processor response delivered", fmt.Sprint(a.ReplayDelivered)})
		}
	}
	PrintTableNoPad(rows, true)
	printVaultItemGuidance(item, actions)
	return nil
}

func printVaultItemGuidance(item *kernel.VaultItemUnion, actions vaultItemActions) {
	if actions.RecoveryRequired {
		guidance := kernelSupportGuidance
		if actions.advertises("restart_verification") {
			guidance = vaultRecoveryHelp
		}
		pterm.Warning.Println("recovery_required: the original operation is unresolved, not declined or expired. " + guidance)
		for _, op := range actions.Operations {
			pterm.Printf("available operation: %s — %s\n", op.Type, op.Description)
		}
		return
	}
	if item.Type == "wallet" && item.Spec.Provider == "link" && item.Spec.Authorization.Client.Type == "customer_managed" && item.State.Status == "degraded" {
		pterm.Warning.Println("imported grant is degraded; there is no in-place reauthorization. import a fresh backend OAuth grant under a new wallet key for new work only. existing cards stay bound to the old wallet; retain them and reconcile uncertain payments before further action.")
	}
	if actions.RequiredAction != "" && actions.ActionURL != "" {
		pterm.Printf("action url:\n%s\n", actions.ActionURL)
	}
	if actions.ApprovalURL != "" {
		pterm.Printf("approval url:\n%s\n", actions.ApprovalURL)
	}
	for _, op := range actions.Operations {
		pterm.Printf("available operation: %s — %s\n", op.Type, op.Description)
	}
	if item.Type == "credential_account" {
		if actions.RequiredAction != "" {
			pterm.Info.Println("share the 1Password authorization url with the account owner. observe the connection with items get --wait 60; never ask for 1Password passwords or codes.")
		} else if item.State.Status == "reconnect_required" || item.State.Status == "declined" {
			pterm.Info.Println("this 1Password account is not connected. ask the owner before running credentials connect again with the same key; use 1pw_recover first if it is advertised.")
		}
		return
	}
	if item.Type == "credential" && item.Spec.Provider == "1password" {
		if item.Action.Instructions != "" {
			pterm.Printf("approval instructions:\n%s\n", item.Action.Instructions)
		}
		pterm.Info.Println("ready means the account owner approved access, not that sign-in succeeded. 1pw_fill can submit the form, so never retry automatically. check the page first before retrying. never retry an access request automatically; after an uncertain outcome, do not delete and recreate the item.")
		return
	}
	if item.Type == "credential" && item.Spec.Provider == "managed_auth" {
		pterm.Info.Println("fill reads the managed auth connection's saved credential at fill time; use -o json for fill binding names. ready means a saved credential exists, not that login succeeded. fill only when advertised; fill does not submit the form.")
		return
	}
	if item.Type == "credential" {
		if actions.RequiredAction != "" {
			pterm.Info.Println("share the collection url with the user to complete the credential form. observe readiness with items get --wait 60; for edits to an already-ready item, compare versions without --wait.")
		}
		pterm.Info.Println("do not use credential items for credit card data. use wallet and card item types for credit cards and payment checkout instead.")
		pterm.Info.Println("ready means required fields are populated, not that login succeeded. fill only when advertised; fill does not submit the form.")
		return
	}
	if item.Type == "card" {
		if item.Spec.Provider == "kernel" {
			switch item.State.Status {
			case "requested", "pending_authorization", "ready":
				pterm.Info.Println("Visa may require the returned spend_approval action; share it privately, never open it for the cardholder. ready means card details can be filled, not that the merchant charged the card. fill delivers details for a few minutes; submit checkout promptly after filling and before expires_at.")
			}
			switch item.State.Status {
			case "ready", "declined", "expired", "consumed":
				pterm.Info.Println(kernelTerminalHelp)
			}
		}
		if item.State.JSON.Preparation.Valid() {
			switch item.State.Status {
			case "preparing":
				pterm.Info.Println("keep the approval page open and poll items get --wait 60 until ready_to_submit before submitting native Pay.")
			case "ready_to_submit":
				pterm.Info.Println("submit native Pay before preparation.expires_at; polling does not extend the deadline.")
			}
			pterm.Info.Println("preparations are single-use, including after failure or expiry. preparation consumed means claimed, not payment success. do not retry automatically.")
		}
		card := item.AsCard()
		for _, expansion := range card.AvailableExpansions {
			pterm.Printf("available expansion: %s — %s\n", expansion.Type, expansion.Description)
		}
		if item.State.JSON.Aliases.Valid() {
			pterm.Info.Println("aliases are non-secret checkout values. use only in a browser created with this vault attached; ready does not mean paid.")
		}
		switch item.Spec.Provider {
		case "kernel":
			pterm.Info.Println(kernelTransactionHelp + ". kernel cards have no stored billing fields; billing_* returns field_unavailable. enter billing details on the merchant page.")
		case "link":
			pterm.Info.Println("fill never submits payment; submit the checkout yourself. if the result of submitting payment is unclear, check the merchant's order confirmation or order history before paying again. if authorization is declined or expires, delete this card and create a new one.")
		default:
			pterm.Info.Println("inspect items events for payment outcomes. never retry automatically; if recovery permits abandonment, delete the card only after explicit user confirmation before creating a replacement.")
		}
	} else {
		wallet := item.AsWallet()
		if item.Description != "" {
			pterm.Info.Println(item.Description)
		}
		if item.Spec.Provider == "kernel" {
			pterm.Info.Println(kernelWalletHelp)
		}
		for _, expansion := range wallet.AvailableExpansions {
			pterm.Printf("available expansion: %s — %s\n", expansion.Type, expansion.Description)
		}
	}
	if item.Expanded.JSON.PaymentMethods.Valid() {
		printVaultPaymentMethods(item.Spec.Provider, item.Expanded.PaymentMethods)
	}
	if actions.RequiredAction != "" {
		pterm.Info.Println("complete the returned action with the provider; never pass card data or OAuth codes to the cli. observe with items get --wait 60.")
	}
}

func printVaultPaymentMethods(provider string, methods []kernel.VaultPaymentMethod) {
	if len(methods) == 0 {
		pterm.Info.Println("no payment methods returned")
		return
	}
	rows := pterm.TableData{{"payment method id", "provider", "type", "label", "brand", "last4", "default", "single-use eligible", "reasons"}}
	for _, m := range methods {
		capability := m.Capabilities.SingleUseCard
		eligible := "unknown"
		if capability.JSON.Eligible.Valid() {
			eligible = fmt.Sprint(capability.Eligible)
		}
		rows = append(rows, []string{m.ID, m.Provider, m.Type, m.Display.Label, m.Display.Brand, m.Display.Last4, fmt.Sprint(m.IsDefault), eligible, strings.Join(capability.Reasons, ", ")})
	}
	PrintTableNoPad(rows, true)
	if provider == "kernel" {
		pterm.Info.Println("kernel cards use the wallet key, not a payment method id. " + kernelEligibilityHelp)
	} else {
		pterm.Info.Println("select an id explicitly in the card --spec json: Link uses payment_method_id; AgentCard uses card_id (or omit it for cardholder selection). capabilities are advisory; missing means unknown, not ineligible.")
	}
}

func printVaultEvents(events []kernel.VaultItemEvent, data []vaultJSON) {
	rows := pterm.TableData{{"event id", "time", "name", "browser id", "outcome data"}}
	for i, event := range events {
		rows = append(rows, []string{event.ID, util.FormatLocal(event.CreatedAt), event.Name, util.OrDash(event.BrowserID), string(data[i]["data"])})
	}
	PrintTableNoPad(rows, true)
}
