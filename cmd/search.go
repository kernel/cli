package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// SearchService defines the subset of the Kernel SDK search client that we use.
type SearchService interface {
	New(ctx context.Context, body kernel.SearchNewParams, opts ...option.RequestOption) (res *kernel.Search, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.Search, err error)
}

// SearchProvidersService defines the subset of the Kernel SDK search provider
// client that we use.
type SearchProvidersService interface {
	List(ctx context.Context, query kernel.SearchProviderListParams, opts ...option.RequestOption) (res *[]kernel.Provider, err error)
}

// SearchContentsService defines the subset of the Kernel SDK search contents
// client that we use.
type SearchContentsService interface {
	Fetch(ctx context.Context, id string, body kernel.SearchContentFetchParams, opts ...option.RequestOption) (err error)
}

// searchProviderSlugs are the concrete providers accepted by --provider,
// --fallback-providers, and the providers --slug filter. Auto and fallback are
// strategies rather than provider entries, so they are not listed here.
var searchProviderSlugs = []string{
	string(kernel.SearchProviderListParamsSlugBrave),
	string(kernel.SearchProviderListParamsSlugExa),
	string(kernel.SearchProviderListParamsSlugPerplexity),
	string(kernel.SearchProviderListParamsSlugContext),
	string(kernel.SearchProviderListParamsSlugParallel),
	string(kernel.SearchProviderListParamsSlugValyu),
	string(kernel.SearchProviderListParamsSlugOcten),
	string(kernel.SearchProviderListParamsSlugYou),
	string(kernel.SearchProviderListParamsSlugTavily),
	string(kernel.SearchProviderListParamsSlugSerpapi),
}

var (
	searchRecencyValues     = []string{"hour", "day", "week", "month", "year"}
	searchSafeSearchValues  = []string{"off", "moderate", "strict"}
	searchContentSources    = []string{"auto", "provider", "browser"}
	searchContentFormats    = []string{"markdown", "text"}
	searchBrowserModes      = []string{"curl", "render"}
	searchFallbackOnValues  = []string{"error", "timeout", "empty"}
	searchDateLayout        = "2006-01-02"
	searchProviderOptsUsage = `Provider-native options as a JSON object keyed by provider slug, e.g. '{"tavily":{"include_answer":true}}'`
)

// searchContentInput holds the portable content-retrieval options shared by
// `kernel search` and `kernel search contents`.
type searchContentInput struct {
	Enabled     bool
	Source      string
	Format      string
	MaxChars    int64
	MaxAgeHours *int64
	TimeoutMs   int64
	BrowserID   string
	BrowserMode string
}

// requested reports whether the user asked for content retrieval at all.
func (c searchContentInput) requested() bool {
	return c.Enabled || c.customized()
}

// customized reports whether any option beyond the bare --content toggle was set,
// which means the options object must be sent rather than the boolean shorthand.
func (c searchContentInput) customized() bool {
	return c.Source != "" || c.Format != "" || c.MaxChars > 0 || c.MaxAgeHours != nil ||
		c.TimeoutMs > 0 || c.BrowserID != "" || c.BrowserMode != ""
}

func (c searchContentInput) validate() error {
	if err := validateSearchEnum("--content-source", c.Source, searchContentSources); err != nil {
		return err
	}
	if err := validateSearchEnum("--content-format", c.Format, searchContentFormats); err != nil {
		return err
	}
	if err := validateSearchEnum("--content-browser-mode", c.BrowserMode, searchBrowserModes); err != nil {
		return err
	}
	if c.BrowserID != "" && c.Source == "provider" {
		return fmt.Errorf("--content-browser-id requires --content-source browser")
	}
	return nil
}

type SearchQueryInput struct {
	Query          string
	Output         string
	Country        string
	Language       string
	MaxResults     int64
	Recency        string
	SafeSearch     string
	StartDate      string
	EndDate        string
	IncludeDomains []string
	ExcludeDomains []string
	StrictParams   bool
	IncludeRaw     bool
	TimeoutMs      int64
	ShowContent    bool

	Content searchContentInput

	Provider          string
	FallbackProviders []string
	FallbackOn        []string
	ProviderOptions   string
}

type SearchGetInput struct {
	ID          string
	Output      string
	ShowContent bool
}

type SearchProvidersInput struct {
	Slug   string
	Output string
}

type SearchContentsInput struct {
	ID        string
	Output    string
	ResultIDs []string
	Limit     int64
	TimeoutMs int64
	Content   searchContentInput
}

// SearchCmd handles search operations independent of cobra.
type SearchCmd struct {
	search    SearchService
	providers SearchProvidersService
	contents  SearchContentsService
}

func (s SearchCmd) Query(ctx context.Context, in SearchQueryInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if strings.TrimSpace(in.Query) == "" {
		return fmt.Errorf("a search query is required")
	}
	if err := validateSearchEnum("--recency", in.Recency, searchRecencyValues); err != nil {
		return err
	}
	if err := validateSearchEnum("--safe-search", in.SafeSearch, searchSafeSearchValues); err != nil {
		return err
	}
	if in.MaxResults < 0 || in.MaxResults > 100 {
		return fmt.Errorf("--max-results must be between 1 and 100")
	}
	if err := in.Content.validate(); err != nil {
		return err
	}

	req := kernel.RequestParam{Query: in.Query}
	if in.Country != "" {
		req.Country = kernel.Opt(in.Country)
	}
	if in.Language != "" {
		req.Language = kernel.Opt(in.Language)
	}
	if in.MaxResults > 0 {
		req.MaxResults = kernel.Opt(in.MaxResults)
	}
	if in.TimeoutMs > 0 {
		req.TimeoutMs = kernel.Opt(in.TimeoutMs)
	}
	if in.StrictParams {
		req.StrictParams = kernel.Opt(true)
	}
	if in.IncludeRaw {
		req.IncludeRaw = kernel.Opt(true)
	}
	if in.Recency != "" {
		req.Recency = kernel.RequestRecency(in.Recency)
	}
	if in.SafeSearch != "" {
		req.SafeSearch = kernel.RequestSafeSearch(in.SafeSearch)
	}
	if in.StartDate != "" {
		t, err := parseSearchDate("--start-date", in.StartDate)
		if err != nil {
			return err
		}
		req.StartDate = kernel.Opt(t)
	}
	if in.EndDate != "" {
		t, err := parseSearchDate("--end-date", in.EndDate)
		if err != nil {
			return err
		}
		req.EndDate = kernel.Opt(t)
	}
	if len(in.IncludeDomains) > 0 {
		req.IncludeDomains = in.IncludeDomains
	}
	if len(in.ExcludeDomains) > 0 {
		req.ExcludeDomains = in.ExcludeDomains
	}

	if in.Content.requested() {
		if in.Content.customized() {
			opts := kernel.RequestContentSearchContentOptionsParam{}
			applySearchContentOptions(in.Content, &opts.Source, &opts.Format, &opts.MaxChars, &opts.MaxAgeHours, &opts.TimeoutMs)
			if in.Content.BrowserID != "" {
				opts.Browser.BrowserID = kernel.Opt(in.Content.BrowserID)
			}
			if in.Content.BrowserMode != "" {
				opts.Browser.Mode = in.Content.BrowserMode
			}
			req.Content = kernel.RequestContentUnionParam{OfRequestContentSearchContentOptions: &opts}
		} else {
			req.Content = kernel.RequestContentUnionParam{OfRequestContentBoolean: kernel.Opt(true)}
		}
	}

	strategy, err := buildSearchStrategy(in.Provider, in.FallbackProviders, in.FallbackOn, in.ProviderOptions)
	if err != nil {
		return err
	}
	if strategy != nil {
		req.Strategy = *strategy
	}

	if in.Output != "json" {
		pterm.Info.Printf("Searching for %q...\n", in.Query)
	}

	result, err := s.search.New(ctx, kernel.SearchNewParams{Request: req})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	return renderSearch(result, in.Output, in.ShowContent)
}

func (s SearchCmd) Get(ctx context.Context, in SearchGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	result, err := s.search.Get(ctx, in.ID)
	if err != nil {
		if util.IsNotFound(err) {
			if in.Output == "json" {
				fmt.Println("null")
				return nil
			}
			pterm.Error.Printf("Search '%s' not found or expired\n", in.ID)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}

	return renderSearch(result, in.Output, in.ShowContent)
}

func (s SearchCmd) Providers(ctx context.Context, in SearchProvidersInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if err := validateSearchEnum("--slug", in.Slug, searchProviderSlugs); err != nil {
		return err
	}

	params := kernel.SearchProviderListParams{}
	if in.Slug != "" {
		params.Slug = kernel.SearchProviderListParamsSlug(in.Slug)
	}

	res, err := s.providers.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var items []kernel.Provider
	if res != nil {
		items = *res
	}

	if in.Output == "json" {
		if len(items) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(items)
	}

	if len(items) == 0 {
		pterm.Info.Println("No search providers found")
		return nil
	}

	rows := pterm.TableData{{"Slug", "Max Results", "Inline Content", "Post-hoc Content", "Freshness Control", "Options Schema"}}
	for _, p := range items {
		rows = append(rows, []string{
			p.Slug,
			strconv.FormatInt(p.MaxResultsCap, 10),
			formatSearchBool(p.Content.Inline),
			formatSearchBool(p.Content.PostHoc),
			formatSearchBool(p.Content.FreshnessControl),
			orSearchDash(p.ProviderOptions.SchemaRef),
		})
	}
	PrintTableNoPad(rows, true)

	// The portable-parameter support matrix and notes only fit in a readable way
	// when a single provider was requested.
	if len(items) == 1 {
		p := items[0]
		paramRows := pterm.TableData{{"Portable Param", "Support", "Notes"}}
		for _, pp := range []struct {
			name    string
			support string
			notes   string
		}{
			{"country", p.Params.Country.Support, p.Params.Country.Notes},
			{"end_date", p.Params.EndDate.Support, p.Params.EndDate.Notes},
			{"exclude_domains", p.Params.ExcludeDomains.Support, p.Params.ExcludeDomains.Notes},
			{"include_domains", p.Params.IncludeDomains.Support, p.Params.IncludeDomains.Notes},
			{"language", p.Params.Language.Support, p.Params.Language.Notes},
			{"recency", p.Params.Recency.Support, p.Params.Recency.Notes},
			{"safe_search", p.Params.SafeSearch.Support, p.Params.SafeSearch.Notes},
			{"start_date", p.Params.StartDate.Support, p.Params.StartDate.Notes},
		} {
			paramRows = append(paramRows, []string{pp.name, orSearchDash(pp.support), orSearchDash(pp.notes)})
		}
		pterm.Println()
		PrintTableNoPad(paramRows, true)

		if len(p.Notes) > 0 {
			pterm.Println()
			pterm.Println("Notes:")
			for _, n := range p.Notes {
				pterm.Printf("  - %s\n", n)
			}
		}
	}

	return nil
}

func (s SearchCmd) Contents(ctx context.Context, in SearchContentsInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if len(in.ResultIDs) > 0 && in.Limit > 0 {
		return fmt.Errorf("--result-ids and --limit are mutually exclusive")
	}
	if err := in.Content.validate(); err != nil {
		return err
	}

	req := kernel.FetchRequestParam{}
	if in.Limit > 0 {
		req.Limit = kernel.Opt(in.Limit)
	}
	if in.TimeoutMs > 0 {
		req.TimeoutMs = kernel.Opt(in.TimeoutMs)
	}
	if len(in.ResultIDs) > 0 {
		req.ResultIDs = in.ResultIDs
	}
	if in.Content.customized() {
		applySearchContentOptions(in.Content, &req.Content.Source, &req.Content.Format, &req.Content.MaxChars, &req.Content.MaxAgeHours, &req.Content.TimeoutMs)
		if in.Content.BrowserID != "" {
			req.Content.Browser.BrowserID = kernel.Opt(in.Content.BrowserID)
		}
		if in.Content.BrowserMode != "" {
			req.Content.Browser.Mode = in.Content.BrowserMode
		}
	}

	if err := s.contents.Fetch(ctx, in.ID, kernel.SearchContentFetchParams{FetchRequest: req}); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		fmt.Println("null")
		return nil
	}
	pterm.Success.Printf("Requested content for search %s\n", in.ID)
	return nil
}

// applySearchContentOptions copies the shared content options onto the
// destination fields of either content options struct. The two SDK structs are
// identical in shape but distinct types, so the fields are passed by pointer.
func applySearchContentOptions(in searchContentInput, source, format *string, maxChars, maxAgeHours, timeoutMs *param.Opt[int64]) {
	if in.Source != "" {
		*source = in.Source
	}
	if in.Format != "" {
		*format = in.Format
	}
	if in.MaxChars > 0 {
		*maxChars = kernel.Opt(in.MaxChars)
	}
	// 0 is meaningful here (it forces a live fetch), so only the pointer tells us
	// whether the flag was supplied.
	if in.MaxAgeHours != nil {
		*maxAgeHours = kernel.Opt(*in.MaxAgeHours)
	}
	if in.TimeoutMs > 0 {
		*timeoutMs = kernel.Opt(in.TimeoutMs)
	}
}

// buildSearchStrategy maps the strategy flags onto one of the three SDK strategy
// variants. Returns nil when no strategy flag was supplied, which lets the API
// apply its auto default.
func buildSearchStrategy(provider string, fallbackProviders, fallbackOn []string, providerOptions string) (*kernel.StrategyUnionParam, error) {
	if provider != "" && len(fallbackProviders) > 0 {
		return nil, fmt.Errorf("--provider and --fallback-providers are mutually exclusive")
	}
	if err := validateSearchEnum("--provider", provider, searchProviderSlugs); err != nil {
		return nil, err
	}
	for _, p := range fallbackProviders {
		if err := validateSearchEnum("--fallback-providers", p, searchProviderSlugs); err != nil {
			return nil, err
		}
	}
	for _, f := range fallbackOn {
		if err := validateSearchEnum("--fallback-on", f, searchFallbackOnValues); err != nil {
			return nil, err
		}
	}

	nativeOptions, err := parseSearchProviderOptions(providerOptions)
	if err != nil {
		return nil, err
	}

	switch {
	case provider != "":
		if len(fallbackOn) > 0 {
			return nil, fmt.Errorf("--fallback-on has no effect with --provider, which pins a single provider")
		}
		target, err := buildSearchProviderTarget(provider, nativeOptions[provider])
		if err != nil {
			return nil, err
		}
		pinned := kernel.StrategyPinnedParam{Provider: target}
		return &kernel.StrategyUnionParam{OfPinned: &pinned}, nil

	case len(fallbackProviders) > 0:
		targets := make([]kernel.ProviderTargetUnionParam, 0, len(fallbackProviders))
		for _, p := range fallbackProviders {
			target, err := buildSearchProviderTarget(p, nativeOptions[p])
			if err != nil {
				return nil, err
			}
			targets = append(targets, target)
		}
		fallback := kernel.StrategyFallbackParam{Providers: targets}
		if len(fallbackOn) > 0 {
			fallback.FallbackOn = fallbackOn
		}
		return &kernel.StrategyUnionParam{OfFallback: &fallback}, nil

	case len(nativeOptions) > 0 || len(fallbackOn) > 0:
		auto := kernel.StrategyAutoParam{}
		if len(fallbackOn) > 0 {
			auto.FallbackOn = fallbackOn
		}
		// Map iteration order is random, so the targets are emitted in the
		// documented slug order to keep requests reproducible.
		for _, slug := range searchProviderSlugs {
			raw, ok := nativeOptions[slug]
			if !ok {
				continue
			}
			target, err := buildSearchProviderTarget(slug, raw)
			if err != nil {
				return nil, err
			}
			auto.ProviderOptions = append(auto.ProviderOptions, target)
		}
		return &kernel.StrategyUnionParam{OfAuto: &auto}, nil
	}

	return nil, nil
}

// parseSearchProviderOptions decodes the --provider-options JSON object, which
// maps a provider slug to that provider's native options object.
func parseSearchProviderOptions(raw string) (map[string]json.RawMessage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var byProvider map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &byProvider); err != nil {
		return nil, fmt.Errorf("invalid --provider-options: must be a JSON object keyed by provider slug: %w", err)
	}
	for slug := range byProvider {
		if err := validateSearchEnum("--provider-options key", slug, searchProviderSlugs); err != nil {
			return nil, err
		}
	}
	return byProvider, nil
}

// buildSearchProviderTarget assembles a provider target from a slug and its raw
// native options. The SDK union is discriminated on "provider", so the target is
// round-tripped through JSON rather than switched on by hand.
func buildSearchProviderTarget(slug string, options json.RawMessage) (kernel.ProviderTargetUnionParam, error) {
	var target kernel.ProviderTargetUnionParam

	payload := map[string]any{"provider": slug}
	if len(options) > 0 {
		payload["options"] = options
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return target, fmt.Errorf("encode provider target for %q: %w", slug, err)
	}
	if err := target.UnmarshalJSON(encoded); err != nil {
		return target, fmt.Errorf("invalid provider options for %q: %w", slug, err)
	}
	return target, nil
}

func renderSearch(result *kernel.Search, output string, showContent bool) error {
	if result == nil || result.ID == "" {
		if output == "json" {
			fmt.Println("null")
			return nil
		}
		pterm.Info.Println("No search returned")
		return nil
	}

	if output == "json" {
		return util.PrintPrettyJSON(result)
	}

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"Search ID", result.ID})
	rows = append(rows, []string{"Query", result.Query})
	rows = append(rows, []string{"Provider", result.Provider})
	rows = append(rows, []string{"Results", strconv.Itoa(len(result.Results))})
	rows = append(rows, []string{"Expires At", util.FormatLocal(result.ExpiresAt)})
	PrintTableNoPad(rows, true)

	if result.Answer != "" {
		pterm.Println()
		pterm.Println("Answer:")
		pterm.Println(result.Answer)
	}

	if len(result.Results) > 0 {
		hasContent := false
		for _, r := range result.Results {
			if r.Content.Status != "" {
				hasContent = true
				break
			}
		}

		header := []string{"#", "Title", "URL", "Source", "Published"}
		if hasContent {
			header = append(header, "Content")
		}
		resultRows := pterm.TableData{header}
		for _, r := range result.Results {
			row := []string{
				strconv.FormatInt(r.Rank, 10),
				orSearchDash(r.Title),
				r.URL,
				orSearchDash(r.Source),
				orSearchDash(r.PublishedDate),
			}
			if hasContent {
				row = append(row, orSearchDash(r.Content.Status))
			}
			resultRows = append(resultRows, row)
		}
		pterm.Println()
		PrintTableNoPad(resultRows, true)
	} else {
		pterm.Println()
		pterm.Info.Println("No results")
	}

	if showContent {
		for _, r := range result.Results {
			if r.Content.Text == "" {
				continue
			}
			pterm.Println()
			pterm.Printf("--- [%d] %s (%s) ---\n", r.Rank, orSearchDash(r.Title), r.URL)
			pterm.Println(r.Content.Text)
		}
	}

	if len(result.Warnings) > 0 {
		warnRows := pterm.TableData{{"Warning", "Param", "Provider", "Message"}}
		for _, w := range result.Warnings {
			warnRows = append(warnRows, []string{w.Code, orSearchDash(w.Param), orSearchDash(w.Provider), w.Message})
		}
		pterm.Println()
		PrintTableNoPad(warnRows, true)
	}

	if len(result.Attempts) > 0 {
		attemptRows := pterm.TableData{{"Attempt Provider", "Outcome", "Duration", "Error", "Retryable"}}
		for _, a := range result.Attempts {
			attemptRows = append(attemptRows, []string{
				a.Provider,
				string(a.Outcome),
				fmt.Sprintf("%dms", a.DurationMs),
				orSearchDash(a.ErrorCode),
				formatSearchBool(a.Retryable),
			})
		}
		pterm.Println()
		PrintTableNoPad(attemptRows, true)
	}

	usage := fmt.Sprintf("\nUsage: %d result(s), %d content fetch(es)", result.Usage.ResultsCount, result.Usage.ContentFetches)
	if result.Usage.Cost > 0 {
		usage += fmt.Sprintf(", $%.6f", result.Usage.Cost)
	}
	pterm.Println(usage)

	return nil
}

func parseSearchDate(flag, value string) (time.Time, error) {
	t, err := time.Parse(searchDateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s %q: expected YYYY-MM-DD", flag, value)
	}
	return t, nil
}

func validateSearchEnum(flag, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("invalid %s %q: must be one of %s", flag, value, strings.Join(allowed, ", "))
}

func orSearchDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func formatSearchBool(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// --- Cobra wiring ---

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the web",
	Long: "Search the web through Kernel's search providers.\n\n" +
		"By default Kernel picks an eligible provider (the auto strategy). Use --provider to pin\n" +
		"one provider, or --fallback-providers to try an ordered chain. Portable filters that a\n" +
		"provider cannot honor are approximated or dropped, and the outcome is reported as a\n" +
		"warning unless --strict-params is set.\n\n" +
		"Results are retained for 24 hours and can be re-read with `kernel search get <id>`.",
	Example: `  kernel search "kernel browser automation"
  kernel search "latest go release" --recency week --max-results 5
  kernel search "site news" --include-domains example.com --content --show-content
  kernel search "ai research" --provider exa --provider-options '{"exa":{"type":"neural"}}'`,
	Args: cobra.ArbitraryArgs,
	RunE: runSearchQuery,
}

var searchGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a retained search by ID",
	Long: "Return a retained search exactly as it was returned by `kernel search`: results, attempts,\n" +
		"warnings, and usage. No provider is called and nothing is billed. Searches expire 24 hours\n" +
		"after completion.",
	Args: cobra.ExactArgs(1),
	RunE: runSearchGet,
}

var searchProvidersCmd = &cobra.Command{
	Use:   "providers",
	Short: "List search providers and their capabilities",
	Long: "List providers, their result caps, content-retrieval capabilities, and the OpenAPI\n" +
		"component backing their native options. Pass --slug to inspect a single provider, which\n" +
		"also prints its portable-parameter support matrix and notes.",
	Args: cobra.NoArgs,
	RunE: runSearchProviders,
}

var searchContentsCmd = &cobra.Command{
	Use:   "contents <id>",
	Short: "Fetch content for results of a retained search",
	Long: "Deferred result-content retrieval for a retained search. This endpoint is reserved and\n" +
		"returns 404 until the retrieval implementation ships; use `kernel search --content` for\n" +
		"inline retrieval in the meantime.",
	Args: cobra.ExactArgs(1),
	RunE: runSearchContents,
}

// addSearchContentFlags registers the portable content-retrieval options shared
// by `kernel search` and `kernel search contents`.
func addSearchContentFlags(cmd *cobra.Command) {
	cmd.Flags().String("content-source", "", "Content retrieval source: auto, provider, or browser")
	cmd.Flags().String("content-format", "", "Extracted content format: markdown or text")
	cmd.Flags().Int64("content-max-chars", 0, "Per-result Unicode character limit after extraction")
	cmd.Flags().Int64("content-max-age-hours", 0, "Maximum acceptable age of cached page content; 0 forces a live fetch")
	cmd.Flags().Int64("content-timeout-ms", 0, "Per-result retrieval deadline in milliseconds")
	cmd.Flags().String("content-browser-id", "", "Existing browser session to retrieve content through (requires --content-source browser)")
	cmd.Flags().String("content-browser-mode", "", "Browser retrieval mode: curl or render")
}

func searchContentFlags(cmd *cobra.Command, enabled bool) searchContentInput {
	source, _ := cmd.Flags().GetString("content-source")
	format, _ := cmd.Flags().GetString("content-format")
	maxChars, _ := cmd.Flags().GetInt64("content-max-chars")
	timeoutMs, _ := cmd.Flags().GetInt64("content-timeout-ms")
	browserID, _ := cmd.Flags().GetString("content-browser-id")
	browserMode, _ := cmd.Flags().GetString("content-browser-mode")

	in := searchContentInput{
		Enabled:     enabled,
		Source:      source,
		Format:      format,
		MaxChars:    maxChars,
		TimeoutMs:   timeoutMs,
		BrowserID:   browserID,
		BrowserMode: browserMode,
	}
	// 0 is a meaningful max age, so it is only sent when explicitly supplied.
	if cmd.Flags().Changed("content-max-age-hours") {
		maxAge, _ := cmd.Flags().GetInt64("content-max-age-hours")
		in.MaxAgeHours = &maxAge
	}
	return in
}

func init() {
	searchCmd.AddCommand(searchGetCmd)
	searchCmd.AddCommand(searchProvidersCmd)
	searchCmd.AddCommand(searchContentsCmd)

	addJSONOutputFlag(searchCmd)
	searchCmd.Flags().String("country", "", "ISO 3166-1 alpha-2 search locale preference")
	searchCmd.Flags().String("language", "", "BCP 47 search language preference")
	searchCmd.Flags().Int64("max-results", 0, "Requested result count, 1 through 100 (clamped to the provider cap)")
	searchCmd.Flags().String("recency", "", "Relative search window: hour, day, week, month, or year")
	searchCmd.Flags().String("safe-search", "", "Safety preference: off, moderate, or strict")
	searchCmd.Flags().String("start-date", "", "Inclusive publication-date lower bound (YYYY-MM-DD)")
	searchCmd.Flags().String("end-date", "", "Inclusive publication-date upper bound (YYYY-MM-DD)")
	searchCmd.Flags().StringSlice("include-domains", nil, "Hostnames (and subdomains) to prefer")
	searchCmd.Flags().StringSlice("exclude-domains", nil, "Hostnames (and subdomains) to exclude")
	searchCmd.Flags().Bool("strict-params", false, "Require every supplied portable parameter to be honored exactly")
	searchCmd.Flags().Bool("include-raw", false, "Include untouched provider payloads in raw fields")
	searchCmd.Flags().Int64("timeout-ms", 0, "Overall deadline across search attempts and inline retrieval")
	searchCmd.Flags().Bool("content", false, "Retrieve page content for each result using portable defaults")
	searchCmd.Flags().Bool("show-content", false, "Print the extracted content text for each result")
	addSearchContentFlags(searchCmd)
	searchCmd.Flags().String("provider", "", "Pin a single provider: "+strings.Join(searchProviderSlugs, ", "))
	searchCmd.Flags().StringSlice("fallback-providers", nil, "Ordered provider chain to try in turn")
	searchCmd.Flags().StringSlice("fallback-on", nil, "Outcomes that advance to the next provider: error, timeout, empty")
	searchCmd.Flags().String("provider-options", "", searchProviderOptsUsage)

	addJSONOutputFlag(searchGetCmd)
	searchGetCmd.Flags().Bool("show-content", false, "Print the extracted content text for each result")

	addJSONOutputFlag(searchProvidersCmd)
	searchProvidersCmd.Flags().String("slug", "", "Filter to a single provider: "+strings.Join(searchProviderSlugs, ", "))

	addJSONOutputFlag(searchContentsCmd)
	searchContentsCmd.Flags().StringSlice("result-ids", nil, "Result IDs from the retained search, in the desired response order")
	searchContentsCmd.Flags().Int64("limit", 0, "Number of results to fetch starting from rank 1 (mutually exclusive with --result-ids)")
	searchContentsCmd.Flags().Int64("timeout-ms", 0, "Overall deadline across all selected results")
	addSearchContentFlags(searchContentsCmd)
}

func newSearchCmd(cmd *cobra.Command) SearchCmd {
	client := getKernelClient(cmd)
	svc := client.Search
	return SearchCmd{search: &svc, providers: &svc.Providers, contents: &svc.Contents}
}

func runSearchQuery(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	output, _ := cmd.Flags().GetString("output")
	country, _ := cmd.Flags().GetString("country")
	language, _ := cmd.Flags().GetString("language")
	maxResults, _ := cmd.Flags().GetInt64("max-results")
	recency, _ := cmd.Flags().GetString("recency")
	safeSearch, _ := cmd.Flags().GetString("safe-search")
	startDate, _ := cmd.Flags().GetString("start-date")
	endDate, _ := cmd.Flags().GetString("end-date")
	includeDomains, _ := cmd.Flags().GetStringSlice("include-domains")
	excludeDomains, _ := cmd.Flags().GetStringSlice("exclude-domains")
	strictParams, _ := cmd.Flags().GetBool("strict-params")
	includeRaw, _ := cmd.Flags().GetBool("include-raw")
	timeoutMs, _ := cmd.Flags().GetInt64("timeout-ms")
	content, _ := cmd.Flags().GetBool("content")
	showContent, _ := cmd.Flags().GetBool("show-content")
	provider, _ := cmd.Flags().GetString("provider")
	fallbackProviders, _ := cmd.Flags().GetStringSlice("fallback-providers")
	fallbackOn, _ := cmd.Flags().GetStringSlice("fallback-on")
	providerOptions, _ := cmd.Flags().GetString("provider-options")

	// --show-content is only useful alongside retrieval, so it implies --content.
	contentIn := searchContentFlags(cmd, content || showContent)

	return newSearchCmd(cmd).Query(cmd.Context(), SearchQueryInput{
		Query:             strings.Join(args, " "),
		Output:            output,
		Country:           country,
		Language:          language,
		MaxResults:        maxResults,
		Recency:           recency,
		SafeSearch:        safeSearch,
		StartDate:         startDate,
		EndDate:           endDate,
		IncludeDomains:    includeDomains,
		ExcludeDomains:    excludeDomains,
		StrictParams:      strictParams,
		IncludeRaw:        includeRaw,
		TimeoutMs:         timeoutMs,
		ShowContent:       showContent,
		Content:           contentIn,
		Provider:          provider,
		FallbackProviders: fallbackProviders,
		FallbackOn:        fallbackOn,
		ProviderOptions:   providerOptions,
	})
}

func runSearchGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	showContent, _ := cmd.Flags().GetBool("show-content")
	return newSearchCmd(cmd).Get(cmd.Context(), SearchGetInput{
		ID:          args[0],
		Output:      output,
		ShowContent: showContent,
	})
}

func runSearchProviders(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	slug, _ := cmd.Flags().GetString("slug")
	return newSearchCmd(cmd).Providers(cmd.Context(), SearchProvidersInput{Slug: slug, Output: output})
}

func runSearchContents(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	resultIDs, _ := cmd.Flags().GetStringSlice("result-ids")
	limit, _ := cmd.Flags().GetInt64("limit")
	timeoutMs, _ := cmd.Flags().GetInt64("timeout-ms")

	return newSearchCmd(cmd).Contents(cmd.Context(), SearchContentsInput{
		ID:        args[0],
		Output:    output,
		ResultIDs: resultIDs,
		Limit:     limit,
		TimeoutMs: timeoutMs,
		Content:   searchContentFlags(cmd, true),
	})
}
