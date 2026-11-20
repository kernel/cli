package cmd

import (
	"testing"

	"github.com/kernel/cli/pkg/auth"
)

func TestAccessScopeLabel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tokens auth.TokenStorage
		want   string
	}{
		{"legacy project scope", auth.TokenStorage{AccessScope: "project", ProjectID: "proj_1"}, "project proj_1"},
		{"native grant for one project", auth.TokenStorage{AccessScope: "organization", Projects: []string{"proj_a"}}, "project proj_a"},
		{"native grant for several projects", auth.TokenStorage{AccessScope: "organization", Projects: []string{"proj_a", "proj_b"}}, "projects proj_a, proj_b"},
		{"every project", auth.TokenStorage{AccessScope: "organization"}, "organization-wide"},
	} {
		if got := accessScopeLabel(&tc.tokens); got != tc.want {
			t.Errorf("%s: accessScopeLabel() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
