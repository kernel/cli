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
	ListTools(ctx context.Context, idOrName string, query kernel.BrowserWebmcpListToolsParams, opts ...option.RequestOption) (*kernel.ToolsResponse, error)
	InvokeTool(ctx context.Context, idOrName string, body kernel.BrowserWebmcpInvokeToolParams, opts ...option.RequestOption) (*kernel.InvocationResult, error)
}

// BrowserWebMCPCustomToolsService defines the subset we use for custom WebMCP tools.
type BrowserWebMCPCustomToolsService interface {
	List(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.CustomToolsResponse, error)
	Add(ctx context.Context, idOrName string, body kernel.BrowserWebmcpCustomToolAddParams, opts ...option.RequestOption) (*kernel.CustomToolsResponse, error)
	Remove(ctx context.Context, id string, body kernel.BrowserWebmcpCustomToolRemoveParams, opts ...option.RequestOption) error
}

type BrowsersWebMCPListInput struct {
	Identifier    string
	Output        string
	ExcludeCustom bool
}

type BrowsersWebMCPCustomToolsListInput struct {
	Identifier string
	Output     string
}

type BrowsersWebMCPCustomToolsAddInput struct {
	Identifier              string
	Namespace               string
	Source                  string
	ForceOverwriteNamespace bool
	Output                  string
}

type BrowsersWebMCPCustomToolsRemoveInput struct {
	Identifier string
	ToolID     string
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
	params := kernel.BrowserWebmcpListToolsParams{}
	if in.ExcludeCustom {
		params.ExcludeCustom = kernel.Opt(true)
	}
	res, err := b.webmcp.ListTools(ctx, in.Identifier, params)
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
	rows := pterm.TableData{{"Name", "Tool Ref", "Page URL", "Tab ID", "Source", "Read Only"}}
	for _, tool := range res.Tools {
		source := "page"
		if tool.Source.JSON.Custom.Valid() {
			source = "custom:" + tool.Source.Custom.Namespace
		}
		rows = append(rows, []string{tool.Tool.Name, tool.ToolRef, tool.Source.PageURL, strconv.FormatInt(tool.Source.TabID, 10), source, webMCPReadOnly(tool.Tool.Annotations)})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func webMCPReadOnly(annotations kernel.ToolAnnotations) string {
	if annotations.JSON.ReadOnlyHint.Valid() {
		return strconv.FormatBool(annotations.ReadOnlyHint)
	}
	return "-"
}

func (b BrowsersCmd) WebMCPCustomToolsList(ctx context.Context, in BrowsersWebMCPCustomToolsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	res, err := b.webmcpCustomTools.List(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return printWebMCPCustomTools(res, in.Output)
}

func (b BrowsersCmd) WebMCPCustomToolsAdd(ctx context.Context, in BrowsersWebMCPCustomToolsAddInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if strings.TrimSpace(in.Namespace) == "" {
		return fmt.Errorf("missing --namespace value")
	}
	if strings.TrimSpace(in.Source) == "" {
		return fmt.Errorf("missing custom tool source")
	}
	req := kernel.AddRequestParam{Namespace: in.Namespace, Source: in.Source}
	if in.ForceOverwriteNamespace {
		req.ForceOverwriteNamespace = kernel.Opt(true)
	}
	res, err := b.webmcpCustomTools.Add(ctx, in.Identifier, kernel.BrowserWebmcpCustomToolAddParams{AddRequest: req})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output != "json" {
		pterm.Success.Printfln("Added %d custom WebMCP tool(s) to namespace %s", len(res.Tools), in.Namespace)
	}
	return printWebMCPCustomTools(res, in.Output)
}

func (b BrowsersCmd) WebMCPCustomToolsRemove(ctx context.Context, in BrowsersWebMCPCustomToolsRemoveInput) error {
	if strings.TrimSpace(in.ToolID) == "" {
		return fmt.Errorf("custom tool ID must not be empty")
	}
	if err := b.webmcpCustomTools.Remove(ctx, in.ToolID, kernel.BrowserWebmcpCustomToolRemoveParams{IDOrName: in.Identifier}); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printfln("Removed custom WebMCP tool %s", in.ToolID)
	return nil
}

func printWebMCPCustomTools(res *kernel.CustomToolsResponse, output string) error {
	if output == "json" {
		return util.PrintPrettyJSON(res)
	}
	if len(res.Tools) == 0 {
		pterm.Info.Println("No custom WebMCP tools found")
		return nil
	}
	rows := pterm.TableData{{"ID", "Namespace", "Name", "Kind", "URL Patterns", "Read Only"}}
	for _, tool := range res.Tools {
		rows = append(rows, []string{tool.ID, tool.Namespace, tool.Tool.Name, tool.Kind, strings.Join(tool.Match.URLPatterns, ", "), webMCPReadOnly(tool.Tool.Annotations)})
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
	list := &cobra.Command{
		Use:   "list <id-or-name>",
		Short: "List WebMCP tools across browser tabs and frames",
		Long: "List native and custom WebMCP tools available across every open tab and embedded frame.\n\n" +
			"Each tool includes an opaque tool_ref for invoking that exact live registration. " +
			"Custom tools include their generated ID and namespace in source. Use --exclude-custom " +
			"to return only page-provided tools.",
		Args: cobra.ExactArgs(1),
		RunE: runBrowsersWebMCPList,
	}
	addJSONOutputFlag(list)
	list.Flags().Bool("json", false, "Output the raw API response as JSON (alias for --output json)")
	list.Flags().Bool("exclude-custom", false, "Exclude custom tools and return only page-provided tools")
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
	root.AddCommand(list, invoke, newBrowsersWebMCPCustomToolsCommand())
	return root
}

func newBrowsersWebMCPCustomToolsCommand() *cobra.Command {
	root := &cobra.Command{Use: "custom-tools", Short: "Manage custom WebMCP tools"}
	list := &cobra.Command{
		Use:   "list <id-or-name>",
		Short: "List custom WebMCP tools",
		Long:  "Returns every registered custom tool with its generated ID, namespace, matcher, and MCP tool metadata.",
		Args:  cobra.ExactArgs(1),
		RunE:  runBrowsersWebMCPCustomToolsList,
	}
	addJSONOutputFlag(list)
	add := &cobra.Command{
		Use:   "add <id-or-name>",
		Short: "Add a namespaced batch of custom WebMCP tools",
		Long: "Add a namespaced batch of custom tools. A custom tool can be page-backed or CDP-backed.\n\n" +
			"Page-backed tools execute in the page via JavaScript. CDP-backed tools execute via CDP and " +
			"can use all browser REPL tools. The source must be a JavaScript expression that evaluates to a " +
			"non-empty array of definitions with URL matchers, tool metadata (including an optional output " +
			"schema), and execute functions. The batch is added atomically.\n\n" +
			"To update one tool, list the tools, remove its ID, and add its replacement. Use " +
			"--force-overwrite-namespace to atomically replace every existing tool in the namespace.",
		Args: cobra.ExactArgs(1),
		RunE: runBrowsersWebMCPCustomToolsAdd,
	}
	add.Flags().String("namespace", "", "Namespace for the batch of custom tools")
	_ = add.MarkFlagRequired("namespace")
	add.Flags().String("source", "", "JavaScript expression that evaluates to an array of custom tool definitions")
	add.Flags().String("source-file", "", "Path to a JavaScript source file (use '-' for stdin)")
	add.MarkFlagsOneRequired("source", "source-file")
	add.MarkFlagsMutuallyExclusive("source", "source-file")
	add.Flags().Bool("force-overwrite-namespace", false, "Atomically replace all existing tools in this namespace with this batch")
	addJSONOutputFlag(add)
	remove := &cobra.Command{
		Use:   "remove <id-or-name> <tool-id>",
		Short: "Remove a custom WebMCP tool by ID",
		Long:  "Removes one custom tool by generated ID. An invocation already in progress is not canceled.",
		Args:  cobra.ExactArgs(2),
		RunE:  runBrowsersWebMCPCustomToolsRemove,
	}
	root.AddCommand(list, add, remove)
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
	excludeCustom, _ := cmd.Flags().GetBool("exclude-custom")
	return b.WebMCPList(cmd.Context(), BrowsersWebMCPListInput{Identifier: args[0], Output: output, ExcludeCustom: excludeCustom})
}

func runBrowsersWebMCPCustomToolsList(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	b := BrowsersCmd{webmcpCustomTools: &client.Browsers.Webmcp.CustomTools}
	return b.WebMCPCustomToolsList(cmd.Context(), BrowsersWebMCPCustomToolsListInput{Identifier: args[0], Output: output})
}

func runBrowsersWebMCPCustomToolsAdd(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	if err := validateJSONOutput(output); err != nil {
		return err
	}
	namespace, _ := cmd.Flags().GetString("namespace")
	source, _ := cmd.Flags().GetString("source")
	if cmd.Flags().Changed("source-file") {
		path, _ := cmd.Flags().GetString("source-file")
		var data []byte
		var err error
		if path == "-" {
			data, err = io.ReadAll(cmd.InOrStdin())
		} else {
			data, err = os.ReadFile(path)
		}
		if err != nil {
			return fmt.Errorf("failed to read source file: %w", err)
		}
		source = string(data)
	}
	force, _ := cmd.Flags().GetBool("force-overwrite-namespace")
	client := getKernelClient(cmd)
	b := BrowsersCmd{webmcpCustomTools: &client.Browsers.Webmcp.CustomTools}
	return b.WebMCPCustomToolsAdd(cmd.Context(), BrowsersWebMCPCustomToolsAddInput{
		Identifier: args[0], Namespace: namespace, Source: source, ForceOverwriteNamespace: force, Output: output,
	})
}

func runBrowsersWebMCPCustomToolsRemove(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	b := BrowsersCmd{webmcpCustomTools: &client.Browsers.Webmcp.CustomTools}
	return b.WebMCPCustomToolsRemove(cmd.Context(), BrowsersWebMCPCustomToolsRemoveInput{Identifier: args[0], ToolID: args[1]})
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
