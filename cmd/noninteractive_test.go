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

// runCreateApp must return prompt errors unwrapped so the CLI output matches
// the documented fail-fast messages ("cannot prompt for ...", "invalid --...")
// without "failed to get ..." prefixes.
func TestCreateFailsFastUnwrappedWhenNonInteractive(t *testing.T) {
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

	t.Run("missing app name", func(t *testing.T) {
		err := runCreateApp(newCreateCmd(nil), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot prompt for app name")
		assert.NotContains(t, err.Error(), "failed to get")
	})

	t.Run("invalid language", func(t *testing.T) {
		err := runCreateApp(newCreateCmd(map[string]string{"name": "my-app", "language": "ruby"}), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --language 'ruby'")
		assert.NotContains(t, err.Error(), "failed to get")
	})

	t.Run("invalid template", func(t *testing.T) {
		err := runCreateApp(newCreateCmd(map[string]string{"name": "my-app", "language": "typescript", "template": "nope"}), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --template 'nope'")
		assert.NotContains(t, err.Error(), "failed to get")
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
