package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

// BrowserPlaywrightService defines the subset we use for Playwright execution.
type BrowserPlaywrightService interface {
	Execute(ctx context.Context, idOrName string, body kernel.BrowserPlaywrightExecuteParams, opts ...option.RequestOption) (res *kernel.BrowserPlaywrightExecuteResponse, err error)
}

// BrowserPlaywrightExecutorService defines the subset we use for Playwright executors.
type BrowserPlaywrightExecutorService interface {
	List(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.ExecutorList, error)
	Delete(ctx context.Context, name string, params kernel.BrowserPlaywrightExecutorDeleteParams, opts ...option.RequestOption) error
}

const playwrightExecutorsLong = `Every Playwright call runs in an executor: a dedicated process with its own
browser connection. Calls on different executors run concurrently; calls on
one executor run one at a time. A timeout or crash in one executor does not
affect the others.

The 'default' executor always exists and binds 'page' to the active tab. Any
other name is a named executor: the first call with a new name creates it, and
it owns a background tab in the default browser context that 'page' is bound
to on every later call. Executor code can still reach other tabs through
'context' and 'browser'. Use named executors to drive several tabs of one
browser in parallel.

A browser can have at most 8 named executors (the default executor does not
count); a call that would create another fails with HTTP 409. Named executors
are not removed automatically while the browser runs, so delete the ones you no
longer need.`

type BrowsersPlaywrightExecuteInput struct {
	Identifier string
	Code       string
	Executor   string
	Timeout    int64
	Output     string
}

type BrowsersPlaywrightExecutorsListInput struct {
	Identifier string
	Output     string
}

type BrowsersPlaywrightExecutorsDeleteInput struct {
	Identifier string
	Name       string
	CloseTab   param.Opt[bool]
}

func (b BrowsersCmd) PlaywrightExecute(ctx context.Context, in BrowsersPlaywrightExecuteInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	if b.playwright == nil {
		pterm.Error.Println("playwright service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier, kernel.BrowserGetParams{})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserPlaywrightExecuteParams{Code: in.Code}
	if in.Executor != "" {
		params.Executor = kernel.Opt(in.Executor)
	}
	if in.Timeout > 0 {
		params.TimeoutSec = kernel.Opt(in.Timeout)
	}
	res, err := b.playwright.Execute(ctx, br.SessionID, params)
	if err != nil {
		return playwrightExecuteError(err, in)
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(res)
	}

	rows := pterm.TableData{{"Property", "Value"}, {"Success", fmt.Sprintf("%t", res.Success)}}
	if res.JSON.Tab.Valid() {
		rows = append(rows, []string{"Tab Target ID", res.Tab.TargetID}, []string{"Tab Created", fmt.Sprintf("%t", res.Tab.Created)})
	}
	PrintTableNoPad(rows, true)

	if res.Stdout != "" {
		pterm.Info.Println("stdout:")
		fmt.Println(res.Stdout)
	}
	if res.Stderr != "" {
		pterm.Info.Println("stderr:")
		fmt.Fprintln(os.Stderr, res.Stderr)
	}
	if res.Result != nil {
		bs, err := json.MarshalIndent(res.Result, "", "  ")
		if err == nil {
			pterm.Info.Println("result:")
			fmt.Println(string(bs))
		}
	}
	if !res.Success && res.Error != "" {
		pterm.Error.Printf("error: %s\n", res.Error)
	}
	return nil
}

// playwrightExecuteError points at the executors commands when a named
// executor call is rejected with 409 because the browser is at its executor limit.
func playwrightExecuteError(err error, in BrowsersPlaywrightExecuteInput) error {
	var apiErr *kernel.Error
	if in.Executor == "" || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		return util.CleanedUpSdkError{Err: err}
	}
	var body struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(apiErr.RawJSON()), &body) != nil || body.Message == "" {
		return util.CleanedUpSdkError{Err: err}
	}
	// Not %w: the root handler re-renders any wrapped *kernel.Error as "code: message", dropping the hint.
	return fmt.Errorf("%s. See them with 'kernel browsers playwright executors list %s' and free one with 'kernel browsers playwright executors delete %s <executor>'",
		strings.TrimSuffix(body.Message, "."), in.Identifier, in.Identifier)
}

func (b BrowsersCmd) PlaywrightExecutorsList(ctx context.Context, in BrowsersPlaywrightExecutorsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	res, err := b.playwrightExecutors.List(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(res)
	}
	if len(res.Executors) == 0 {
		pterm.Info.Println("No Playwright executors found")
		return nil
	}
	rows := pterm.TableData{{"Name", "Busy", "Created At", "Last Used At", "Target ID", "URL"}}
	for _, e := range res.Executors {
		rows = append(rows, []string{e.Name, strconv.FormatBool(e.Busy), util.FormatLocal(e.CreatedAt), util.FormatLocal(e.LastUsedAt), util.OrDash(e.TargetID), util.OrDash(e.URL)})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func (b BrowsersCmd) PlaywrightExecutorsDelete(ctx context.Context, in BrowsersPlaywrightExecutorsDeleteInput) error {
	params := kernel.BrowserPlaywrightExecutorDeleteParams{IDOrName: in.Identifier, CloseTab: in.CloseTab}
	if err := b.playwrightExecutors.Delete(ctx, in.Name, params); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Name == "default" {
		pterm.Success.Println("Restarted the default Playwright executor")
	} else {
		pterm.Success.Printf("Deleted Playwright executor %q\n", in.Name)
	}
	return nil
}

func newBrowsersPlaywrightCommand() *cobra.Command {
	root := &cobra.Command{Use: "playwright", Short: "Playwright operations"}
	execute := &cobra.Command{
		Use:   "execute <id> [code]",
		Short: "Execute Playwright/TypeScript code against the browser",
		Long: "Execute Playwright/TypeScript code against the browser.\n\n" +
			"Code may be passed as an argument or piped via stdin. It has access to 'page', " +
			"'context', and 'browser', and may return a value.\n\n" + playwrightExecutorsLong,
		Args: cobra.MinimumNArgs(1),
		RunE: runBrowsersPlaywrightExecute,
	}
	execute.Flags().String("executor", "", "Named executor to run the call in; each owns its own tab (default: the active tab)")
	execute.Flags().Int64("timeout", 0, "Maximum execution time in seconds (default per server)")
	addJSONOutputFlag(execute)
	root.AddCommand(execute, newBrowsersPlaywrightExecutorsCommand())
	return root
}

func newBrowsersPlaywrightExecutorsCommand() *cobra.Command {
	root := &cobra.Command{Use: "executors", Short: "List and delete the Playwright executors of a browser", Long: playwrightExecutorsLong}
	list := &cobra.Command{
		Use:   "list <id-or-name>",
		Short: "List the browser's Playwright executors",
		Long: "List the browser's Playwright executors, the default executor first. Each entry reports " +
			"whether a call is running on it and, for named executors, the target ID and URL of the tab it owns.",
		Args: cobra.ExactArgs(1),
		RunE: runBrowsersPlaywrightExecutorsList,
	}
	addJSONOutputFlag(list)
	del := &cobra.Command{
		Use:   "delete <id-or-name> <executor>",
		Short: "Delete a Playwright executor and, by default, close its tab",
		Long: "Stop a Playwright executor's process and, by default, close the tab it owns. A call running " +
			"on it fails with an error saying the executor was deleted; the name can be reused afterwards.\n\n" +
			"Deleting 'default' restarts it instead of removing it: queued and later calls run on a new " +
			"process. It owns no tab, so --close-tab has no effect on it.",
		Args: cobra.ExactArgs(2),
		RunE: runBrowsersPlaywrightExecutorsDelete,
	}
	del.Flags().Bool("close-tab", true, "Close the tab owned by the executor")
	root.AddCommand(list, del)
	return root
}

func runBrowsersPlaywrightExecute(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers

	var code string
	if len(args) >= 2 {
		code = strings.Join(args[1:], " ")
	} else {
		// Read code from stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			pterm.Error.Println("no code provided. Provide code as an argument or pipe via stdin")
			return nil
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			pterm.Error.Printf("failed to read stdin: %v\n", err)
			return nil
		}
		code = string(data)
	}
	executor, _ := cmd.Flags().GetString("executor")
	timeout, _ := cmd.Flags().GetInt64("timeout")
	output, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc, playwright: &svc.Playwright}
	return b.PlaywrightExecute(cmd.Context(), BrowsersPlaywrightExecuteInput{Identifier: args[0], Code: strings.TrimSpace(code), Executor: executor, Timeout: timeout, Output: output})
}

func runBrowsersPlaywrightExecutorsList(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	b := BrowsersCmd{playwrightExecutors: &client.Browsers.Playwright.Executors}
	return b.PlaywrightExecutorsList(cmd.Context(), BrowsersPlaywrightExecutorsListInput{Identifier: args[0], Output: output})
}

func runBrowsersPlaywrightExecutorsDelete(cmd *cobra.Command, args []string) error {
	var closeTab param.Opt[bool]
	if cmd.Flags().Changed("close-tab") {
		value, _ := cmd.Flags().GetBool("close-tab")
		closeTab = kernel.Opt(value)
	}
	client := getKernelClient(cmd)
	b := BrowsersCmd{playwrightExecutors: &client.Browsers.Playwright.Executors}
	return b.PlaywrightExecutorsDelete(cmd.Context(), BrowsersPlaywrightExecutorsDeleteInput{Identifier: args[0], Name: args[1], CloseTab: closeTab})
}
