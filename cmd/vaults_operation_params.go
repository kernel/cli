package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
)

type vaultOperationParams struct {
	Fill     *vaultFillParams
	Checkout *kernel.VaultCheckoutContextParam
}

type vaultFillParams struct {
	BrowserID string           `json:"browser_id"`
	PageURL   string           `json:"page_url,omitempty"`
	Fields    []vaultFillField `json:"fields"`
	TimeoutMS *int             `json:"timeout_ms,omitempty"`
}

type vaultFillField struct {
	Field    string `json:"field"`
	Selector string `json:"selector"`
	Format   string `json:"format,omitempty"`
}

var vaultFillPageURLPattern = regexp.MustCompile(`^https?://[^/?#@*\s]+(?:[/?#][^\s]*)?$`)

// Declared credential field names; card field names also satisfy this pattern.
var vaultFieldNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)

// Card fields the API fills from the decrypted card, excluding the combined
// expiration field, which additionally requires a format.
var vaultCardFillFields = []string{"number", "cvc", "exp_month", "exp_year", "billing_name", "billing_line1", "billing_line2", "billing_city", "billing_state", "billing_postal_code", "billing_country"}

// Reject duplicate and unknown keys without including payloads in diagnostics.
func vaultParamsObject(raw, allowed string) (map[string]json.RawMessage, error) {
	invalid := fmt.Errorf("operation parameters must contain JSON objects with only supported, non-duplicate properties")
	dec := json.NewDecoder(strings.NewReader(raw))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return nil, invalid
	}
	keys := strings.Fields(allowed)
	object := make(map[string]json.RawMessage)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, invalid
		}
		key, ok := token.(string)
		if !ok {
			return nil, invalid
		}
		if key == "type" {
			return nil, fmt.Errorf("operation parameters must not contain type; the positional operation supplies it")
		}
		if _, duplicate := object[key]; duplicate || !slices.Contains(keys, key) {
			return nil, invalid
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, invalid
		}
		object[key] = value
	}
	if _, err := dec.Token(); err != nil {
		return nil, invalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, invalid
	}
	return object, nil
}

func parseVaultOperationParams(operation, raw string, paramsSet, openSet bool) (*vaultOperationParams, error) {
	if strings.TrimSpace(operation) == "" {
		return nil, fmt.Errorf("operation must not be empty")
	}
	if openSet && operation != "authorize" && operation != "collect" && operation != "prepare_checkout" {
		return nil, fmt.Errorf("--open is only supported for authorize, collect, and prepare_checkout")
	}
	if len(raw) > 128*1024 {
		return nil, fmt.Errorf("operation parameters exceed 128 KiB")
	}
	if operation == "prepare_checkout" {
		if !paramsSet {
			return nil, fmt.Errorf("prepare_checkout requires --params or --spec-file with checkout")
		}
		checkout, err := parseVaultCheckoutParams(raw)
		if err != nil {
			return nil, err
		}
		return &vaultOperationParams{Checkout: checkout}, nil
	}
	if operation != "fill" {
		if paramsSet {
			return nil, fmt.Errorf("--params is only supported for fill and prepare_checkout; authorize takes no parameters")
		}
		return nil, nil
	}
	if !paramsSet {
		return nil, fmt.Errorf("fill requires --params or --spec-file with browser_id and fields")
	}
	fill, err := parseVaultFillParams(raw)
	if err != nil {
		return nil, err
	}
	return &vaultOperationParams{Fill: fill}, nil
}

func parseVaultFillParams(raw string) (*vaultFillParams, error) {
	object, err := vaultParamsObject(raw, "browser_id page_url fields timeout_ms")
	if err != nil {
		return nil, err
	}
	var params vaultFillParams
	if json.Unmarshal(object["browser_id"], &params.BrowserID) != nil || strings.TrimSpace(params.BrowserID) == "" {
		return nil, fmt.Errorf("browser_id must be a non-empty browser session ID, not a name")
	}
	if rawURL, present := object["page_url"]; present {
		if json.Unmarshal(rawURL, &params.PageURL) != nil || !vaultFillPageURLPattern.MatchString(params.PageURL) {
			return nil, fmt.Errorf("page_url must be an exact HTTP or HTTPS URL without credentials or a wildcard host")
		}
		u, err := url.Parse(params.PageURL)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
			return nil, fmt.Errorf("page_url must be an exact HTTP or HTTPS URL without credentials")
		}
	}
	if timeout, ok := object["timeout_ms"]; ok {
		if json.Unmarshal(timeout, &params.TimeoutMS) != nil || params.TimeoutMS == nil || *params.TimeoutMS < 1 || *params.TimeoutMS > 30000 {
			return nil, fmt.Errorf("timeout_ms must be an integer between 1 and 30000")
		}
	}
	var fields []json.RawMessage
	if json.Unmarshal(object["fields"], &fields) != nil || len(fields) < 1 || len(fields) > 32 {
		return nil, fmt.Errorf("fields must be an array of 1-32 field bindings")
	}
	params.Fields = make([]vaultFillField, 0, len(fields))
	for i, rawField := range fields {
		field, err := vaultParamsObject(string(rawField), "field selector format")
		if err != nil {
			return nil, fmt.Errorf("fields[%d]: %w", i, err)
		}
		var binding vaultFillField
		if json.Unmarshal(field["selector"], &binding.Selector) != nil || strings.TrimSpace(binding.Selector) == "" {
			return nil, fmt.Errorf("fields[%d].selector must be a non-empty CSS selector", i)
		}
		if json.Unmarshal(field["field"], &binding.Field) != nil || strings.TrimSpace(binding.Field) == "" || len(binding.Field) > 64 {
			return nil, fmt.Errorf("fields[%d].field must be a non-empty field name of at most 64 bytes", i)
		}
		if format, present := field["format"]; present {
			if json.Unmarshal(format, &binding.Format) != nil || (binding.Format != "MM/YY" && binding.Format != "MM/YYYY") {
				return nil, fmt.Errorf("fields[%d].format must be MM/YY or MM/YYYY", i)
			}
		}
		params.Fields = append(params.Fields, binding)
	}
	return &vaultOperationParams{Fill: &params}, nil
}

// Preparations are single-use even after failure or expiry, so reject a
// malformed checkout context before spending one.
func parseVaultCheckoutContext(raw string, paramsSet bool) (*vaultCheckoutContext, error) {
	if !paramsSet {
		return nil, fmt.Errorf("prepare_checkout requires --params with browser_id, merchant_origin, and environment")
	}
	object, err := vaultParamsObject(raw, "browser_id merchant_origin environment")
	if err != nil {
		return nil, err
	}
	var checkout vaultCheckoutContext
	if json.Unmarshal(object["browser_id"], &checkout.BrowserID) != nil || strings.TrimSpace(checkout.BrowserID) == "" {
		return nil, fmt.Errorf("--params.browser_id must be a non-empty browser session ID, not a name")
	}
	if json.Unmarshal(object["environment"], &checkout.Environment) != nil || (checkout.Environment != "production" && checkout.Environment != "sandbox") {
		return nil, fmt.Errorf("--params.environment must be production or sandbox; it describes Square, not the AgentCard credential mode")
	}
	if json.Unmarshal(object["merchant_origin"], &checkout.MerchantOrigin) != nil {
		return nil, fmt.Errorf("--params.merchant_origin must be the top-level merchant document's origin, not the Square iframe")
	}
	origin, err := vaultMerchantOrigin(checkout.MerchantOrigin)
	if err != nil {
		return nil, err
	}
	checkout.MerchantOrigin = origin
	return &checkout, nil
}

// A canonical origin carries no path, query, fragment, or credentials. HTTP is
// accepted only for loopback test merchants.
func vaultMerchantOrigin(value string) (string, error) {
	invalid := fmt.Errorf("--params.merchant_origin must be a canonical HTTPS origin such as https://shop.example.com (http accepted only for localhost), without a path, query, or fragment")
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", invalid
	}
	if u.Path != "" && u.Path != "/" {
		return "", invalid
	}
	switch u.Scheme {
	case "https":
	case "http":
		if host := u.Hostname(); host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return "", invalid
		}
	default:
		return "", invalid
	}
	return u.Scheme + "://" + u.Host, nil
}

// Card and credential items accept different bindings, and the item type is only
// known after the item is read. Reject mismatches before any browser writes.
func validateVaultFillForItem(item *kernel.VaultItemUnion, params *vaultFillParams) error {
	if item.Type == "credential" {
		for i, field := range params.Fields {
			if field.Format != "" {
				return fmt.Errorf("--params.fields[%d].format is only supported for a card's combined expiration field", i)
			}
			if _, declared := item.Spec.Fields[field.Field]; !declared {
				return fmt.Errorf("--params.fields[%d].field %q is not declared on this credential item", i, field.Field)
			}
		}
		return nil
	}
	if params.PageURL == "" {
		return fmt.Errorf("--params.page_url is required for card items; only credential items may omit it")
	}
	for i, field := range params.Fields {
		if field.Field != "expiration" && !slices.Contains(vaultCardFillFields, field.Field) {
			return fmt.Errorf("--params.fields[%d].field must be a supported card field", i)
		}
	}
	return nil
}

func validateVaultFillItem(params *vaultFillParams, item *kernel.VaultItemUnion) error {
	switch item.Type {
	case "credential":
		var definition struct {
			Spec struct {
				Fields map[string]json.RawMessage `json:"fields"`
			} `json:"spec"`
		}
		if json.Unmarshal([]byte(item.RawJSON()), &definition) != nil || len(definition.Spec.Fields) == 0 {
			return fmt.Errorf("credential field definitions unavailable; fill was not invoked")
		}
		for i, field := range params.Fields {
			if len(field.Selector) > 2048 {
				return fmt.Errorf("fill binding %d: credential selectors must not exceed 2048 bytes", i)
			}
			if _, exists := definition.Spec.Fields[field.Field]; !exists {
				return fmt.Errorf("fill binding %d must reference a declared credential field", i)
			}
			if field.Format != "" {
				return fmt.Errorf("fill binding %d: format is not supported for credentials", i)
			}
		}
	case "card":
		if !strings.HasPrefix(params.PageURL, "https://") {
			return fmt.Errorf("card fill requires an exact HTTPS page_url")
		}
		for i, field := range params.Fields {
			switch field.Field {
			case "expiration":
				if field.Format != "MM/YY" && field.Format != "MM/YYYY" {
					return fmt.Errorf("fill binding %d: expiration requires format MM/YY or MM/YYYY", i)
				}
			case "number", "cvc", "exp_month", "exp_year", "billing_name", "billing_line1", "billing_line2", "billing_city", "billing_state", "billing_postal_code", "billing_country":
				if field.Format != "" {
					return fmt.Errorf("fill binding %d: format is only supported for expiration", i)
				}
			default:
				return fmt.Errorf("fill binding %d must reference a supported card field", i)
			}
		}
	default:
		return fmt.Errorf("fill is not supported for this item type")
	}
	return nil
}
