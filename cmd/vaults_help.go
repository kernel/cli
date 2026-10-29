package cmd

// Keep these help types in sync with https://api.onkernel.com/spec.yaml.
const vaultSpecHelp = `
--spec takes the specification object, not the {type, spec} request envelope.
--provider supplies spec.provider; omit it from json or supply the same value.
the api validates provider-specific fields. values are forwarded without defaults
or normalization. never include card data, OAuth tokens, or provider secrets in --spec.

// keep these types in sync with https://api.onkernel.com/spec.yaml.
// TypeScript notation: ? means optional. other fields are required.
// the provider field below is supplied by --provider.
`

const vaultRecoveryHelp = "only if restart_verification is advertised in available_operations, invoke it with explicit user approval each time to get a fresh spend_approval link. restart grants no spend consent: the cardholder must verify and explicitly approve spending again on the new link. if the result is unclear, re-get and share the new action if present; invoke once more only if restart_verification is still advertised and the user approves again. otherwise " + kernelSupportGuidance

const shopPayRecoveryHelp = "follow status_reason. Shop Pay issuance recovery may be explicitly abandoned by deleting the card directly after user confirmation, then creating a new purchase for separate approval. recovery while reporting a merchant transaction cannot be deleted; reconcile it with support. deletion or recreation is never proof that no payment occurred"

const shopPayFlowHelp = `Shop Pay checkout (the item's available_operations descriptions are authoritative):
1. create a Shop Pay wallet with --provider shop_pay and Kernel-managed OAuth authorization,
   share the returned shop_pay_oauth URL unchanged with the shopper, and wait for connected.
2. inspect wallets payment-methods. a card spec may select payment_method_id or omit it to
   use the shopper's preferred card.
3. create a card with amount, USD currency, merchant details, shopper-facing context, and
   totals containing exactly one positive total no greater than amount.
4. after explicit user approval, invoke authorize. at final checkout pass --browser-id and
   --page-url together so Shop receives that page as the merchant and fill is locked to its
   origin; omit both to use merchant_url. share any approval URL unchanged and observe the item.
5. when fill is advertised, fill only the fields requested by the checkout and submit it.
   ready means a credential was issued, not that the merchant accepted payment.
6. report the observed merchant outcome once with confirm_transaction. Shop Pay accepts
   PURCHASE, AUTHORIZATION, or CAPTURE; APPROVED may include order_id/order_url, while other
   outcomes may include failure_code. reporting consumes the card whatever the outcome.
never infer success or retry an unknown payment; reconcile the merchant order and original item.`

const vaultWalletSpecHelp = `
for --provider kernel, use --spec '{}'. if creation returns card_enrollment,
share it with the cardholder; never enter card details in the cli. where the wallet
advertises add_payment_method, creation instead returns an empty connected wallet and
each card is enrolled through that operation. use wallets get --wait 60 to observe
connection, then wallets payment-methods to inspect card status and token eligibility.
a connected wallet may have zero cards or cards that are still ineligible while network
token enrollment runs.
` + kernelWalletHelp + `. ` + kernelEligibilityHelp + `.
kernel wallets accept neither provider configuration flags nor --tokens-file.
Shop Pay wallets also accept neither: they always use Kernel-managed OAuth. creation returns
shop_pay_oauth; share its URL unchanged with the shopper and observe with wallets get --wait 60.
for Link and AgentCard, omit config selection to preserve managed defaults.
--provider-config-id and --provider-config-name are mutually exclusive.
for Link with selection flags, use --spec '{}' and --tokens-file <path|->.
alternatively, set the customer_managed client and provider_config in --spec,
with tokens still supplied only through --tokens-file. its json object must have
access_token and refresh_token strings from the same currently valid grant.
config client credentials belong in vault-provider-configs, not in a wallet.

your backend completes OAuth before import and must stop refreshing the grant
after import: KERNEL owns subsequent rotation. duplicate creation never replaces
tokens. bindings are immutable; renaming a config preserves the resolved id.
if an imported wallet becomes degraded, obtain a fresh grant in your backend and
import it under a new wallet key for new work. this does not rebind existing cards
or resolve uncertain payments. retain old items for reconciliation; do not repeat
an uncertain payment through the new wallet. there is no in-place reauthorization.

type ProviderConfigReference = { id: string } | { name: string };

type LinkWalletSpec = {
  provider: "link";
  authorization: {
    method: "oauth";
    client: { type: "kernel_managed" } |
      { type: "customer_managed"; provider_config: ProviderConfigReference }; // requires --tokens-file
  };
};

type AgentCardWalletSpec = {
  provider: "agentcard";
  provider_config?: ProviderConfigReference; // omit for KERNEL-managed credentials
  user_id?: string; // usr_...; enrolled in this organization under the same config
};

// cards stored with KERNEL-managed credentials. each is a wallet payment method.
// where add_payment_method is advertised, creation returns an empty connected wallet;
// invoke add_payment_method once for each card enrollment. otherwise creation returns
// the default card's card_enrollment action and connects after that card is stored.
// KERNEL enrolls stored cards for agentic network tokens when supported; until then
// payment-methods can report capabilities.single_use_card.eligible=false.
// card numbers never reach KERNEL or the cli. no provider config or tokens.
type KernelWalletSpec = {
  provider: "kernel";
};

type ShopPayWalletSpec = {
  provider: "shop_pay";
  authorization: {
    method: "oauth";
    client: { type: "kernel_managed" };
  };
};

` + shopPayFlowHelp + `
`

const vaultCardSpecHelp = `
type LinkCardSpec = {
  provider: "link";
  wallet: string;             // wallet item key
  payment_method_id: string;  // from wallets payment-methods
  amount: number;             // integer minor units; 1..50000; with recurring, the per-period spending limit
  currency: string;           // three letters
  merchant_name: string;      // 1..255 characters
  merchant_url: string;       // uri; a virtual card authorized without a checkout fills only this origin
  context: string;            // at least 100 characters
  line_items?: LinkLineItem[];
  totals?: LinkTotal[];
  metadata?: Record<string, string>;
  recurring?: LinkRecurring;
};

// recurring terms cannot change after authorization. interval_count defaults to 1.
type LinkRecurring = {
  interval: "day" | "week" | "month" | "year";
  interval_count?: number;    // integer >= 1; e.g. 2 with week means every two weeks
};

type AgentCardCardSpec = {
  provider: "agentcard";
  wallet: string;             // wallet item key
  merchant: string;           // approval-screen name; 1..120 characters
  amount: number;             // integer minor units; 1..9007199254740991
  currency: string;           // three letters
  card_id?: string;           // opaque AgentCard id, pass through unchanged; else chosen at approval
  checkout_origin?: string;   // top-level checkout origin for autopilot matching; update omission removes it
};

// one live purchase with a KERNEL-enrolled card. authorize obtains a network token and
// one-time code for fill on merchant_url's origin until expires_at. Mastercard purchases
// need no hosted approval. a Visa purchase returns a spend_approval action: the cardholder
// approves it with a Visa passkey (the link expires after 30 minutes) before the code is
// issued. when the card network cannot issue an agentic token, spend_approval instead
// collects a fresh security code; after collection, fill before expires_at, normally within
// five minutes. an expired purchase requires a new card. updates are not supported.
type KernelCardSpec = {
  provider: "kernel";
  wallet: string;             // KERNEL wallet item key
  payment_method_id?: string; // from wallets payment-methods; omit only for the legacy default card; a wallet with no default card requires it
  amount: number;             // integer minor units; 1..1000000
  currency: string;           // iso 4217 three letters
  merchant_name: string;      // 1..255 characters
  merchant_url: string;       // https merchant checkout url; fill is locked to its origin
  merchant_country?: string;  // iso 3166-1 alpha-2; required for Visa cards
  merchant_category?: string; // actual category, 1..255 characters; omit rather than invent
  merchant_category_code?: string; // actual four-digit MCC, not 0000; omit rather than invent
};

type ShopPayCardSpec = {
  provider: "shop_pay";
  wallet: string;             // connected Shop Pay wallet item key
  payment_method_id?: string; // from wallets payment-methods; omit for the shopper's preferred card
  amount: number;             // approval limit in minor units; 1..1000000
  currency: string;           // USD only
  merchant_name: string;      // 1..255 characters
  merchant_url: string;       // merchant https URL; used when authorize omits checkout context
  merchant_country?: string;  // iso 3166-1 alpha-2
  context: string;            // 100..500 characters shown to the shopper
  line_items?: ShopPayLineItem[];
  totals: ShopPayTotal[];     // exactly one positive total, no greater than amount
};

type ShopPayLineItem = {
  name: string;
  quantity: number;           // integer >= 1
  unit_amount: number;        // integer minor units >= 0
};

type ShopPayTotal = {
  type: string;               // e.g. subtotal, fulfillment, tax, discount, total
  amount: number;             // integer minor units; discounts are negative
};

type LinkLineItem = {
  name: string;
  quantity?: number;          // integer >= 1
  unit_amount?: number;       // integer minor units
  description?: string;
  sku?: string;
  url?: string;
  image_url?: string;
  product_url?: string;
  totals?: LinkTotal[];
};

type LinkTotal = {
  type: string;
  display_text: string;
  amount: number;             // integer minor units
};

for a recurring Link card, amount is the spending limit for each period. the merchant
schedules later charges, and the card stays ready after each charge until it expires or is
deleted; do not create a new card each period. deleting the card, its wallet, or its vault
first revokes the Link credential so later charges decline. if deletion returns 409
provider_revocation_pending, retry deletion. deletion does not cancel the merchant subscription. recurring
terms cannot be changed after authorization.

kernel cards cannot be updated. ` + kernelReplacementHelp + `.
in recovery_required, ` + vaultRecoveryHelp + `
invoke authorize only when advertised. for Visa, present the returned
spend_approval url to the cardholder and poll items get --wait 60 until ready.
Mastercard may become ready without hosted approval. neither ready nor fill
confirms merchant payment. fill and submit before expires_at; never pass PAN to cli.
after the user submits checkout, report the observed outcome once with confirm_transaction
when advertised. ` + kernelTransactionHelp + `. ` + kernelTerminalHelp + `.
kernel cards have no stored billing fields; billing_* returns field_unavailable before
any write. enter billing details on the merchant page.

card updates replace the whole spec, so omitting checkout_origin from an update removes
its existing value. for non-prepared authorization, checkout_origin is forwarded to
AgentCard for eligible autopilot rule matching. use a canonical https origin (lowercase
host, omit the default port, and include no path), or localhost http for test pages.
KERNEL does not validate this declaration against the page open in the browser. omitting
it retains the current approval flow; even when supplied, autopilot may not apply and
user approval may still be required. this field does not guarantee payment success.
prepared checkout uses preparation merchant_origin instead.

permitted domains are not configurable in the spec. Link and Shop Pay card fill is limited
to the checkout origin when authorized with browser_id/page_url, or merchant_url's origin
when authorized without them; AgentCard domains are provider-assigned.
for KERNEL purchases, ` + kernelFundingStatusHelp + `.

` + shopPayFlowHelp + `
`

const vaultPaymentRequestSpecHelp = `
payment_request items are one Link or KERNEL purchase parented by a wallet. they have
the same authorize, fill, confirmation, recovery, and deletion lifecycle as the
corresponding card purchase. availability is organization-controlled. AgentCard reusable
cards remain card items. create and update require type payment_request on the wire; the
cli supplies it. updates are subject to the same authorization-state restrictions as cards.

type LinkPaymentRequestSpec = {
  provider: "link";
  wallet: string;             // connected Link wallet item key
  payment_method_id: string;  // from wallets payment-methods
  amount: number;             // integer minor units; 1..50000; with recurring, per-period limit
  currency: string;           // three letters
  merchant_name: string;      // 1..255 characters
  merchant_url: string;       // uri; virtual-card fill is limited to this origin
  context: string;            // at least 100 characters
  line_items?: LinkLineItem[];
  totals?: LinkTotal[];
  metadata?: Record<string, string>;
  recurring?: LinkRecurring;
};

type KernelPaymentRequestSpec = {
  provider: "kernel";
  wallet: string;             // KERNEL wallet item key
  payment_method_id: string;  // required; active eligible method from wallets payment-methods
  amount: number;             // integer minor units; 1..1000000
  currency: string;           // iso 4217 three letters
  merchant_name: string;      // 1..255 characters
  merchant_url: string;       // https merchant checkout url; fill is locked to its origin
  merchant_country?: string;  // iso 3166-1 alpha-2; required for Visa cards
  merchant_category?: string; // actual category; omit rather than invent
  merchant_category_code?: string; // actual four-digit MCC, not 0000; omit rather than invent
};

type LinkRecurring = {
  interval: "day" | "week" | "month" | "year";
  interval_count?: number;    // integer >= 1
};

type LinkLineItem = {
  name: string;
  quantity?: number;
  unit_amount?: number;
  description?: string;
  sku?: string;
  url?: string;
  image_url?: string;
  product_url?: string;
  totals?: LinkTotal[];
};

type LinkTotal = { type: string; display_text: string; amount: number };

KERNEL payment requests must select payment_method_id and remain bound to it. removing
that method makes the purchase unusable rather than moving it to another card. for KERNEL
purchases, ` + kernelFundingStatusHelp + `. Link payment requests follow the Link checkout
flow below. never retry an uncertain payment by creating another payment_request;
reconcile the original item and merchant outcome.
`
