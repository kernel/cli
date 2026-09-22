package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FakeSearchService implements SearchService.
type FakeSearchService struct {
	NewFunc  func(ctx context.Context, body kernel.SearchNewParams, opts ...option.RequestOption) (*kernel.Search, error)
	GetFunc  func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Search, error)
	LastBody kernel.SearchNewParams
}

func (f *FakeSearchService) New(ctx context.Context, body kernel.SearchNewParams, opts ...option.RequestOption) (*kernel.Search, error) {
	f.LastBody = body
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.Search{ID: "srch_1", Query: body.Request.Query, Provider: "brave", ExpiresAt: time.Unix(0, 0)}, nil
}

func (f *FakeSearchService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Search, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id, opts...)
	}
	return &kernel.Search{ID: id, Query: "cached", Provider: "brave", ExpiresAt: time.Unix(0, 0)}, nil
}

// FakeSearchProvidersService implements SearchProvidersService.
type FakeSearchProvidersService struct {
	ListFunc  func(ctx context.Context, query kernel.SearchProviderListParams, opts ...option.RequestOption) (*[]kernel.Provider, error)
	LastQuery kernel.SearchProviderListParams
}

func (f *FakeSearchProvidersService) List(ctx context.Context, query kernel.SearchProviderListParams, opts ...option.RequestOption) (*[]kernel.Provider, error) {
	f.LastQuery = query
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	items := []kernel.Provider{{Slug: "brave", MaxResultsCap: 20}}
	return &items, nil
}

// FakeSearchContentsService implements SearchContentsService.
type FakeSearchContentsService struct {
	FetchFunc func(ctx context.Context, id string, body kernel.SearchContentFetchParams, opts ...option.RequestOption) error
	LastID    string
	LastBody  kernel.SearchContentFetchParams
}

func (f *FakeSearchContentsService) Fetch(ctx context.Context, id string, body kernel.SearchContentFetchParams, opts ...option.RequestOption) error {
	f.LastID = id
	f.LastBody = body
	if f.FetchFunc != nil {
		return f.FetchFunc(ctx, id, body, opts...)
	}
	return nil
}

// requestJSON marshals the captured request so tests can assert on the exact
// wire payload, which is where the union and strategy encoding actually matters.
func requestJSON(t *testing.T, params kernel.SearchNewParams) map[string]any {
	t.Helper()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestSearchQueryBuildsPortableParams(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeSearchService{}
	s := SearchCmd{search: fake}

	err := s.Query(context.Background(), SearchQueryInput{
		Query:          "kernel browsers",
		Country:        "US",
		Language:       "en",
		MaxResults:     5,
		Recency:        "week",
		SafeSearch:     "moderate",
		StartDate:      "2026-01-02",
		EndDate:        "2026-02-03",
		IncludeDomains: []string{"example.com"},
		ExcludeDomains: []string{"spam.example"},
		StrictParams:   true,
		IncludeRaw:     true,
		TimeoutMs:      15000,
	})
	require.NoError(t, err)

	body := requestJSON(t, fake.LastBody)
	assert.Equal(t, "kernel browsers", body["query"])
	assert.Equal(t, "US", body["country"])
	assert.Equal(t, "en", body["language"])
	assert.EqualValues(t, 5, body["max_results"])
	assert.Equal(t, "week", body["recency"])
	assert.Equal(t, "moderate", body["safe_search"])
	assert.Equal(t, "2026-01-02", body["start_date"])
	assert.Equal(t, "2026-02-03", body["end_date"])
	assert.Equal(t, []any{"example.com"}, body["include_domains"])
	assert.Equal(t, []any{"spam.example"}, body["exclude_domains"])
	assert.Equal(t, true, body["strict_params"])
	assert.Equal(t, true, body["include_raw"])
	assert.EqualValues(t, 15000, body["timeout_ms"])
	// No strategy flags were supplied, so the API applies its auto default.
	assert.NotContains(t, body, "strategy")
}

func TestSearchQueryContentBooleanShorthand(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeSearchService{}
	s := SearchCmd{search: fake}

	require.NoError(t, s.Query(context.Background(), SearchQueryInput{
		Query:   "q",
		Content: searchContentInput{Enabled: true},
	}))

	body := requestJSON(t, fake.LastBody)
	assert.Equal(t, true, body["content"])
}

func TestSearchQueryContentOptionsObject(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeSearchService{}
	s := SearchCmd{search: fake}

	maxAge := int64(0)
	require.NoError(t, s.Query(context.Background(), SearchQueryInput{
		Query: "q",
		Content: searchContentInput{
			Enabled:     true,
			Source:      "browser",
			Format:      "text",
			MaxChars:    2000,
			MaxAgeHours: &maxAge,
			TimeoutMs:   9000,
			BrowserID:   "br_123",
			BrowserMode: "render",
		},
	}))

	body := requestJSON(t, fake.LastBody)
	content, ok := body["content"].(map[string]any)
	require.True(t, ok, "content should be an options object, got %#v", body["content"])
	assert.Equal(t, "browser", content["source"])
	assert.Equal(t, "text", content["format"])
	assert.EqualValues(t, 2000, content["max_chars"])
	// 0 is meaningful: it forces a live fetch, so it must survive to the wire.
	assert.EqualValues(t, 0, content["max_age_hours"])
	assert.EqualValues(t, 9000, content["timeout_ms"])
	assert.Equal(t, map[string]any{"browser_id": "br_123", "mode": "render"}, content["browser"])
}

func TestSearchQueryPinnedStrategyWithNativeOptions(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeSearchService{}
	s := SearchCmd{search: fake}

	require.NoError(t, s.Query(context.Background(), SearchQueryInput{
		Query:           "q",
		Provider:        "tavily",
		ProviderOptions: `{"tavily":{"include_answer":true}}`,
	}))

	body := requestJSON(t, fake.LastBody)
	strategy, ok := body["strategy"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "pinned", strategy["type"])
	provider, ok := strategy["provider"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tavily", provider["provider"])
	assert.Equal(t, map[string]any{"include_answer": true}, provider["options"])
}

func TestSearchQueryFallbackStrategyPreservesOrder(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeSearchService{}
	s := SearchCmd{search: fake}

	require.NoError(t, s.Query(context.Background(), SearchQueryInput{
		Query:             "q",
		FallbackProviders: []string{"exa", "brave"},
		FallbackOn:        []string{"error", "empty"},
	}))

	body := requestJSON(t, fake.LastBody)
	strategy, ok := body["strategy"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "fallback", strategy["type"])
	assert.Equal(t, []any{"error", "empty"}, strategy["fallback_on"])
	providers, ok := strategy["providers"].([]any)
	require.True(t, ok)
	require.Len(t, providers, 2)
	assert.Equal(t, "exa", providers[0].(map[string]any)["provider"])
	assert.Equal(t, "brave", providers[1].(map[string]any)["provider"])
}

func TestSearchQueryAutoStrategyFromProviderOptions(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeSearchService{}
	s := SearchCmd{search: fake}

	require.NoError(t, s.Query(context.Background(), SearchQueryInput{
		Query:           "q",
		FallbackOn:      []string{"timeout"},
		ProviderOptions: `{"brave":{"safesearch":"strict"},"exa":{}}`,
	}))

	body := requestJSON(t, fake.LastBody)
	strategy, ok := body["strategy"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "auto", strategy["type"])
	assert.Equal(t, []any{"timeout"}, strategy["fallback_on"])
	opts, ok := strategy["provider_options"].([]any)
	require.True(t, ok)
	require.Len(t, opts, 2)
	// Emitted in documented slug order rather than map order.
	assert.Equal(t, "brave", opts[0].(map[string]any)["provider"])
	assert.Equal(t, "exa", opts[1].(map[string]any)["provider"])
}

func TestSearchQueryValidationErrors(t *testing.T) {
	_ = capturePtermOutput(t)
	s := SearchCmd{search: &FakeSearchService{}}

	cases := []struct {
		name    string
		in      SearchQueryInput
		wantErr string
	}{
		{"empty query", SearchQueryInput{Query: "  "}, "a search query is required"},
		{"bad recency", SearchQueryInput{Query: "q", Recency: "decade"}, "invalid --recency"},
		{"bad safe search", SearchQueryInput{Query: "q", SafeSearch: "maybe"}, "invalid --safe-search"},
		{"max results too high", SearchQueryInput{Query: "q", MaxResults: 101}, "--max-results must be between 1 and 100"},
		{"bad start date", SearchQueryInput{Query: "q", StartDate: "01-02-2026"}, "invalid --start-date"},
		{"bad content source", SearchQueryInput{Query: "q", Content: searchContentInput{Source: "psychic"}}, "invalid --content-source"},
		{"browser id with provider source", SearchQueryInput{Query: "q", Content: searchContentInput{Source: "provider", BrowserID: "br_1"}}, "--content-browser-id requires --content-source browser"},
		{"bad provider", SearchQueryInput{Query: "q", Provider: "askjeeves"}, "invalid --provider"},
		{"provider with fallback chain", SearchQueryInput{Query: "q", Provider: "brave", FallbackProviders: []string{"exa"}}, "mutually exclusive"},
		{"fallback-on with pinned provider", SearchQueryInput{Query: "q", Provider: "brave", FallbackOn: []string{"error"}}, "--fallback-on has no effect with --provider"},
		{"bad fallback-on", SearchQueryInput{Query: "q", FallbackProviders: []string{"exa"}, FallbackOn: []string{"sometimes"}}, "invalid --fallback-on"},
		{"malformed provider options", SearchQueryInput{Query: "q", ProviderOptions: "not json"}, "invalid --provider-options"},
		{"unknown provider options key", SearchQueryInput{Query: "q", ProviderOptions: `{"altavista":{}}`}, "invalid --provider-options key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.Query(context.Background(), tc.in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestSearchQueryRendersResults(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeSearchService{
		NewFunc: func(ctx context.Context, body kernel.SearchNewParams, opts ...option.RequestOption) (*kernel.Search, error) {
			return &kernel.Search{
				ID:        "srch_abc",
				Query:     body.Request.Query,
				Provider:  "brave",
				Answer:    "42",
				ExpiresAt: time.Unix(0, 0),
				Results: []kernel.Result{{
					ID:      "res_1",
					Rank:    1,
					URL:     "https://example.com/a",
					Title:   "Example A",
					Source:  "example.com",
					Content: kernel.ResultContent{Status: "ok", Text: "extracted body"},
				}},
				Warnings: []kernel.Warning{{Code: "max_results_clamped", Message: "clamped to 20"}},
				Attempts: []kernel.Attempt{{Provider: "brave", Outcome: "success", DurationMs: 120}},
				Usage:    kernel.Usage{ResultsCount: 1, ContentFetches: 1},
			}, nil
		},
	}
	s := SearchCmd{search: fake}

	require.NoError(t, s.Query(context.Background(), SearchQueryInput{Query: "q", ShowContent: true}))

	out := buf.String()
	assert.Contains(t, out, "srch_abc")
	assert.Contains(t, out, "Example A")
	assert.Contains(t, out, "https://example.com/a")
	assert.Contains(t, out, "42")
	assert.Contains(t, out, "extracted body")
	assert.Contains(t, out, "max_results_clamped")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "Usage: 1 result(s), 1 content fetch(es)")
}

func TestSearchGetRendersRetainedSearch(t *testing.T) {
	buf := capturePtermOutput(t)
	s := SearchCmd{search: &FakeSearchService{}}

	require.NoError(t, s.Get(context.Background(), SearchGetInput{ID: "srch_xyz"}))
	assert.Contains(t, buf.String(), "srch_xyz")
}

func TestSearchProvidersFiltersBySlugAndPrintsDetail(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeSearchProvidersService{
		ListFunc: func(ctx context.Context, query kernel.SearchProviderListParams, opts ...option.RequestOption) (*[]kernel.Provider, error) {
			items := []kernel.Provider{{
				Slug:            "exa",
				MaxResultsCap:   25,
				Content:         kernel.ProviderContent{Inline: true, PostHoc: false, FreshnessControl: true},
				ProviderOptions: kernel.ProviderProviderOptions{SchemaRef: "ExaOptions"},
				Params:          kernel.ProviderParams{Recency: kernel.ProviderParamsRecency{Support: "emulated", Notes: "widened to days"}},
				Notes:           []string{"neural search is slower"},
			}}
			return &items, nil
		},
	}
	s := SearchCmd{providers: fake}

	require.NoError(t, s.Providers(context.Background(), SearchProvidersInput{Slug: "exa"}))

	assert.Equal(t, kernel.SearchProviderListParamsSlugExa, fake.LastQuery.Slug)
	out := buf.String()
	assert.Contains(t, out, "exa")
	assert.Contains(t, out, "25")
	assert.Contains(t, out, "ExaOptions")
	assert.Contains(t, out, "emulated")
	assert.Contains(t, out, "neural search is slower")
}

func TestSearchProvidersRejectsUnknownSlug(t *testing.T) {
	_ = capturePtermOutput(t)
	s := SearchCmd{providers: &FakeSearchProvidersService{}}
	err := s.Providers(context.Background(), SearchProvidersInput{Slug: "altavista"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --slug")
}

func TestSearchProvidersEmptyJSON(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeSearchProvidersService{
		ListFunc: func(ctx context.Context, query kernel.SearchProviderListParams, opts ...option.RequestOption) (*[]kernel.Provider, error) {
			items := []kernel.Provider{}
			return &items, nil
		},
	}
	s := SearchCmd{providers: fake}
	require.NoError(t, s.Providers(context.Background(), SearchProvidersInput{Output: "json"}))
}

func TestSearchContentsBuildsFetchRequest(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeSearchContentsService{}
	s := SearchCmd{contents: fake}

	require.NoError(t, s.Contents(context.Background(), SearchContentsInput{
		ID:        "srch_abc",
		ResultIDs: []string{"res_2", "res_1"},
		TimeoutMs: 5000,
		Content:   searchContentInput{Source: "auto", Format: "markdown", MaxChars: 500},
	}))

	assert.Equal(t, "srch_abc", fake.LastID)
	raw, err := json.Marshal(fake.LastBody)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	assert.Equal(t, []any{"res_2", "res_1"}, body["result_ids"])
	assert.EqualValues(t, 5000, body["timeout_ms"])
	assert.Equal(t, map[string]any{"source": "auto", "format": "markdown", "max_chars": float64(500)}, body["content"])
	assert.NotContains(t, body, "limit")
}

func TestSearchContentsRejectsResultIDsWithLimit(t *testing.T) {
	_ = capturePtermOutput(t)
	s := SearchCmd{contents: &FakeSearchContentsService{}}
	err := s.Contents(context.Background(), SearchContentsInput{ID: "srch_abc", ResultIDs: []string{"res_1"}, Limit: 5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestSearchCommandFlagsAreWired(t *testing.T) {
	for _, name := range []string{
		"country", "language", "max-results", "recency", "safe-search", "start-date", "end-date",
		"include-domains", "exclude-domains", "strict-params", "include-raw", "timeout-ms",
		"content", "content-source", "content-format", "content-max-chars", "content-max-age-hours",
		"content-timeout-ms", "content-browser-id", "content-browser-mode", "show-content",
		"provider", "fallback-providers", "fallback-on", "provider-options", "output",
	} {
		assert.NotNil(t, searchCmd.Flags().Lookup(name), "kernel search is missing --%s", name)
	}
	for _, name := range []string{"result-ids", "limit", "timeout-ms", "content-source", "output"} {
		assert.NotNil(t, searchContentsCmd.Flags().Lookup(name), "kernel search contents is missing --%s", name)
	}
	assert.NotNil(t, searchProvidersCmd.Flags().Lookup("slug"))
	assert.NotNil(t, searchGetCmd.Flags().Lookup("show-content"))

	var names []string
	for _, c := range searchCmd.Commands() {
		names = append(names, c.Name())
	}
	assert.ElementsMatch(t, []string{"get", "providers", "contents"}, names, "got %s", strings.Join(names, ","))
}
