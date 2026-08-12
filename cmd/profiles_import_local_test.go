package cmd

import (
	"context"
	"testing"

	localbrowser "github.com/kernel/cli/internal/browserimport"
	"github.com/kernel/cli/internal/passwordmanager"
	"github.com/kernel/cli/pkg/interactive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSitesFlattensDeduplicatesAndSorts(t *testing.T) {
	sites, err := normalizeSites([]string{" GitHub.com,example.com ", "github.com"})
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com", "github.com"}, sites)
}

type fakePasswordManager struct{ candidates []passwordmanager.Candidate }

func (fakePasswordManager) Name() string { return "Bitwarden" }
func (f fakePasswordManager) Candidates(context.Context, []string) ([]passwordmanager.Candidate, error) {
	return f.candidates, nil
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
	assert.Empty(t, selected.candidates)
	assert.Contains(t, err.Error(), "github.com has 2 matching logins")
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
