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
	// Abandonable is true when an unresolved AgentCard checkout returned no
	// authorization ID. The API lets that card be deleted explicitly to abandon
	// the attempt so a replacement can be created; its wallet and vault stay
	// blocked, and deletion is not proof that the payment did not occur.
	Abandonable    bool
	RequiredAction string
	ActionURL      string
	ApprovalURL    string
	Operations     []vaultItemOperation
}

// Execution and human output use this policy; JSON preserves the API-advertised
// fields through the separate display-safe projection.
func effectiveVaultItemActions(item *kernel.VaultItemUnion) (vaultItemActions, error) {
	if item.State.Status == "recovery_required" {
		return vaultItemActions{
			RecoveryRequired: true,
			Abandonable:      item.Type == "card" && item.State.Provider == "agentcard" && item.State.Authorization.ID == "",
		}, nil
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
