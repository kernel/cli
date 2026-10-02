package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
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

type vaultWebMCPParams struct {
	BrowserID string
	ToolRef   string
	PageURL   string
	// Input members stay raw: the SDK encodes json.Number as a string, which would change numeric arguments.
	Input      map[string]json.RawMessage
	Bindings   []vaultWebMCPBinding
	TimeoutSec *int64
}

type vaultWebMCPBinding struct {
	Field     string
	InputPath string
	Format    string
}

type vaultWebMCPResult struct {
	Type         string          `json:"type"`
	Status       string          `json:"status"`
	InvocationID *string         `json:"invocation_id,omitempty"`
	Output       json.RawMessage `json:"output,omitempty"`
	ErrorText    *string         `json:"error_text,omitempty"`
}

func parseVaultWebMCPParams(raw string) (*vaultWebMCPParams, error) {
	object, err := vaultParamsObject(raw, "browser_id tool_ref page_url input bindings timeout_sec")
	if err != nil {
		return nil, err
	}
	var params vaultWebMCPParams
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
		var b vaultWebMCPBinding
		if json.Unmarshal(binding["field"], &b.Field) != nil {
			return nil, fmt.Errorf("bindings[%d].field must be a string", i)
		}
		if json.Unmarshal(binding["input_path"], &b.InputPath) != nil {
			return nil, fmt.Errorf("bindings[%d].input_path must be a string", i)
		}
		if format, ok := binding["format"]; ok && (json.Unmarshal(format, &b.Format) != nil || b.Format == "") {
			return nil, fmt.Errorf("bindings[%d].format must be MM/YY or MM/YYYY", i)
		}
		params.Bindings = append(params.Bindings, b)
	}
	if timeout, ok := object["timeout_sec"]; ok && (json.Unmarshal(timeout, &params.TimeoutSec) != nil || params.TimeoutSec == nil) {
		return nil, fmt.Errorf("timeout_sec must be an integer between 1 and 120")
	}
	if err := validateVaultWebMCPParams(&params, func(i int) string { return fmt.Sprintf("bindings[%d]", i) }); err != nil {
		return nil, err
	}
	return &params, nil
}

// decodeVaultWebMCPInput accepts only a JSON object without echoing its contents in errors.
func decodeVaultWebMCPInput(raw []byte) (map[string]json.RawMessage, error) {
	invalid := fmt.Errorf("input must be a JSON object of public tool arguments (maximum 64 KiB)")
	dec := json.NewDecoder(bytes.NewReader(raw))
	var input map[string]json.RawMessage
	if dec.Decode(&input) != nil || input == nil {
		return nil, invalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, invalid
	}
	return input, nil
}

// binding names a binding by index in diagnostics.
func validateVaultWebMCPParams(params *vaultWebMCPParams, binding func(int) string) error {
	if strings.TrimSpace(params.BrowserID) == "" {
		return fmt.Errorf("browser_id must be a non-empty browser session ID, not a name")
	}
	if strings.TrimSpace(params.ToolRef) == "" || len(params.ToolRef) > 128 {
		return fmt.Errorf("tool_ref must be the opaque tool_ref from browsers webmcp list (at most 128 bytes)")
	}
	if u, err := url.ParseRequestURI(params.PageURL); err != nil || u.Scheme == "" || strings.Contains(params.PageURL, "#") {
		return fmt.Errorf("page_url must be the exact source.page_url from browsers webmcp list (absolute, without a fragment)")
	}
	if params.TimeoutSec != nil && (*params.TimeoutSec < 1 || *params.TimeoutSec > 120) {
		return fmt.Errorf("timeout_sec must be an integer between 1 and 120")
	}
	if params.Input == nil {
		return fmt.Errorf("input must be a JSON object of public tool arguments (maximum 64 KiB)")
	}
	encoded, err := json.Marshal(params.Input)
	if err != nil || len(encoded) > maxVaultWebMCPInputBytes {
		return fmt.Errorf("input must be a JSON object of public tool arguments (maximum 64 KiB)")
	}
	dec := json.NewDecoder(bytes.NewReader(encoded))
	dec.UseNumber()
	var input any
	if dec.Decode(&input) != nil {
		return fmt.Errorf("input must be a JSON object of public tool arguments (maximum 64 KiB)")
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
		if b.Format != "" && b.Format != "MM/YY" && b.Format != "MM/YYYY" {
			return fmt.Errorf("%s format must be MM/YY or MM/YYYY", binding(i))
		}
		if !vaultWebMCPNullSlot(input, b.InputPath) {
			return fmt.Errorf("%s input_path must be an RFC 6901 JSON Pointer to an existing null value in input", binding(i))
		}
	}
	return nil
}

// vaultWebMCPNullSlot mirrors the API: a binding replaces an existing null and never
// creates a property or array entry.
func vaultWebMCPNullSlot(input any, path string) bool {
	if len(path) < 2 || len(path) > 2048 || path[0] != '/' {
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

func vaultWebMCPRequestError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) {
		guidance := vaultWebMCPUncertain
		switch apiErr.StatusCode {
		case 400, 403, 404, 409:
			guidance = "the tool was not invoked by this request; inspect and correct the cause before deciding on a new invocation; do not automatically retry"
		}
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(apiErr.RawJSON()), &body) == nil {
			if message, ok := vaultWebMCPErrorMessages[body.Code]; ok {
				return fmt.Errorf("webmcp_invoke failed: %s (HTTP %d): %s; %s", body.Code, apiErr.StatusCode, message, guidance)
			}
		}
		return fmt.Errorf("webmcp_invoke request failed (HTTP %d); %s", apiErr.StatusCode, guidance)
	}
	// Do not wrap SDK/transport errors: they can contain request or response data.
	return fmt.Errorf("webmcp_invoke result unavailable; %s", vaultWebMCPUncertain)
}

func (c VaultsCmd) webMCPInvoke(ctx context.Context, vault, key string, params *vaultWebMCPParams, output string) error {
	request := kernel.WebmcpInvokeVaultItemOperationRequestParam{
		Type:      kernel.WebmcpInvokeVaultItemOperationRequestTypeWebmcpInvoke,
		BrowserID: params.BrowserID,
		ToolRef:   params.ToolRef,
		PageURL:   params.PageURL,
		Input:     make(map[string]any, len(params.Input)),
		Bindings:  make([]kernel.VaultWebmcpBindingParam, 0, len(params.Bindings)),
	}
	for name, value := range params.Input {
		request.Input[name] = value
	}
	for _, binding := range params.Bindings {
		b := kernel.VaultWebmcpBindingParam{Field: binding.Field, InputPath: binding.InputPath}
		if binding.Format != "" {
			b.Format = kernel.Opt(binding.Format)
		}
		request.Bindings = append(request.Bindings, b)
	}
	if params.TimeoutSec != nil {
		request.TimeoutSec = kernel.Opt(*params.TimeoutSec)
	}
	response, err := c.vaults.Items.PerformOperation(ctx, key, kernel.VaultItemPerformOperationParams{IDOrName: vault, OfWebmcpInvoke: &request}, option.WithMaxRetries(0))
	if err != nil {
		return vaultWebMCPRequestError(err)
	}
	if response == nil {
		return fmt.Errorf("empty webmcp_invoke result; %s", vaultWebMCPUncertain)
	}
	var result vaultWebMCPResult
	if json.Unmarshal([]byte(response.RawJSON()), &result) != nil || result.Type != "webmcp_invoke" {
		return fmt.Errorf("invalid webmcp_invoke result; %s", vaultWebMCPUncertain)
	}
	switch result.Status {
	case "completed", "awaiting_submission", "canceled", "error", "unknown":
	default:
		return fmt.Errorf("invalid webmcp_invoke result; %s", vaultWebMCPUncertain)
	}
	if output == "json" {
		if err := printVaultJSON(result); err != nil {
			return err
		}
	} else if err := printVaultWebMCPResult(result); err != nil {
		return err
	}
	if result.Status != "completed" && result.Status != "awaiting_submission" {
		return vaultFillOutcomeError{status: result.Status}
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
	switch result.Status {
	case "completed":
		pterm.Println("The tool reported completion; this does not confirm the website accepted the action. Inspect the page.")
	case "awaiting_submission":
		pterm.Println("The tool populated a form with the supplied values without submitting it. Inspect the form and obtain any required confirmation, then submit it with 'kernel browsers playwright execute' or 'kernel browsers computer'; do not invoke the tool again.")
	case "canceled", "error":
		pterm.Printf("The tool reported %s and may have had side effects. Inspect the page before deciding on a new invocation; do not retry automatically.\n", result.Status)
	default:
		pterm.Println(vaultWebMCPUncertain)
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

func vaultWebMCPParamsFromFlags(cmd *cobra.Command) (*vaultWebMCPParams, error) {
	var params vaultWebMCPParams
	params.BrowserID, _ = cmd.Flags().GetString("browser-id")
	params.ToolRef, _ = cmd.Flags().GetString("tool-ref")
	params.PageURL, _ = cmd.Flags().GetString("page-url")
	input, _ := cmd.Flags().GetString("input")
	data := []byte(input)
	if cmd.Flags().Changed("input-file") {
		path, _ := cmd.Flags().GetString("input-file")
		var reader io.Reader = cmd.InOrStdin()
		if path != "-" {
			f, err := os.Open(path)
			if err != nil {
				return nil, fmt.Errorf("could not open --input-file")
			}
			defer f.Close()
			reader = f
		}
		const limit = 128 * 1024
		var err error
		if data, err = io.ReadAll(io.LimitReader(reader, limit+1)); err != nil || len(data) > limit {
			return nil, fmt.Errorf("could not read --input-file (maximum 128 KiB)")
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
		binding := vaultWebMCPBinding{Field: field, InputPath: path}
		if name, format, hasFormat := strings.Cut(field, ":"); hasFormat {
			if format == "" {
				return nil, fmt.Errorf("binding %d (--bind) format must be MM/YY or MM/YYYY", i+1)
			}
			binding.Field, binding.Format = name, format
		}
		params.Bindings = append(params.Bindings, binding)
	}
	if cmd.Flags().Changed("timeout-sec") {
		timeout, _ := cmd.Flags().GetInt64("timeout-sec")
		params.TimeoutSec = &timeout
	}
	if err := validateVaultWebMCPParams(&params, func(i int) string { return fmt.Sprintf("binding %d (--bind)", i+1) }); err != nil {
		return nil, err
	}
	return &params, nil
}
