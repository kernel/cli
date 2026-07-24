package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain doubles as an entry point for end-to-end subprocess tests: when
// KERNEL_CLI_E2E_EXEC=1 the test binary runs the real CLI (cmd.Execute) with
// os.Args instead of the test suite, so tests can assert on the final
// rendered stdout/stderr and exit code.
func TestMain(m *testing.M) {
	if os.Getenv("KERNEL_CLI_E2E_EXEC") == "1" {
		Execute(Metadata{Version: "0.0.0-test"})
		return
	}
	os.Exit(m.Run())
}

// forceNonInteractive arranges the condition under test explicitly instead of
// relying on the harness's ambient stdin (which may be a PTY).
func forceNonInteractive(t *testing.T) {
	t.Helper()
	t.Cleanup(interactive.ForceTerminal(false))
}

// Any command path that would show an interactive confirmation must fail fast
// with a --yes hint instead of prompting (which would otherwise hang forever
// in agent/CI shells).

func TestAPIKeysDeleteFailsFastWhenNonInteractive(t *testing.T) {
	forceNonInteractive(t)
	_ = capturePtermOutput(t)
	fake := &FakeAPIKeysService{
		DeleteFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			t.Fatal("delete must not be called without confirmation")
			return nil
		},
	}
	c := APIKeysCmd{apiKeys: fake}

	err := c.Delete(context.Background(), APIKeysDeleteInput{ID: "key_123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete API key 'key_123'")
	assert.Contains(t, err.Error(), "--yes")
	assert.Contains(t, err.Error(), "not an interactive terminal")
}

func TestExtensionsDeleteFailsFastWhenNonInteractive(t *testing.T) {
	forceNonInteractive(t)
	_ = capturePtermOutput(t)
	fake := &FakeExtensionsService{
		DeleteFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
			t.Fatal("delete must not be called without confirmation")
			return nil
		},
	}
	e := ExtensionsCmd{extensions: fake}

	err := e.Delete(context.Background(), ExtensionsDeleteInput{Identifier: "e1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete extension 'e1'")
	assert.Contains(t, err.Error(), "--yes")
}

// In a non-interactive shell, `kernel create` must report every missing or
// invalid input in a single error so one retry can fix everything.
func TestCreateFailsFastAggregatedWhenNonInteractive(t *testing.T) {
	forceNonInteractive(t)

	newCreateCmd := func(flags map[string]string) *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().String("name", "", "")
		cmd.Flags().String("language", "", "")
		cmd.Flags().String("template", "", "")
		cmd.Flags().Bool("yes", false, "")
		for flag, value := range flags {
			require.NoError(t, cmd.Flags().Set(flag, value))
		}
		return cmd
	}

	t.Run("no flags reports all three inputs in one error", func(t *testing.T) {
		err := runCreateApp(newCreateCmd(nil), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--name is required")
		assert.Contains(t, err.Error(), "--language is required")
		assert.Contains(t, err.Error(), "--template is required")
		assert.NotContains(t, err.Error(), "failed to get")
	})

	t.Run("mixed missing and invalid flags aggregated", func(t *testing.T) {
		err := runCreateApp(newCreateCmd(map[string]string{"language": "ruby", "template": "nope"}), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--name is required")
		assert.Contains(t, err.Error(), "--language 'ruby' is invalid")
		assert.Contains(t, err.Error(), "--template 'nope' is invalid")
	})

	t.Run("single problem stays a single-line error", func(t *testing.T) {
		err := runCreateApp(newCreateCmd(map[string]string{"name": "my-app", "language": "typescript", "template": "nope"}), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--template 'nope' is invalid for language 'typescript'")
		assert.NotContains(t, err.Error(), "\n")
	})
}

var ansiEscapes = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]|\x1b\][^\x07]*\x07`)

// TestCreateNonInteractiveE2E runs the real CLI as a subprocess with stdin
// explicitly connected to /dev/null and asserts on the final rendered stderr:
// the process must exit 1 promptly (not hang on a prompt) and every flag
// token must survive rendering intact — i.e. not be split across lines by
// terminal-width re-wrapping.
func TestCreateNonInteractiveE2E(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	devNull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer devNull.Close()

	cmd := exec.CommandContext(ctx, exe, "create", "--language", "ruby")
	cmd.Env = append(os.Environ(), "KERNEL_CLI_E2E_EXEC=1", "KERNEL_NO_UPDATE_CHECK=1")
	cmd.Stdin = devNull
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	require.NoError(t, ctx.Err(), "CLI hung instead of failing fast; stderr=%q", stderr.String())
	var exitErr *exec.ExitError
	require.ErrorAs(t, runErr, &exitErr, "expected non-zero exit; stdout=%q stderr=%q", stdout.String(), stderr.String())
	assert.Equal(t, 1, exitErr.ExitCode())

	rendered := ansiEscapes.ReplaceAllString(stderr.String(), "")
	for _, token := range []string{
		"--name is required",
		"--language 'ruby' is invalid",
		"--template is required",
		"not an interactive terminal",
	} {
		assert.Contains(t, rendered, token, "rendered stderr must contain %q intact; full stderr:\n%s", token, rendered)
	}
	// Each problem renders on its own line (no width-based re-wrapping that
	// would split flag tokens or merge problems).
	for _, line := range []string{
		"\n  - --name is required",
		"\n  - --language 'ruby' is invalid",
		"\n  - --template is required",
	} {
		assert.Contains(t, rendered, line, "each problem must start its own line; full stderr:\n%s", rendered)
	}
}
