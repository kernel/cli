package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuggestUpgradeCommandForMethod(t *testing.T) {
	tests := []struct {
		method   InstallMethod
		expected string
	}{
		{InstallMethodBrew, "brew update && brew upgrade kernel/tap/kernel"},
		{InstallMethodNPM, "npm i -g @onkernel/cli@latest"},
		{InstallMethodPNPM, "pnpm add -g @onkernel/cli@latest"},
		{InstallMethodBun, "bun add -g @onkernel/cli@latest"},
		{InstallMethodUnknown, "brew update && brew upgrade kernel/tap/kernel"},
	}
	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			assert.Equal(t, tt.expected, suggestUpgradeCommandForMethod(tt.method))
		})
	}
}

func TestPathMatchesNPM(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/home/user/.npm-global/bin/kernel", true},
		{"/home/user/.npm/bin/kernel", true},
		{"/usr/local/lib/node_modules/.bin/kernel", true},
		{"/home/user/.local/share/npm/bin/kernel", true},
		{"/opt/homebrew/bin/kernel", false},
		{"/home/user/.bun/bin/kernel", false},
		{"/home/user/.local/share/pnpm/kernel", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, pathMatchesNPM(tt.path))
		})
	}
}

func TestPathMatchesBun(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/home/user/.bun/bin/kernel", true},
		{"/home/user/.npm-global/bin/kernel", false},
		{"/opt/homebrew/bin/kernel", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, pathMatchesBun(tt.path))
		})
	}
}

func TestPathMatchesPNPM(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/home/user/.local/share/pnpm/kernel", true},
		{"/home/user/.pnpm/global/kernel", true},
		{"/home/user/.npm-global/bin/kernel", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, pathMatchesPNPM(tt.path))
		})
	}
}

func TestPathMatchesHomebrew(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/opt/homebrew/bin/kernel", true},
		{"/usr/local/Cellar/kernel/1.0/bin/kernel", true},
		{"/home/linuxbrew/.linuxbrew/Cellar/kernel/1.0/bin/kernel", true},
		{"/home/user/.npm-global/bin/kernel", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, pathMatchesHomebrew(tt.path))
		})
	}
}

func TestInstallMethodRulesPathPrecedence(t *testing.T) {
	rules := installMethodRules()

	detect := func(path string) InstallMethod {
		for _, r := range rules {
			if r.check(path) {
				return r.method
			}
		}
		return InstallMethodUnknown
	}

	assert.Equal(t, InstallMethodNPM, detect("/home/user/.npm-global/bin/kernel"))
	assert.Equal(t, InstallMethodBun, detect("/home/user/.bun/bin/kernel"))
	assert.Equal(t, InstallMethodBrew, detect("/opt/homebrew/bin/kernel"))
	assert.Equal(t, InstallMethodPNPM, detect("/home/user/.local/share/pnpm/kernel"))
	assert.Equal(t, InstallMethodUnknown, detect("/usr/local/bin/kernel"))
}

func TestIsVeryOldVersion(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
		wantErr bool
	}{
		{"same version", "v0.19.1", "v0.19.1", false, false},
		{"one minor behind", "v0.18.0", "v0.19.0", false, false},
		{"four minor behind", "v0.15.0", "v0.19.0", false, false},
		{"five minor behind escalates", "v0.14.0", "v0.19.0", true, false},
		{"many minor behind", "v0.5.0", "v0.19.1", true, false},
		{"single major bump does not escalate", "v1.2.3", "v2.0.0", false, false},
		{"single major bump from 0.x does not escalate", "v0.19.2", "v1.0.0", false, false},
		{"two majors behind escalates", "v1.2.3", "v3.0.0", true, false},
		{"patch behind only", "v0.19.0", "v0.19.5", false, false},
		{"v prefix tolerated", "0.10.0", "v0.19.0", true, false},
		{"non-semver returns error", "dev", "v0.19.0", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsVeryOldVersion(tt.current, tt.latest)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
