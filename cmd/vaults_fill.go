package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
)

type vaultFillResult struct {
	Type   string                 `json:"type"`
	Status string                 `json:"status"`
	Fields []vaultFillFieldResult `json:"fields"`
}

type vaultFillFieldResult struct {
	Index     *int   `json:"index"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
}

var vaultFillResultFields = vaultOutputFields{
	"type": nil, "status": nil,
	"fields": vaultFieldsOf("index status error_code"),
}

const vaultFillUncertain = "browser fields may have been written; fill never submits, so it is safe to retry; do not fall back to aliases"

var vaultFillErrorMessages = map[string]string{
	"invalid_request":      "check field names, formats, and browser parameters",
	"invalid_selector":     "inspect the page and correct the selector",
	"duplicate_target":     "multiple bindings resolve to the same element; use distinct targets",
	"timeout":              "the fill deadline elapsed",
	"target_changed":       "the page or target changed; inspect the current page",
	"page_not_found":       "no open page matches page_url; use the exact current URL",
	"ambiguous_page":       "more than one page matches; identify exactly one open page",
	"element_not_found":    "no editable target matches a selector; inspect the page and correct the binding",
	"ambiguous_selector":   "a selector matches multiple targets across frames; use a unique selector",
	"element_not_editable": "choose an editable input or select",
	"option_not_found":     "the select has no matching option value",
	"field_unavailable":    "a field has no usable stored value; inspect definitions and presence, and collect missing values",
	"conflict":             "the item or browser is not ready; inspect readiness, binding, and unresolved prior operations",
	"destination_denied":   "destination or browser vault binding is not authorized; check the bound browser and destination",
	"not_found":            "check the vault, item, browser identifiers, and project",
	"execution_failed":     "fill execution failed",
}

// vaultFillLookupError reports the item lookup status without response details; the fill was never sent.
func vaultFillLookupError(err error, operation string) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) {
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(apiErr.RawJSON()), &body) == nil {
			if _, ok := vaultFillErrorMessages[body.Code]; ok {
				return fmt.Errorf("could not retrieve vault item: %s (HTTP %d); %s was not invoked", body.Code, apiErr.StatusCode, operation)
			}
		}
		return fmt.Errorf("could not retrieve vault item (HTTP %d); %s was not invoked", apiErr.StatusCode, operation)
	}
	return fmt.Errorf("could not retrieve vault item; %s was not invoked", operation)
}

// vaultOperationRequestError reports a failed operation request. rejected maps the HTTP
// statuses the API returns before the operation runs to their guidance; every other
// failure, including transport loss, gets the uncertain guidance.
func vaultOperationRequestError(err error, operation string, messages map[string]string, rejected map[int]string, uncertain string) error {
	var apiErr *kernel.Error
	if !errors.As(err, &apiErr) {
		// Do not wrap SDK/transport errors: they can contain request or response data,
		// and the root error handler extracts raw SDK error messages through Unwrap.
		return fmt.Errorf("%s result unavailable; %s", operation, uncertain)
	}
	guidance, ok := rejected[apiErr.StatusCode]
	if !ok {
		guidance = uncertain
	}
	var body struct {
		Code string `json:"code"`
	}
	if json.Unmarshal([]byte(apiErr.RawJSON()), &body) == nil {
		if message, ok := messages[body.Code]; ok {
			return fmt.Errorf("%s failed: %s (HTTP %d): %s; %s", operation, body.Code, apiErr.StatusCode, message, guidance)
		}
	}
	return fmt.Errorf("%s request failed (HTTP %d); %s", operation, apiErr.StatusCode, guidance)
}

const vaultFillNotWritten = "no fields were written by this request; correct the cause and retry"

var vaultFillRejected = map[int]string{400: vaultFillNotWritten, 403: vaultFillNotWritten, 404: vaultFillNotWritten, 409: vaultFillNotWritten}

func (c VaultsCmd) fill(ctx context.Context, vault, key string, params *vaultFillParams, output string) error {
	request := kernel.FillVaultItemOperationRequestParam{
		BrowserID: params.BrowserID,
		Type:      kernel.FillVaultItemOperationRequestTypeFill,
		Fields:    make([]kernel.VaultFillFieldParam, 0, len(params.Fields)),
	}
	if params.PageURL != "" {
		request.PageURL = kernel.Opt(params.PageURL)
	}
	if params.TimeoutMS != nil {
		request.TimeoutMs = kernel.Opt(int64(*params.TimeoutMS))
	}
	for _, field := range params.Fields {
		binding := kernel.VaultFillFieldParam{Field: field.Field, Selector: field.Selector}
		if field.Format != "" {
			binding.Format = kernel.VaultFillFieldFormat(field.Format)
		}
		request.Fields = append(request.Fields, binding)
	}
	response, err := c.vaults.Items.PerformOperation(ctx, key, kernel.VaultItemPerformOperationParams{IDOrName: vault, OfFill: &request}, option.WithMaxRetries(0))
	if err != nil {
		return vaultOperationRequestError(err, "fill", vaultFillErrorMessages, vaultFillRejected, vaultFillUncertain)
	}
	if response == nil {
		return fmt.Errorf("empty fill result; %s", vaultFillUncertain)
	}
	result, err := parseVaultFillResult(json.RawMessage(response.RawJSON()), len(params.Fields))
	if err != nil {
		return err
	}
	if output == "json" {
		if err := printVaultJSON(result); err != nil {
			return err
		}
	} else {
		pterm.Printf("Fill: %s\n", result.Status)
		rows := pterm.TableData{{"Field index", "Status", "Error code"}}
		for _, field := range result.Fields {
			rows = append(rows, []string{strconv.Itoa(*field.Index), field.Status, field.ErrorCode})
		}
		PrintTableNoPad(rows, true)
		if result.Status == "completed" {
			pterm.Println("Fields filled; this does not confirm website acceptance or form submission.")
		} else {
			pterm.Println(vaultFillUncertain)
		}
	}
	if result.Status != "completed" {
		return vaultOperationOutcomeError{operation: "fill", status: result.Status}
	}
	return nil
}

// The result has already been printed; retain a nonzero exit without diagnostics.
type vaultOperationOutcomeError struct{ operation, status string }

func (e vaultOperationOutcomeError) Error() string { return e.operation + " " + e.status }
func (e vaultOperationOutcomeError) Silent() bool  { return true }

func parseVaultFillResult(raw json.RawMessage, count int) (*vaultFillResult, error) {
	invalid := fmt.Errorf("invalid fill result; %s", vaultFillUncertain)
	safe, err := filterVaultJSON(raw, vaultFillResultFields)
	if err != nil {
		return nil, invalid
	}
	var result vaultFillResult
	if json.Unmarshal(safe, &result) != nil || result.Type != "fill" || len(result.Fields) != count {
		return nil, invalid
	}
	status := "completed"
	stopped := false
	for i, field := range result.Fields {
		if field.Index == nil || *field.Index != i {
			return nil, invalid
		}
		if stopped {
			if field.Status != "not_attempted" {
				return nil, invalid
			}
		} else {
			switch field.Status {
			case "filled":
			case "failed", "unknown":
				status, stopped = field.Status, true
			default:
				return nil, invalid
			}
		}
		if field.ErrorCode != "" {
			if field.Status != "failed" && field.Status != "unknown" {
				return nil, invalid
			}
			switch field.ErrorCode {
			case "target_changed", "element_not_found", "ambiguous_selector", "element_not_editable", "option_not_found", "timeout", "execution_failed":
			default:
				return nil, invalid
			}
		}
	}
	if result.Status != status {
		return nil, invalid
	}
	return &result, nil
}

const onePasswordFillUncertain = "the form may have been submitted; inspect the browser and do not retry in the same browser"

const onePasswordFillNotSubmitted = "nothing was submitted by this request; inspect the item, browser, and page_url before deciding on a new fill; do not automatically retry"

// Only these statuses are returned before the extension is invoked.
var onePasswordFillRejected = map[int]string{
	400: onePasswordFillNotSubmitted, 403: onePasswordFillNotSubmitted, 404: onePasswordFillNotSubmitted,
	409: "nothing was submitted by this request; if several approved entries match this page, pass entry_id from items get -o json; do not automatically retry",
}

var onePasswordFillResultFields = vaultFieldsOf("type status error_code")

func (c VaultsCmd) onePasswordFill(ctx context.Context, vault, key string, request *kernel.VaultItemPerformOperationParams, output string) error {
	params := *request
	params.IDOrName = vault
	response, err := c.vaults.Items.PerformOperation(ctx, key, params, option.WithMaxRetries(0))
	if err != nil {
		var apiErr *kernel.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == 503 {
			return fmt.Errorf("1pw_fill unavailable (HTTP 503): %s", onePasswordUnavailable)
		}
		return vaultOperationRequestError(err, "1pw_fill", vaultFillErrorMessages, onePasswordFillRejected, onePasswordFillUncertain)
	}
	if response == nil {
		return fmt.Errorf("empty 1pw_fill result; %s", onePasswordFillUncertain)
	}
	safe, err := filterVaultJSON(json.RawMessage(response.RawJSON()), onePasswordFillResultFields)
	if err != nil {
		return fmt.Errorf("invalid 1pw_fill result; %s", onePasswordFillUncertain)
	}
	var result struct {
		Type      string `json:"type"`
		Status    string `json:"status"`
		ErrorCode string `json:"error_code,omitempty"`
	}
	if json.Unmarshal(safe, &result) != nil || result.Type != "1pw_fill" {
		return fmt.Errorf("invalid 1pw_fill result; %s", onePasswordFillUncertain)
	}
	switch result.Status {
	case "fill_submitted", "fill_failed", "fill_unknown":
	default:
		return fmt.Errorf("invalid 1pw_fill result; %s", onePasswordFillUncertain)
	}
	if output == "json" {
		if err := printVaultJSON(result); err != nil {
			return err
		}
	} else {
		pterm.Printf("1Password fill: %s\n", result.Status)
		if result.ErrorCode != "" {
			pterm.Printf("Error code: %s\n", result.ErrorCode)
		}
		switch result.Status {
		case "fill_submitted":
			pterm.Println("The extension filled and submitted the form; this does not confirm the website accepted the login.")
		case "fill_failed":
			pterm.Println("The extension reported a failure. Inspect the page before deciding on a new fill; do not retry automatically.")
		default:
			pterm.Println(onePasswordFillUncertain)
		}
	}
	if result.Status != "fill_submitted" {
		return vaultOperationOutcomeError{operation: "1pw_fill", status: result.Status}
	}
	return nil
}
