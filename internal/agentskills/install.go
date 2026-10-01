package agentskills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// Target is an installed agent's skill directory.
type Target struct {
	Agent string
	Path  string
}

var relativeSkillDirectories = []Target{
	{Agent: "Agent", Path: ".agent/skills"},
	{Agent: "Agents", Path: ".agents/skills"},
	{Agent: "Claude", Path: ".claude/skills"},
	{Agent: "Codex", Path: ".codex/skills"},
	{Agent: "Pi", Path: ".pi/skills"},
	{Agent: "OMP", Path: ".omp/skills"},
	{Agent: "Factory", Path: ".factory/skills"},
}

// Detect returns skill roots whose parent agent directory already exists.
func Detect(workingDirectory, home string) []Target {
	seen := make(map[string]struct{})
	result := make([]Target, 0)
	for _, base := range []string{workingDirectory, home} {
		for _, candidate := range relativeSkillDirectories {
			path := filepath.Join(base, candidate.Path)
			info, err := os.Lstat(filepath.Dir(path))
			if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				continue
			}
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			result = append(result, Target{Agent: candidate.Agent, Path: path})
		}
	}
	return result
}

// Install writes the Kernel Managed Auth skill into the selected agent roots.
func Install(targets []Target) error {
	for _, target := range targets {
		if err := preflight(target); err != nil {
			return err
		}
	}
	for _, target := range targets {
		directory := filepath.Join(target.Path, "kernel-managed-auth")
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create %s skill directory: %w", target.Agent, err)
		}
		destination := filepath.Join(directory, "SKILL.md")
		if data, err := os.ReadFile(destination); err == nil && bytes.Equal(data, []byte(skill)) {
			continue
		}
		temporary, err := os.CreateTemp(directory, ".SKILL.md-*")
		if err != nil {
			return fmt.Errorf("prepare %s skill: %w", target.Agent, err)
		}
		temporaryName := temporary.Name()
		if _, err := temporary.WriteString(skill); err != nil {
			temporary.Close()
			os.Remove(temporaryName)
			return fmt.Errorf("write %s skill: %w", target.Agent, err)
		}
		if err := temporary.Close(); err != nil {
			os.Remove(temporaryName)
			return err
		}
		if err := os.Rename(temporaryName, destination); err != nil {
			os.Remove(temporaryName)
			return fmt.Errorf("install %s skill: %w", target.Agent, err)
		}
	}
	return nil
}

func preflight(target Target) error {
	for _, path := range []string{target.Path, filepath.Join(target.Path, "kernel-managed-auth"), filepath.Join(target.Path, "kernel-managed-auth", "SKILL.md")} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlinked skill path %s", path)
		}
	}
	destination := filepath.Join(target.Path, "kernel-managed-auth", "SKILL.md")
	if data, err := os.ReadFile(destination); err == nil && !bytes.Equal(data, []byte(skill)) {
		return fmt.Errorf("refusing to overwrite customized skill %s", destination)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

const skill = `---
name: kernel-managed-auth
description: Use Kernel Managed Auth connections when a browser needs to sign in or reauthenticate.
---

# Kernel Managed Auth

1. List matching connections with ` + "`kernel auth connections list --domain <domain>`" + `.
2. Start authentication with ` + "`kernel auth connections login <connection-id>`" + `.
3. Follow progress with ` + "`kernel auth connections follow <connection-id>`" + `.
4. If human input is requested, show the prompt; never request or print the stored password or TOTP secret.
5. Start subsequent browsers with the Managed Auth connection's saved profile.
`
