package passwordmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type onePasswordProvider struct{ path string }

func (*onePasswordProvider) Name() string { return "1Password" }

type onePasswordSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URLs  []struct {
		Href string `json:"href"`
	} `json:"urls"`
	Vault struct {
		ID string `json:"id"`
	} `json:"vault"`
}

type onePasswordVault struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type onePasswordItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Vault struct {
		ID string `json:"id"`
	} `json:"vault"`
	URLs []struct {
		Href string `json:"href"`
	} `json:"urls"`
	Fields []struct {
		ID, Label, Type, Purpose string
		Value                    any `json:"value"`
	} `json:"fields"`
}

func (p *onePasswordProvider) Candidates(ctx context.Context, sites []string) ([]Candidate, error) {
	summaries, err := p.personalSummaries(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0)
	needsDetails := make([]onePasswordSummary, 0)
	for _, summary := range summaries {
		if len(summary.URLs) == 0 {
			needsDetails = append(needsDetails, summary)
			continue
		}
		candidates = append(candidates, onePasswordSummaryCandidates(summary, sites)...)
	}
	if len(needsDetails) == 0 {
		return candidates, nil
	}
	input, err := json.Marshal(needsDetails)
	if err != nil {
		return nil, err
	}
	itemOutput, err := commandInput(ctx, p.path, nil, input, "item", "get", "-", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("read 1Password login metadata: %w", err)
	}
	items, err := decodeOnePasswordItems(itemOutput)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		matched := make(map[string]struct{})
		for _, itemURL := range item.URLs {
			domain := selectedDomain(itemURL.Href, sites)
			if domain == "" {
				continue
			}
			if _, exists := matched[domain]; exists {
				continue
			}
			matched[domain] = struct{}{}
			username, _, _ := onePasswordFields(item)
			candidates = append(candidates, Candidate{Provider: "1password", ID: item.ID, VaultID: item.Vault.ID, Name: item.Title, Domain: domain, Username: username})
		}
	}
	return candidates, nil
}

func (p *onePasswordProvider) Reveal(ctx context.Context, selected []Candidate) ([]Record, error) {
	summaries := make([]onePasswordSummary, 0, len(selected))
	seen := make(map[string]struct{}, len(selected))
	for _, candidate := range selected {
		key := candidate.VaultID + ":" + candidate.ID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		var summary onePasswordSummary
		summary.ID = candidate.ID
		summary.Title = candidate.Name
		summary.Vault.ID = candidate.VaultID
		summaries = append(summaries, summary)
	}
	input, err := json.Marshal(summaries)
	if err != nil {
		return nil, err
	}
	itemOutput, err := commandInput(ctx, p.path, nil, input, "item", "get", "-", "--format", "json", "--reveal")
	if err != nil {
		return nil, fmt.Errorf("read approved 1Password logins: %w", err)
	}
	items, err := decodeOnePasswordItems(itemOutput)
	if err != nil {
		return nil, err
	}
	byID := make(map[string][]Candidate, len(selected))
	for _, candidate := range selected {
		key := candidate.VaultID + ":" + candidate.ID
		byID[key] = append(byID[key], candidate)
	}
	records := make([]Record, 0, len(items))
	for _, item := range items {
		candidates, ok := byID[item.Vault.ID+":"+item.ID]
		if !ok {
			continue
		}
		username, password, totp := onePasswordFields(item)
		for _, candidate := range candidates {
			records = append(records, Record{Provider: "1password", ID: item.Vault.ID + ":" + item.ID, Name: item.Title, Domain: candidate.Domain, Username: username, Password: password, TOTPSecret: normalizeTOTP(totp)})
		}
	}
	return deduplicate(records), nil
}

func (p *onePasswordProvider) personalSummaries(ctx context.Context) ([]onePasswordSummary, error) {
	vaultList, err := command(ctx, p.path, nil, "vault", "list", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("authorize 1Password with Touch ID: %w", err)
	}
	vaultDetails, err := commandInput(ctx, p.path, nil, vaultList, "vault", "get", "-", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("read 1Password vault metadata: %w", err)
	}
	vaults, err := decodeOnePasswordVaults(vaultDetails)
	if err != nil {
		return nil, err
	}
	summaries := make([]onePasswordSummary, 0)
	for _, vault := range vaults {
		if vault.ID == "" || !strings.EqualFold(vault.Type, "PERSONAL") {
			continue
		}
		output, err := command(ctx, p.path, nil, "item", "list", "--categories", "Login", "--vault", vault.ID, "--long", "--format", "json")
		if err != nil {
			return nil, fmt.Errorf("list 1Password personal logins: %w", err)
		}
		var listed []onePasswordSummary
		if err := json.Unmarshal(output, &listed); err != nil {
			return nil, fmt.Errorf("decode 1Password login list: %w", err)
		}
		summaries = append(summaries, listed...)
		if len(summaries) > 5000 {
			return nil, fmt.Errorf("1Password has more than 5000 personal login items; narrow the vault before importing")
		}
	}
	if len(summaries) == 0 {
		return nil, fmt.Errorf("1Password has no personal login items")
	}
	return summaries, nil
}

func onePasswordSummaryCandidates(summary onePasswordSummary, sites []string) []Candidate {
	result := make([]Candidate, 0)
	seen := make(map[string]struct{})
	for _, itemURL := range summary.URLs {
		domain := selectedDomain(itemURL.Href, sites)
		if domain == "" {
			continue
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, Candidate{Provider: "1password", ID: summary.ID, VaultID: summary.Vault.ID, Name: summary.Title, Domain: domain})
	}
	return result
}

func decodeOnePasswordVaults(output []byte) ([]onePasswordVault, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	vaults := make([]onePasswordVault, 0)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				return vaults, nil
			}
			return nil, fmt.Errorf("decode 1Password vaults: %w", err)
		}
		if len(raw) > 0 && raw[0] == '[' {
			var batch []onePasswordVault
			if err := json.Unmarshal(raw, &batch); err != nil {
				return nil, err
			}
			vaults = append(vaults, batch...)
			continue
		}
		var vault onePasswordVault
		if err := json.Unmarshal(raw, &vault); err != nil {
			return nil, err
		}
		vaults = append(vaults, vault)
	}
}

func decodeOnePasswordItems(output []byte) ([]onePasswordItem, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	items := make([]onePasswordItem, 0)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				return items, nil
			}
			return nil, fmt.Errorf("decode 1Password login: %w", err)
		}
		if len(raw) > 0 && raw[0] == '[' {
			var batch []onePasswordItem
			if err := json.Unmarshal(raw, &batch); err != nil {
				return nil, fmt.Errorf("decode 1Password logins: %w", err)
			}
			items = append(items, batch...)
			continue
		}
		var item onePasswordItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("decode 1Password login: %w", err)
		}
		items = append(items, item)
	}
}

func onePasswordFields(item onePasswordItem) (string, string, string) {
	var username, password, totp string
	for _, field := range item.Fields {
		value, ok := field.Value.(string)
		if !ok {
			continue
		}
		switch {
		case strings.EqualFold(field.Type, "OTP"):
			totp = value
		case strings.EqualFold(field.Purpose, "USERNAME"):
			username = value
		case strings.EqualFold(field.Purpose, "PASSWORD"):
			password = value
		}
	}
	return username, password, totp
}
