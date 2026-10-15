package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
)

var kernelVaultErrorURL = regexp.MustCompile(`(?i)https?://\S+`)
var kernelVaultErrorCode = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

type kernelVaultTransportError struct {
	message string
	cause   error
}

func (e kernelVaultTransportError) Error() string        { return e.message }
func (e kernelVaultTransportError) Is(target error) bool { return errors.Is(e.cause, target) }

func safeKernelAPIError(apiErr *kernel.Error, code, message string) error {
	if !kernelVaultErrorCode.MatchString(code) {
		code = ""
	}
	raw, err := json.Marshal(map[string]string{"code": code, "message": message})
	if err != nil {
		return fmt.Errorf("request failed (http %d)", apiErr.StatusCode)
	}
	var safe kernel.Error
	if err := json.Unmarshal(raw, &safe); err != nil {
		return fmt.Errorf("request failed (http %d)", apiErr.StatusCode)
	}
	safe.StatusCode = apiErr.StatusCode
	return util.CleanedUpSdkError{Err: &safe}
}

func vaultSpecProvider(spec json.Marshaler) string {
	raw, err := spec.MarshalJSON()
	if err != nil {
		return ""
	}
	var value struct {
		Provider string `json:"provider"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value.Provider
}

func kernelVaultRetryGuidance(operation string) string {
	switch operation {
	case "create":
		return "retry create once with the identical key and identical spec; it retrieves the existing item if creation succeeded. do not use a new key to recover a lost create response"
	case "authorize", "confirm_transaction", "restart_verification":
		guidance := "re-get the item and inspect state/events; invoke " + operation + " once more only if freshly advertised in available_operations, never in an automatic loop. otherwise follow the current state. never replay merchant checkout"
		if operation == "restart_verification" {
			guidance += ". share any new spend_approval privately; obtain user approval each time and new cardholder spend consent"
		}
		return guidance
	default:
		return "inspect item state and events before taking further action; no automatic retries"
	}
}

// Preserve SDK classification with a sanitized copy; the root handler prints SDK bodies.
func kernelVaultRequestError(err error, operation string) error {
	guidance := kernelVaultRetryGuidance(operation)
	var apiErr *kernel.Error
	if !errors.As(err, &apiErr) {
		return kernelVaultTransportError{message: fmt.Sprintf("%s result unavailable; %s", operation, guidance), cause: err}
	}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal([]byte(apiErr.RawJSON()), &body)
	detail := kernelVaultErrorURL.ReplaceAllString(body.Message, "[redacted url]")
	switch {
	case apiErr.StatusCode == 429:
		guidance = retryAfterWait(apiErr) + "; " + guidance
	case apiErr.StatusCode == 409 && operation == "authorize":
		guidance = "authorization may be in flight, unresolved or unavailable in this state; there are no authorize inputs to fix. " + guidance
	case apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != 408:
		if operation == "create" {
			guidance = "inspect current state and correct the reported inputs or access before another request; never replay merchant checkout"
		}
		guidance = "the request was rejected; fix the reported inputs or access. " + guidance
	}
	return safeKernelAPIError(apiErr, body.Code, fmt.Sprintf("%s failed (http %d): %s; %s", operation, apiErr.StatusCode, detail, guidance))
}

// retryAfterWait says how long to wait after a 429, using a valid Retry-After header when present.
func retryAfterWait(apiErr *kernel.Error) string {
	if apiErr.Response != nil {
		after := apiErr.Response.Header.Get("Retry-After")
		_, secondsErr := strconv.ParseUint(after, 10, 64)
		_, dateErr := http.ParseTime(after)
		if secondsErr == nil || dateErr == nil {
			return "wait until Retry-After (" + after + ") has elapsed"
		}
	}
	return "wait for the rate limit to clear"
}

const kernelSupportGuidance = "do not retry payment, delete, or replace the item. contact support@kernel.sh privately with project, vault, item key, status_reason and relevant event timestamps (never card data, api keys or capability urls) to reconcile the original operation and establish whether a new purchase is safe; do not assume a new card is safe or promise a resulting state"

const kernelTransactionHelp = "report one observed merchant outcome only when confirm_transaction is advertised. PENDING means the merchant explicitly confirms an accepted transaction is pending; ERROR means an explicit merchant transaction error, not transport loss or an ambiguous processing page. these are final reports, not polling states. unknown means no reliable merchant outcome: ask the user to check order confirmation, merchant account or statement, then report the confirmed outcome even after card expiry. never infer success from fill. if reporting fails or the response is lost, re-get and send the identical observed report once more only if confirm_transaction is freshly advertised; otherwise observe state/events (recorded, in flight or recovery). never repeat merchant checkout"

const kernelEligibilityHelp = "eligible=false means do not authorize. for network_token_pending, re-check later at most once in this attempt; network_token_unsupported or network_token_failed needs a different card. if still ineligible or unknown, ask the cardholder before enrolling a different card under a new wallet key; do not poll indefinitely or replay an uncertain purchase"

const kernelWalletHelp = "share card_enrollment only with the intended cardholder privately, never open it for them. reconnect_required: share the new card_enrollment action. degraded: with user confirmation, delete the wallet only if the api permits it before creating a new wallet under a new key for one enrollment; reconcile unresolved child cards first. connected does not imply token eligibility; inspect wallets payment-methods"

const kernelReplacementHelp = "after user confirmation, delete only if the api permits it and create one new card under a new key for a new purchase; reconcile unresolved outcomes first"

const kernelTerminalHelp = "merchant decline: report the observed DECLINED outcome when confirm_transaction is advertised. expired approval link or code before checkout: do not submit. if checkout was submitted, reconcile and report the observed outcome first (reporting may remain available after expiry). if never submitted, or after a confirmed decline, " + kernelReplacementHelp + "; ask for a different card or merchant after decline. unknown outcome: ask the user to check the merchant order page, email or statement and report only after they confirm; never guess or repeat checkout"
