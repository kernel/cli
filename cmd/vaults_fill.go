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

const vaultFillUncertain = "browser fields may have been written; inspect the browser and do not retry or fall back to aliases"

func vaultFillRequestError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) {
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(apiErr.RawJSON()), &body) == nil {
			switch body.Code {
			case "invalid_request", "invalid_selector", "duplicate_target", "timeout", "target_changed", "page_not_found", "ambiguous_page", "element_not_found", "ambiguous_selector", "element_not_editable", "option_not_found", "field_unavailable", "conflict", "destination_denied", "execution_failed":
				return fmt.Errorf("fill failed: %s (HTTP %d); %s", body.Code, apiErr.StatusCode, vaultFillUncertain)
			}
		}
		return fmt.Errorf("fill request failed (HTTP %d); %s", apiErr.StatusCode, vaultFillUncertain)
	}
	// Do not wrap SDK/transport errors: they can contain request or response data,
	// and the root error handler extracts raw SDK error messages through Unwrap.
	return fmt.Errorf("fill result unavailable; %s", vaultFillUncertain)
}

func (c VaultsCmd) fill(ctx context.Context, vault, key, itemType string, params *vaultFillParams, output string) error {
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
