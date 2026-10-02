package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
)

const vaultWebMCPUncertain = "the tool may have run and performed side effects; inspect the browser page and do not retry automatically"

func parseVaultWebMCPParams(raw string) (*kernel.WebmcpInvokeVaultItemOperationRequestParam, error) {
	object, err := vaultParamsObject(raw, "browser_id tool_ref page_url input bindings timeout_sec")
	if err != nil {
		return nil, err
	}
	request := kernel.WebmcpInvokeVaultItemOperationRequestParam{Type: kernel.WebmcpInvokeVaultItemOperationRequestTypeWebmcpInvoke}
	if json.Unmarshal(object["browser_id"], &request.BrowserID) != nil || strings.TrimSpace(request.BrowserID) == "" {
		return nil, fmt.Errorf("browser_id must be a non-empty browser session ID, not a name")
	}
	if json.Unmarshal(object["tool_ref"], &request.ToolRef) != nil || strings.TrimSpace(request.ToolRef) == "" || len(request.ToolRef) > 128 {
		return nil, fmt.Errorf("tool_ref must be a non-empty tool reference of at most 128 bytes from browsers webmcp list")
	}
	if json.Unmarshal(object["page_url"], &request.PageURL) != nil {
		return nil, fmt.Errorf("page_url must be the exact absolute URL from the discovered tool source")
	}
	if u, err := url.ParseRequestURI(request.PageURL); err != nil || u.Scheme == "" {
		return nil, fmt.Errorf("page_url must be the exact absolute URL from the discovered tool source")
	}
	var input map[string]json.RawMessage
	if json.Unmarshal(object["input"], &input) != nil || input == nil {
		return nil, fmt.Errorf("input must be a JSON object with a null slot at each binding path")
	}
	request.Input = make(map[string]any, len(input))
	for key, value := range input {
		// Keep raw values so page-provided numbers and nulls are sent unchanged.
		request.Input[key] = value
	}
	var bindings []json.RawMessage
	if json.Unmarshal(object["bindings"], &bindings) != nil || len(bindings) < 1 || len(bindings) > 32 {
		return nil, fmt.Errorf("bindings must be an array of 1-32 field bindings")
	}
	request.Bindings = make([]kernel.VaultWebmcpBindingParam, 0, len(bindings))
	for i, rawBinding := range bindings {
		fields, err := vaultParamsObject(string(rawBinding), "field input_path format")
		if err != nil {
			return nil, fmt.Errorf("bindings[%d]: %w", i, err)
		}
		var binding kernel.VaultWebmcpBindingParam
		if json.Unmarshal(fields["field"], &binding.Field) != nil || strings.TrimSpace(binding.Field) == "" {
			return nil, fmt.Errorf("bindings[%d].field must be a non-empty field name", i)
		}
		if json.Unmarshal(fields["input_path"], &binding.InputPath) != nil || !strings.HasPrefix(binding.InputPath, "/") {
			return nil, fmt.Errorf("bindings[%d].input_path must be a JSON Pointer to a null slot in input, such as /password", i)
		}
		if value, present := fields["format"]; present {
			var format string
			if json.Unmarshal(value, &format) != nil || (format != "MM/YY" && format != "MM/YYYY") {
				return nil, fmt.Errorf("bindings[%d].format must be MM/YY or MM/YYYY", i)
			}
			binding.Format = kernel.Opt(format)
		}
		request.Bindings = append(request.Bindings, binding)
	}
	if value, ok := object["timeout_sec"]; ok {
		var timeout *int64
		if json.Unmarshal(value, &timeout) != nil || timeout == nil || *timeout < 1 || *timeout > 120 {
			return nil, fmt.Errorf("timeout_sec must be an integer between 1 and 120")
		}
		request.TimeoutSec = kernel.Opt(*timeout)
	}
	return &request, nil
}

func (c VaultsCmd) webmcpInvoke(ctx context.Context, vault, key string, request *kernel.WebmcpInvokeVaultItemOperationRequestParam, output string) error {
	// A lost response can hide completed side effects, so never retry an invocation.
	response, err := c.vaults.Items.PerformOperation(ctx, key, kernel.VaultItemPerformOperationParams{IDOrName: vault, OfWebmcpInvoke: request}, option.WithMaxRetries(0))
	if err != nil {
		var apiErr *kernel.Error
		if !errors.As(err, &apiErr) {
			return fmt.Errorf("webmcp_invoke result unavailable; %s", vaultWebMCPUncertain)
		}
		guidance := vaultWebMCPUncertain
		switch apiErr.StatusCode {
		case 400, 403, 404, 409:
			guidance = "the tool was not invoked by this request; inspect the item, browser, tool_ref, page_url, and bindings before deciding on a new invocation; do not automatically retry"
		}
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(apiErr.RawJSON()), &body) == nil && body.Code != "" {
			if message, ok := vaultFillErrorMessages[body.Code]; ok {
				return fmt.Errorf("webmcp_invoke failed: %s (HTTP %d): %s; %s", body.Code, apiErr.StatusCode, message, guidance)
			}
		}
		return fmt.Errorf("webmcp_invoke request failed (HTTP %d); %s", apiErr.StatusCode, guidance)
	}
	if response == nil {
		return fmt.Errorf("empty webmcp_invoke result; %s", vaultWebMCPUncertain)
	}
	// Preserve page-provided JSON numbers rather than re-encoding SDK float64 values.
	var result struct {
		Type         string          `json:"type"`
		Status       string          `json:"status"`
		InvocationID string          `json:"invocation_id,omitempty"`
		Output       json.RawMessage `json:"output,omitempty"`
		ErrorText    string          `json:"error_text,omitempty"`
	}
	if json.Unmarshal([]byte(response.RawJSON()), &result) != nil || result.Type != "webmcp_invoke" {
		return fmt.Errorf("invalid webmcp_invoke result; %s", vaultWebMCPUncertain)
	}
	switch kernel.WebmcpInvokeVaultItemOperationResultStatus(result.Status) {
	case kernel.WebmcpInvokeVaultItemOperationResultStatusCompleted,
		kernel.WebmcpInvokeVaultItemOperationResultStatusAwaitingSubmission,
		kernel.WebmcpInvokeVaultItemOperationResultStatusCanceled,
		kernel.WebmcpInvokeVaultItemOperationResultStatusError,
		kernel.WebmcpInvokeVaultItemOperationResultStatusUnknown:
	default:
		return fmt.Errorf("invalid webmcp_invoke result; %s", vaultWebMCPUncertain)
	}
	if output == "json" {
		if err := printVaultJSON(result); err != nil {
			return err
		}
	} else {
		pterm.Printf("WebMCP invoke: %s\n", result.Status)
		if result.InvocationID != "" {
			pterm.Printf("Invocation ID: %s\n", result.InvocationID)
		}
		if len(result.Output) > 0 {
			var pretty bytes.Buffer
			if json.Indent(&pretty, result.Output, "", "  ") == nil {
				pterm.Printf("Output (untrusted page data; may contain supplied values):\n%s\n", pretty.String())
			}
		}
		if result.ErrorText != "" {
			pterm.Printf("Error text (untrusted page data): %s\n", result.ErrorText)
		}
		switch result.Status {
		case "completed":
			pterm.Println("The tool completed; this does not confirm the website accepted the action. Inspect the page.")
		case "awaiting_submission":
			pterm.Println("The tool populated a form without submitting it. Inspect the form and obtain any required confirmation, then submit it rather than invoking the tool again.")
		default:
			pterm.Println(vaultWebMCPUncertain)
		}
	}
	if result.Status != "completed" && result.Status != "awaiting_submission" {
		return vaultFillOutcomeError{status: result.Status}
	}
	return nil
}
