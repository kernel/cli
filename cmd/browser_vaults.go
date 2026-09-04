package cmd

import (
	"fmt"

	kernel "github.com/kernel/kernel-go-sdk"
)

func buildBrowserVaults(values []string) ([]kernel.VaultReferenceParam, error) {
	if len(values) > 20 {
		return nil, fmt.Errorf("at most 20 --vault references may be attached")
	}
	var refs []kernel.VaultReferenceParam
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if err := validateVaultName(value, "--vault"); err != nil {
			return nil, err
		}
		if seen[value] {
			return nil, fmt.Errorf("duplicate --vault reference")
		}
		seen[value] = true
		ref := kernel.VaultReferenceParam{}
		if cuidRegex.MatchString(value) {
			ref.ID = kernel.Opt(value)
		} else {
			ref.Name = kernel.Opt(value)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}
