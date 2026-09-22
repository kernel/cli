package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
)

func newSearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "search [query]",
		Short:   "Search the web and return the full result as JSON",
		Long:    "Search the web using automatic routing or a pinned provider. Returns the full JSON resource, including warnings, attempts, usage, and expiry. Use --request or --request-file for the complete Search API request, including fallback strategies, portable filters, content, and provider-native options. Requires Search API access for your organization.",
		Example: "  kernel search 'browser automation' --provider exa --max-results 5\n  kernel search --request-file request.json\n  kernel search get srch_123\n  kernel search providers --slug exa",
		Args:    cobra.MaximumNArgs(1),
		RunE:    runSearch,
	}
	cmd.Flags().String("provider", "", "Pin a provider slug (default: automatic routing)")
	cmd.Flags().Int("max-results", 10, "Requested result count (1–100; subject to provider cap)")
	cmd.Flags().String("request", "", "Complete Search API request as a JSON object; cannot be combined with query flags")
	cmd.Flags().String("request-file", "", "Complete Search API request file (use '-' for stdin)")
	cmd.Flags().String("idempotency-key", "", "Idempotency key for safely replaying the same request")
	cmd.MarkFlagsMutuallyExclusive("request", "request-file")
	for _, input := range []string{"request", "request-file"} {
		cmd.MarkFlagsMutuallyExclusive(input, "provider")
		cmd.MarkFlagsMutuallyExclusive(input, "max-results")
	}
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
	cmd.AddCommand(get, providers)
	return cmd
}

func runSearch(cmd *cobra.Command, args []string) error {
	var body json.RawMessage
	if cmd.Flags().Changed("request") || cmd.Flags().Changed("request-file") {
		if len(args) != 0 {
			return fmt.Errorf("query cannot be combined with --request or --request-file")
		}
		input, _ := cmd.Flags().GetString("request")
		if cmd.Flags().Changed("request-file") {
			path, _ := cmd.Flags().GetString("request-file")
			var data []byte
			var err error
			if path == "-" {
				data, err = io.ReadAll(cmd.InOrStdin())
			} else {
				data, err = os.ReadFile(path)
			}
			if err != nil {
				return fmt.Errorf("read search request: %w", err)
			}
			input = string(data)
		}
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
			return fmt.Errorf("provide a query or --request/--request-file")
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
		if cmd.Flags().Changed("idempotency-key") {
			key, _ := cmd.Flags().GetString("idempotency-key")
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("--idempotency-key must not be empty")
			}
			opts = append(opts, option.WithHeader("Idempotency-Key", key))
		}
	}
	if err := client.Execute(cmd.Context(), method, path, body, &response, opts...); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
