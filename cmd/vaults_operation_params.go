package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
)

type vaultOperationParams struct {
	Fill     *vaultFillParams
	Checkout *kernel.VaultCheckoutContextParam
	// OnePassword is a complete 1pw_* request body; Invoke supplies the vault.
	OnePassword *kernel.VaultItemPerformOperationParams
}

func isOnePasswordOperation(operation string) bool {
	return strings.HasPrefix(operation, "1pw_")
}

// vaultOperationTakesParams reports whether an operation accepts --params or --spec-file.
func vaultOperationTakesParams(operation string) bool {
	return operation == "fill" || operation == "prepare_checkout" || (isOnePasswordOperation(operation) && operation != "1pw_recover")
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
	if openSet && operation != "authorize" && operation != "collect" && operation != "prepare_checkout" && operation != "1pw_recover" {
		return nil, fmt.Errorf("--open is only supported for authorize, collect, prepare_checkout, and 1pw_recover")
	}
	if len(raw) > 128*1024 {
		return nil, fmt.Errorf("operation parameters exceed 128 KiB")
	}
	if isOnePasswordOperation(operation) {
		if operation == "1pw_recover" {
			if paramsSet {
				return nil, fmt.Errorf("1pw_recover takes no parameters")
			}
			return &vaultOperationParams{OnePassword: &kernel.VaultItemPerformOperationParams{Of1pwRecover: &kernel.OnePasswordRecoverVaultItemOperationRequestParam{Type: kernel.OnePasswordRecoverVaultItemOperationRequestType1pwRecover}}}, nil
		}
		if !paramsSet {
			return nil, fmt.Errorf("%s requires --params or --spec-file", operation)
		}
		request, err := parseOnePasswordOperationParams(operation, raw)
		if err != nil {
			return nil, err
		}
		return &vaultOperationParams{OnePassword: request}, nil
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
			return nil, fmt.Errorf("--params is only supported for fill, prepare_checkout, and 1Password operations; authorize takes no parameters")
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
		if json.Unmarshal(rawURL, &params.PageURL) != nil {
			return nil, fmt.Errorf("page_url must be an absolute URL")
		}
		u, err := url.ParseRequestURI(params.PageURL)
		if err != nil || u.Scheme == "" {
			return nil, fmt.Errorf("page_url must be an absolute URL")
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
	return &params, nil
}

func parseOnePasswordOperationParams(operation, raw string) (*kernel.VaultItemPerformOperationParams, error) {
	allowed := map[string]string{
		"1pw_request_access":   "browser_id goal reason keywords",
		"1pw_poll_access":      "browser_id timeout_seconds",
		"1pw_reconcile_access": "acknowledge_unconfirmed",
		"1pw_fill":             "browser_id page_url timeout_ms",
	}[operation]
	if allowed == "" {
		return nil, fmt.Errorf("unsupported 1Password operation %q", operation)
	}
	object, err := vaultParamsObject(raw, allowed)
	if err != nil {
		return nil, err
	}
	var browserID string
	if strings.Contains(allowed, "browser_id") {
		if json.Unmarshal(object["browser_id"], &browserID) != nil || strings.TrimSpace(browserID) == "" {
			return nil, fmt.Errorf("browser_id must be a non-empty browser session ID, not a name")
		}
	}
	switch operation {
	case "1pw_request_access":
		request := kernel.OnePasswordRequestAccessVaultItemOperationRequestParam{BrowserID: browserID, Type: kernel.OnePasswordRequestAccessVaultItemOperationRequestType1pwRequestAccess}
		for _, name := range []string{"goal", "reason"} {
			if value, ok := object[name]; ok {
				var text string
				if json.Unmarshal(value, &text) != nil {
					return nil, fmt.Errorf("%s must be a string", name)
				}
				if name == "goal" {
					request.Goal = kernel.Opt(text)
				} else {
					request.Reason = kernel.Opt(text)
				}
			}
		}
		if value, ok := object["keywords"]; ok && json.Unmarshal(value, &request.Keywords) != nil {
			return nil, fmt.Errorf("keywords must be an array of strings")
		}
		return &kernel.VaultItemPerformOperationParams{Of1pwRequestAccess: &request}, nil
	case "1pw_poll_access":
		request := kernel.VaultItemPerformOperationParamsBody1pwPollAccess{BrowserID: browserID}
		if value, ok := object["timeout_seconds"]; ok {
			var timeout *int64
			if json.Unmarshal(value, &timeout) != nil || timeout == nil || *timeout < 0 || *timeout > 120 {
				return nil, fmt.Errorf("timeout_seconds must be an integer between 0 and 120")
			}
			request.TimeoutSeconds = kernel.Opt(*timeout)
		}
		return &kernel.VaultItemPerformOperationParams{Of1pwPollAccess: &request}, nil
	case "1pw_reconcile_access":
		var acknowledged bool
		if json.Unmarshal(object["acknowledge_unconfirmed"], &acknowledged) != nil || !acknowledged {
			return nil, fmt.Errorf("acknowledge_unconfirmed must be true; first check 1Password for an existing request")
		}
		return &kernel.VaultItemPerformOperationParams{Of1pwReconcileAccess: &kernel.VaultItemPerformOperationParamsBody1pwReconcileAccess{AcknowledgeUnconfirmed: true}}, nil
	default:
		request := kernel.OnePasswordFillVaultItemOperationRequestParam{BrowserID: browserID, Type: kernel.OnePasswordFillVaultItemOperationRequestType1pwFill}
		if json.Unmarshal(object["page_url"], &request.PageURL) != nil {
			return nil, fmt.Errorf("page_url must be the exact absolute URL of the open login page")
		}
		if u, err := url.ParseRequestURI(request.PageURL); err != nil || u.Scheme == "" {
			return nil, fmt.Errorf("page_url must be the exact absolute URL of the open login page")
		}
		if value, ok := object["timeout_ms"]; ok {
			var timeout *int64
			if json.Unmarshal(value, &timeout) != nil || timeout == nil || *timeout < 1 || *timeout > 30000 {
				return nil, fmt.Errorf("timeout_ms must be an integer between 1 and 30000")
			}
			request.TimeoutMs = kernel.Opt(*timeout)
		}
		return &kernel.VaultItemPerformOperationParams{Of1pwFill: &request}, nil
	}
}
