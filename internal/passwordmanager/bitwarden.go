package passwordmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type bitwardenProvider struct{ path string }

func (*bitwardenProvider) Name() string { return "Bitwarden" }

type bitwardenStatus struct {
	Status string `json:"status"`
}

type bitwardenItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	OrganizationID string `json:"organizationId"`
	Login          *struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	} `json:"login"`
}

type bitwardenCandidateItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	OrganizationID string `json:"organizationId"`
	Login          *struct {
		Username string `json:"username"`
	} `json:"login"`
}

func (p *bitwardenProvider) Candidates(ctx context.Context, sites []string) ([]Candidate, error) {
	environment, err := p.authorizedEnvironment(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	candidates := make([]Candidate, 0)
	for _, site := range sites {
		items, err := p.candidateItems(ctx, environment, site)
		if err != nil {
			return nil, fmt.Errorf("find Bitwarden logins for %s: %w", site, err)
		}
		for _, item := range items {
			if item.Login == nil || item.OrganizationID != "" {
				continue
			}
			key := item.ID + "\x00" + site
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, Candidate{Provider: "bitwarden", ID: item.ID, Name: item.Name, Domain: site, Username: item.Login.Username})
		}
	}
	return candidates, nil
}

func (p *bitwardenProvider) candidateItems(ctx context.Context, environment map[string]string, site string) ([]bitwardenCandidateItem, error) {
	cmd := exec.CommandContext(ctx, p.path, "list", "items", "--url", site)
	cmd.Env = os.Environ()
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(stdout)
	token, err := decoder.Token()
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("Bitwarden item list is not an array")
	}
	items := make([]bitwardenCandidateItem, 0)
	for decoder.More() {
		var item bitwardenCandidateItem
		if err := decoder.Decode(&item); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, err
		}
		items = append(items, item)
		if len(items) > 1000 {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, fmt.Errorf("more than 1000 Bitwarden items matched %s", site)
		}
	}
	if _, err := decoder.Token(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *bitwardenProvider) Reveal(ctx context.Context, selected []Candidate) ([]Record, error) {
	environment, err := p.authorizedEnvironment(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(selected))
	for _, candidate := range selected {
		output, err := command(ctx, p.path, environment, "get", "item", candidate.ID)
		if err != nil {
			return nil, fmt.Errorf("read approved Bitwarden login %q: %w", candidate.Name, err)
		}
		var item bitwardenItem
		if err := json.Unmarshal(output, &item); err != nil {
			return nil, fmt.Errorf("decode approved Bitwarden login: %w", err)
		}
		if item.Login == nil || item.OrganizationID != "" {
			continue
		}
		records = append(records, Record{Provider: "bitwarden", ID: item.ID, Name: item.Name, Domain: candidate.Domain, Username: item.Login.Username, Password: item.Login.Password, TOTPSecret: normalizeTOTP(item.Login.TOTP)})
	}
	return deduplicate(records), nil
}

func (p *bitwardenProvider) authorizedEnvironment(ctx context.Context) (map[string]string, error) {
	environment := map[string]string(nil)
	if session := os.Getenv("BW_SESSION"); session != "" {
		environment = map[string]string{"BW_SESSION": session}
	}
	output, err := command(ctx, p.path, environment, "status")
	if err != nil {
		return nil, err
	}
	var status bitwardenStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("decode Bitwarden status: %w", err)
	}
	if status.Status != "unlocked" {
		return nil, fmt.Errorf("unlock Bitwarden for this terminal with `export BW_SESSION=$(bw unlock --raw)`, then retry")
	}
	return environment, nil
}
