package cmd

import (
	"context"
	"testing"

	"github.com/charmbracelet/x/ansi"
	localbrowser "github.com/kernel/cli/internal/browserimport"
	"github.com/kernel/cli/internal/passwordmanager"
	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProjectListService struct{ projects []kernel.Project }

func (f fakeProjectListService) List(context.Context, kernel.ProjectListParams, ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
	return &pagination.OffsetPagination[kernel.Project]{Items: f.projects}, nil
}

func TestNormalizeSitesFlattensDeduplicatesAndSorts(t *testing.T) {
	sites, err := normalizeSites([]string{" GitHub.com,example.com ", "github.com"})
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com", "github.com"}, sites)
}

func TestChooseImportProjectUsesOnlyActiveProject(t *testing.T) {
	project, err := chooseImportProject(t.Context(), fakeProjectListService{projects: []kernel.Project{
		{ID: "archived", Name: "Old", Status: kernel.ProjectStatusArchived},
		{ID: "active", Name: "Default", Status: kernel.ProjectStatusActive},
	}}, interactive.NewPrompterWithTerminal(false), "", false)
	require.NoError(t, err)
	assert.Equal(t, "active", project)
}

func TestChooseImportProjectRequiresFlagForMultipleNonInteractiveProjects(t *testing.T) {
	_, err := chooseImportProject(t.Context(), fakeProjectListService{projects: []kernel.Project{
		{ID: "one", Name: "One", Status: kernel.ProjectStatusActive},
		{ID: "two", Name: "Two", Status: kernel.ProjectStatusActive},
	}}, interactive.NewPrompterWithTerminal(false), "", true)
	require.ErrorContains(t, err, "pass --project")
}

func TestCompactFieldPreventsLoginRowsFromWrapping(t *testing.T) {
	assert.Equal(t, "short", compactField("short", 8))
	assert.Equal(t, "very-lo…", compactField("very-long-value", 8))
	assert.LessOrEqual(t, ansi.StringWidth(compactField("界界界界界", 6)), 6)
	assert.Equal(t, "safe fake", compactField("\x1b[31msafe\x1b[0m\n\x1b]8;;https://example.com\x07fake\x1b]8;;\x07", 20))

	label := loginCandidateLabel(1, "Bitwarden", passwordmanager.Candidate{
		Domain:   "accounts.google.com",
		Username: "same-account@example.com",
		Name:     "personal google account",
	}, "abc123")
	assert.LessOrEqual(t, ansi.StringWidth(label), 72)
	assert.Contains(t, label, "personal google")

	withoutUsername := loginCandidateLabel(2, "1Password", passwordmanager.Candidate{
		Domain: "example.com",
		Name:   "personal account",
	}, "def456")
	assert.Contains(t, withoutUsername, "no username")
	assert.Contains(t, withoutUsername, "personal account")

	for _, index := range []int{999, 9999} {
		label := loginCandidateLabel(index, "Bitwarden", passwordmanager.Candidate{
			Domain:   "accounts.google.com",
			Username: "same-account@example.com",
			Name:     "personal google account",
		}, "abc123")
		assert.LessOrEqual(t, ansi.StringWidth(label), 72)
	}
}

func TestProjectOptionLabelsAreUniqueForDuplicateNames(t *testing.T) {
	first := projectOptionLabel(0, kernel.Project{Name: "Duplicate"})
	second := projectOptionLabel(1, kernel.Project{Name: "Duplicate"})
	assert.NotEqual(t, first, second)
}

type fakePasswordManager struct {
	name       string
	candidates []passwordmanager.Candidate
	err        error
}

func (f fakePasswordManager) Name() string {
	if f.name != "" {
		return f.name
	}
	return "Bitwarden"
}
func (f fakePasswordManager) Candidates(context.Context, []string) ([]passwordmanager.Candidate, error) {
	return f.candidates, f.err
}
func (f fakePasswordManager) Reveal(_ context.Context, candidates []passwordmanager.Candidate) ([]passwordmanager.Record, error) {
	records := make([]passwordmanager.Record, 0, len(candidates))
	for _, candidate := range candidates {
		records = append(records, passwordmanager.Record{Provider: candidate.Provider, ID: candidate.ID, Domain: candidate.Domain, Username: candidate.Username, Name: candidate.Name})
	}
	return records, nil
}

func TestChooseManagedAuthLoginsRequiresChoiceForAmbiguousSite(t *testing.T) {
	records := []passwordmanager.Candidate{
		{ID: "one", Domain: "github.com", Username: "one", Name: "One"},
		{ID: "two", Domain: "github.com", Username: "two", Name: "Two"},
		{ID: "three", Domain: "example.com", Username: "three", Name: "Three"},
	}
	command := ProfilesImportLocalCmd{prompter: interactive.NewPrompterWithTerminal(false), providers: func() []passwordmanager.Provider {
		return []passwordmanager.Provider{fakePasswordManager{candidates: records}}
	}}
	selected, err := command.chooseManagedAuthLogins(context.Background(), []string{"github.com", "example.com"}, "bitwarden", true, false)
	require.Error(t, err)
	assert.Empty(t, selected.providers)
	assert.Contains(t, err.Error(), "github.com has 2 matching logins")
}

func TestChooseManagedAuthLoginsCombinesSelectedProviders(t *testing.T) {
	command := ProfilesImportLocalCmd{prompter: interactive.NewPrompterWithTerminal(false), providers: func() []passwordmanager.Provider {
		return []passwordmanager.Provider{
			fakePasswordManager{name: "Bitwarden", candidates: []passwordmanager.Candidate{{ID: "bw", Domain: "github.com", Name: "GitHub personal"}}},
			fakePasswordManager{name: "1Password", candidates: []passwordmanager.Candidate{{ID: "op", Domain: "example.com", Name: "Example work"}}},
		}
	}}

	selected, err := command.chooseManagedAuthLogins(context.Background(), []string{"github.com", "example.com"}, "bitwarden,1password", true, false)
	require.NoError(t, err)
	require.Len(t, selected.providers, 2)
	assert.Equal(t, "Bitwarden", selected.providers[0].provider.Name())
	assert.Equal(t, "github.com", selected.providers[0].candidates[0].Domain)
	assert.Equal(t, "1Password", selected.providers[1].provider.Name())
	assert.Equal(t, "example.com", selected.providers[1].candidates[0].Domain)
}

func TestChooseManagedAuthLoginsDeduplicatesRequestedProviders(t *testing.T) {
	command := ProfilesImportLocalCmd{prompter: interactive.NewPrompterWithTerminal(false), providers: func() []passwordmanager.Provider {
		return []passwordmanager.Provider{
			fakePasswordManager{name: "Bitwarden", candidates: []passwordmanager.Candidate{{ID: "bw", Domain: "github.com", Name: "GitHub personal"}}},
		}
	}}

	selected, err := command.chooseManagedAuthLogins(context.Background(), []string{"github.com"}, "bitwarden,bitwarden", true, false)
	require.NoError(t, err)
	require.Len(t, selected.providers, 1)
	require.Len(t, selected.providers[0].candidates, 1)
}

func TestChooseManagedAuthLoginsSkipsBrokenInteractiveProvider(t *testing.T) {
	broken, err := discoverProviderCandidates(t.Context(), fakePasswordManager{name: "Bitwarden", err: assert.AnError}, []string{"example.com"}, false, false)
	require.NoError(t, err)
	assert.Empty(t, broken)

	healthy, err := discoverProviderCandidates(t.Context(), fakePasswordManager{name: "1Password", candidates: []passwordmanager.Candidate{{ID: "op", Domain: "example.com"}}}, []string{"example.com"}, false, false)
	require.NoError(t, err)
	require.Len(t, healthy, 1)
}

func TestDuplicateSelectedDomainAcrossProviders(t *testing.T) {
	provider := fakePasswordManager{name: "Bitwarden"}
	candidates := map[string]sourcedPasswordManagerCandidate{
		"bitwarden": {provider: provider, candidate: passwordmanager.Candidate{Domain: "google.com"}},
		"1password": {provider: fakePasswordManager{name: "1Password"}, candidate: passwordmanager.Candidate{Domain: "google.com"}},
	}

	assert.Equal(t, "google.com", duplicateSelectedDomain([]string{"bitwarden", "1password"}, candidates))
	assert.Empty(t, duplicateSelectedDomain([]string{"bitwarden"}, candidates))
}

func TestChooseSitesUsesRequestedDomainsWithoutPrompting(t *testing.T) {
	command := ProfilesImportLocalCmd{prompter: interactive.NewPrompterWithTerminal(false)}
	selected, err := command.chooseSites(nil, []string{"github.com"}, 5, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"github.com"}, selected)
}

func TestChooseSitesUsesTopFiveWithYes(t *testing.T) {
	recent := make([]localbrowser.Site, 0, 7)
	for _, domain := range []string{"one.com", "two.com", "three.com", "four.com", "five.com", "six.com", "seven.com"} {
		recent = append(recent, localbrowser.Site{Domain: domain})
	}
	command := ProfilesImportLocalCmd{prompter: interactive.NewPrompterWithTerminal(false)}
	selected, err := command.chooseSites(recent, nil, 5, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"one.com", "two.com", "three.com", "four.com", "five.com"}, selected)
}

func TestCookieSiteLabelShowsRankingAndCookieCount(t *testing.T) {
	label := cookieSiteLabel(localbrowser.Site{Domain: "google.com", Visits: 2347, CookieCount: 64})
	assert.Contains(t, label, "google.com")
	assert.Contains(t, label, "2347 visits")
	assert.Contains(t, label, "64 cookies")
	assert.LessOrEqual(t, ansi.StringWidth(label), 64)
	assert.LessOrEqual(t, ansi.StringWidth(cookieSiteLabel(localbrowser.Site{Domain: "界界界界界界界界界界界界界界界界", Visits: int(^uint(0) >> 1), CookieCount: int(^uint(0) >> 1)})), 64)
	assert.Equal(t, "1.00e+09", boundedCount(1_000_000_000))
}

func TestSitesWithCookiesOmitsEmptySitesAndPreservesRanking(t *testing.T) {
	sites := sitesWithCookies([]localbrowser.Site{
		{Domain: "first.com", Visits: 10, CookieCount: 2},
		{Domain: "empty.com", Visits: 9},
		{Domain: "second.com", Visits: 8, CookieCount: 1},
	})
	assert.Equal(t, []string{"first.com", "second.com"}, []string{sites[0].Domain, sites[1].Domain})
}

func TestChooseSitesFailsFastWithoutTTYOrFlags(t *testing.T) {
	command := ProfilesImportLocalCmd{prompter: interactive.NewPrompterWithTerminal(false)}
	_, err := command.chooseSites([]localbrowser.Site{{Domain: "example.com"}}, nil, 5, false)
	var promptError *interactive.PromptError
	require.ErrorAs(t, err, &promptError)
	assert.Contains(t, promptError.Error(), "pass --sites or --yes")
}

func TestDefaultImportedProfileName(t *testing.T) {
	profile := localbrowser.Profile{Name: "Ilyaas Personal", Browser: localbrowser.Browser{ID: "chrome"}}
	assert.Equal(t, "chrome-ilyaas-personal", defaultImportedProfileName(profile))
}

func TestProfilesImportLocalRejectsUnsupportedOutputBeforeDiscovery(t *testing.T) {
	command := ProfilesImportLocalCmd{prompter: interactive.NewPrompterWithTerminal(false)}
	err := command.Run(t.Context(), ProfilesImportLocalInput{Output: "yaml", Count: 5, Days: 30})
	assert.EqualError(t, err, `unsupported --output value "yaml"; use "json" or omit --output for human-readable output`)
}

func TestProfilesImportStatusRejectsUnsupportedOutputBeforeAuthentication(t *testing.T) {
	profilesImportStatusCmd.Flags().Set("output", "yaml")
	t.Cleanup(func() { _ = profilesImportStatusCmd.Flags().Set("output", "") })
	err := runProfilesImportStatus(profilesImportStatusCmd, []string{"imp_test"})
	assert.EqualError(t, err, `unsupported --output value "yaml"; use "json" or omit --output for human-readable output`)
}

func TestChooseProfileRejectsDuplicateFriendlyName(t *testing.T) {
	profiles := []localbrowser.Profile{
		{ID: "one", Name: "Personal", Browser: localbrowser.Browser{Name: "Google Chrome"}},
		{ID: "two", Name: "Personal", Browser: localbrowser.Browser{Name: "Google Chrome"}},
	}
	command := ProfilesImportLocalCmd{prompter: interactive.NewPrompterWithTerminal(false)}
	_, err := command.chooseProfile(profiles, "Google Chrome / Personal")
	assert.EqualError(t, err, `browser profile "Google Chrome / Personal" is ambiguous; use its profile ID`)
}
