package cmd

import (
	"context"
	"testing"

	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Under `go test` stdin is not a terminal, so any command path that would
// show an interactive confirmation must fail fast with a --yes hint instead
// of prompting (which would otherwise hang forever in agent/CI shells).

func TestAPIKeysDeleteFailsFastWhenNonInteractive(t *testing.T) {
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

// In a non-interactive shell, `kernel create` must report every missing or
// invalid input in a single unwrapped error (no "failed to get ..."
// prefixes), so one retry can fix everything.
func TestCreateFailsFastAggregatedWhenNonInteractive(t *testing.T) {
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

func TestExtensionsDeleteFailsFastWhenNonInteractive(t *testing.T) {
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
