package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

type vaultFillParams struct {
	BrowserID string           `json:"browser_id"`
	PageURL   string           `json:"page_url"`
	Fields    []vaultFillField `json:"fields"`
	TimeoutMS *int             `json:"timeout_ms,omitempty"`
}

type vaultFillField struct {
	Field    string `json:"field"`
	Selector string `json:"selector"`
	Format   string `json:"format,omitempty"`
}

var vaultFillPageURLPattern = regexp.MustCompile(`^https://[^/?#@*\s]+(?:[/?#][^\s]*)?$`)

// Reject duplicate and unknown keys without including payloads in diagnostics.
func vaultParamsObject(raw, allowed string) (map[string]json.RawMessage, error) {
	invalid := fmt.Errorf("--params must contain JSON objects with only supported, non-duplicate properties")
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
			return nil, fmt.Errorf("--params must not contain type; the positional operation supplies it")
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

func parseVaultOperationParams(operation, raw string, paramsSet, openSet bool) (*vaultFillParams, error) {
	if strings.TrimSpace(operation) == "" {
		return nil, fmt.Errorf("operation must not be empty")
	}
	if openSet && operation != "authorize" {
		return nil, fmt.Errorf("--open is only supported for authorize")
	}
	if operation != "fill" {
		if paramsSet {
			return nil, fmt.Errorf("--params is only supported for fill; authorize takes no parameters")
		}
		return nil, nil
	}
	if !paramsSet {
		return nil, fmt.Errorf("fill requires --params with browser_id, page_url, and fields")
	}
	object, err := vaultParamsObject(raw, "browser_id page_url fields timeout_ms")
	if err != nil {
		return nil, err
	}
	var params vaultFillParams
	if json.Unmarshal(object["browser_id"], &params.BrowserID) != nil || strings.TrimSpace(params.BrowserID) == "" {
		return nil, fmt.Errorf("--params.browser_id must be a non-empty browser session ID, not a name")
	}
	if json.Unmarshal(object["page_url"], &params.PageURL) != nil || !vaultFillPageURLPattern.MatchString(params.PageURL) {
		return nil, fmt.Errorf("--params.page_url must be an exact HTTPS URL without credentials or a wildcard host")
	}
	u, err := url.Parse(params.PageURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return nil, fmt.Errorf("--params.page_url must be an exact HTTPS URL without credentials")
	}
	if timeout, ok := object["timeout_ms"]; ok {
		if json.Unmarshal(timeout, &params.TimeoutMS) != nil || params.TimeoutMS == nil || *params.TimeoutMS < 1 || *params.TimeoutMS > 30000 {
			return nil, fmt.Errorf("--params.timeout_ms must be an integer between 1 and 30000")
		}
	}
	var fields []json.RawMessage
	if json.Unmarshal(object["fields"], &fields) != nil || len(fields) < 1 || len(fields) > 32 {
		return nil, fmt.Errorf("--params.fields must be an array of 1-32 field bindings")
	}
	params.Fields = make([]vaultFillField, 0, len(fields))
	for i, rawField := range fields {
		field, err := vaultParamsObject(string(rawField), "field selector format")
		if err != nil {
			return nil, fmt.Errorf("--params.fields[%d]: %w", i, err)
		}
		var binding vaultFillField
		if json.Unmarshal(field["selector"], &binding.Selector) != nil || strings.TrimSpace(binding.Selector) == "" {
			return nil, fmt.Errorf("--params.fields[%d].selector must be a non-empty CSS selector", i)
		}
		if json.Unmarshal(field["field"], &binding.Field) != nil {
			return nil, fmt.Errorf("--params.fields[%d].field must be a supported card field", i)
		}
		switch binding.Field {
		case "expiration":
			if json.Unmarshal(field["format"], &binding.Format) != nil || (binding.Format != "MM/YY" && binding.Format != "MM/YYYY") {
				return nil, fmt.Errorf("--params.fields[%d].format must be MM/YY or MM/YYYY for expiration", i)
			}
		case "number", "cvc", "exp_month", "exp_year", "billing_name", "billing_line1", "billing_line2", "billing_city", "billing_state", "billing_postal_code", "billing_country":
			if _, ok := field["format"]; ok {
				return nil, fmt.Errorf("--params.fields[%d].format is only supported for expiration", i)
			}
		default:
			return nil, fmt.Errorf("--params.fields[%d].field must be a supported card field", i)
		}
		params.Fields = append(params.Fields, binding)
	}
	return &params, nil
}
