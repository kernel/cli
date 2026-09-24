package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const webMCPCustomSourceMaxBytes = 8_000_000

var webMCPCustomNamespacePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
var webMCPCustomIDPattern = regexp.MustCompile(`^ct_[a-z][a-z0-9]{23}$`)

type BrowserWebMCPCustomToolsService interface {
	List(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.CustomToolsResponse, error)
	Add(ctx context.Context, idOrName string, body kernel.BrowserWebmcpCustomToolAddParams, opts ...option.RequestOption) (*kernel.CustomToolsResponse, error)
	Remove(ctx context.Context, id string, body kernel.BrowserWebmcpCustomToolRemoveParams, opts ...option.RequestOption) error
}

type BrowsersWebMCPCustomToolsCmd struct {
	tools BrowserWebMCPCustomToolsService
}

type BrowsersWebMCPCustomToolsListInput struct {
	Identifier string
	Output     string
}

type BrowsersWebMCPCustomToolsAddInput struct {
	Identifier              string
	Namespace               string
	Source                  string
	ForceOverwriteNamespace param.Opt[bool]
	Output                  string
}

func (b BrowsersWebMCPCustomToolsCmd) List(ctx context.Context, in BrowsersWebMCPCustomToolsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	res, err := b.tools.List(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return printWebMCPCustomTools(res, in.Output)
}

func (b BrowsersWebMCPCustomToolsCmd) Add(ctx context.Context, in BrowsersWebMCPCustomToolsAddInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if !webMCPCustomNamespacePattern.MatchString(in.Namespace) {
		return fmt.Errorf("invalid --namespace: use 1-128 letters, digits, underscores, dots, or hyphens")
	}
	if strings.TrimSpace(in.Source) == "" {
		return fmt.Errorf("custom tool source must not be empty")
	}
	if len(in.Source) > webMCPCustomSourceMaxBytes {
		return fmt.Errorf("custom tool source exceeds %d UTF-8 bytes", webMCPCustomSourceMaxBytes)
	}
	if !utf8.ValidString(in.Source) {
		return fmt.Errorf("custom tool source must be valid UTF-8")
	}
	// Retrying a lost response could register the same batch twice.
	res, err := b.tools.Add(ctx, in.Identifier, kernel.BrowserWebmcpCustomToolAddParams{
		AddRequest: kernel.AddRequestParam{
			Namespace: in.Namespace, Source: in.Source, ForceOverwriteNamespace: in.ForceOverwriteNamespace,
		},
	}, option.WithMaxRetries(0))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return printWebMCPCustomTools(res, in.Output)
}

func (b BrowsersWebMCPCustomToolsCmd) Remove(ctx context.Context, identifier, id string) error {
	if !webMCPCustomIDPattern.MatchString(id) {
		return fmt.Errorf("invalid custom tool ID: expected ct_ followed by a lowercase letter and 23 lowercase letters or digits")
	}
	if err := b.tools.Remove(ctx, id, kernel.BrowserWebmcpCustomToolRemoveParams{IDOrName: identifier}); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Removed custom WebMCP tool: %s\n", id)
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
	rows := pterm.TableData{{"ID", "Namespace", "Kind", "Name", "URL Patterns"}}
	for _, tool := range res.Tools {
		rows = append(rows, []string{tool.ID, tool.Namespace, tool.Kind, tool.Tool.Name, strings.Join(tool.Match.URLPatterns, ", ")})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func newBrowsersWebMCPCustomToolsCommand() *cobra.Command {
	root := &cobra.Command{Use: "custom-tools", Short: "Manage registered custom WebMCP tools"}
	list := &cobra.Command{Use: "list <id-or-name>", Short: "List registered custom tools, including tools not currently matching a page", Args: cobra.ExactArgs(1), RunE: runBrowsersWebMCPCustomToolsList}
	add := &cobra.Command{
		Use:   "add <id-or-name>",
		Short: "Register custom tools from a JavaScript source file or stdin",
		Long: "Register a namespaced batch of page-backed or CDP-backed custom tools.\n\n" +
			"Source must be a JavaScript expression evaluating to a non-empty array of tool " +
			"definitions with URL matchers, tool metadata, and execute functions (at most 8,000,000 UTF-8 bytes). " +
			"By default, existing tools are retained. --force-overwrite-namespace atomically replaces " +
			"all tools in that namespace; existing invocations continue. Registration is not automatically retried.",
		Args: cobra.ExactArgs(1),
		RunE: runBrowsersWebMCPCustomToolsAdd,
	}
	add.Flags().String("namespace", "", "Tool namespace (1-128 letters, digits, underscores, dots, or hyphens)")
	add.Flags().String("source-file", "", "Path to JavaScript source (use '-' for stdin)")
	add.Flags().Bool("force-overwrite-namespace", false, "Atomically replace all existing tools in this namespace")
	_ = add.MarkFlagRequired("namespace")
	_ = add.MarkFlagRequired("source-file")
	for _, cmd := range []*cobra.Command{list, add} {
		addJSONOutputFlag(cmd)
		cmd.Flags().Bool("json", false, "Output the raw API response as JSON (alias for --output json)")
	}
	remove := &cobra.Command{
		Use: "remove <id-or-name> <tool-id>", Short: "Remove a registered custom tool by ID without canceling active invocations",
		Args: cobra.ExactArgs(2), RunE: runBrowsersWebMCPCustomToolsRemove,
	}
	root.AddCommand(list, add, remove)
	return root
}

func webMCPCustomToolsOutput(cmd *cobra.Command) (string, error) {
	output, _ := cmd.Flags().GetString("output")
	if err := validateJSONOutput(output); err != nil {
		return "", err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		output = "json"
	}
	return output, nil
}

func runBrowsersWebMCPCustomToolsList(cmd *cobra.Command, args []string) error {
	output, err := webMCPCustomToolsOutput(cmd)
	if err != nil {
		return err
	}
	client := getKernelClient(cmd)
	b := BrowsersWebMCPCustomToolsCmd{tools: &client.Browsers.Webmcp.CustomTools}
	return b.List(cmd.Context(), BrowsersWebMCPCustomToolsListInput{Identifier: args[0], Output: output})
}

func runBrowsersWebMCPCustomToolsAdd(cmd *cobra.Command, args []string) error {
	output, err := webMCPCustomToolsOutput(cmd)
	if err != nil {
		return err
	}
	path, _ := cmd.Flags().GetString("source-file")
	reader := cmd.InOrStdin()
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to read source file: %w", err)
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, webMCPCustomSourceMaxBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}
	namespace, _ := cmd.Flags().GetString("namespace")
	var force param.Opt[bool]
	if cmd.Flags().Changed("force-overwrite-namespace") {
		value, _ := cmd.Flags().GetBool("force-overwrite-namespace")
		force = kernel.Opt(value)
	}
	client := getKernelClient(cmd)
	b := BrowsersWebMCPCustomToolsCmd{tools: &client.Browsers.Webmcp.CustomTools}
	return b.Add(cmd.Context(), BrowsersWebMCPCustomToolsAddInput{
		Identifier: args[0], Namespace: namespace, Source: string(data), ForceOverwriteNamespace: force, Output: output,
	})
}

func runBrowsersWebMCPCustomToolsRemove(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	b := BrowsersWebMCPCustomToolsCmd{tools: &client.Browsers.Webmcp.CustomTools}
	return b.Remove(cmd.Context(), args[0], args[1])
}
