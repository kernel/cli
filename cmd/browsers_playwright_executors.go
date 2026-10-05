package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

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

type BrowsersPlaywrightExecutorsListInput struct {
	Identifier string
	Output     string
}

type BrowsersPlaywrightExecutorsDeleteInput struct {
	Identifier string
	Name       string
	CloseTab   param.Opt[bool]
}

func (b BrowsersCmd) PlaywrightExecutorsList(ctx context.Context, in BrowsersPlaywrightExecutorsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	res, err := b.executors.List(ctx, in.Identifier)
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
	if err := b.executors.Delete(ctx, in.Name, params); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Name == "default" {
		pterm.Success.Println("Restarted the default Playwright executor")
	} else {
		pterm.Success.Printf("Deleted Playwright executor %q\n", in.Name)
	}
	return nil
}

// playwrightExecuteError turns the 409 returned when a call would exceed the
// named executor limit into an error that names the current executors.
func playwrightExecuteError(err error) error {
	var apiErr *kernel.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		return util.CleanedUpSdkError{Err: err}
	}
	var body struct {
		Message   string            `json:"message"`
		Executors []kernel.Executor `json:"executors"`
	}
	if json.Unmarshal([]byte(apiErr.RawJSON()), &body) != nil || body.Message == "" {
		return util.CleanedUpSdkError{Err: err}
	}
	var sb strings.Builder
	sb.WriteString(body.Message)
	if len(body.Executors) > 0 {
		sb.WriteString("\nCurrent executors:")
		for _, e := range body.Executors {
			fmt.Fprintf(&sb, "\n  %s", e.Name)
			if e.Busy {
				sb.WriteString(" (busy)")
			}
			if e.URL != "" {
				fmt.Fprintf(&sb, " %s", e.URL)
			}
		}
	}
	sb.WriteString("\nDelete one with 'kernel browsers playwright executors delete <id-or-name> <executor>'")
	return errors.New(sb.String())
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
	execute.Flags().String("executor", "", "Executor to run the call in. Calls on different executors run concurrently in separate tabs of the same browser; calls on one executor run one at a time. Omit to use the always-present 'default' executor bound to the active tab. Any other name creates a named executor on first use that owns a background tab 'page' is bound to. At most 8 named executors per browser (409 when exceeded)")
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

func runBrowsersPlaywrightExecutorsList(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	b := BrowsersCmd{executors: &client.Browsers.Playwright.Executors}
	return b.PlaywrightExecutorsList(cmd.Context(), BrowsersPlaywrightExecutorsListInput{Identifier: args[0], Output: output})
}

func runBrowsersPlaywrightExecutorsDelete(cmd *cobra.Command, args []string) error {
	var closeTab param.Opt[bool]
	if cmd.Flags().Changed("close-tab") {
		value, _ := cmd.Flags().GetBool("close-tab")
		closeTab = kernel.Opt(value)
	}
	client := getKernelClient(cmd)
	b := BrowsersCmd{executors: &client.Browsers.Playwright.Executors}
	return b.PlaywrightExecutorsDelete(cmd.Context(), BrowsersPlaywrightExecutorsDeleteInput{Identifier: args[0], Name: args[1], CloseTab: closeTab})
}
