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
	case "add_payment_method":
		return "list the wallet's payment methods with wallets payment-methods before adding another; a lost response may have created a pending_enrollment method, listed with its link until the link expires after 30 minutes. never add payment methods in an automatic loop"
	case "remove_payment_method":
		return "list the wallet's payment methods with wallets payment-methods; invoke remove_payment_method again only if the method is still listed, never in an automatic loop. removal is not proof that a payment did not occur"
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
	case apiErr.StatusCode == 403 && body.Code == "insufficient_scope" && (operation == "add_payment_method" || operation == "remove_payment_method"):
		guidance = "nothing was changed. adding or removing payment methods requires an api key with vaults:write in addition to vaults:use on this vault"
	case apiErr.StatusCode == 409 && body.Code == "purchase_unresolved":
		guidance = "the payment method was not removed. reconcile the unresolved purchase it funded first: inspect that item and its events, report the observed outcome with confirm_transaction when advertised, or delete the purchase if the api permits it. then retry the removal once"
	case apiErr.StatusCode == 409 && body.Code == "provider_revocation_pending":
		guidance = "the payment method stays removal_pending and refuses payments; invoke remove_payment_method again later, never in an automatic loop. removal is not proof that a payment did not occur"
	case apiErr.StatusCode == 409 && operation == "add_payment_method":
		guidance = "no enrollment was started. a wallet holds at most 10 payment methods and 3 open enrollment links; remove an unused method or let open links expire before adding another. " + guidance
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

const kernelEligibilityHelp = "eligible=false means do not authorize. for network_token_pending, re-check later at most once in this attempt; network_token_unsupported or network_token_failed needs a different card. if still ineligible or unknown, ask the cardholder before enrolling a different card, with add_payment_method when the wallet advertises it or otherwise under a new wallet key; do not poll indefinitely or replay an uncertain purchase"

const kernelWalletHelp = "a connected kernel wallet holds zero or more cards as payment methods; connected does not imply that any card can pay, so inspect wallets payment-methods. when the wallet advertises add_payment_method and remove_payment_method, add each card with wallets add-payment-method and remove one with wallets remove-payment-method; scoped api keys need vaults:write in addition to vaults:use for both. otherwise the wallet enrolls one default card (payment method id card) through the card_enrollment action returned at creation and connects once it is stored. share card_enrollment only with the intended cardholder privately, never open it for them. degraded: the default card's enrollment outcome is unknown; with user confirmation, delete the wallet only if the api permits it before creating a new wallet under a new key; reconcile unresolved child cards first. reconnect_required: the wallet predates payment methods, and payment and deletion are refused until it is upgraded"

const kernelPaymentMethodHelp = "kernel card items select a method with optional payment_method_id; omitting it selects the default card, id card, and never falls back to another card. payment_request items require payment_method_id. only an active method with single_use_card.eligible=true can fund a new purchase, and a purchase stays bound to its method. pending_enrollment: share its card_enrollment url privately with the cardholder. removal_pending: invoke remove_payment_method again later. recovery_required: read status_reason; remove the method if its enrollment outcome is unknown, or contact support@kernel.sh if its stored card cannot be read"

const kernelEnrollmentHelp = "share the card_enrollment url only with the intended cardholder privately, never open it for them; it is single-use and expires after 30 minutes. card details go to KERNEL's hosted page, never to the cli. a failed or expired enrollment does not affect the wallet's other payment methods. observe the method with wallets payment-methods until it is active and eligible; an unused link expires on its own, so add another only when the cardholder needs a new link"

const kernelRemovedPaymentMethodHelp = "removal blocks new purchases, fills and confirmations funded by this payment method; other payment methods are unaffected and the wallet stays connected. purchases it funded report funding_status removed and keep their status. removal is not proof that a payment did not occur"

const kernelReplacementHelp = "after user confirmation, delete only if the api permits it and create one new card under a new key for a new purchase; reconcile unresolved outcomes first"

const kernelTerminalHelp = "merchant decline: report the observed DECLINED outcome when confirm_transaction is advertised. expired approval link or code before checkout: do not submit. if checkout was submitted, reconcile and report the observed outcome first (reporting may remain available after expiry). if never submitted, or after a confirmed decline, " + kernelReplacementHelp + "; ask for a different card or merchant after decline. unknown outcome: ask the user to check the merchant order page, email or statement and report only after they confirm; never guess or repeat checkout"
