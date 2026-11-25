package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/kernel/cli/pkg/auth"
	"github.com/pterm/pterm"
	"github.com/zalando/go-keyring"
)

func TestAuthReportsTheSourceCommandsUse(t *testing.T) {
	for _, tc := range []struct {
		name                string
		apiKey, rule, login bool
		env                 map[string]string
		want                []string
		notWant             string
	}{
		{name: "API key first", apiKey: true, rule: true, login: true,
			want: []string{"Authentication method: API Key"}, notWant: "Workload identity"},
		{name: "workload identity before a saved login", rule: true, login: true,
			want: []string{
				"Authentication method: Workload identity federation",
				"Federation rule: wifr_status",
				"Cached token: not in use; the next command exchanges a new one",
			}, notWant: "OAuth"},
		{name: "workload identity with the cache off", rule: true,
			env:  map[string]string{auth.EnvFederationTokenCache: "off"},
			want: []string{"Cached token: not in use (turned off by KERNEL_FEDERATION_TOKEN_CACHE); every command exchanges a new one"}},
		{name: "workload identity without an identity token source", rule: true, login: true,
			env:  map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": ""},
			want: []string{"Federation rule: wifr_status", "no identity token source could be configured"}, notWant: "OAuth"},
		{name: "saved login", login: true,
			want: []string{"Authenticated with OAuth"}, notWant: "Workload identity"},
		{name: "nothing", want: []string{"not authenticated"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			keyring.MockInit()
			env := map[string]string{
				"HOME": home, "USERPROFILE": home,
				"KERNEL_API_KEY": "", "KERNEL_FEDERATION_RULE_ID": "",
				"KERNEL_IDENTITY_TOKEN_SOURCE": "", "KERNEL_IDENTITY_TOKEN_FILE": "", "KERNEL_IDENTITY_TOKEN": "",
				auth.EnvFederationTokenCache: "",
				"GITHUB_ACTIONS":             "true", "RUNNER_TEMP": t.TempDir(),
				"ACTIONS_ID_TOKEN_REQUEST_URL": "http://127.0.0.1:1/idtoken", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "request-credential",
			}
			if tc.apiKey {
				env["KERNEL_API_KEY"] = "sk_test_0123456789"
			}
			if tc.rule {
				env["KERNEL_FEDERATION_RULE_ID"] = "wifr_status"
			}
			for name, value := range tc.env {
				env[name] = value
			}
			for name, value := range env {
				t.Setenv(name, value)
			}
			if tc.login {
				if err := auth.SaveTokens(&auth.TokenStorage{AccessToken: "login-token", ExpiresAt: time.Now().Add(48 * time.Hour)}); err != nil {
					t.Fatal(err)
				}
			}
			previous := logger
			logger = pterm.DefaultLogger.WithLevel(pterm.LogLevelWarn)
			t.Cleanup(func() { logger = previous })
			output := capturePtermOutput(t)

			if err := runAuth(authCmd, nil); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("output does not contain %q:\n%s", want, output.String())
				}
			}
			if tc.notWant != "" && strings.Contains(output.String(), tc.notWant) {
				t.Errorf("output contains %q:\n%s", tc.notWant, output.String())
			}
			for _, secret := range []string{"login-token", "request-credential", "sk_test_0123456789"} {
				if strings.Contains(output.String(), secret) {
					t.Errorf("output contains the secret %q:\n%s", secret, output.String())
				}
			}
		})
	}
}

func TestAuthReportsACachedWorkloadIdentityToken(t *testing.T) {
	output := capturePtermOutput(t)
	printFederationStatus(&auth.FederationStatus{RuleID: "wifr_status", CachedTokenExpiresAt: time.Now().Add(3 * time.Minute)})
	if !strings.Contains(output.String(), "Cached token: in use, expires in 3m0s") {
		t.Fatalf("output = %s, want the cached token reported in use", output.String())
	}
}
