package cmd

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

type vaultConfirmTransactionParams struct {
	Type            string `json:"type"`
	Status          string `json:"status"`
	TransactionType string `json:"transaction_type"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	OccurredAt      string `json:"occurred_at"`
}

func parseVaultConfirmTransactionParams(raw string) (*vaultConfirmTransactionParams, error) {
	object, err := vaultParamsObject(raw, "status transaction_type amount currency occurred_at")
	if err != nil {
		return nil, err
	}
	params := vaultConfirmTransactionParams{Type: "confirm_transaction"}
	if json.Unmarshal(object["status"], &params.Status) != nil || !slices.Contains([]string{"APPROVED", "DECLINED", "PENDING", "ERROR", "CANCELLED"}, params.Status) {
		return nil, fmt.Errorf("status must be APPROVED, DECLINED, PENDING, ERROR, or CANCELLED")
	}
	if json.Unmarshal(object["transaction_type"], &params.TransactionType) != nil || !slices.Contains([]string{"PURCHASE", "AUTHORIZATION", "CAPTURE", "REFUND", "REVERSAL", "VERIFICATION", "CHARGEBACK", "FRAUD"}, params.TransactionType) {
		return nil, fmt.Errorf("transaction_type must be PURCHASE, AUTHORIZATION, CAPTURE, REFUND, REVERSAL, VERIFICATION, CHARGEBACK, or FRAUD")
	}
	var amount *int64
	if json.Unmarshal(object["amount"], &amount) != nil || amount == nil || *amount < 0 || *amount > 50000 {
		return nil, fmt.Errorf("amount must be an integer between 0 and 50000 minor units, no greater than the approved amount")
	}
	params.Amount = *amount
	if json.Unmarshal(object["currency"], &params.Currency) != nil || len(params.Currency) != 3 {
		return nil, fmt.Errorf("currency must be a three-letter uppercase code")
	}
	for _, letter := range params.Currency {
		if letter < 'A' || letter > 'Z' {
			return nil, fmt.Errorf("currency must be a three-letter uppercase code")
		}
	}
	if json.Unmarshal(object["occurred_at"], &params.OccurredAt) != nil {
		return nil, fmt.Errorf("occurred_at must be an RFC3339 timestamp")
	}
	if _, err := time.Parse(time.RFC3339, params.OccurredAt); err != nil {
		return nil, fmt.Errorf("occurred_at must be an RFC3339 timestamp")
	}
	return &params, nil
}
