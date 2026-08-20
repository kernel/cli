package agentskills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectAndInstallOnlyExistingAgentDirectories(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".codex"), 0o755))
	targets := Detect(root, t.TempDir())
	require.Len(t, targets, 1)
	assert.Equal(t, "Codex", targets[0].Agent)
	require.NoError(t, Install(targets))
	data, err := os.ReadFile(filepath.Join(root, ".codex", "skills", "kernel-managed-auth", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "kernel auth connections login")
}

func TestInstallRefusesCustomizedSkill(t *testing.T) {
	root := t.TempDir()
	target := Target{Agent: "Codex", Path: filepath.Join(root, ".codex", "skills")}
	directory := filepath.Join(target.Path, "kernel-managed-auth")
	require.NoError(t, os.MkdirAll(directory, 0o755))
	destination := filepath.Join(directory, "SKILL.md")
	require.NoError(t, os.WriteFile(destination, []byte("my custom skill"), 0o600))

	err := Install([]Target{target})
	require.ErrorContains(t, err, "refusing to overwrite customized skill")
	data, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	assert.Equal(t, "my custom skill", string(data))
}

func TestInstallRefusesSymlinkedSkillRoot(t *testing.T) {
	root := t.TempDir()
	realDirectory := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(realDirectory, 0o755))
	symlink := filepath.Join(root, "skills")
	require.NoError(t, os.Symlink(realDirectory, symlink))

	err := Install([]Target{{Agent: "Codex", Path: symlink}})
	require.ErrorContains(t, err, "refusing symlinked skill path")
}
