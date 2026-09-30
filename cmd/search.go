package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/spf13/cobra"
)

func newSearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search the web and return the full result as JSON",
		Long: "Search the web using automatic routing or a pinned provider. Returns the full JSON resource, including warnings, attempts, usage, and expiry.\n\n" +
			"Use --request with a JSON object for advanced searches. The request schema includes query (required), max_results, strategy (auto, pinned, or fallback), content, include_raw, include_domains, exclude_domains, date and locale filters, strict_params, and provider-specific options. Strategy objects accept provider configuration; fallback strategies also accept fallback_on. The API validates provider-specific and advanced fields.\n\n" +
			"Requires Search API access for your organization.",
		Example: "  kernel search 'browser automation' --provider exa --max-results 5\n  kernel search --request '{\"query\":\"browser automation\",\"include_domains\":[\"example.com\"],\"strict_params\":true}'\n  kernel search get srch_123\n  kernel search providers --slug exa\n  kernel search contents srch_123 --limit 3 --content-source browser",
		Args:    cobra.MaximumNArgs(1),
		RunE:    runSearch,
	}
	cmd.Flags().String("provider", "", "Pin a provider slug (default: automatic routing)")
	cmd.Flags().Int("max-results", 10, "Requested result count (1–100; subject to provider cap)")
	cmd.Flags().String("request", "", "Complete Search API request as a JSON object; cannot be combined with query flags")
	cmd.MarkFlagsMutuallyExclusive("request", "provider")
	cmd.MarkFlagsMutuallyExclusive("request", "max-results")
	get := &cobra.Command{Use: "get <id>", Short: "Retrieve a retained search as JSON without running it again", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(args[0]) == "" {
			return fmt.Errorf("search ID must not be empty")
		}
		return executeSearchRequest(cmd, http.MethodGet, "search/"+url.PathEscape(args[0]), nil)
	}}
	providers := &cobra.Command{Use: "providers", Short: "List configured providers and their capabilities as JSON", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		path := "search/providers"
		if cmd.Flags().Changed("slug") {
			slug, _ := cmd.Flags().GetString("slug")
			path += "?" + url.Values{"slug": {slug}}.Encode()
		}
		return executeSearchRequest(cmd, http.MethodGet, path, nil)
	}}
	providers.Flags().String("slug", "", "Filter by provider slug")
	contents := &cobra.Command{
		Use:   "contents <id>",
		Short: "Fetch content for selected results of a retained search as JSON",
		Long: "Retrieves selected results from a retained search. Provide exactly one of --result-ids or --limit; the latter fetches the top results. Content defaults to source auto. " +
			"Responses preserve --result-ids order and include one outcome per selected result, including timeout entries for work unfinished at the overall deadline. " +
			"Browser retrievals run sequentially in result order and are billed like any other browser.",
		Example: "  kernel search contents srch_123 --limit 3\n  kernel search contents srch_123 --result-ids res_1,res_2 --content-source browser --content-browser-mode render",
		Args:    cobra.ExactArgs(1),
		RunE:    runSearchContents,
	}
	contents.Flags().StringSlice("result-ids", nil, "Result IDs from the retained search, in desired response order (mutually exclusive with --limit)")
	contents.Flags().Int("limit", 0, "Number of results to fetch starting from rank 1 (1–100; mutually exclusive with --result-ids)")
	contents.Flags().Int("timeout-ms", 0, "Overall deadline across all selected results in milliseconds (1000–120000; default 60000)")
	contents.Flags().String("content-source", "", "Content source: auto, provider, or browser (default auto)")
	contents.Flags().String("content-format", "", "Content format: markdown or text")
	contents.Flags().Int("content-max-chars", 0, "Per-result Unicode character limit after extraction")
	contents.Flags().Int("content-max-age-hours", 0, "For source auto, maximum age of retained provider content; 0 fetches every result through a browser")
	contents.Flags().Int("content-timeout-ms", 0, "Per-result deadline in milliseconds, including capacity acquisition, retrieval, and extraction")
	contents.Flags().String("content-browser-id", "", "Existing browser session to retrieve through (requires source auto or browser)")
	contents.Flags().String("content-browser-mode", "", "Browser retrieval mode: curl or render")
	contents.MarkFlagsMutuallyExclusive("result-ids", "limit")
	contents.MarkFlagsOneRequired("result-ids", "limit")
	cmd.AddCommand(get, providers, contents)
	return cmd
}

func runSearchContents(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("search ID must not be empty")
	}
	flags := cmd.Flags()
	var request kernel.FetchRequestParam
	if flags.Changed("result-ids") {
		ids, _ := flags.GetStringSlice("result-ids")
		if len(ids) == 0 || len(ids) > 100 {
			return fmt.Errorf("--result-ids must contain 1–100 IDs")
		}
		for _, id := range ids {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("--result-ids must not contain empty IDs")
			}
		}
		request.ResultIDs = ids
	}
	if flags.Changed("limit") {
		limit, _ := flags.GetInt("limit")
		if limit < 1 || limit > 100 {
			return fmt.Errorf("--limit must be between 1 and 100")
		}
		request.Limit = kernel.Int(int64(limit))
	}
	if flags.Changed("timeout-ms") {
		timeout, _ := flags.GetInt("timeout-ms")
		if timeout < 1000 || timeout > 120000 {
			return fmt.Errorf("--timeout-ms must be between 1000 and 120000")
		}
		request.TimeoutMs = kernel.Int(int64(timeout))
	}
	enumFlag := func(name string, allowed ...string) (string, error) {
		value, _ := flags.GetString(name)
		if flags.Changed(name) && !slices.Contains(allowed, value) {
			return "", fmt.Errorf("--%s must be one of: %s", name, strings.Join(allowed, ", "))
		}
		return value, nil
	}
	var err error
	content := &request.Content
	if content.Source, err = enumFlag("content-source", "auto", "provider", "browser"); err != nil {
		return err
	}
	if content.Format, err = enumFlag("content-format", "markdown", "text"); err != nil {
		return err
	}
	if content.Browser.Mode, err = enumFlag("content-browser-mode", "curl", "render"); err != nil {
		return err
	}
	for name, target := range map[string]*param.Opt[int64]{
		"content-max-chars":     &content.MaxChars,
		"content-max-age-hours": &content.MaxAgeHours,
		"content-timeout-ms":    &content.TimeoutMs,
	} {
		if flags.Changed(name) {
			value, _ := flags.GetInt(name)
			if value < 0 {
				return fmt.Errorf("--%s must not be negative", name)
			}
			*target = kernel.Int(int64(value))
		}
	}
	if flags.Changed("content-browser-id") {
		browserID, _ := flags.GetString("content-browser-id")
		if strings.TrimSpace(browserID) == "" {
			return fmt.Errorf("--content-browser-id must not be empty")
		}
		content.Browser.BrowserID = kernel.String(browserID)
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return executeSearchRequest(cmd, http.MethodPost, "search/"+url.PathEscape(args[0])+"/contents", body)
}

func runSearch(cmd *cobra.Command, args []string) error {
	var body json.RawMessage
	if cmd.Flags().Changed("request") {
		if len(args) != 0 {
			return fmt.Errorf("query cannot be combined with --request")
		}
		input, _ := cmd.Flags().GetString("request")
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(input), &fields); err != nil {
			return fmt.Errorf("search request must be a JSON object: %w", err)
		}
		if fields == nil {
			return fmt.Errorf("search request must be a JSON object")
		}
		var query string
		if err := json.Unmarshal(fields["query"], &query); err != nil {
			return fmt.Errorf("search request must contain a string query")
		}
		if err := validateSearchQuery(query); err != nil {
			return err
		}
		body = json.RawMessage(input)
	} else {
		if len(args) != 1 {
			return fmt.Errorf("provide a query or --request")
		}
		if err := validateSearchQuery(args[0]); err != nil {
			return err
		}
		request := map[string]any{"query": args[0]}
		if cmd.Flags().Changed("max-results") {
			count, _ := cmd.Flags().GetInt("max-results")
			if count < 1 || count > 100 {
				return fmt.Errorf("--max-results must be between 1 and 100")
			}
			request["max_results"] = count
		}
		if cmd.Flags().Changed("provider") {
			provider, _ := cmd.Flags().GetString("provider")
			if strings.TrimSpace(provider) == "" {
				return fmt.Errorf("--provider must not be empty")
			}
			request["strategy"] = map[string]any{"type": "pinned", "provider": map[string]string{"provider": provider}}
		}
		var err error
		body, err = json.Marshal(request)
		if err != nil {
			return err
		}
	}
	return executeSearchRequest(cmd, http.MethodPost, "search", body)
}

func validateSearchQuery(query string) error {
	if strings.TrimSpace(query) == "" || utf8.RuneCountInString(query) > 2048 {
		return fmt.Errorf("query must contain 1–2048 characters and not be blank")
	}
	return nil
}

func executeSearchRequest(cmd *cobra.Command, method, path string, body json.RawMessage) error {
	client := getKernelClient(cmd)
	var response json.RawMessage
	var opts []option.RequestOption
	if method == http.MethodPost {
		// Avoid duplicate billable searches after an ambiguous failure.
		opts = append(opts, option.WithMaxRetries(0))
	}
	var requestBody any
	if method == http.MethodPost {
		requestBody = body
	}
	if err := client.Execute(cmd.Context(), method, path, requestBody, &response, opts...); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return err
}
