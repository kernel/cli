package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkloadIdentityCommandMethodCoverage(t *testing.T) {
	assertCommandNames(t, serviceAccountsCmd, []string{"create", "delete", "get", "list", "update"})
	assertCommandNames(t, workloadIdentityCmd, []string{"authentication-events", "federation-rules", "issuers"})
	assertCommandNames(t, workloadIdentityIssuersCmd, []string{"create", "delete", "get", "list", "update", "verify"})
	assertCommandNames(t, workloadIdentityFederationRulesCmd, []string{"create", "delete", "get", "list", "update"})
	assertCommandNames(t, workloadIdentityAuthenticationEventsCmd, []string{"list"})
}

func TestWorkloadIdentityCommandParameterCoverage(t *testing.T) {
	assertLocalFlags(t, serviceAccountsCreateCmd, []string{"description", "name", "output"})
	assertLocalFlags(t, serviceAccountsUpdateCmd, []string{"description", "expected-version", "name", "output"})
	assertLocalFlags(t, serviceAccountsListCmd, []string{"output", "page", "per-page", "query", "status"})

	issuerMutationFlags := []string{
		"description", "enabled", "issuer-url", "jwks-ca-cert-pem", "jwks-discovery-base-url", "jwks-keys", "jwks-type", "jwks-url",
		"name", "output", "preset-type", "token-max-lifetime-seconds", "token-replay-protection",
	}
	assertLocalFlags(t, workloadIdentityIssuersCreateCmd, issuerMutationFlags)
	assertLocalFlags(t, workloadIdentityIssuersUpdateCmd, append(append([]string{}, issuerMutationFlags...), "expected-version"))
	assertLocalFlags(t, workloadIdentityIssuersListCmd, []string{"enabled", "output", "page", "per-page", "query", "status"})

	ruleMutationFlags := []string{
		"description", "enabled", "expected-audience", "issuer-id", "match-claim", "match-environment", "match-event-name",
		"match-job-workflow-ref", "match-namespace", "match-ref", "match-repository", "match-repository-id", "match-repository-owner-id",
		"match-service-account", "match-subject", "match-subject-prefix", "match-type", "match-workflow", "match-workflow-ref",
		"max-token-ttl-seconds", "name", "output", "policy", "project-id", "service-account-id", "token-audience",
	}
	assertLocalFlags(t, workloadIdentityFederationRulesCreateCmd, ruleMutationFlags)
	assertLocalFlags(t, workloadIdentityFederationRulesUpdateCmd, append(append([]string{}, ruleMutationFlags...), "expected-version"))
	assertLocalFlags(t, workloadIdentityFederationRulesListCmd, []string{
		"issuer-id", "output", "page", "per-page", "project-id", "query", "service-account-id", "status",
	})
	assertLocalFlags(t, workloadIdentityAuthenticationEventsListCmd, []string{
		"end-time", "federation-rule-id", "issuer-id", "outcome", "output", "page", "per-page", "project-id", "service-account-id", "start-time",
	})
}

func TestWorkloadIdentityListQueryMapping(t *testing.T) {
	t.Run("service accounts", func(t *testing.T) {
		capturePtermOutput(t)
		server := newWorkloadIdentityListServer(t, "/org/service_accounts", map[string]string{
			"limit": "3", "offset": "4", "query": "deploy", "status": "all",
		})
		defer server.Close()
		client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
		err := (ServiceAccountsCmd{serviceAccounts: &client.ServiceAccounts}).List(context.Background(), ServiceAccountsListInput{
			Page: 3, PerPage: 2, Query: "deploy", Status: "all",
		})
		require.NoError(t, err)
	})

	t.Run("issuers", func(t *testing.T) {
		capturePtermOutput(t)
		server := newWorkloadIdentityListServer(t, "/org/workload_identity_issuers", map[string]string{
			"limit": "4", "offset": "3", "query": "github", "status": "archived", "enabled": "false",
		})
		defer server.Close()
		client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
		err := (WorkloadIdentityIssuersCmd{issuers: &client.WorkloadIdentityIssuers}).List(context.Background(), WorkloadIdentityIssuersListInput{
			Page: 2, PerPage: 3, Query: "github", Status: "archived", Enabled: BoolFlag{Set: true, Value: false},
		})
		require.NoError(t, err)
	})

	t.Run("federation rules", func(t *testing.T) {
		capturePtermOutput(t)
		server := newWorkloadIdentityListServer(t, "/org/workload_identity_federation_rules", map[string]string{
			"limit": "5", "offset": "4", "issuer_id": "iss_123", "project_id": "proj_123", "query": "release",
			"service_account_id": "sa_123", "status": "active",
		})
		defer server.Close()
		client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
		err := (WorkloadIdentityFederationRulesCmd{rules: &client.WorkloadIdentityFederationRules}).List(context.Background(), WorkloadIdentityFederationRulesListInput{
			Page: 2, PerPage: 4, IssuerID: "iss_123", ProjectID: "proj_123", Query: "release", ServiceAccountID: "sa_123", Status: "active",
		})
		require.NoError(t, err)
	})

	t.Run("authentication events", func(t *testing.T) {
		capturePtermOutput(t)
		server := newWorkloadIdentityListServer(t, "/org/workload_identity_authentication_events", map[string]string{
			"limit": "6", "offset": "5", "issuer_id": "iss_123", "federation_rule_id": "rule_123",
			"service_account_id": "sa_123", "project_id": "proj_123", "outcome": "failure",
			"start_time": "2026-01-01T00:00:00Z", "end_time": "2026-01-02T00:00:00Z",
		})
		defer server.Close()
		client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
		err := (WorkloadIdentityAuthenticationEventsCmd{events: &client.WorkloadIdentityAuthenticationEvents}).List(context.Background(), WorkloadIdentityAuthenticationEventsListInput{
			Page: 2, PerPage: 5, IssuerID: "iss_123", FederationRuleID: "rule_123", ServiceAccountID: "sa_123",
			ProjectID: "proj_123", Outcome: "failure", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-02T00:00:00Z",
		})
		require.NoError(t, err)
	})
}

func TestBuildWorkloadIdentityIssuerNestedParams(t *testing.T) {
	url := "https://issuer.example/jwks.json"
	discovery := "https://issuer.example"
	ca := "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
	keys := `[{"kty":"RSA","kid":"one","n":"abc","e":"AQAB"}]`
	jwks, set, err := buildWorkloadIdentityKeySource(WorkloadIdentityKeySourceInput{
		Set: true, Type: "inline", URL: &url, DiscoveryBaseURL: &discovery, CACertPEM: &ca, KeysJSON: &keys,
	})
	require.NoError(t, err)
	assert.True(t, set)
	payload := marshalParamObject(t, jwks)
	assert.Equal(t, "inline", payload["type"])
	assert.Equal(t, url, payload["url"])
	assert.Equal(t, discovery, payload["discovery_base_url"])
	assert.Equal(t, ca, payload["ca_cert_pem"])
	require.Len(t, payload["keys"], 1)

	replay := "allow_reuse"
	validation, set, err := buildWorkloadIdentityTokenValidation(WorkloadIdentityTokenValidationInput{
		Set:                true,
		MaxLifetimeSeconds: Int64Flag{Set: true, Value: 900},
		ReplayProtection:   &replay,
	})
	require.NoError(t, err)
	assert.True(t, set)
	payload = marshalParamObject(t, validation)
	assert.Equal(t, float64(900), payload["max_lifetime_seconds"])
	assert.Equal(t, "allow_reuse", payload["replay_protection"])
}

func TestBuildWorkloadIdentityRuleNestedParams(t *testing.T) {
	subject := "repo:kernel/cli:ref:refs/heads/main"
	subjectPrefix := "repo:kernel/"
	repository := "kernel/cli"
	repositoryID := "123"
	repositoryOwnerID := "456"
	workflow := "release"
	workflowRef := "kernel/cli/.github/workflows/release.yml@refs/heads/main"
	jobWorkflowRef := "kernel/shared/.github/workflows/deploy.yml@refs/heads/main"
	ref := "refs/heads/main"
	environment := "production"
	namespace := "default"
	serviceAccount := "deployer"
	match, set, err := buildWorkloadIdentityMatch(WorkloadIdentityMatchInput{
		Set: true, Type: "github_actions", Subject: &subject, SubjectPrefix: &subjectPrefix,
		Claims: map[string]string{"/actor_id": "1234"}, ClaimsSet: true,
		Repository: &repository, RepositoryID: &repositoryID, RepositoryOwnerID: &repositoryOwnerID,
		Workflow: &workflow, WorkflowRef: &workflowRef, JobWorkflowRef: &jobWorkflowRef, Ref: &ref,
		Environment: &environment, EventNames: []string{"push", "workflow_dispatch"}, EventNamesSet: true,
		Namespace: &namespace, ServiceAccount: &serviceAccount,
	})
	require.NoError(t, err)
	assert.True(t, set)
	payload := marshalParamObject(t, match)
	assert.Equal(t, "github_actions", payload["type"])
	assert.Equal(t, subject, payload["subject"])
	assert.Equal(t, subjectPrefix, payload["subject_prefix"])
	assert.Equal(t, repository, payload["repository"])
	assert.Equal(t, repositoryID, payload["repository_id"])
	assert.Equal(t, repositoryOwnerID, payload["repository_owner_id"])
	assert.Equal(t, workflow, payload["workflow"])
	assert.Equal(t, workflowRef, payload["workflow_ref"])
	assert.Equal(t, jobWorkflowRef, payload["job_workflow_ref"])
	assert.Equal(t, ref, payload["ref"])
	assert.Equal(t, environment, payload["environment"])
	assert.Equal(t, namespace, payload["namespace"])
	assert.Equal(t, serviceAccount, payload["service_account"])
	assert.Equal(t, map[string]any{"/actor_id": "1234"}, payload["claims"])
	assert.Equal(t, []any{"push", "workflow_dispatch"}, payload["event_names"])

	authorization, set, err := buildWorkloadIdentityAuthorization(WorkloadIdentityAuthorizationInput{
		Set:           true,
		ProjectID:     "proj_123",
		Policy:        `{"statements":[{"action":"browsers:use","resource":"projects/proj_123/browsers/*"}]}`,
		PolicySet:     true,
		TokenAudience: "https://api.kernel.sh",
		MaxTokenTTLSeconds: Int64Flag{
			Set: true, Value: 300,
		},
	})
	require.NoError(t, err)
	assert.True(t, set)
	payload = marshalParamObject(t, authorization)
	assert.Equal(t, "proj_123", payload["project_id"])
	assert.Equal(t, "https://api.kernel.sh", payload["token_audience"])
	assert.Equal(t, float64(300), payload["max_token_ttl_seconds"])
	policy, ok := payload["policy"].(map[string]any)
	require.True(t, ok)
	statements, ok := policy["statements"].([]any)
	require.True(t, ok)
	require.Len(t, statements, 1)
}

func TestWorkloadIdentityValidationMatchesSpecBounds(t *testing.T) {
	_, _, err := buildWorkloadIdentityKeySource(WorkloadIdentityKeySourceInput{Set: true, Type: "inline", KeysJSON: stringPointer(`[null]`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not null")

	_, _, err = buildWorkloadIdentityTokenValidation(WorkloadIdentityTokenValidationInput{
		Set: true, MaxLifetimeSeconds: Int64Flag{Set: true, Value: 86401},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "86400")

	_, _, err = buildWorkloadIdentityAuthorization(WorkloadIdentityAuthorizationInput{
		Set: true, ProjectID: "proj_123", Policy: `{"statements":[{"action":"browsers:use","resource":"projects/proj_123/browsers/*"}]}`,
		PolicySet: true, TokenAudience: "aud", MaxTokenTTLSeconds: Int64Flag{Set: true, Value: 601},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "600")
}

func newWorkloadIdentityListServer(t *testing.T, expectedPath string, expectedQuery map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assert.Equal(t, expectedPath, request.URL.Path)
		for key, value := range expectedQuery {
			assert.Equal(t, value, request.URL.Query().Get(key), key)
		}
		assert.Len(t, request.URL.Query(), len(expectedQuery))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
}

func assertCommandNames(t *testing.T, command *cobra.Command, expected []string) {
	t.Helper()
	actual := make([]string, 0, len(command.Commands()))
	for _, child := range command.Commands() {
		if child.Name() != "help" {
			actual = append(actual, child.Name())
		}
	}
	assert.ElementsMatch(t, expected, actual)
}

func assertLocalFlags(t *testing.T, command *cobra.Command, expected []string) {
	t.Helper()
	actual := []string{}
	command.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		actual = append(actual, flag.Name)
	})
	assert.ElementsMatch(t, expected, actual)
}

func marshalParamObject(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	return payload
}

func stringPointer(value string) *string { return &value }
