package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	kernel "github.com/kernel/kernel-go-sdk"
)

var vaultCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
var vaultFailureCodePattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

func parseVaultConfirmTransactionParams(raw string) (*kernel.ConfirmTransactionVaultItemOperationRequestParam, error) {
	object, err := vaultParamsObject(raw, "status transaction_type amount currency occurred_at failure_code order_id order_url")
	if err != nil {
		return nil, err
	}

	var status string
	if json.Unmarshal(object["status"], &status) != nil || !slices.Contains([]string{"APPROVED", "DECLINED", "PENDING", "ERROR", "CANCELLED"}, status) {
		return nil, fmt.Errorf("status must be APPROVED, DECLINED, PENDING, ERROR, or CANCELLED")
	}
	var transactionType string
	if json.Unmarshal(object["transaction_type"], &transactionType) != nil || !slices.Contains([]string{"PURCHASE", "AUTHORIZATION", "CAPTURE", "REFUND", "REVERSAL", "VERIFICATION", "CHARGEBACK", "FRAUD"}, transactionType) {
		return nil, fmt.Errorf("transaction_type must be PURCHASE, AUTHORIZATION, CAPTURE, REFUND, REVERSAL, VERIFICATION, CHARGEBACK, or FRAUD")
	}
	var amount *int64
	if json.Unmarshal(object["amount"], &amount) != nil || amount == nil || *amount < 0 || *amount > 1_000_000 {
		return nil, fmt.Errorf("amount must be an integer between 0 and 1000000 minor units, no greater than the approved amount")
	}
	var currency string
	if json.Unmarshal(object["currency"], &currency) != nil || !vaultCurrencyPattern.MatchString(currency) {
		return nil, fmt.Errorf("currency must be a three-letter uppercase code")
	}
	var occurredAtText string
	if json.Unmarshal(object["occurred_at"], &occurredAtText) != nil {
		return nil, fmt.Errorf("occurred_at must be an RFC3339 timestamp")
	}
	occurredAt, err := time.Parse(time.RFC3339, occurredAtText)
	if err != nil {
		return nil, fmt.Errorf("occurred_at must be an RFC3339 timestamp")
	}

	params := kernel.ConfirmTransactionVaultItemOperationRequestParam{
		Amount:          *amount,
		Currency:        currency,
		OccurredAt:      occurredAt,
		Status:          kernel.ConfirmTransactionVaultItemOperationRequestStatus(status),
		TransactionType: kernel.ConfirmTransactionVaultItemOperationRequestTransactionType(transactionType),
		Type:            kernel.ConfirmTransactionVaultItemOperationRequestTypeConfirmTransaction,
	}
	// The SDK's date-time encoder uses whole-second RFC3339. Preserve the user's
	// valid fractional seconds exactly while still using the generated request type.
	params.SetExtraFields(map[string]any{"occurred_at": occurredAtText})

	if rawOrderID, ok := object["order_id"]; ok {
		var orderID string
		if json.Unmarshal(rawOrderID, &orderID) != nil || strings.TrimSpace(orderID) == "" || utf8.RuneCountInString(orderID) > 255 {
			return nil, fmt.Errorf("order_id must be a non-empty string of at most 255 characters")
		}
		params.OrderID = kernel.Opt(orderID)
	}
	if rawOrderURL, ok := object["order_url"]; ok {
		var orderURL string
		if json.Unmarshal(rawOrderURL, &orderURL) != nil || len(orderURL) > 2048 {
			return nil, fmt.Errorf("order_url must be an https URL of at most 2048 characters")
		}
		u, err := url.ParseRequestURI(orderURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, fmt.Errorf("order_url must be an https URL of at most 2048 characters")
		}
		params.OrderURL = kernel.Opt(orderURL)
	}
	if rawFailureCode, ok := object["failure_code"]; ok {
		var failureCode string
		if json.Unmarshal(rawFailureCode, &failureCode) != nil || !vaultFailureCodePattern.MatchString(failureCode) {
			return nil, fmt.Errorf("failure_code must contain 1-64 lowercase letters, digits, or underscores")
		}
		params.FailureCode = kernel.Opt(failureCode)
	}

	approved := params.Status == kernel.ConfirmTransactionVaultItemOperationRequestStatusApproved
	if approved && params.FailureCode.Valid() {
		return nil, fmt.Errorf("failure_code is only for outcomes other than APPROVED")
	}
	if !approved && (params.OrderID.Valid() || params.OrderURL.Valid()) {
		return nil, fmt.Errorf("order_id and order_url are only for an APPROVED outcome")
	}
	if params.FailureCode.Valid() {
		if params.FailureCode.Value == "payment_declined" && params.Status != kernel.ConfirmTransactionVaultItemOperationRequestStatusDeclined {
			return nil, fmt.Errorf("failure_code payment_declined requires status DECLINED")
		}
		if params.Status == kernel.ConfirmTransactionVaultItemOperationRequestStatusDeclined && params.FailureCode.Value != "payment_declined" {
			return nil, fmt.Errorf("status DECLINED accepts only failure_code payment_declined")
		}
	}
	return &params, nil
}
