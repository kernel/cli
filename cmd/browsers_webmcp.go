package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// BrowserWebMCPService defines the subset we use for native page tools.
type BrowserWebMCPService interface {
	ListTools(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.ToolsResponse, error)
	InvokeTool(ctx context.Context, idOrName string, body kernel.BrowserWebmcpInvokeToolParams, opts ...option.RequestOption) (*kernel.InvocationResult, error)
}

type BrowsersWebMCPListInput struct {
	Identifier string
	Output     string
}

type BrowsersWebMCPInvokeInput struct {
	Identifier string
	ToolRef    string
	Input      string
	TimeoutSec param.Opt[int64]
}

func (b BrowsersCmd) WebMCPList(ctx context.Context, in BrowsersWebMCPListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	res, err := b.webmcp.ListTools(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(res)
	}
	if len(res.Tools) == 0 {
		pterm.Info.Println("No WebMCP tools found")
		return nil
	}
	rows := pterm.TableData{{"Name", "Tool Ref", "Page URL", "Tab ID", "Read Only"}}
	for _, tool := range res.Tools {
		readOnly := "-"
		if tool.Annotations.JSON.ReadOnly.Valid() {
			readOnly = strconv.FormatBool(tool.Annotations.ReadOnly)
		}
		rows = append(rows, []string{tool.Name, tool.ToolRef, tool.Source.PageURL, strconv.FormatInt(tool.Source.TabID, 10), readOnly})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func (b BrowsersCmd) WebMCPInvoke(ctx context.Context, in BrowsersWebMCPInvokeInput) error {
	if strings.TrimSpace(in.ToolRef) == "" {
		return fmt.Errorf("missing --tool-ref value")
	}
	if in.TimeoutSec.Valid() && in.TimeoutSec.Value <= 0 {
		return fmt.Errorf("invalid --timeout-sec value: must be greater than zero")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(in.Input), &fields); err != nil {
		return fmt.Errorf("invalid input: expected a JSON object: %w", err)
	}
	if fields == nil {
		return fmt.Errorf("invalid input: expected a JSON object")
	}
	input := make(map[string]any, len(fields))
	for key, value := range fields {
		input[key] = value
	}
	params := kernel.BrowserWebmcpInvokeToolParams{InvokeRequest: kernel.InvokeRequestParam{
		ToolRef: in.ToolRef, Input: input, TimeoutSec: in.TimeoutSec,
	}}
	// A lost response can hide completed side effects, so never retry an invocation.
	res, err := b.webmcp.InvokeTool(ctx, in.Identifier, params, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	switch res.Status {
	case kernel.InvocationResultStatusCompleted:
	case kernel.InvocationResultStatusAwaitingSubmission:
		// A populated form is a result, not a failure. Re-invoking would refill the
		// same fields, so point the caller at submitting the form they already have.
		pterm.Warning.Printfln("WebMCP invocation %s populated a form without submitting it. Inspect the form and obtain any required confirmation, then submit it with 'kernel browsers playwright execute' or 'kernel browsers computer' rather than invoking the tool again.", res.InvocationID)
	default:
		return fmt.Errorf("WebMCP invocation %s: %s: %s", res.InvocationID, res.Status, res.ErrorText)
	}
	// Preserve page-provided JSON numbers rather than re-encoding SDK float64 values.
	var result struct {
		Output json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal([]byte(res.RawJSON()), &result); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result.Output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func newBrowsersWebMCPCommand() *cobra.Command {
	root := &cobra.Command{Use: "webmcp", Short: "Discover and invoke native page tools"}
	list := &cobra.Command{Use: "list <id-or-name>", Short: "List WebMCP tools across browser tabs and frames", Args: cobra.ExactArgs(1), RunE: runBrowsersWebMCPList}
	addJSONOutputFlag(list)
	list.Flags().Bool("json", false, "Output the raw API response as JSON (alias for --output json)")
	invoke := &cobra.Command{
		Use:   "invoke <id-or-name>",
		Short: "Invoke a WebMCP tool without automatic retries",
		Long: "Invoke a WebMCP tool without automatic retries.\n\n" +
			"Most tools wait for a terminal result, including across navigation. A tool " +
			"backed by a declarative form that does not autosubmit instead returns once " +
			"its fields are populated: inspect the form, obtain any required confirmation, " +
			"then submit it through Playwright or computer interaction without invoking " +
			"the tool again.",
		Args: cobra.ExactArgs(1),
		RunE: runBrowsersWebMCPInvoke,
	}
	invoke.Flags().String("tool-ref", "", "Opaque tool reference from webmcp list")
	_ = invoke.MarkFlagRequired("tool-ref")
	invoke.Flags().String("input", "", "Tool input as a JSON object")
	invoke.Flags().String("input-file", "", "Path to a JSON object file (use '-' for stdin)")
	invoke.MarkFlagsOneRequired("input", "input-file")
	invoke.MarkFlagsMutuallyExclusive("input", "input-file")
	invoke.Flags().Int64("timeout-sec", 0, "Maximum execution time in seconds (default per server)")
	root.AddCommand(list, invoke)
	return root
}

func runBrowsersWebMCPList(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	if err := validateJSONOutput(output); err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		output = "json"
	}
	client := getKernelClient(cmd)
	b := BrowsersCmd{webmcp: &client.Browsers.Webmcp}
	return b.WebMCPList(cmd.Context(), BrowsersWebMCPListInput{Identifier: args[0], Output: output})
}

func runBrowsersWebMCPInvoke(cmd *cobra.Command, args []string) error {
	toolRef, _ := cmd.Flags().GetString("tool-ref")
	input, _ := cmd.Flags().GetString("input")
	if cmd.Flags().Changed("input-file") {
		path, _ := cmd.Flags().GetString("input-file")
		var data []byte
		var err error
		if path == "-" {
			data, err = io.ReadAll(cmd.InOrStdin())
		} else {
			data, err = os.ReadFile(path)
		}
		if err != nil {
			return fmt.Errorf("failed to read input file: %w", err)
		}
		input = string(data)
	}
	var timeout param.Opt[int64]
	if cmd.Flags().Changed("timeout-sec") {
		value, _ := cmd.Flags().GetInt64("timeout-sec")
		timeout = kernel.Opt(value)
	}
	client := getKernelClient(cmd)
	b := BrowsersCmd{webmcp: &client.Browsers.Webmcp}
	return b.WebMCPInvoke(cmd.Context(), BrowsersWebMCPInvokeInput{Identifier: args[0], ToolRef: toolRef, Input: input, TimeoutSec: timeout})
}
