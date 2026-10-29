package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/spf13/cobra"
)

// linkCardFlowHelp follows the API's Link wallet and card operation descriptions.
// Every command that documents Link checkout includes it, so the flow reads the same everywhere.
const linkCardFlowHelp = `Link card checkout and payment_request checkout (each item's available_operations descriptions are authoritative):
1. reach the final checkout page in a browser created with --vault. note the amount,
   currency, merchant name and url, and a payment_method_id from wallets payment-methods.
2. create the card with cards create --provider link, or where enabled create a
   payment_request with payment-requests create --provider link. this does not contact
   Link; the item is requested and advertises authorize. to change it before authorizing,
   use the matching cards update or payment-requests update command.
3. once the user has approved this purchase, authorize with items invoke <vault> <key>
   authorize --browser-id <browser-session-id> --page-url <exact current top-level checkout
   url>. KERNEL inspects the checkout, chooses how the credential is delivered (a Link Pay
   Token or a virtual card), limits fill to that checkout's origin, and sends the user an
   approval request. do not inspect or invoke WebMCP yourself.
4. share the approval url from the item's action with the user, then use items get --wait 60
   until fill is advertised. to re-check sooner, invoke authorize again; checkout context is
   not needed, and re-checking never creates another payment request.
5. fill as the item's fill description says, in any browser with this vault attached: a Link
   Pay Token takes browser_id and page_url and omits fields; a virtual card also takes
   field/selector bindings.
6. submit the checkout yourself; fill never submits payment. failed or unknown fills are safe
   to retry, but only after checking the page and not in an automatic loop.
if authorize returns page_not_found, ambiguous_page, browser_not_found, browser_unavailable,
destination_denied, timeout, or browser_error, no payment request was made: make sure the
checkout is open in exactly one tab at page_url, then authorize again. if checkout inspection
keeps failing, authorize without --browser-id and --page-url to issue a virtual card that fills
any page on the merchant_url origin. once a card is authorized at a checkout, cards update
returns 409: delete the card and create a new one. the same rule applies to payment-requests
update: delete the payment_request and create a new one. if authorization is declined or
expires, read status_reason, then delete the item and create a new one. if the result of submitting
payment is unclear, check the merchant's order confirmation or order history before paying again.
for a recurring card, amount is the limit for each declared period. the merchant schedules
later charges and the same card remains ready until it expires or is deleted; do not create a
new card each period. deleting the card, wallet, or vault revokes the Link credential so later
charges decline, but does not cancel the merchant subscription. if deletion returns 409
provider_revocation_pending, retry deletion.`

const linkAuthorizeBrowserGuidance = "no payment request was made. make sure the checkout is open in exactly one tab at page_url in a browser with this vault attached, then authorize again. if checkout inspection keeps failing, authorize without --browser-id and --page-url to issue a virtual card that fills any page on the merchant_url origin"

// Authorize returns these codes when checkout inspection fails, before any provider request.
var linkAuthorizeBrowserCodes = map[string]bool{
	"page_not_found": true, "ambiguous_page": true, "browser_not_found": true, "browser_unavailable": true,
	"destination_denied": true, "timeout": true, "browser_error": true,
}

// linkCheckoutPageURL matches the API's page_url pattern for authorize.
var linkCheckoutPageURL = regexp.MustCompile(`^https://[^/?#@*\s]+(?:[/?#][^\s]*)?$`)

const linkAuthorizeServerErrorGuidance = "if the browser was not created with --vault for this vault, was deleted, or is a pool browser that is not leased, checkout inspection cannot run: create a browser with --vault, open the checkout in it, and authorize again with that browser's session id. otherwise re-get the item and follow its advertised operations"

// parseVaultAuthorizeCheckout returns the authorize request for --browser-id and --page-url,
// or nil when neither flag is set.
func parseVaultAuthorizeCheckout(cmd *cobra.Command, operation string) (*kernel.AuthorizeVaultItemOperationRequestParam, error) {
	browserSet, pageSet := cmd.Flags().Changed("browser-id"), cmd.Flags().Changed("page-url")
	if !browserSet && !pageSet {
		return nil, nil
	}
	if operation != "authorize" {
		if isPasskeyOperation(operation) {
			// Passkey flags are validated and converted by parseVaultPasskeyOperationFlags.
			return nil, nil
		}
		if vaultOperationTakesParams(operation) {
			return nil, fmt.Errorf("--browser-id and --page-url are only supported for authorize; pass %s inputs with --params or --spec-file", operation)
		}
		return nil, fmt.Errorf("--browser-id and --page-url are only supported for authorize")
	}
	if browserSet != pageSet {
		return nil, fmt.Errorf("--browser-id and --page-url must be passed together; authorize was not invoked")
	}
	browserID, _ := cmd.Flags().GetString("browser-id")
	pageURL, _ := cmd.Flags().GetString("page-url")
	if strings.TrimSpace(browserID) == "" {
		return nil, fmt.Errorf("--browser-id must be a non-empty browser session ID, not a name")
	}
	if _, err := url.Parse(pageURL); err != nil || !linkCheckoutPageURL.MatchString(pageURL) {
		return nil, fmt.Errorf("--page-url must be the exact current top-level https checkout URL")
	}
	return &kernel.AuthorizeVaultItemOperationRequestParam{
		Type:      kernel.AuthorizeVaultItemOperationRequestTypeAuthorize,
		BrowserID: kernel.Opt(browserID),
		PageURL:   kernel.Opt(pageURL),
	}, nil
}

// linkAuthorizeError keeps the API's error code and message and adds the recovery step.
// checkout reports whether the request carried browser_id and page_url.
func linkAuthorizeError(err error, checkout bool) error {
	var apiErr *kernel.Error
	if !errors.As(err, &apiErr) {
		cause := kernelVaultErrorURL.ReplaceAllString(err.Error(), "[redacted url]")
		return kernelVaultTransportError{message: fmt.Sprintf("authorize result unavailable (%s); re-get the item and follow its advertised operations. once approval is requested, authorize only re-checks it and never creates another payment request", cause), cause: err}
	}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal([]byte(apiErr.RawJSON()), &body)
	var guidance string
	switch {
	case linkAuthorizeBrowserCodes[body.Code]:
		guidance = linkAuthorizeBrowserGuidance
	case apiErr.StatusCode == 429:
		guidance = retryAfterWait(apiErr) + ", then authorize again"
	case checkout && apiErr.StatusCode >= 500:
		guidance = linkAuthorizeServerErrorGuidance
	case body.Code == "invalid_request":
		guidance = "fix the reported input, then authorize again. if it reports that the amount exceeds the Link virtual card limit, delete this card and tell the user this checkout cannot be paid with a Link virtual card"
	default:
		guidance = "re-get the item and follow its advertised operations"
	}
	message := kernelVaultErrorURL.ReplaceAllString(body.Message, "[redacted url]")
	return safeKernelAPIError(apiErr, body.Code, fmt.Sprintf("authorize failed (http %d): %s; %s", apiErr.StatusCode, message, guidance))
}

func shopPayAuthorizeError(err error, checkout bool) error {
	var apiErr *kernel.Error
	if !errors.As(err, &apiErr) {
		cause := kernelVaultErrorURL.ReplaceAllString(err.Error(), "[redacted url]")
		return kernelVaultTransportError{message: fmt.Sprintf("Shop Pay authorize result unavailable (%s); re-get the item and follow its advertised operations. once approval is requested, authorize only re-checks it", cause), cause: err}
	}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal([]byte(apiErr.RawJSON()), &body)
	guidance := "re-get the item and follow its advertised operations"
	switch {
	case linkAuthorizeBrowserCodes[body.Code]:
		guidance = "no Shop Pay purchase request was made. make sure the checkout is open in exactly one tab at page_url in a browser with this vault attached, then authorize again. if checkout inspection keeps failing, authorize without --browser-id and --page-url to use merchant_url"
	case apiErr.StatusCode == 429:
		guidance = retryAfterWait(apiErr) + ", then authorize again"
	case checkout && apiErr.StatusCode >= 500:
		guidance = linkAuthorizeServerErrorGuidance
	case body.Code == "invalid_request":
		guidance = "fix the reported input, then authorize again only if the item still advertises authorize"
	}
	message := kernelVaultErrorURL.ReplaceAllString(body.Message, "[redacted url]")
	return safeKernelAPIError(apiErr, body.Code, fmt.Sprintf("Shop Pay authorize failed (http %d): %s; %s", apiErr.StatusCode, message, guidance))
}
