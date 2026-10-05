package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const maxVaultWebMCPInputBytes = 64 * 1024

const vaultWebMCPUncertain = "the tool may have run and submitted or changed site state; inspect the browser and do not retry automatically"

var vaultWebMCPErrorMessages = map[string]string{
	"invalid_request":    "check bindings, null input slots, field names, formats, timeout_sec, and that input matches the tool's inputSchema",
	"target_changed":     "the tool_ref is no longer live or its source.page_url differs; list tools again with browsers webmcp list",
	"timeout":            "the deadline elapsed before the tool was invoked",
	"field_unavailable":  "a field has no usable stored value; inspect definitions and presence, and collect missing values",
	"conflict":           "the item or browser is not ready; inspect readiness, binding, and unresolved prior operations",
	"destination_denied": "the tool's page or registering frame is not an authorized destination for this item, or the browser is not bound to the vault",
	"not_found":          "check the vault, item, browser identifiers, and project",
	"execution_failed":   "WebMCP invocation failed",
}

type vaultWebMCPResult struct {
	Type         string          `json:"type"`
	Status       string          `json:"status"`
	InvocationID *string         `json:"invocation_id,omitempty"`
	Output       json.RawMessage `json:"output,omitempty"`
	ErrorText    *string         `json:"error_text,omitempty"`
}

// vaultWebMCPStatuses maps each result status to whether it exits 0 and its human hint.
var vaultWebMCPStatuses = map[string]struct {
	ok   bool
	hint string
}{
	"completed":           {true, "The tool reported completion; this does not confirm the website accepted the action. Inspect the page."},
	"awaiting_submission": {true, "The tool populated a form with the supplied values without submitting it. " + webMCPAwaitingSubmissionHint},
	"canceled":            {false, "The tool reported cancellation and may have had side effects. Inspect the page before deciding on a new invocation; do not retry automatically."},
	"error":               {false, "The tool reported an error and may have had side effects. Inspect the page before deciding on a new invocation; do not retry automatically."},
	"unknown":             {false, vaultWebMCPUncertain},
}

const vaultWebMCPNotInvoked = "the tool was not invoked by this request; inspect and correct the cause before deciding on a new invocation; do not automatically retry"

var vaultWebMCPRejected = map[int]string{400: vaultWebMCPNotInvoked, 403: vaultWebMCPNotInvoked, 404: vaultWebMCPNotInvoked, 409: vaultWebMCPNotInvoked}

func parseVaultWebMCPParams(raw string) (*kernel.WebmcpInvokeVaultItemOperationRequestParam, error) {
	object, err := vaultParamsObject(raw, "browser_id tool_ref page_url input bindings timeout_sec")
	if err != nil {
		return nil, err
	}
	params := kernel.WebmcpInvokeVaultItemOperationRequestParam{Type: kernel.WebmcpInvokeVaultItemOperationRequestTypeWebmcpInvoke}
	for name, target := range map[string]*string{"browser_id": &params.BrowserID, "tool_ref": &params.ToolRef, "page_url": &params.PageURL} {
		if value, ok := object[name]; ok && json.Unmarshal(value, target) != nil {
			return nil, fmt.Errorf("%s must be a string", name)
		}
	}
	if params.Input, err = decodeVaultWebMCPInput(object["input"]); err != nil {
		return nil, err
	}
	var bindings []json.RawMessage
	if json.Unmarshal(object["bindings"], &bindings) != nil {
		return nil, fmt.Errorf("bindings must be an array of 1-32 field bindings")
	}
	for i, rawBinding := range bindings {
		binding, err := vaultParamsObject(string(rawBinding), "field input_path format")
		if err != nil {
			return nil, fmt.Errorf("bindings[%d]: %w", i, err)
		}
		var b kernel.VaultWebmcpBindingParam
		if json.Unmarshal(binding["field"], &b.Field) != nil {
			return nil, fmt.Errorf("bindings[%d].field must be a string", i)
		}
		if json.Unmarshal(binding["input_path"], &b.InputPath) != nil {
			return nil, fmt.Errorf("bindings[%d].input_path must be a string", i)
		}
		if rawFormat, ok := binding["format"]; ok {
			var format string
			if json.Unmarshal(rawFormat, &format) != nil || format == "" {
				return nil, fmt.Errorf("bindings[%d].format must be MM/YY or MM/YYYY", i)
			}
			b.Format = kernel.Opt(format)
		}
		params.Bindings = append(params.Bindings, b)
	}
	if rawTimeout, ok := object["timeout_sec"]; ok {
		var timeout *int64
		if json.Unmarshal(rawTimeout, &timeout) != nil || timeout == nil {
			return nil, fmt.Errorf("timeout_sec must be an integer between 1 and 120")
		}
		params.TimeoutSec = kernel.Opt(*timeout)
	}
	if err := validateVaultWebMCPParams(&params, func(i int) string { return fmt.Sprintf("bindings[%d]", i) }); err != nil {
		return nil, err
	}
	return &params, nil
}

// decodeVaultWebMCPInput accepts only a JSON object without echoing its contents in errors.
// Members stay json.RawMessage: the SDK encodes json.Number as a string, which would change
// numeric arguments.
func decodeVaultWebMCPInput(raw []byte) (map[string]any, error) {
	invalid := fmt.Errorf("input must be a JSON object of public tool arguments (maximum 64 KiB)")
	dec := json.NewDecoder(bytes.NewReader(raw))
	var members map[string]json.RawMessage
	if dec.Decode(&members) != nil || members == nil {
		return nil, invalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, invalid
	}
	input := make(map[string]any, len(members))
	for name, value := range members {
		input[name] = value
	}
	return input, nil
}

// validateVaultWebMCPParams mirrors the API's webmcp_invoke request rules so mistakes are
// reported locally with specifics; the API remains the source of truth. binding names a
// binding by index in diagnostics.
func validateVaultWebMCPParams(params *kernel.WebmcpInvokeVaultItemOperationRequestParam, binding func(int) string) error {
	if strings.TrimSpace(params.BrowserID) == "" {
		return fmt.Errorf("browser_id must be a non-empty browser session ID, not a name")
	}
	if strings.TrimSpace(params.ToolRef) == "" || len(params.ToolRef) > 128 {
		return fmt.Errorf("tool_ref must be the opaque tool_ref from browsers webmcp list (at most 128 bytes)")
	}
	if u, err := url.ParseRequestURI(params.PageURL); err != nil || u.Scheme == "" || strings.Contains(params.PageURL, "#") {
		return fmt.Errorf("page_url must be the exact source.page_url from browsers webmcp list (absolute, without a fragment)")
	}
	if params.TimeoutSec.Valid() && (params.TimeoutSec.Value < 1 || params.TimeoutSec.Value > 120) {
		return fmt.Errorf("timeout_sec must be an integer between 1 and 120")
	}
	invalidInput := fmt.Errorf("input must be a JSON object of public tool arguments (maximum 64 KiB)")
	if params.Input == nil {
		return invalidInput
	}
	encoded, err := json.Marshal(params.Input)
	if err != nil || len(encoded) > maxVaultWebMCPInputBytes {
		return invalidInput
	}
	dec := json.NewDecoder(bytes.NewReader(encoded))
	dec.UseNumber()
	var input any
	if dec.Decode(&input) != nil {
		return invalidInput
	}
	if len(params.Bindings) < 1 || len(params.Bindings) > 32 {
		return fmt.Errorf("bindings must contain 1-32 field bindings")
	}
	fields, paths := make(map[string]bool), make(map[string]bool)
	for i, b := range params.Bindings {
		if strings.TrimSpace(b.Field) == "" || len(b.Field) > 64 {
			return fmt.Errorf("%s field must be a non-empty field name of at most 64 bytes", binding(i))
		}
		if fields[b.Field] {
			return fmt.Errorf("%s field repeats an earlier binding; bind each field once", binding(i))
		}
		if paths[b.InputPath] {
			return fmt.Errorf("%s input_path repeats an earlier binding; bind each path once", binding(i))
		}
		fields[b.Field], paths[b.InputPath] = true, true
		if b.Format.Valid() && b.Format.Value != "MM/YY" && b.Format.Value != "MM/YYYY" {
			return fmt.Errorf("%s format must be MM/YY or MM/YYYY", binding(i))
		}
		if !vaultWebMCPNullSlot(input, b.InputPath) {
			return fmt.Errorf("%s input_path must be an RFC 6901 JSON Pointer to an existing null value in input", binding(i))
		}
	}
	return nil
}

// vaultWebMCPNullSlot mirrors the API: a binding replaces an existing null and never
// creates a property or array entry. "/" addresses the empty-string key; only the
// root pointer "" is rejected.
func vaultWebMCPNullSlot(input any, path string) bool {
	if len(path) < 1 || len(path) > 2048 || path[0] != '/' {
		return false
	}
	parts := strings.Split(path[1:], "/")
	if len(parts) > 32 {
		return false
	}
	value := input
	for _, part := range parts {
		if strings.Contains(strings.NewReplacer("~0", "", "~1", "").Replace(part), "~") {
			return false
		}
		token := strings.NewReplacer("~1", "/", "~0", "~").Replace(part)
		switch container := value.(type) {
		case map[string]any:
			child, ok := container[token]
			if !ok {
				return false
			}
			value = child
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || strconv.Itoa(index) != token || index >= len(container) {
				return false
			}
			value = container[index]
		default:
			return false
		}
	}
	return value == nil
}

func (c VaultsCmd) webMCPInvoke(ctx context.Context, vault, key string, request *kernel.WebmcpInvokeVaultItemOperationRequestParam, output string) error {
	response, err := c.vaults.Items.PerformOperation(ctx, key, kernel.VaultItemPerformOperationParams{IDOrName: vault, OfWebmcpInvoke: request}, option.WithMaxRetries(0))
	if err != nil {
		return vaultOperationRequestError(err, "webmcp_invoke", vaultWebMCPErrorMessages, vaultWebMCPRejected, vaultWebMCPUncertain)
	}
	if response == nil {
		return fmt.Errorf("empty webmcp_invoke result; %s", vaultWebMCPUncertain)
	}
	var result vaultWebMCPResult
	if json.Unmarshal([]byte(response.RawJSON()), &result) != nil || result.Type != "webmcp_invoke" {
		return fmt.Errorf("invalid webmcp_invoke result; %s", vaultWebMCPUncertain)
	}
	status, known := vaultWebMCPStatuses[result.Status]
	if !known {
		return fmt.Errorf("invalid webmcp_invoke result; %s", vaultWebMCPUncertain)
	}
	if output == "json" {
		if err := printVaultJSON(result); err != nil {
			return err
		}
	} else {
		if err := printVaultWebMCPResult(result); err != nil {
			return err
		}
		pterm.Println(status.hint)
	}
	if !status.ok {
		return vaultOperationOutcomeError{operation: "webmcp_invoke", status: result.Status}
	}
	return nil
}

func printVaultWebMCPResult(result vaultWebMCPResult) error {
	pterm.Printf("WebMCP invoke: %s\n", result.Status)
	if result.InvocationID != nil {
		pterm.Printf("Invocation ID: %s\n", strconv.Quote(*result.InvocationID))
	}
	// Output and error text are page-provided; JSON encoding escapes terminal control characters.
	if len(result.Output) > 0 {
		data, err := json.MarshalIndent(result.Output, "", "  ")
		if err != nil {
			return err
		}
		pterm.Println("Output (untrusted page data; may contain supplied vault values):")
		pterm.Println(string(data))
	}
	if result.ErrorText != nil {
		data, err := json.Marshal(*result.ErrorText)
		if err != nil {
			return err
		}
		pterm.Printf("Error text (untrusted page data; may contain supplied vault values): %s\n", data)
	}
	return nil
}

func newVaultWebMCPCommand() *cobra.Command {
	root := &cobra.Command{Use: "webmcp", Short: "Invoke live WebMCP tools with vault item fields"}
	invoke := &cobra.Command{
		Use:   "invoke <vault> <key>",
		Short: "Invoke a live WebMCP tool with vault fields substituted into null input slots",
		Args:  cobra.ExactArgs(2),
		Long: `Invoke a live WebMCP tool with values from a credential item or ready Link card.
This sends the advertised webmcp_invoke operation; it is equivalent to
items invoke <vault> <key> webmcp_invoke --spec-file with the same parameters.

Discover the tool first with kernel browsers webmcp list <session-id> -o json and pass its
tool_ref and exact source.page_url (fragment omitted). The vault must already be attached
to the browser. --input holds only public tool arguments; put null at each slot a vault
field fills, and never include vault values. Each --bind maps one vault field to an
RFC 6901 JSON Pointer to an existing null value in input (no root or append paths).
Each field and path may be bound once. Use <field>:<format>=<pointer> for card
expiration (MM/YY or MM/YYYY); credential fields take no format. TOTP fields supply a
fresh code, never the seed. --timeout-sec is 1-120 (API default 15).

The tool may submit forms or perform other side effects. Output and error_text are
untrusted page-provided data returned without redaction and may contain supplied vault
values. completed and awaiting_submission exit 0; neither confirms the website accepted
the action. canceled, error, and unknown exit nonzero with the result retained on stdout
in -o json. unknown means the tool may have run. Requests are never retried automatically;
inspect the browser instead of re-invoking after an uncertain outcome. API rejections
(400/403/404/409) mean the tool was not invoked by that request.`,
		Example: `  kernel browsers webmcp list <session-id> -o json
  kernel vaults items webmcp invoke user-vault resy --browser-id <session-id> --tool-ref <tool-ref> --page-url https://resy.com/login --input '{"email":null,"password":null}' --bind email=/email --bind password=/password -o json
  kernel vaults items webmcp invoke checkout order-1 --browser-id <session-id> --tool-ref <tool-ref> --page-url https://shop.example/checkout --input-file input.json --bind number=/card/number --bind expiration:MM/YY=/card/expiry`,
		PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := vaultWebMCPParamsFromFlags(cmd)
			if err != nil {
				return err
			}
			return getVaultsHandler(cmd).Invoke(cmd.Context(), args[0], args[1], "webmcp_invoke", &vaultOperationParams{WebMCP: params}, vaultOutput(cmd), false)
		},
	}
	invoke.Flags().String("browser-id", "", "Browser session ID the vault is attached to, not a name (required)")
	invoke.Flags().String("tool-ref", "", "Opaque tool_ref from browsers webmcp list (required)")
	invoke.Flags().String("page-url", "", "Exact source.page_url from browsers webmcp list, without fragment (required)")
	invoke.Flags().String("input", "", "Public tool input JSON object with null slots for vault fields")
	invoke.Flags().String("input-file", "", "Path to the input JSON object (use '-' for stdin)")
	invoke.Flags().StringArray("bind", nil, "Vault field binding <field>=<json-pointer> or <field>:<format>=<json-pointer>; repeatable (required)")
	invoke.Flags().Int64("timeout-sec", 0, "Tool invocation timeout in seconds, 1-120 (API default 15)")
	for _, name := range []string{"browser-id", "tool-ref", "page-url", "bind"} {
		_ = invoke.MarkFlagRequired(name)
	}
	invoke.MarkFlagsOneRequired("input", "input-file")
	invoke.MarkFlagsMutuallyExclusive("input", "input-file")
	addVaultJSONOutputFlag(invoke)
	root.AddCommand(invoke)
	return root
}

func vaultWebMCPParamsFromFlags(cmd *cobra.Command) (*kernel.WebmcpInvokeVaultItemOperationRequestParam, error) {
	params := kernel.WebmcpInvokeVaultItemOperationRequestParam{Type: kernel.WebmcpInvokeVaultItemOperationRequestTypeWebmcpInvoke}
	params.BrowserID, _ = cmd.Flags().GetString("browser-id")
	params.ToolRef, _ = cmd.Flags().GetString("tool-ref")
	params.PageURL, _ = cmd.Flags().GetString("page-url")
	input, _ := cmd.Flags().GetString("input")
	data := []byte(input)
	if cmd.Flags().Changed("input-file") {
		var err error
		if data, err = readVaultJSONFile(cmd, "input-file"); err != nil {
			return nil, err
		}
	}
	var err error
	if params.Input, err = decodeVaultWebMCPInput(data); err != nil {
		return nil, err
	}
	binds, _ := cmd.Flags().GetStringArray("bind")
	for i, bind := range binds {
		field, path, ok := strings.Cut(bind, "=")
		if !ok || !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("binding %d (--bind) must be <field>=<json-pointer> or <field>:<format>=<json-pointer>, e.g. password=/password", i+1)
		}
		binding := kernel.VaultWebmcpBindingParam{Field: field, InputPath: path}
		if name, format, hasFormat := strings.Cut(field, ":"); hasFormat {
			if format == "" {
				return nil, fmt.Errorf("binding %d (--bind) format must be MM/YY or MM/YYYY", i+1)
			}
			binding.Field, binding.Format = name, kernel.Opt(format)
		}
		params.Bindings = append(params.Bindings, binding)
	}
	if cmd.Flags().Changed("timeout-sec") {
		timeout, _ := cmd.Flags().GetInt64("timeout-sec")
		params.TimeoutSec = kernel.Opt(timeout)
	}
	if err := validateVaultWebMCPParams(&params, func(i int) string { return fmt.Sprintf("binding %d (--bind)", i+1) }); err != nil {
		return nil, err
	}
	return &params, nil
}
