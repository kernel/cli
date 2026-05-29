package util

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserErrorHelpers(t *testing.T) {
	assert.Equal(t, "--name is required; add --name <name>", RequiredFlag("--name", "<name>").Error())
	assert.Equal(t, "missing browser ID; use: kernel browsers get <id>", RequiredArg("browser ID", "kernel browsers get <id>").Error())
	assert.Equal(t, "choose only one of --profile-id or --profile-name", ChooseOnlyOne("--profile-id", "--profile-name").Error())
	assert.Equal(t, "set at least one of --proxy-id, --profile-id, or --viewport", SetAtLeastOne("--proxy-id", "--profile-id", "--viewport").Error())
	assert.Equal(t, "invalid --status \"bad\"; use one of: active, deleted, all", InvalidChoice("--status", "bad", "active", "deleted", "all").Error())
	assert.Equal(t, "Browser \"brw_123\" not found; run `kernel browsers list` to find valid IDs", NotFound("Browser", "brw_123", "kernel browsers list").Error())
}

func TestCleanedUpSdkErrorPreservesOuterContext(t *testing.T) {
	apiErr := newKernelError(t, `{"code":"not_found","message":"missing app"}`)

	assert.Equal(t, "not_found: missing app", CleanedUpSdkError{Err: apiErr}.Error())
	assert.Equal(t,
		"list applications failed; check your auth and retry: not_found: missing app",
		CleanedUpSdkError{Err: fmt.Errorf("list applications failed; check your auth and retry: %w", apiErr)}.Error(),
	)
}

func TestCleanedUpSdkErrorExplainsDashboardBaseURL(t *testing.T) {
	t.Setenv("KERNEL_BASE_URL", "https://dashboard.onkernel.com")

	err := CleanedUpSdkError{
		Err: fmt.Errorf("list applications failed; check your auth and retry: expected destination type of 'string' or '[]byte' for responses with content-type 'text/html; charset=utf-8' that is not 'application/json'"),
	}

	assert.Equal(t,
		"list applications failed: server returned HTML instead of Kernel API JSON; KERNEL_BASE_URL resolves to https://dashboard.onkernel.com. Use an API base URL, not the dashboard URL. For production, unset KERNEL_BASE_URL or set it to https://api.onkernel.com.",
		err.Error(),
	)
}

func newKernelError(t *testing.T, raw string) *kernel.Error {
	t.Helper()

	var err kernel.Error
	require.NoError(t, json.Unmarshal([]byte(raw), &err))
	req, reqErr := http.NewRequest(http.MethodGet, "https://api.example.test/apps", nil)
	require.NoError(t, reqErr)
	err.Request = req
	err.Response = &http.Response{StatusCode: http.StatusNotFound}
	return &err
}
