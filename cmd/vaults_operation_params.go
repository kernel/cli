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
	if openSet && operation != "collect" && operation != "prepare_checkout" {
		return nil, fmt.Errorf("--open is only supported for collect and prepare_checkout")
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
			return nil, fmt.Errorf("--params is only supported for fill and prepare_checkout")
		}
		return nil, nil
	}
	if !paramsSet {
		return nil, fmt.Errorf("fill requires --params or --spec-file with browser_id")
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
	if rawFields, present := object["fields"]; present {
		if json.Unmarshal(rawFields, &fields) != nil || len(fields) < 1 || len(fields) > 32 {
			return nil, fmt.Errorf("fields must be an array of 1-32 field bindings")
		}
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
