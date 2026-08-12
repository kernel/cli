package cmd

import (
	"testing"

	localbrowser "github.com/kernel/cli/internal/browserimport"
	"github.com/kernel/cli/pkg/interactive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSitesFlattensDeduplicatesAndSorts(t *testing.T) {
	assert.Equal(t, []string{"example.com", "github.com"}, normalizeSites([]string{" GitHub.com,example.com ", "github.com"}))
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
