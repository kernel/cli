package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/kernel/cli/internal/passwordmanager"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
)

type managedAuthCapacity struct {
	remaining int
	unlimited bool
}

type orgLimitsGetter interface {
	Get(context.Context, ...option.RequestOption) (*kernel.OrgLimits, error)
}

func loadManagedAuthCapacity(ctx context.Context, limits orgLimitsGetter) (managedAuthCapacity, error) {
	orgLimits, err := limits.Get(ctx)
	if err != nil {
		return managedAuthCapacity{}, err
	}
	return decodeManagedAuthCapacity(orgLimits.RawJSON())
}

func decodeManagedAuthCapacity(raw string) (managedAuthCapacity, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return managedAuthCapacity{}, fmt.Errorf("decode organization limits: %w", err)
	}
	maxRaw, hasMax := fields["max_auth_connections"]
	usedRaw, hasUsed := fields["auth_connections_used"]
	if !hasMax || !hasUsed {
		return managedAuthCapacity{}, fmt.Errorf("Kernel API does not expose Managed Auth capacity through organization limits")
	}
	if string(maxRaw) == "null" {
		return managedAuthCapacity{unlimited: true}, nil
	}
	var maxConnections, usedConnections int
	if err := json.Unmarshal(maxRaw, &maxConnections); err != nil {
		return managedAuthCapacity{}, fmt.Errorf("decode max auth connections: %w", err)
	}
	if err := json.Unmarshal(usedRaw, &usedConnections); err != nil {
		return managedAuthCapacity{}, fmt.Errorf("decode used auth connections: %w", err)
	}
	if maxConnections < 0 || usedConnections < 0 {
		return managedAuthCapacity{}, fmt.Errorf("Kernel API returned invalid Managed Auth capacity")
	}
	return managedAuthCapacity{remaining: max(0, maxConnections-usedConnections)}, nil
}

type managedAuthProvisioner interface {
	Provision(context.Context, string, []passwordmanager.Record) ([]string, error)
	Existing(context.Context, string, []passwordmanager.Candidate) (map[string]bool, error)
}

type kernelManagedAuthProvisioner struct {
	credentials interface {
		New(context.Context, kernel.CredentialNewParams, ...option.RequestOption) (*kernel.Credential, error)
		Get(context.Context, string, ...option.RequestOption) (*kernel.Credential, error)
		Update(context.Context, string, kernel.CredentialUpdateParams, ...option.RequestOption) (*kernel.Credential, error)
	}
	connections interface {
		New(context.Context, kernel.AuthConnectionNewParams, ...option.RequestOption) (*kernel.ManagedAuth, error)
		List(context.Context, kernel.AuthConnectionListParams, ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuth], error)
	}
}

func (p kernelManagedAuthProvisioner) Provision(ctx context.Context, profileName string, records []passwordmanager.Record) ([]string, error) {
	connectionIDs := make([]string, 0, len(records))
	for _, record := range records {
		name := importedCredentialName(record)
		existing, err := p.findConnection(ctx, profileName, record.Domain, name)
		if err != nil {
			return connectionIDs, fmt.Errorf("check managed auth for %s: %w", record.Domain, err)
		}
		if existing.match != nil {
			if err := p.refreshCredential(ctx, name, record); err != nil {
				return connectionIDs, err
			}
			connectionIDs = append(connectionIDs, existing.match.ID)
			continue
		}
		if existing.conflict != nil {
			return connectionIDs, fmt.Errorf("managed auth for %s already uses credential %q", record.Domain, existing.conflict.Credential.Name)
		}

		values := map[string]string{"username": record.Username, "password": record.Password}
		credentialParams := kernel.CredentialNewParams{CreateCredentialRequest: kernel.CreateCredentialRequestParam{Name: name, Domain: record.Domain, Values: values}}
		if record.TOTPSecret != "" {
			credentialParams.CreateCredentialRequest.TotpSecret = kernel.Opt(record.TOTPSecret)
		}
		credential, err := p.credentials.New(ctx, credentialParams)
		if err != nil {
			var getErr error
			credential, getErr = p.credentials.Get(ctx, name)
			if getErr != nil || credential == nil {
				return connectionIDs, fmt.Errorf("create credential for %s: %w", record.Domain, err)
			}
			if err := p.refreshCredential(ctx, name, record); err != nil {
				return connectionIDs, err
			}
		}
		connection, err := p.connections.New(ctx, kernel.AuthConnectionNewParams{ManagedAuthCreateRequest: kernel.ManagedAuthCreateRequestParam{
			Domain: record.Domain, ProfileName: profileName,
			Credential: kernel.ManagedAuthCreateRequestCredentialParam{Name: kernel.Opt(name)},
		}})
		if err != nil {
			reconciled, listErr := p.findConnection(ctx, profileName, record.Domain, name)
			if listErr == nil && reconciled.match != nil {
				connectionIDs = append(connectionIDs, reconciled.match.ID)
				continue
			}
			return connectionIDs, fmt.Errorf("create managed auth for %s: %w", record.Domain, err)
		}
		connectionIDs = append(connectionIDs, connection.ID)
	}
	return connectionIDs, nil
}

func (p kernelManagedAuthProvisioner) Existing(ctx context.Context, profileName string, candidates []passwordmanager.Candidate) (map[string]bool, error) {
	existingNames := make(map[string]struct{})
	const pageSize = 100
	for offset := int64(0); ; offset += pageSize {
		page, err := p.connections.List(ctx, kernel.AuthConnectionListParams{
			ProfileName: kernel.Opt(profileName), Limit: kernel.Opt(int64(pageSize)), Offset: kernel.Opt(offset),
		})
		if err != nil {
			return nil, err
		}
		if page == nil {
			break
		}
		for _, connection := range page.Items {
			existingNames[connection.Credential.Name] = struct{}{}
		}
		if len(page.Items) < pageSize {
			break
		}
	}

	result := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		name := importedCredentialNameFor(candidate.Provider, candidateImportID(candidate), candidate.Domain)
		_, result[candidateKey(candidate)] = existingNames[name]
	}
	return result, nil
}

type connectionLookup struct {
	match    *kernel.ManagedAuth
	conflict *kernel.ManagedAuth
}

func (p kernelManagedAuthProvisioner) findConnection(ctx context.Context, profileName, domain, credentialName string) (connectionLookup, error) {
	const pageSize = 100
	result := connectionLookup{}
	for offset := int64(0); ; offset += pageSize {
		page, err := p.connections.List(ctx, kernel.AuthConnectionListParams{
			Domain: kernel.Opt(domain), ProfileName: kernel.Opt(profileName), Limit: kernel.Opt(int64(pageSize)), Offset: kernel.Opt(offset),
		})
		if err != nil {
			return connectionLookup{}, err
		}
		if page == nil {
			return result, nil
		}
		for index := range page.Items {
			connection := &page.Items[index]
			if connection.Credential.Name == credentialName {
				copy := *connection
				result.match = &copy
			} else if result.conflict == nil {
				copy := *connection
				result.conflict = &copy
			}
		}
		if len(page.Items) < pageSize {
			return result, nil
		}
	}
}

func (p kernelManagedAuthProvisioner) refreshCredential(ctx context.Context, name string, record passwordmanager.Record) error {
	update := kernel.CredentialUpdateParams{UpdateCredentialRequest: kernel.UpdateCredentialRequestParam{
		Values:     map[string]string{"username": record.Username, "password": record.Password},
		TotpSecret: kernel.Opt(record.TOTPSecret),
	}}
	if _, err := p.credentials.Update(ctx, name, update); err != nil {
		return fmt.Errorf("refresh credential for %s: %w", record.Domain, err)
	}
	return nil
}

var importedCredentialCharacters = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func importedCredentialName(record passwordmanager.Record) string {
	return importedCredentialNameFor(record.Provider, record.ID, record.Domain)
}

func importedCredentialNameFor(provider, id, recordDomain string) string {
	digest := sha256.Sum256([]byte(provider + "\x00" + id))
	prefix := strings.Trim(importedCredentialCharacters.ReplaceAllString(strings.ToLower("import-"+provider), "-"), "-")
	domain := strings.Trim(importedCredentialCharacters.ReplaceAllString(strings.ToLower(recordDomain), "-"), "-")
	suffix := fmt.Sprintf("-%x", digest[:5])
	maxDomainLength := 255 - len(prefix) - len(suffix) - 1
	if len(domain) > maxDomainLength {
		domain = strings.Trim(domain[:maxDomainLength], "-")
	}
	return prefix + "-" + domain + suffix
}

func candidateKey(candidate passwordmanager.Candidate) string {
	return candidate.Provider + "\x00" + candidateImportID(candidate) + "\x00" + candidate.Domain
}

func candidateImportID(candidate passwordmanager.Candidate) string {
	if candidate.Provider == "1password" && candidate.VaultID != "" {
		return candidate.VaultID + ":" + candidate.ID
	}
	return candidate.ID
}
