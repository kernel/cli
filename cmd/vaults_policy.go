package cmd

import (
	"encoding/json"
	"fmt"

	kernel "github.com/kernel/kernel-go-sdk"
)

type vaultItemOperation struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type vaultItemActions struct {
	RecoveryRequired bool
	RequiredAction   string
	ActionURL        string
	ApprovalURL      string
	Operations       []vaultItemOperation
}

// Execution and human output use this policy; JSON preserves the API-advertised
// fields through the separate display-safe projection.
func effectiveVaultItemActions(item *kernel.VaultItemUnion) (vaultItemActions, error) {
	if item.State.Status == "recovery_required" {
		return vaultItemActions{RecoveryRequired: true}, nil
	}
	var fields struct {
		Operations []vaultItemOperation `json:"available_operations"`
	}
	if err := json.Unmarshal([]byte(item.RawJSON()), &fields); err != nil {
		return vaultItemActions{}, fmt.Errorf("invalid vault item operations: %w", err)
	}
	approvalURL := item.State.Authorization.ApprovalURL
	if item.State.Status == "preparing" && item.State.Preparation.ApprovalURL != "" {
		approvalURL = item.State.Preparation.ApprovalURL
	}
	return vaultItemActions{
		RequiredAction: item.Action.Name,
		ActionURL:      item.Action.URL,
		ApprovalURL:    approvalURL,
		Operations:     fields.Operations,
	}, nil
}
