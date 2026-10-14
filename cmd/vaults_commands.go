package cmd

import (
	"encoding/json"
	"fmt"

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
use wallet and card item types for credit cards and payment checkout instead.

` + vaultCredentialPathsHelp + `

KERNEL-hosted credential flow (fill never submits website forms; WebMCP tools may):
1. create a vault per end user and create a browser with --vault <id-or-name>.
2. navigate to a sensitive form and define its fields in natural top-to-bottom order with credentials create --spec-file; that array order controls the user-facing collection form.
3. present the returned collection url to the user. poll items get --wait 60 for ready.
4. once ready, for ordinary web forms use items invoke <vault> <key> fill --spec-file with
   browser_id and field selectors (writes fields without submitting). when the item
   advertises webmcp_invoke, items webmcp invoke instead binds credential fields to existing
   null inputs of a live WebMCP tool (the tool may submit or have side effects).
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

1. create/select a vault, then create a provider wallet and follow its returned action.
   for kernel, share the hosted card_enrollment url; card details stay on that page.
2. inspect wallets payment-methods for eligibility and reasons before creating a card.
   for Link, select a payment method id explicitly.
3. attach the vault with browsers create --vault <id-or-name>; attachment is required for
   authorizing a Link card at a checkout and for fill. for Link, reach the final checkout
   page before creating the card.
4. create a card request with --provider and --spec json.
5. inspect items get, then use items invoke <vault> <key> <operation> only when advertised.
   follow the operation description and any returned provider action.
6. ready Link and kernel cards use only advertised fill with --params for browser checkout.
   neither exposes aliases or supports egress substitution. kernel fill is locked to
   merchant_url's origin; submit the merchant checkout before the card expires.
   kernel cards have no stored billing fields; enter billing details on the merchant page.
   AgentCard-only checkout aliases support egress substitution with checkout hold,
   approval, and replay; they are not a fallback after fill.
7. after the user submits kernel card checkout, report the observed outcome once with
   confirm_transaction when advertised. ` + kernelTransactionHelp + `.
   in recovery_required, ` + vaultRecoveryHelp + `

` + linkCardFlowHelp + `

permitted checkout domains are displayed when returned. Link card fill is limited to the
origin of the checkout it was authorized at (or merchant_url's origin for a virtual card
authorized without a checkout); kernel card fill uses the exact origin of merchant_url;
AgentCard domains are provider-assigned. there is no domain-setting api.
never supply card data, OAuth codes, ciphertext, or secrets in shell arguments.
use vault-provider-configs for client credentials and wallets create --tokens-file
for imported Link grants; both accept protected files or stdin.
do not automatically repeat failed, timed-out, rejected, or indeterminate payments: inspect
the item, its events, and the merchant's order history, then follow the advertised operations.
when an item explicitly permits user-confirmed abandonment, delete that card before creating a replacement.
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

	items := &cobra.Command{Use: "items", Short: "inspect readiness and collection urls, or invoke collect/fill/webmcp_invoke", Long: "use get --wait 60 to observe readiness and get -o json for schema/version/presence.\nuse invoke collect to obtain a collection url, or invoke fill --spec-file to fill a browser.\nuse webmcp invoke to call a live WebMCP tool with item fields bound to null input slots.\n1Password credentials use the advertised 1pw_* operations instead of collect/fill/webmcp_invoke.\ncreate and edit credentials with vaults credentials; payment items use wallets/cards."}
	itemList := &cobra.Command{Use: "list <vault>", Short: "list items by vault id or name", Args: cobra.ExactArgs(1), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getVaultsHandler(cmd).ListItems(cmd.Context(), args[0], vaultOutput(cmd))
		}}
	addVaultJSONOutputFlag(itemList)
	itemGet := &cobra.Command{Use: "get <vault> <key>", Short: "get item state and any required action", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "get item state, available operations, provider actions, and returned AgentCard checkout aliases.\n--wait is a single bounded server-side observation, not a retry or a guarantee of readiness.\nan item still pending after the wait is returned as-is; ready means populated for credentials, not logged in or paid.\nfor credential edits on an already-ready item, compare versions without --wait. explicitly non-sensitive text/email values are returned; sensitive values and totp seeds are omitted.\nrecovery_required stops waiting and means unresolved, not declined or expired.\n" + vaultRecoveryHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			wait, _ := cmd.Flags().GetInt64("wait")
			expand, _ := cmd.Flags().GetStringSlice("expand")
			open, _ := cmd.Flags().GetBool("open")
			project, _ := cmd.Flags().GetString("project")
			return getVaultsHandler(cmd).GetItem(cmd.Context(), args[0], args[1], wait, expand, resolveProjectSelection(project), vaultOutput(cmd), open)
		}}
	itemGet.Flags().Int64("wait", 0, "hold while pending for up to this many seconds (0-60); observe only")
	itemGet.Flags().StringSlice("expand", nil, "advertised live data to fetch: payment_methods")
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
authorize sends {"type":"authorize"} for payment authorization. for a Link card at the final
checkout page, add --browser-id and --page-url (both or neither) to send browser_id and
page_url; see Link card checkout below.
read the operation description and follow any approval requirements before invoking.
fill requires --params json or --spec-file <path|-> with browser_id (session id, not name)
and the inputs the item's advertised fill description asks for: 1-32 ordered fields
(field, selector) for credentials, kernel cards, and Link virtual cards; no fields for a
Link Pay Token. do not include type, values, or frame ids.
the vault must already be attached to the browser. page_url selects an existing page;
fill never navigates. credentials use declared field names, must omit format, and may
omit page_url only when the api can resolve a unique page. totp codes stay server-generated.
cards require an exact https page_url. Link virtual card fields: number, cvc, exp_month (MM),
exp_year (YYYY), billing_name, billing_line1, billing_line2, billing_city,
billing_state, billing_postal_code, billing_country. expiration requires format MM/YY
or MM/YYYY. optional timeout_ms is 1-30000 (default 10000).
kernel cards support number, expiration (format MM/YY or MM/YYYY), cvc, exp_month, and exp_year.
kernel cards have no stored billing fields: billing_* returns field_unavailable (400) before
any write. enter billing details on the merchant page.
the api searches the page and descendant frames, including payment iframes.
fill is available for credential items and ready Link or kernel cards when advertised, not AgentCard.
Link and kernel cards do not expose aliases or support egress substitution. kernel card fill
is locked to merchant_url's origin and its one-time code expires; submit checkout before expiry.
fill writes real values into the browser; unrestricted browser/cdp access can read them.
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
confirm_transaction requires --params or --spec-file without type, with all of:
status (APPROVED, DECLINED, PENDING, ERROR, CANCELLED), transaction_type (PURCHASE,
AUTHORIZATION, CAPTURE, REFUND, REVERSAL, VERIFICATION, CHARGEBACK, FRAUD), amount
(integer minor units 0..50000, no greater than the approved amount), currency (three
uppercase letters matching the approved purchase currency), and occurred_at (RFC3339).
no other fields are accepted. ` + kernelTransactionHelp + `.
restart_verification takes no parameters. in recovery_required, ` + vaultRecoveryHelp + `
it rotates the approval link without a provider call, only after a classified initial
attestation failure with proof no intent was dispatched. other recovery states must not be retried.
collect/authorize/prepare_checkout/1pw_recover/restart_verification may use --open.
--open on card_enrollment or spend_approval is for a human cardholder on their own machine.
agents must omit --open and share capability links privately, never open them for cardholders.
fill returns value-free per-field outcomes;
completed exits 0, failed/unknown exit nonzero with valid json retained on stdout in -o json.

webmcp_invoke (credential items and ready Link cards when advertised; see items webmcp invoke --help
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

1Password credentials (see credentials --help) use --params or --spec-file without type:
1pw_create_access_request: no browser needed; optional goal (<=140), reason (<=100),
  keywords (1-5 strings); reason and keywords only for single-entry credentials.
  present the returned onepassword:// approval link and instructions to the account
  owner unchanged; invoke it once per credential.
1pw_access_request_status: no browser needed; optional timeout_seconds 0-120 (default 10).
  check status after the account owner has the approval link.
1pw_fill: browser_id and the exact page_url of one open login page on a requested
  origin; entry_id when several approved entries share that origin; optional
  timeout_ms 1-30000. the extension selects fields and submits. fill_submitted exits 0
  and does not confirm sign-in; inspect the page. fill_failed and fill_unknown exit
  nonzero. 1pw_fill can submit the form, so never retry automatically. check the page
  first before retrying.
1pw_update_access_token (stored-token credentials only): --spec-file, never --params,
  with access_token only.
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
  kernel vaults items invoke user-vault github 1pw_fill --params '{"browser_id":"<browser-id>","page_url":"https://github.com/login","entry_id":"<entry-id>"}'
  kernel vaults items invoke checkout order-1 fill --params '{"browser_id":"browser-session-id","page_url":"https://shop.example/checkout","fields":[{"field":"number","selector":"#cardNumber"},{"field":"expiration","format":"MM/YY","selector":"#cardExpiry"},{"field":"cvc","selector":"#cardCvc"}]}' -o json
  kernel vaults items invoke checkout order-1 confirm_transaction --params '{"status":"APPROVED","transaction_type":"PURCHASE","amount":100,"currency":"USD","occurred_at":"2026-09-01T12:00:00Z"}'
  kernel vaults items invoke checkout order-1 restart_verification --open`,
		RunE: func(cmd *cobra.Command, args []string) error {
			open, _ := cmd.Flags().GetBool("open")
			raw, _ := cmd.Flags().GetString("params")
			paramsSet := cmd.Flags().Changed("params")
			if args[2] == "1pw_update_access_token" && paramsSet {
				return fmt.Errorf("1pw_update_access_token accepts the access token only through --spec-file <path|->, never command-line arguments")
			}
			if cmd.Flags().Changed("spec-file") {
				if !vaultOperationTakesParams(args[2]) {
					return fmt.Errorf("--spec-file is only supported for fill, webmcp_invoke, prepare_checkout, confirm_transaction, and 1Password operations with parameters")
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
			authorize, err := parseVaultAuthorizeCheckout(cmd, args[2])
			if err != nil {
				return err
			}
			if authorize != nil {
				params = &vaultOperationParams{Authorize: authorize}
			}
			return getVaultsHandler(cmd).Invoke(cmd.Context(), args[0], args[1], args[2], params, vaultOutput(cmd), open)
		}}
	invoke.Flags().String("params", "", "operation parameters json for fill, webmcp_invoke, prepare_checkout, confirm_transaction, or 1pw_* (maximum 128 KiB); omit type and credential values; 1pw_update_access_token requires --spec-file")
	invoke.Flags().String("spec-file", "", "operation parameters json file (use '-' for stdin; maximum 128 KiB)")
	invoke.MarkFlagsMutuallyExclusive("params", "spec-file")
	invoke.Flags().Bool("open", false, "open a returned https action url; cardholder links are human-only, agents must share privately")
	invoke.Flags().String("browser-id", "", "authorize a Link card at checkout: browser session id with this vault attached (requires --page-url)")
	invoke.Flags().String("page-url", "", "authorize a Link card at checkout: exact current top-level checkout url open in --browser-id (requires --browser-id)")
	addVaultJSONOutputFlag(invoke)
	items.AddCommand(itemList, itemGet, itemEvents, invoke, newVaultWebMCPCommand(), newVaultDeleteCommand(true))

	wallets := &cobra.Command{Use: "wallets", Short: "connect provider wallets and inspect funding methods"}
	walletCreate := &cobra.Command{Use: "create <vault> <key> --provider <link|agentcard|kernel> --spec '<json>'", Short: "create a wallet and display its connection or enrollment action", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "create a wallet at an immutable key and follow the returned provider action.\na connected Link wallet pays at checkout through cards; its returned description and the flow below describe how.\n" + vaultSpecHelp + vaultWalletSpecHelp + "\n" + linkCardFlowHelp,
		Example: `  kernel vaults wallets create checkout wallet-1 \
    --provider link --spec '{
      "authorization": {
        "method": "oauth",
        "client": {"type": "kernel_managed"}
      }
    }' --open

  kernel vaults wallets create checkout wallet-1 \
    --provider agentcard --spec '{}'

  kernel vaults wallets create checkout cardholder --provider kernel --spec '{}' --open`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := vaultWalletSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).CreateWallet(cmd.Context(), args[0], args[1], spec, vaultOutput(cmd), open)
		}}
	addVaultSpecFlags(walletCreate)
	walletCreate.Flags().String("provider-config-id", "", "bind the new wallet to this provider configuration id (immutable)")
	walletCreate.Flags().String("provider-config-name", "", "bind the new wallet to this provider configuration name (immutable)")
	walletCreate.MarkFlagsMutuallyExclusive("provider-config-id", "provider-config-name")
	walletCreate.Flags().String("tokens-file", "", "import Link access_token and refresh_token json from a file (use '-' for stdin)")
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
	cmd.AddCommand(items, wallets, cards, newVaultCredentialsCommand())
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
	cmd.Flags().BoolP("yes", "y", false, "skip confirmation prompt; abandon recovery only after explicit user confirmation")
	return cmd
}

func newVaultCardCommand(update bool) *cobra.Command {
	use, short := "create", "create a card request without authorizing it"
	if update {
		use, short = "update", "update a card spec before it is authorized at a checkout"
	}
	providers := "link|agentcard|kernel"
	if update {
		providers = "link|agentcard"
	}
	cmd := &cobra.Command{Use: use + " <vault> <key> --provider <" + providers + "> --spec '<json>'", Short: short, Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: short + `. neither create nor update authorizes a card.
kernel cards cannot be updated. other requested cards accept a replacement spec.
pending issuance updates preserve omitted optional fields; explicit empty lists clear them. the api restricts fields after
authorization starts; wallet/provider bindings cannot change. an uncertain update
enters recovery_required and must not be retried. checkout cards can be edited
between authorizations, but a Link card authorized at a checkout cannot be updated:
delete it and create a new one. identical creates return existing state without resetting it.
never reconfigure the same item to retry a failed, timed-out, rejected, or indeterminate payment.
an AgentCard recovery item that permits abandonment must be deleted after explicit user confirmation before creating a replacement.

` + linkCardFlowHelp + "\n" + vaultSpecHelp + vaultCardSpecHelp,
		Example: "  kernel vaults cards " + use + ` checkout order-1 \
    --provider agentcard --spec '{
      "wallet": "wallet-1",
      "merchant": "Example Shop",
      "amount": 1234,
      "currency": "usd"
    }'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := vaultSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			if update && string(spec["provider"]) == `"kernel"` {
				return fmt.Errorf("kernel card updates are not supported; " + kernelReplacementHelp)
			}
			return getVaultsHandler(cmd).SaveCard(cmd.Context(), args[0], args[1], param.Override[kernel.CardVaultItemSpecUnionParam](spec), update, vaultOutput(cmd))
		}}
	addVaultSpecFlags(cmd)
	addVaultJSONOutputFlag(cmd)
	return cmd
}

func addVaultSpecFlags(cmd *cobra.Command) {
	cmd.Flags().String("provider", "", "provider: link, agentcard, or kernel (required)")
	cmd.Flags().String("spec", "", "raw json specification object (required); see types and examples above")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("spec")
}

func vaultSpecFromFlags(cmd *cobra.Command) (map[string]json.RawMessage, error) {
	provider, _ := cmd.Flags().GetString("provider")
	if provider != "link" && provider != "agentcard" && provider != "kernel" {
		return nil, fmt.Errorf("--provider must be link, agentcard, or kernel")
	}
	raw, _ := cmd.Flags().GetString("spec")
	var spec map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &spec); err != nil || spec == nil {
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
	if provider == "kernel" && vaultSpecHasCardData(json.RawMessage(raw)) {
		return nil, fmt.Errorf("--spec must not contain card details; use hosted enrollment")
	}
	spec["provider"], _ = json.Marshal(provider)
	return spec, nil
}
