package passwordmanager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Record is one login approved for import into Kernel Managed Auth.
type Record struct {
	Provider   string
	ID         string
	Name       string
	Domain     string
	Username   string
	Password   string
	TOTPSecret string
}

// Candidate is non-secret metadata used for local selection.
type Candidate struct {
	Provider string
	ID       string
	Name     string
	Domain   string
	Username string
	VaultID  string
}

// Provider reads matching logins from a locally authorized password-manager CLI.
type Provider interface {
	Name() string
	Candidates(context.Context, []string) ([]Candidate, error)
	Reveal(context.Context, []Candidate) ([]Record, error)
}

// InteractiveAuthorizer lets the CLI prepare a provider after local user consent.
type InteractiveAuthorizer interface {
	AuthorizationRequired(context.Context) (bool, error)
	Authorize(context.Context) error
}

const maxCommandOutput = 16 << 20

// Detect returns supported password-manager CLIs installed on this machine.
func Detect() []Provider {
	providers := make([]Provider, 0, 2)
	if path, err := exec.LookPath("bw"); err == nil {
		providers = append(providers, &bitwardenProvider{path: path})
	}
	if path, err := exec.LookPath("op"); err == nil {
		providers = append(providers, &onePasswordProvider{path: path})
	}
	return providers
}

func command(ctx context.Context, path string, environment map[string]string, args ...string) ([]byte, error) {
	return commandInput(ctx, path, environment, nil, args...)
}

func commandInput(ctx context.Context, path string, environment map[string]string, input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	if input == nil {
		cmd.Stdin = os.Stdin
	} else {
		cmd.Stdin = bytes.NewReader(input)
	}
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxCommandOutput+1))
	if len(output) > maxCommandOutput {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("%s output exceeds 16 MiB", filepathBase(path))
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, readErr
	}
	err = waitErr
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s %s: %w", filepathBase(path), strings.Join(args, " "), err)
	}
	return output, nil
}

func selectedDomain(value string, selected []string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), ".")
	for _, domain := range selected {
		if host == domain || strings.HasSuffix(host, "."+domain) || strings.HasSuffix(domain, "."+host) {
			return domain
		}
	}
	return ""
}

func deduplicate(records []Record) []Record {
	seen := make(map[string]struct{}, len(records))
	result := make([]Record, 0, len(records))
	for _, record := range records {
		key := record.Provider + "\x00" + record.ID + "\x00" + record.Domain
		if record.Domain == "" || record.ID == "" || (record.Username == "" && record.Password == "") {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Domain == result[j].Domain {
			return result[i].Name < result[j].Name
		}
		return result[i].Domain < result[j].Domain
	})
	return result
}

func filepathBase(path string) string {
	parts := strings.Split(path, string(os.PathSeparator))
	return parts[len(parts)-1]
}
