package passwordmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sync/errgroup"
)

type bitwardenProvider struct {
	path    string
	session string
}

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
	results, err := fetchBitwardenCandidateSites(ctx, sites, func(ctx context.Context, site string) ([]bitwardenCandidateItem, error) {
		return p.candidateItems(ctx, environment, site)
	})
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	candidates := make([]Candidate, 0)
	for index, site := range sites {
		for _, item := range results[index] {
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

func fetchBitwardenCandidateSites(ctx context.Context, sites []string, fetch func(context.Context, string) ([]bitwardenCandidateItem, error)) ([][]bitwardenCandidateItem, error) {
	results := make([][]bitwardenCandidateItem, len(sites))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(min(4, len(sites)))
	for index, site := range sites {
		index, site := index, site
		group.Go(func() error {
			items, err := fetch(groupCtx, site)
			if err != nil {
				return fmt.Errorf("find Bitwarden logins for %s: %w", site, err)
			}
			results[index] = items
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	return results, nil
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
	items, err := fetchBitwardenApprovedItems(ctx, selected, func(ctx context.Context, candidate Candidate) (bitwardenItem, error) {
		output, err := command(ctx, p.path, environment, "get", "item", candidate.ID)
		if err != nil {
			return bitwardenItem{}, err
		}
		var item bitwardenItem
		if err := json.Unmarshal(output, &item); err != nil {
			return bitwardenItem{}, fmt.Errorf("decode approved Bitwarden login: %w", err)
		}
		return item, nil
	})
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(selected))
	for index, candidate := range selected {
		item := items[index]
		if item.Login == nil || item.OrganizationID != "" {
			continue
		}
		records = append(records, Record{Provider: "bitwarden", ID: item.ID, Name: item.Name, Domain: candidate.Domain, Username: item.Login.Username, Password: item.Login.Password, TOTPSecret: normalizeTOTP(item.Login.TOTP)})
	}
	return deduplicate(records), nil
}

func fetchBitwardenApprovedItems(ctx context.Context, selected []Candidate, fetch func(context.Context, Candidate) (bitwardenItem, error)) ([]bitwardenItem, error) {
	unique := make([]Candidate, 0, len(selected))
	indices := make(map[string]int, len(selected))
	for _, candidate := range selected {
		if _, exists := indices[candidate.ID]; exists {
			continue
		}
		indices[candidate.ID] = len(unique)
		unique = append(unique, candidate)
	}
	items := make([]bitwardenItem, len(unique))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(min(4, len(unique)))
	for index, candidate := range unique {
		index, candidate := index, candidate
		group.Go(func() error {
			item, err := fetch(groupCtx, candidate)
			if err != nil {
				return fmt.Errorf("read approved Bitwarden login %q: %w", candidate.Name, err)
			}
			items[index] = item
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	ordered := make([]bitwardenItem, len(selected))
	for index, candidate := range selected {
		ordered[index] = items[indices[candidate.ID]]
	}
	return ordered, nil
}

func (p *bitwardenProvider) authorizedEnvironment(ctx context.Context) (map[string]string, error) {
	environment := map[string]string(nil)
	if p.session != "" {
		environment = map[string]string{"BW_SESSION": p.session}
	} else if session := os.Getenv("BW_SESSION"); session != "" {
		environment = map[string]string{"BW_SESSION": session}
	}
	status, err := p.status(ctx, environment)
	if err != nil {
		return nil, err
	}
	if status.Status != "unlocked" {
		return nil, fmt.Errorf("Bitwarden is locked")
	}
	return environment, nil
}

// AuthorizationRequired reports whether Bitwarden needs a local unlock.
func (p *bitwardenProvider) AuthorizationRequired(ctx context.Context) (bool, error) {
	environment := map[string]string(nil)
	if p.session != "" {
		environment = map[string]string{"BW_SESSION": p.session}
	} else if session := os.Getenv("BW_SESSION"); session != "" {
		environment = map[string]string{"BW_SESSION": session}
	}
	status, err := p.status(ctx, environment)
	if err != nil {
		return false, err
	}
	switch status.Status {
	case "unlocked":
		return false, nil
	case "locked":
		return true, nil
	case "unauthenticated":
		return false, fmt.Errorf("sign in to Bitwarden with `bw login`, then retry")
	default:
		return false, fmt.Errorf("unsupported Bitwarden status %q", status.Status)
	}
}

// Authorize asks Bitwarden to unlock through its own terminal prompt and keeps
// the resulting session key only in this process.
func (p *bitwardenProvider) Authorize(ctx context.Context) error {
	output, err := command(ctx, p.path, map[string]string{"BW_SESSION": ""}, "unlock", "--raw")
	if err != nil {
		return fmt.Errorf("unlock Bitwarden: %w", err)
	}
	session := strings.TrimSpace(string(output))
	if session == "" || strings.ContainsAny(session, "\r\n\t ") {
		return fmt.Errorf("Bitwarden returned an invalid session key")
	}
	p.session = session
	if _, err := p.authorizedEnvironment(ctx); err != nil {
		p.session = ""
		return fmt.Errorf("verify Bitwarden unlock: %w", err)
	}
	return nil
}

func (p *bitwardenProvider) status(ctx context.Context, environment map[string]string) (bitwardenStatus, error) {
	output, err := command(ctx, p.path, environment, "status")
	if err != nil {
		return bitwardenStatus{}, err
	}
	var status bitwardenStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return bitwardenStatus{}, fmt.Errorf("decode Bitwarden status: %w", err)
	}
	return status, nil
}
