package cmd

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/kernel/cli/pkg/interactive"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newVaultsCommand())
}

func getVaultsHandler(cmd *cobra.Command) VaultsCmd {
	client := getKernelClient(cmd)
	return VaultsCmd{vaults: &client.Vaults, prompter: interactive.NewPrompter(), openURL: browser.OpenURL}
}

func addVaultJSONOutputFlag(cmd *cobra.Command) {
	addJSONOutputFlag(cmd)
	cmd.Flags().Lookup("output").Usage = "output format: json for display-safe api fields"
}

func vaultOutput(cmd *cobra.Command) string {
	output, _ := cmd.Flags().GetString("output")
	return output
}

func vaultPreRun(cmd *cobra.Command, args []string) error {
	if err := validateJSONOutput(vaultOutput(cmd)); err != nil {
		return err
	}
	for i, arg := range args {
		if i > 1 {
			break
		}
		label := "vault id or name"
		if i == 1 {
			label = "item key"
		}
		if err := validateVaultName(arg, label); err != nil {
			return err
		}
	}
	return nil
}

func newVaultsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "vaults", Aliases: []string{"vault"}, Short: "collect user credentials and manage payment credentials",
		Long: `collect user credentials and manage payment credentials.

do not use credential items to store, collect, or fill credit card data.
use wallet and card item types for credit cards and payment checkout instead; where enabled,
use payment_request items for one Link or KERNEL purchase.

` + vaultCredentialPathsHelp + `

KERNEL-hosted credential flow (fill never submits website forms; WebMCP tools may):
1. create a vault per end user and create a browser with --vault <id-or-name>.
2. navigate to a sensitive form and define its fields in natural top-to-bottom order with credentials create --spec-file; that array order controls the user-facing collection form.
3. present the returned collection url to the user. poll items get --wait 60 for ready.
4. once ready, for ordinary web forms use items invoke <vault> <key> fill --spec-file with
   browser_id and field selectors (writes fields without submitting). when the item
   advertises webmcp_invoke, items webmcp invoke instead binds credential fields to existing
   null inputs of a live WebMCP tool (the tool may submit or have side effects).
   passkey fields are never filled: use register_passkey to create one in a vault-bound
   browser, arm_passkey to sign in, and disarm_passkey as soon as the task is done.
   fill never submits, so a retry is safe. retry only after checking the page, and
   never in an automatic loop.
use credentials update --version for edits, or items invoke collect to reopen the form.
credential values belong in protected files/stdin, never command-line arguments.
see credentials --help, items invoke --help, and items webmcp invoke --help for examples.

1Password credential flow: reuse or connect the owner's account with credentials connect,
create a 1password credential for the site's login entries, then invoke only the
advertised 1pw_* operations. see credentials --help.

payment credential flow:

optionally select a project with --project <id-or-name> or KERNEL_PROJECT.
otherwise, the api resolves the project from your credentials and its defaults.
vault names, item keys, and project ownership are immutable.

1. create/select a vault, then create a provider wallet and follow any returned action.
   for kernel, share a hosted card_enrollment url with the cardholder, or use
   add_payment_method --enrollment server when the card's issuer will post to card_ingest.
   for Shop Pay, share shop_pay_oauth. a VGS wallet instead binds a required VGS provider
   configuration and connects without storing cards or payment methods. card details and
   OAuth approval stay with their provider.
2. inspect wallets payment-methods for providers that advertise it before creating a purchase.
   for Link, select a payment method id explicitly. VGS wallets have no payment methods.
3. attach the vault with browsers create --vault <id-or-name>; attachment is required for
   authorizing a Link or Shop Pay card at a checkout and for every fill, including VGS.
   the same applies to a Link payment_request. reach the final checkout page before
   checkout-bound Link or Shop Pay authorization.
4. create a purchase with cards create (Link, AgentCard, KERNEL, Shop Pay, or VGS), or where
   enabled use payment-requests create for a Link or KERNEL purchase. pass --provider and --spec json.
   caller-supplied KERNEL card details and VGS card aliases are exceptions: pass the full
   spec through cards create --spec-file <path|-> so protected values never appear in shell arguments.
5. inspect items get, then use items invoke <vault> <key> <operation> only when advertised.
   follow the operation description and any returned provider action.
6. ready Link, KERNEL, Shop Pay, and VGS cards use only advertised fill with --params for browser checkout;
   equivalent Link and KERNEL payment_request items do too. Link, KERNEL, and Shop Pay fill real
   card values and do not use egress substitution. VGS fill writes Kernel aliases that the VGS
   outbound proxy replaces only when the browser submits checkout; the CLI never receives the card.
   payment-method-funded KERNEL fill and VGS fill are locked to merchant_url's exact origin;
   a KERNEL single-use card fills on any https page in merchant_url's registrable domain.
   submit the merchant checkout before the purchase expires.
   payment-method-funded KERNEL cards and KERNEL payment requests have no stored billing fields;
   a single-use card fills only the cardholder and billing fields supplied at creation.
   AgentCard-only checkout aliases support egress substitution with checkout hold,
   approval, and replay; they are not a fallback after fill.
7. after the user submits KERNEL card/payment_request or Shop Pay card checkout, report the
   observed outcome once with confirm_transaction when advertised. ` + kernelTransactionHelp + `.
   in recovery_required, ` + vaultRecoveryHelp + `

` + linkCardFlowHelp + `

` + shopPayFlowHelp + `

` + vgsCardFlowHelp + `

permitted checkout domains are displayed when returned. Link card/payment_request and Shop Pay
card fill is limited to the origin where the purchase was authorized (or merchant_url's origin
when authorized without a checkout); payment-method-funded KERNEL card, KERNEL payment_request,
and VGS card fill use merchant_url's exact origin, while a KERNEL single-use card uses its
registrable domain. AgentCard domains are provider-assigned. there is no domain-setting api.
never supply card data, VGS aliases, OAuth codes, ciphertext, or secrets in shell arguments. use
protected file or stdin inputs where documented, including cards create --spec-file for a
single-use KERNEL card or VGS card.
use vault-provider-configs for client credentials and wallets create --tokens-file
for imported Link grants; both accept protected files or stdin.
do not automatically repeat failed, timed-out, rejected, or indeterminate payments: inspect
the item, its events, and the merchant's order history, then follow the advertised operations.
when an item explicitly permits user-confirmed abandonment, delete that purchase item before creating a replacement.
json output preserves returned public fields but omits unknown/opaque provider data.`,
		Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
	}

	create := &cobra.Command{Use: "create --name <name>", Short: "create or retrieve a vault by immutable name", Args: cobra.NoArgs, PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			return getVaultsHandler(cmd).Create(cmd.Context(), name, vaultOutput(cmd))
		}}
	create.Flags().String("name", "", "immutable vault name (required)")
	_ = create.MarkFlagRequired("name")
	addVaultJSONOutputFlag(create)

	list := &cobra.Command{Use: "list", Short: "list vaults in the effective project", Args: cobra.NoArgs, PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, _ := cmd.Flags().GetInt64("limit")
			offset, _ := cmd.Flags().GetInt64("offset")
			query, _ := cmd.Flags().GetString("query")
			project, _ := cmd.Flags().GetString("project")
			return getVaultsHandler(cmd).List(cmd.Context(), limit, offset, query, resolveProjectSelection(project), vaultOutput(cmd))
		}}
	list.Flags().Int64("limit", 20, "maximum vaults to return (1-100)")
	list.Flags().Int64("offset", 0, "number of vaults to skip")
	list.Flags().String("query", "", "case-insensitive substring match against vault name; ids match by exact value")
	addVaultJSONOutputFlag(list)

	get := &cobra.Command{Use: "get <vault>", Short: "get a vault by id or name", Args: cobra.ExactArgs(1), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getVaultsHandler(cmd).Get(cmd.Context(), args[0], vaultOutput(cmd))
		}}
	addVaultJSONOutputFlag(get)

	encryptionKey := &cobra.Command{Use: "get-encryption-key <vault>", Short: "get a vault's public key for browser-encrypted credential values", Args: cobra.ExactArgs(1), PreRunE: vaultPreRun,
		Long: `get the public key that custom credential collection web apps use to encrypt values in the browser.

use this only if you run your own credential collection web app and want values
encrypted in the browser, sent to your backend still encrypted, and forwarded to
KERNEL's api still encrypted. in every other case, including server-side code that
already holds the plaintext, use value. the page encrypts each value to this key as a
compact jwe (alg ECDH-ES, enc A256GCM, kid in the protected header), and your backend
forwards the ciphertext unchanged as encrypted_value in credentials create/update specs.
each vault has its own key; it is created on first request and may be cached.`,
		Example: "  kernel vaults get-encryption-key user-vault -o json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return getVaultsHandler(cmd).GetEncryptionKey(cmd.Context(), args[0], vaultOutput(cmd))
		}}
	addJSONOutputFlag(encryptionKey)
	cmd.AddCommand(create, list, get, encryptionKey, newVaultDeleteCommand(false))

	items := &cobra.Command{Use: "items", Short: "inspect item readiness or invoke advertised credential and payment operations", Long: "use get --wait 60 to observe readiness and get -o json for schema/version/presence.\nuse invoke collect to obtain a collection url, invoke fill --spec-file to fill a browser, or invoke advertised passkey operations with --browser-id.\nuse webmcp invoke to call a live WebMCP tool with item fields bound to null input slots.\n1Password credentials use the advertised 1pw_* operations instead of collect/fill/webmcp_invoke.\ncreate and edit credentials with vaults credentials; payment items use wallets/cards/payment-requests."}
	itemList := &cobra.Command{Use: "list <vault>", Short: "list items by vault id or name", Args: cobra.ExactArgs(1), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getVaultsHandler(cmd).ListItems(cmd.Context(), args[0], vaultOutput(cmd))
		}}
	addVaultJSONOutputFlag(itemList)
	itemGet := &cobra.Command{Use: "get <vault> <key>", Short: "get item state, available operations, and requested live data", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long:    "get item state, available operations, provider actions, and returned AgentCard checkout aliases.\n--expand fetches live, non-persisted provider data only when the item advertises that expansion; requesting unavailable data fails rather than returning a partial item. Link user info and shipping addresses can contain personal data. use -o json to preserve null versus omitted fields.\n--wait is a single bounded server-side observation, not a retry or a guarantee of readiness.\nan item still pending after the wait is returned as-is; ready means populated for credentials, not logged in or paid.\nfor credential edits on an already-ready item, compare versions without --wait. explicitly non-sensitive text/email values are returned; sensitive values and totp seeds are omitted.\nrecovery_required stops waiting and means unresolved, not declined or expired.\n" + vaultRecoveryHelp,
		Example: "  kernel vaults items get checkout wallet-1 --expand payment_methods\n  kernel vaults items get checkout wallet-1 --expand link_user_info,link_shipping_addresses -o json",
		RunE: func(cmd *cobra.Command, args []string) error {
			wait, _ := cmd.Flags().GetInt64("wait")
			expand, _ := cmd.Flags().GetStringSlice("expand")
			open, _ := cmd.Flags().GetBool("open")
			project, _ := cmd.Flags().GetString("project")
			return getVaultsHandler(cmd).GetItem(cmd.Context(), args[0], args[1], wait, expand, resolveProjectSelection(project), vaultOutput(cmd), open)
		}}
	itemGet.Flags().Int64("wait", 0, "hold while pending for up to this many seconds (0-60); observe only")
	itemGet.Flags().StringSlice("expand", nil, "advertised live data to fetch: payment_methods, link_user_info, or link_shipping_addresses")
	itemGet.Flags().Bool("open", false, "open a returned https action url; cardholder links are human-only, agents must share privately")
	addVaultJSONOutputFlag(itemGet)
	itemEvents := &cobra.Command{Use: "events <vault> <key>", Short: "read immutable item events without retrying payments", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			after, _ := cmd.Flags().GetString("after")
			wait, _ := cmd.Flags().GetInt64("wait")
			return getVaultsHandler(cmd).Events(cmd.Context(), args[0], args[1], after, wait, vaultOutput(cmd))
		}}
	itemEvents.Flags().String("after", "", "return events after this event id (use the last id from the previous response)")
	itemEvents.Flags().Int64("wait", 0, "long-poll once for new events (0-60 seconds)")
	addVaultJSONOutputFlag(itemEvents)
	invoke := &cobra.Command{Use: "invoke <vault> <key> <operation>", Short: "invoke an operation advertised by an item", Args: cobra.ExactArgs(3), PreRunE: vaultPreRun,
		Long: `retrieve the item and invoke only an operation listed in available_operations.
collect returns a time-scoped url for the full credential form without clearing values.
authorize sends {"type":"authorize"} for payment authorization. for a Link card or
payment_request, or a Shop Pay card, at the final checkout page, add --browser-id and
--page-url (both or neither) to send browser_id and page_url; see provider flows below.
read the operation description and follow any approval requirements before invoking.
fill requires --params json or --spec-file <path|-> with browser_id (session id, not name)
and the inputs the item's advertised fill description asks for: 1-32 ordered fields
(field, selector) for credentials, KERNEL card/payment_request items, Shop Pay cards,
and Link virtual cards/payment requests; no fields for a Link Pay Token. do not include type, values, or frame ids.
the vault must already be attached to the browser. page_url selects an existing page;
fill never navigates. credentials use declared field names, must omit format, and may
omit page_url only when the api can resolve a unique page. totp codes stay server-generated.
payment card and payment_request items require an exact https page_url. Link virtual card fields: number, cvc, exp_month (MM),
exp_year (YYYY), billing_name, billing_line1, billing_line2, billing_city,
billing_state, billing_postal_code, billing_country. expiration requires format MM/YY
or MM/YYYY. deprecated timeout_ms is accepted for compatibility but ignored; fill sets no operation deadline and adapts typing pace to value length.
KERNEL card and payment_request items support number, expiration (format MM/YY or MM/YYYY), cvc, exp_month, and exp_year.
a KERNEL single-use card also supports billing_name and the billing_* fields supplied at creation; omitted fields return field_unavailable.
VGS cards support number, cvc, exp_month, exp_year, and expiration; a cvc_only VGS card supports only cvc, and any other field returns field_unavailable.
Shop Pay cards support card fields plus available billing fields; billing_first_name and billing_last_name are available only when Shop returned them.
payment-method-funded KERNEL cards and KERNEL payment_request items have no stored billing fields: billing_* returns field_unavailable (400) before
any write. enter billing details on the merchant page.
the api searches the page and descendant frames, including payment iframes.
fill is available for credential items and ready Link, KERNEL, Shop Pay, or VGS cards when advertised, not AgentCard.
equivalent Link and KERNEL payment_request items also use fill when advertised.
Link, KERNEL, and Shop Pay cards do not expose aliases or support egress substitution; payment_request
items have the same boundary. VGS cards also expose no aliases to the CLI, but fill writes Kernel
aliases that the VGS outbound proxy replaces with card details when checkout is submitted.
KERNEL fill is locked to merchant_url's exact origin for payment-method-funded cards and payment requests;
VGS fill is also locked to merchant_url's exact origin. a KERNEL single-use card instead fills on
any https page in that registrable domain. submit checkout before expiry.
fill writes real credential or card values into the browser except for VGS, where it writes protected
Kernel aliases; unrestricted browser/CDP access can read whatever was filled.
fill never explicitly submits forms or clicks buttons, but input/change events may trigger site behavior.
completed means fields were filled, not website acceptance, login, or payment success.
failed may leave partial writes; unknown quarantines the browser. fill never submits,
so a retry is safe. retry only after checking the page, and never in an automatic loop.
never fall back to aliases. requests are not automatically retried.
api validation errors (400/403/404/409) include http status, recognized error codes,
and corrective guidance; no fields were written by that request. inspect and correct
the cause before deciding on a new fill. transport loss remains an uncertain outcome.
prepare_checkout requires checkout.browser_id, checkout.merchant_origin (canonical https
origin of the top-level merchant page, not a processor iframe), and checkout.environment
(production, sandbox, or shared). optional checkout.psp selects the checkout processor:
square, braintree, worldpay, bambora, mercado_pago, or adyen. omit psp for Square; non-Square
processors require multi-processor preparation enablement. use production or sandbox for
square, braintree, worldpay and adyen; shared for bambora and mercado_pago. shared endpoints do
not establish test mode; merchant credentials determine it. adyen prepares fresh-card Sessions
requests on Adyen hosts only: fill public dummy card fields, not vault aliases. Adyen device
approval and browser Authorised responses are not capture or fulfillment evidence.
use only when advertised for an AgentCard card. keep the returned approval page open,
poll until ready_to_submit, then submit native Pay before preparation.expires_at.
preparations are single-use, including after failure or expiry; never retry automatically.
confirm_transaction reports a real merchant outcome for an issued KERNEL Visa or Shop Pay
credential. it requires --params or --spec-file without type, with all of: status (APPROVED,
DECLINED, PENDING, ERROR, CANCELLED), transaction_type (PURCHASE, AUTHORIZATION, CAPTURE,
REFUND, REVERSAL, VERIFICATION, CHARGEBACK, FRAUD), amount (integer minor units 0..1000000,
no greater than the approved amount), currency (three uppercase letters matching the approved
purchase currency), and occurred_at (RFC3339). Shop Pay accepts only PURCHASE, AUTHORIZATION,
or CAPTURE. an APPROVED outcome may add order_id (1..255 characters) and order_url (https,
max 2048 characters); other outcomes may add failure_code (1..64 lowercase letters, digits,
or underscores). payment_declined is only for DECLINED, which accepts no other failure code.
unknown fields are rejected. never infer success; each purchase accepts one report. ` + kernelTransactionHelp + `.
add_payment_method is valid only when advertised by a connected KERNEL wallet.
--enrollment hosted is the default and returns a single-use card_enrollment url for the
cardholder that expires after 30 minutes. --enrollment server returns a single-use
card_ingest url for the card's issuer to post the card to from its backend; it expires after
5 minutes and returns 409 when server enrollment is not configured for the organization's
card backend. each call creates a separate pending enrollment. never log an action url or
retry automatically; a failed or expired enrollment does not affect other methods.
remove_payment_method requires {"payment_method_id":"<id>"}. it blocks new purchases, fills, and confirmations
funded by that method without disconnecting the wallet or changing other methods. an
unresolved purchase can block removal; an uncertain provider outcome leaves removal_pending
and must be re-checked before an explicit retry. removal is not proof that a payment did
not occur. scoped keys need both vaults:use and vaults:write.
restart_verification takes no parameters. in recovery_required, ` + vaultRecoveryHelp + `
it rotates the approval link without a provider call, only after a classified initial
attestation failure with proof no intent was dispatched. other recovery states must not be retried.
collect/authorize/prepare_checkout/1pw_recover/restart_verification may use --open.
--open on card_enrollment or spend_approval is for a human cardholder on their own machine.
agents must omit --open and share capability links privately, never open them for cardholders.
fill returns value-free per-field outcomes;
completed exits 0, failed/unknown exit nonzero with valid json retained on stdout in -o json.

` + vaultPasskeyOperationHelp + `

webmcp_invoke (credential items and ready Link card/payment_request items when advertised; see items webmcp invoke --help
for the flag-based form) requires browser_id, the opaque live tool_ref and exact source.page_url
from browsers webmcp list, input (public json object with a null at each bound slot; never vault
values), and 1-32 bindings (field, input_path as an rfc 6901 json pointer to an existing null in
input; format MM/YY or MM/YYYY only for card expiration). optional timeout_sec is 1-120 (default 15).
the tool may submit or have other side effects. the result returns status, invocation_id, output,
and error_text; output and error_text are untrusted page data and may contain supplied values.
completed/awaiting_submission exit 0 without confirming site acceptance; canceled/error/unknown
exit nonzero with json retained on stdout. unknown means the tool may have run: inspect the browser
and never retry automatically. 400/403/404/409 rejections mean the tool was not invoked.

` + linkCardFlowHelp + `

` + shopPayFlowHelp + `

` + vgsCardFlowHelp + `

1Password credentials (see credentials --help) use --params or --spec-file without type:
1pw_create_access_request: no browser needed; optional goal (<=140), reason (<=100),
  keywords (1-5 strings); reason and keywords only for single-entry credentials.
  present the returned onepassword:// approval link and instructions to the account
  owner unchanged. native iOS, Android, and desktop apps running 1Password 8.12.46
  or later may append &return_url=<URL-encoded URL>. invoke it once per advertised
  request and never request another link automatically.
1pw_access_request_status: no browser needed; optional timeout_seconds 0-120 (default 10)
  controls how long to keep checking. KERNEL checks immediately, then every 2 seconds,
  slowing to 10- and 20-second intervals. each call reads fresh status and may wait up
  to 20 seconds for its first scheduled check even when timeout_seconds is shorter.
  honor a 429 Retry-After. after 3 minutes the approval prompt closes: ask whether the
  owner approved and check once more, or invoke a newly advertised
  1pw_create_access_request for another link. expired returns the item to
  pending_authorization; unknown is treated as failed.
1pw_fill: browser_id and the exact page_url of an open login page; the 1Password
  extension decides whether an approved login matches and refuses pages that do not.
  use target_id when several open pages have that exact url, and entry_id when several
  entries are approved and the page origin does not identify exactly one. optional timeout_ms
  is 1-30000. the extension selects fields and submits. fill_submitted exits 0
  and does not confirm sign-in; inspect the page. fill_failed and fill_unknown exit
  nonzero. 1pw_fill can submit the form, so never retry automatically. check the page
  first before retrying.
1pw_update_access_token (stored-token credentials only): --spec-file, never --params,
  with access_token and optional access_token_expires_at (RFC3339). omitting the
  expiry clears the previously supplied expiry.
update_access_token (imported Link wallets with token_refresh=customer): --spec-file,
  never --params, with only access_token. the token must belong to the original grant.
  KERNEL stores it without contacting Link and never refreshes or revokes the grant;
  existing cards and pending purchases keep their state. update before each operation
  that reaches Link, and retry a 409 stale-token rejection only after updating.
1pw_recover (credential accounts, no parameters): returns a new 1Password link that
  recovers a failed account connection. share it with the account owner, then run
  credentials connect again with the same key; never delete the item to recover.
never automatically retry an access request or recovery, and never delete and recreate
an item to reset an uncertain outcome; stop and tell the user instead.`,
		Example: `  kernel vaults items invoke user-vault login collect
  kernel vaults items invoke user-vault login fill --spec-file - <<'JSON'
{"browser_id":"<browser-id>","fields":[{"field":"username","selector":"#username"},{"field":"password","selector":"#password"}]}
JSON
  kernel vaults items invoke user-vault login webmcp_invoke --spec-file - <<'JSON'
{"browser_id":"<browser-id>","tool_ref":"<tool-ref>","page_url":"https://example.com/login","input":{"email":null,"password":null},"bindings":[{"field":"email","input_path":"/email"},{"field":"password","input_path":"/password"}]}
JSON
  kernel vaults items invoke checkout order-1 authorize --open
  kernel vaults items invoke checkout order-1 authorize --browser-id <browser-id> --page-url https://shop.example/checkout
  kernel vaults items invoke checkout order-1 fill --params '{"browser_id":"<browser-id>","page_url":"https://shop.example/checkout"}' -o json
  kernel vaults items invoke user-vault github 1pw_create_access_request --params '{"reason":"Sign in to GitHub"}'
  kernel vaults items invoke user-vault github 1pw_access_request_status --params '{"timeout_seconds":60}'
  kernel vaults items invoke user-vault github 1pw_fill --params '{"browser_id":"<browser-id>","page_url":"https://github.com/login"}'
  kernel vaults items invoke user-vault github 1pw_fill --params '{"browser_id":"<browser-id>","page_url":"https://github.com/login","entry_id":"<entry-id>","target_id":"<cdp-target-id>"}'
  kernel vaults items invoke checkout cardholder add_payment_method
  kernel vaults items invoke checkout cardholder add_payment_method --enrollment server
  kernel vaults items invoke checkout cardholder remove_payment_method --params '{"payment_method_id":"<payment-method-id>"}'
  kernel vaults items invoke checkout wallet-1 update_access_token --spec-file ./link-access-token.json
  kernel vaults items invoke user-vault login register_passkey --browser-id <browser-session-id> --field passkey
  kernel vaults items invoke user-vault login arm_passkey --browser-id <browser-session-id> --field passkey
  kernel vaults items invoke user-vault login disarm_passkey --browser-id <browser-session-id>
  kernel vaults items invoke checkout order-1 fill --params '{"browser_id":"browser-session-id","page_url":"https://shop.example/checkout","fields":[{"field":"number","selector":"#cardNumber"},{"field":"expiration","format":"MM/YY","selector":"#cardExpiry"},{"field":"cvc","selector":"#cardCvc"}]}' -o json
  kernel vaults items invoke checkout order-1 confirm_transaction --params '{"status":"APPROVED","transaction_type":"PURCHASE","amount":100,"currency":"USD","occurred_at":"2026-09-01T12:00:00Z"}'
  kernel vaults items invoke checkout order-1 restart_verification --open`,
		RunE: func(cmd *cobra.Command, args []string) error {
			open, _ := cmd.Flags().GetBool("open")
			raw, _ := cmd.Flags().GetString("params")
			paramsSet := cmd.Flags().Changed("params")
			if (args[2] == "1pw_update_access_token" || args[2] == "update_access_token") && paramsSet {
				return fmt.Errorf("%s accepts the access token only through --spec-file <path|->, never command-line arguments", args[2])
			}
			if cmd.Flags().Changed("spec-file") {
				if !vaultOperationTakesParams(args[2]) {
					return fmt.Errorf("--spec-file is only supported for fill, webmcp_invoke, prepare_checkout, confirm_transaction, remove_payment_method, update_access_token, and 1Password operations with parameters; passkey operations use --browser-id and optional --field")
				}
				data, err := readVaultJSONFile(cmd, "spec-file")
				if err != nil {
					return err
				}
				raw, paramsSet = string(data), true
			}
			params, err := parseVaultOperationParams(args[2], raw, paramsSet, cmd.Flags().Changed("open"))
			if err != nil {
				return err
			}
			enrollment, _ := cmd.Flags().GetString("enrollment")
			if err := applyVaultAddPaymentMethodEnrollment(args[2], params, enrollment, cmd.Flags().Changed("enrollment")); err != nil {
				return err
			}
			passkey, err := parseVaultPasskeyOperationFlags(cmd, args[2])
			if err != nil {
				return err
			}
			authorize, err := parseVaultAuthorizeCheckout(cmd, args[2])
			if err != nil {
				return err
			}
			if passkey != nil {
				params = &vaultOperationParams{Passkey: passkey}
			} else if authorize != nil {
				params = &vaultOperationParams{Authorize: authorize}
			}
			return getVaultsHandler(cmd).Invoke(cmd.Context(), args[0], args[1], args[2], params, vaultOutput(cmd), open)
		}}
	invoke.Flags().String("params", "", "operation parameters json for fill, webmcp_invoke, prepare_checkout, confirm_transaction, remove_payment_method, or 1pw_* (maximum 128 KiB); omit type and credential values; token update operations require --spec-file")
	invoke.Flags().String("spec-file", "", "operation parameters json file (use '-' for stdin; maximum 128 KiB)")
	invoke.MarkFlagsMutuallyExclusive("params", "spec-file")
	invoke.Flags().Bool("open", false, "open a returned https action url; cardholder links are human-only, agents must share privately")
	invoke.Flags().String("browser-id", "", "browser session id, not a reusable name; used by Link or Shop Pay authorize (with --page-url) and passkey operations")
	invoke.Flags().String("page-url", "", "Link or Shop Pay authorize only: exact current top-level checkout url open in --browser-id")
	invoke.Flags().String("field", "", "passkey field to arm, receive a registration, or disarm; arm/register require it when ambiguous, while disarm omits it to affect every field")
	invoke.Flags().String("enrollment", "", "add_payment_method enrollment flow: hosted (default, cardholder link) or server (issuer backend ingest url)")
	addVaultJSONOutputFlag(invoke)
	items.AddCommand(itemList, itemGet, itemEvents, invoke, newVaultWebMCPCommand(), newVaultDeleteCommand(true))

	wallets := &cobra.Command{Use: "wallets", Short: "connect provider wallets and inspect funding methods"}
	walletCreate := &cobra.Command{Use: "create <vault> <key> --provider <link|agentcard|kernel|shop_pay|vgs> --spec '<json>'", Short: "create a wallet and display its connection or enrollment action", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "create a wallet at an immutable key and follow any returned provider action.\na connected Link wallet pays through card or payment_request items; connected Shop Pay and VGS wallets pay through card items. VGS wallets hold no cards or payment methods themselves. returned descriptions and the flows below are authoritative.\n" + vaultSpecHelp + vaultWalletSpecHelp + "\n" + linkCardFlowHelp + "\n" + vgsCardFlowHelp,
		Example: `  kernel vaults wallets create checkout wallet-1 \
    --provider link --spec '{
      "authorization": {
        "method": "oauth",
        "client": {"type": "kernel_managed"}
      }
    }' --open

  kernel vaults wallets create checkout wallet-1 \
    --provider link --spec '{}' --provider-config-id <config-id> \
    --token-refresh customer --tokens-file ./link-access-token.json

  kernel vaults wallets create checkout wallet-1 \
    --provider agentcard --spec '{}'

  kernel vaults wallets create checkout cardholder --provider kernel --spec '{}' --open

  kernel vaults wallets create checkout shopper --provider shop_pay --spec '{
    "authorization":{"method":"oauth","client":{"type":"kernel_managed"}}
  }' --open

  kernel vaults wallets create checkout vgs-wallet --provider vgs --spec '{}' \
    --provider-config-name my-vgs-vault`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := vaultWalletSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).CreateWallet(cmd.Context(), args[0], args[1], spec, vaultOutput(cmd), open)
		}}
	addVaultSpecFlags(walletCreate, false)
	walletCreate.Flags().String("provider-config-id", "", "bind the new wallet to this provider configuration id (immutable)")
	walletCreate.Flags().String("provider-config-name", "", "bind the new wallet to this provider configuration name (immutable)")
	walletCreate.MarkFlagsMutuallyExclusive("provider-config-id", "provider-config-name")
	walletCreate.Flags().String("token-refresh", "", "imported Link grant refresher: kernel (default) or customer; fixed at wallet creation")
	walletCreate.Flags().String("tokens-file", "", "import Link token json from a file (use '-' for stdin): access_token plus refresh_token for kernel refresh, access_token only for customer refresh")
	walletCreate.Flags().Bool("open", false, "open a returned https connection/enrollment url; cardholder use only, agents must share privately")
	addVaultJSONOutputFlag(walletCreate)
	methods := &cobra.Command{Use: "payment-methods <vault> <key>", Short: "fetch advertised live wallet payment methods", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "fetch payment_methods through the item's GET expansion. the wallet must advertise this expansion.\ndisplays selectable ids and advisory capabilities; never automatically chooses a funding method.\njson returns the item with expanded.payment_methods, like items get --expand payment_methods.",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, _ := cmd.Flags().GetString("project")
			return getVaultsHandler(cmd).GetItem(cmd.Context(), args[0], args[1], 0, []string{"payment_methods"}, resolveProjectSelection(project), vaultOutput(cmd), false)
		}}
	addVaultJSONOutputFlag(methods)
	walletGet := &cobra.Command{Use: "get <vault> <key>", Short: "get a wallet's state and hosted enrollment action", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			project, _ := cmd.Flags().GetString("project")
			wait, _ := cmd.Flags().GetInt64("wait")
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).GetItem(cmd.Context(), args[0], args[1], wait, nil, resolveProjectSelection(project), vaultOutput(cmd), open)
		}}
	walletGet.Flags().Int64("wait", 0, "hold while pending for up to this many seconds (0-60); observe only")
	walletGet.Flags().Bool("open", false, "open a returned https enrollment url; cardholder use only, agents must share privately")
	addVaultJSONOutputFlag(walletGet)
	wallets.AddCommand(walletCreate, walletGet, methods)

	cards := &cobra.Command{Use: "cards", Short: "configure card requests"}
	cards.AddCommand(newVaultCardCommand(false), newVaultCardCommand(true))
	paymentRequests := &cobra.Command{Use: "payment-requests", Short: "configure one-purchase Link or KERNEL payment requests"}
	paymentRequests.AddCommand(newVaultPaymentRequestCommand(false), newVaultPaymentRequestCommand(true))
	cmd.AddCommand(items, wallets, cards, paymentRequests, newVaultCredentialsCommand())
	return cmd
}

func newVaultDeleteCommand(item bool) *cobra.Command {
	use, short, nargs := "delete <vault>", "delete a vault and invalidate all its items", 1
	if item {
		use, short, nargs = "delete <vault> <key>", "delete an item and invalidate its credential", 2
	}
	cmd := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(nargs), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			if item {
				key = args[1]
			}
			yes, _ := cmd.Flags().GetBool("yes")
			return getVaultsHandler(cmd).Delete(cmd.Context(), args[0], key, yes)
		}}
	cmd.Flags().BoolP("yes", "y", false, "skip confirmation prompt; Link, KERNEL, Shop Pay, or VGS recovery may be abandoned only by deleting the card directly after explicit user confirmation")
	return cmd
}

func newVaultCardCommand(update bool) *cobra.Command {
	use, short := "create", "create a card request without authorizing it"
	if update {
		use, short = "update", "update a card spec before it is authorized at a checkout"
	}
	providers := "link|agentcard|kernel|shop_pay|vgs"
	if update {
		providers = "link|agentcard|shop_pay"
	}
	specInput := "--spec '<json>'"
	if !update {
		specInput = "(--spec '<json>' | --spec-file <path|->)"
	}
	example := "  kernel vaults cards " + use + ` checkout order-1 \
    --provider agentcard --spec '{
      "wallet": "wallet-1",
      "merchant": "Example Shop",
      "amount": 1234,
      "currency": "usd"
    }'`
	if !update {
		example += `

  kernel vaults cards create checkout single-use-order --provider kernel --spec-file - <<'JSON'
{"wallet":"wallet-1","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout","single_use_card":{"number":"<card-number>","cvc":"<cvc>","exp_month":"11","exp_year":"2030","name":"Jane Doe","billing_address":{"line1":"1 Main St","city":"Springfield","state":"IL","postal_code":"62701","country":"US"},"reference":"issuer-card-id"}}
JSON

  kernel vaults cards create checkout vgs-order --provider vgs --spec-file - <<'JSON'
{"wallet":"vgs-wallet","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout","card":{"pan_alias":"<persistent-pan-alias>","cvc_alias":"<fresh-cvc-alias>","exp_month":11,"exp_year":2030,"last4":"4242"}}
JSON

  # a card the merchant already holds on file; its checkout asks only for the security code
  kernel vaults cards create checkout vgs-saved-card --provider vgs --spec-file - <<'JSON'
{"wallet":"vgs-wallet","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout","card_mode":"cvc_only","card":{"cvc_alias":"<fresh-cvc-alias>"}}
JSON`
	}
	cmd := &cobra.Command{Use: use + " <vault> <key> --provider <" + providers + "> " + specInput, Short: short, Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: short + `. neither create nor update authorizes a card.
kernel and VGS cards cannot be updated. other requested cards accept a replacement spec.
pending issuance updates preserve omitted optional fields; explicit empty lists clear them. the api restricts fields after
authorization starts; wallet/provider bindings cannot change. an uncertain update
enters recovery_required and must not be retried. checkout cards can be edited
between authorizations, but a Link card authorized at a checkout cannot be updated:
delete it and create a new one. identical creates return existing state without resetting it.
never reconfigure the same item to retry a failed, timed-out, rejected, or indeterminate payment.
an AgentCard recovery item that permits abandonment must be deleted after explicit user confirmation before creating a replacement.

` + linkCardFlowHelp + "\n" + vaultSpecHelp + vaultCardSpecHelp,
		Example: example,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := vaultSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			provider := string(spec["provider"])
			if !update && provider == `"vgs"` && !cmd.Flags().Changed("spec-file") {
				return fmt.Errorf("VGS card aliases are accepted only through --spec-file <path|->, never --spec")
			}
			if update && provider == `"kernel"` {
				return fmt.Errorf("kernel card updates are not supported; " + kernelReplacementHelp)
			}
			if update && provider == `"vgs"` {
				return fmt.Errorf("VGS card updates are not supported; delete the card and create a new item with fresh aliases")
			}
			return getVaultsHandler(cmd).SaveCard(cmd.Context(), args[0], args[1], param.Override[kernel.CardVaultItemSpecUnionParam](spec), update, vaultOutput(cmd))
		}}
	addVaultSpecFlags(cmd, !update)
	addVaultJSONOutputFlag(cmd)
	return cmd
}

func newVaultPaymentRequestCommand(update bool) *cobra.Command {
	use, short := "create", "create a one-purchase payment request without authorizing it"
	if update {
		use, short = "update", "update a payment request spec before checkout authorization"
	}
	cmd := &cobra.Command{
		Use:     use + " <vault> <key> --provider <link|kernel> --spec '<json>'",
		Short:   short,
		Args:    cobra.ExactArgs(2),
		PreRunE: vaultPreRun,
		Long: short + `. payment_request is available only where enabled for the organization.
Link and KERNEL payment requests are parented by their wallet and otherwise follow the
same purchase lifecycle and advertised operations as the corresponding card item.
AgentCard reusable cards remain card items. identical creates retrieve existing state;
never create another request to retry an uncertain payment.

` + linkCardFlowHelp + "\n" + vaultSpecHelp + vaultPaymentRequestSpecHelp,
		Example: "  kernel vaults payment-requests " + use + ` checkout order-1 \
    --provider kernel --spec '{
      "wallet": "cardholder",
      "payment_method_id": "<payment-method-id>",
      "amount": 1234,
      "currency": "usd",
      "merchant_name": "Example Shop",
      "merchant_url": "https://shop.example/checkout",
      "merchant_country": "US"
    }'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := vaultSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			if string(spec["provider"]) != `"link"` && string(spec["provider"]) != `"kernel"` {
				return fmt.Errorf("payment_request items support only link or kernel; AgentCard, Shop Pay, and VGS purchases use vaults cards")
			}
			return getVaultsHandler(cmd).SavePaymentRequest(cmd.Context(), args[0], args[1], param.Override[kernel.PaymentRequestVaultItemSpecUnionParam](spec), update, vaultOutput(cmd))
		},
	}
	addVaultSpecFlags(cmd, false)
	cmd.Flags().Lookup("provider").Usage = "provider: link or kernel (required)"
	addVaultJSONOutputFlag(cmd)
	return cmd
}

func addVaultSpecFlags(cmd *cobra.Command, allowFile bool) {
	cmd.Flags().String("provider", "", "provider: link, agentcard, kernel, shop_pay, or vgs (required)")
	cmd.Flags().String("spec", "", "raw json specification object; see types and examples above")
	_ = cmd.MarkFlagRequired("provider")
	if allowFile {
		cmd.Flags().String("spec-file", "", "protected json specification file (use '-' for stdin; required for caller-supplied KERNEL card details or VGS card aliases)")
		cmd.MarkFlagsOneRequired("spec", "spec-file")
		cmd.MarkFlagsMutuallyExclusive("spec", "spec-file")
	} else {
		_ = cmd.MarkFlagRequired("spec")
	}
}

func vaultSpecFromFlags(cmd *cobra.Command) (map[string]json.RawMessage, error) {
	provider, _ := cmd.Flags().GetString("provider")
	if provider != "link" && provider != "agentcard" && provider != "kernel" && provider != "shop_pay" && provider != "vgs" {
		return nil, fmt.Errorf("--provider must be link, agentcard, kernel, shop_pay, or vgs")
	}
	raw, _ := cmd.Flags().GetString("spec")
	fromFile := cmd.Flags().Lookup("spec-file") != nil && cmd.Flags().Changed("spec-file")
	if fromFile {
		data, err := readVaultJSONFile(cmd, "spec-file")
		if err != nil {
			return nil, err
		}
		raw = string(data)
	}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &spec); err != nil || spec == nil {
		if fromFile {
			return nil, fmt.Errorf("--spec-file must contain a json object")
		}
		return nil, fmt.Errorf("--spec must be a json object")
	}
	if value, ok := spec["provider"]; ok {
		var embedded string
		if err := json.Unmarshal(value, &embedded); err != nil || embedded != provider {
			return nil, fmt.Errorf("spec.provider must match --provider")
		}
	}
	if vaultSpecHasSecrets(json.RawMessage(raw)) {
		return nil, fmt.Errorf("--spec must not contain credentials or tokens; use the dedicated file/stdin inputs")
	}
	scanned := json.RawMessage(raw)
	if provider == "link" {
		// Link card and payment request metadata is caller-defined and forwarded to Link as is, so its keys are not card fields.
		withoutMetadata := maps.Clone(spec)
		delete(withoutMetadata, "metadata")
		scanned, _ = json.Marshal(withoutMetadata)
	}
	if vaultSpecHasCardData(scanned) {
		protectedField := ""
		switch provider {
		case "kernel":
			protectedField = "single_use_card"
		case "vgs":
			protectedField = "card"
		}
		protected, hasProtected := spec[protectedField]
		remainderFields := make(map[string]json.RawMessage, len(spec))
		for key, value := range spec {
			if key != protectedField {
				remainderFields[key] = value
			}
		}
		remainder, _ := json.Marshal(remainderFields)
		if protectedField == "" || !fromFile || !hasProtected || !vaultSpecHasCardData(protected) || vaultSpecHasCardData(remainder) {
			return nil, fmt.Errorf("card details are accepted only in a KERNEL cards create --spec-file single_use_card object or a VGS cards create --spec-file card alias object; otherwise use hosted enrollment")
		}
	}
	spec["provider"], _ = json.Marshal(provider)
	return spec, nil
}
