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
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	Instruction string                 `json:"instruction,omitempty"`
	Fields      []vaultFillFieldResult `json:"fields"`
}

type vaultFillFieldResult struct {
	Index     *int   `json:"index"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
}

var vaultFillResultFields = vaultOutputFields{
	"type": nil, "status": nil, "instruction": nil,
	"fields": vaultFieldsOf("index status error_code"),
}

const vaultFillUncertain = "browser fields may have been written; inspect the browser and do not retry or fall back to aliases"

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

func vaultFillRequestError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) {
		var body struct {
			Code string `json:"code"`
		}
		guidance := vaultFillUncertain
		switch apiErr.StatusCode {
		case 400, 403, 404, 409:
			guidance = "no fields were written by this request; inspect and correct the cause before deciding on a new fill; do not automatically retry"
		}
		if json.Unmarshal([]byte(apiErr.RawJSON()), &body) == nil {
			if message, ok := vaultFillErrorMessages[body.Code]; ok {
				return fmt.Errorf("fill failed: %s (HTTP %d): %s; %s", body.Code, apiErr.StatusCode, message, guidance)
			}
		}
		return fmt.Errorf("fill request failed (HTTP %d); %s", apiErr.StatusCode, guidance)
	}
	// Do not wrap SDK/transport errors: they can contain request or response data,
	// and the root error handler extracts raw SDK error messages through Unwrap.
	return fmt.Errorf("fill result unavailable; %s", vaultFillUncertain)
}

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
		return vaultFillRequestError(err)
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
		if len(result.Fields) > 0 {
			rows := pterm.TableData{{"Field index", "Status", "Error code"}}
			for _, field := range result.Fields {
				rows = append(rows, []string{strconv.Itoa(*field.Index), field.Status, field.ErrorCode})
			}
			PrintTableNoPad(rows, true)
		}
		if result.Status == "completed" {
			if result.Instruction != "" {
				pterm.Println(result.Instruction)
			} else {
				pterm.Println("Fields filled; this does not confirm website acceptance or form submission.")
			}
		} else {
			pterm.Println(vaultFillUncertain)
		}
	}
	if result.Status != "completed" {
		return vaultFillOutcomeError{status: result.Status}
	}
	return nil
}

// The result has already been printed; retain a nonzero exit without diagnostics.
type vaultFillOutcomeError struct{ status string }

func (e vaultFillOutcomeError) Error() string { return "fill " + e.status }
func (e vaultFillOutcomeError) Silent() bool  { return true }

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
