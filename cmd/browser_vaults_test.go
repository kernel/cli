package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBrowserVaults(t *testing.T) {
	const id = "abcdefghijklmnopqrstuvwx"
	refs, err := buildBrowserVaults([]string{id, "checkout"})
	require.NoError(t, err)
	require.Len(t, refs, 2)
	assert.Equal(t, id, refs[0].ID.Value)
	assert.False(t, refs[0].Name.Valid())
	assert.Equal(t, "checkout", refs[1].Name.Value)
	assert.False(t, refs[1].ID.Valid())
	for _, values := range [][]string{{""}, {" "}, {"../checkout"}, {".."}, {"checkout", "checkout"}, make([]string, 21)} {
		_, err := buildBrowserVaults(values)
		require.Error(t, err)
	}
	refs, err = buildBrowserVaults(nil)
	require.NoError(t, err)
	body, err := json.Marshal(kernel.BrowserNewParams{Vaults: refs})
	require.NoError(t, err)
	assert.NotContains(t, string(body), "vaults")
	assert.NotNil(t, browsersCreateCmd.Flags().Lookup("vault"))
	assert.Nil(t, browsersUpdateCmd.Flags().Lookup("vault"))
	assert.False(t, poolLeaseAllowedFlags()["vault"])
}

func browserVaultTestCommand(client kernel.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("project", "", "")
	cmd.Flags().StringArray("vault", nil, "")
	cmd.Flags().String("pool-id", "", "")
	cmd.Flags().String("pool-name", "", "")
	cmd.Flags().Bool("yes", false, "")
	addJSONOutputFlag(cmd)
	cmd.SetContext(context.WithValue(context.Background(), util.KernelClientKey, client))
	return cmd
}

func TestBrowserVaultPoolAndProjectValidation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid attachment reached API") })
	for _, flags := range [][]string{
		{"--vault", "checkout", "--pool-id", "pool-1", "--yes"},
		{"--vault", "checkout", "--pool-name", "pool", "--yes"},
		{"--vault", "checkout"},
		{"--vault=", "--project", "project-test"},
	} {
		cmd := browserVaultTestCommand(client)
		require.NoError(t, cmd.ParseFlags(flags))
		err := runBrowsersCreate(cmd, nil)
		require.Error(t, err)
	}
}

func TestBrowserCreateVaultRequestAndReturnedAttachments(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	const body = `{"session_id":"browser-1","cdp_ws_url":"ws://example.test/cdp","vaults":[{"id":"vault-1","name":"checkout"}]}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/browsers", r.URL.Path)
		payload, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"vaults":[{"name":"checkout"}]}`, string(payload))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
	for _, output := range []string{"", "json"} {
		cmd := browserVaultTestCommand(client)
		require.NoError(t, cmd.Flags().Set("vault", "checkout"))
		require.NoError(t, cmd.Flags().Set("output", output))
		buf := capturePtermOutput(t)
		out := captureStdout(t, func() { require.NoError(t, runBrowsersCreate(cmd, nil)) })
		if output == "json" {
			assert.JSONEq(t, body, out)
		} else {
			assert.Contains(t, buf.String(), "Attached vault ID")
			assert.Contains(t, buf.String(), "vault-1")
			assert.Contains(t, buf.String(), "checkout")
		}
	}
}

func TestBrowserCreateInvalidVaultNeverCallsSDK(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{NewFunc: func(ctx context.Context, body kernel.BrowserNewParams, opts ...option.RequestOption) (*kernel.BrowserNewResponse, error) {
		t.Fatal("invalid vault reference should not reach SDK")
		return nil, nil
	}}}
	require.Error(t, b.Create(context.Background(), BrowsersCreateInput{Vaults: []string{strings.Repeat("x", 256)}}))
}
