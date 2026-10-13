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

func (a vaultItemActions) advertises(operation string) bool {
	for _, op := range a.Operations {
		if op.Type == operation {
			return true
		}
	}
	return false
}

// Recovery exposes only the explicitly supported restart operation on every surface.
func effectiveVaultItemActions(item *kernel.VaultItemUnion) (vaultItemActions, error) {
	var fields struct {
		Operations []vaultItemOperation `json:"available_operations"`
	}
	if err := json.Unmarshal([]byte(item.RawJSON()), &fields); err != nil {
		return vaultItemActions{}, fmt.Errorf("invalid vault item operations: %w", err)
	}
	if item.State.Status == "recovery_required" {
		actions := vaultItemActions{RecoveryRequired: true}
		for _, op := range fields.Operations {
			if op.Type == "restart_verification" {
				actions.Operations = append(actions.Operations, op)
			}
		}
		return actions, nil
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
