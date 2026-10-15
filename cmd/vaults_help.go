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

const vaultWalletSpecHelp = `
for --provider kernel, use --spec '{}'. share the returned card_enrollment url
with the cardholder; never enter card details in the cli. use wallets get --wait 60
to observe connection, then wallets payment-methods to inspect token eligibility.
a connected wallet may still be ineligible while network token enrollment runs.
` + kernelWalletHelp + `. ` + kernelEligibilityHelp + `.
kernel wallets accept neither provider configuration flags nor --tokens-file.
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

// one card stored with KERNEL-managed credentials. creation returns a card_enrollment
// action: the cardholder enters the card on a KERNEL-hosted page, and the wallet
// connects once the card is stored. KERNEL then enrolls it for an agentic network
// token when the issuer supports it; until then payment-methods reports
// capabilities.single_use_card.eligible=false and authorize returns 400.
// the card number never reaches KERNEL or the cli. no provider config or tokens.
type KernelWalletSpec = {
  provider: "kernel";
};
`

const vaultCardSpecHelp = `
type LinkCardSpec = {
  provider: "link";
  wallet: string;             // wallet item key
  payment_method_id: string;  // from wallets payment-methods
  amount: number;             // integer minor units; 1..500000; a virtual card allows at most 50000
  currency: string;           // three letters
  merchant_name: string;      // 1..255 characters
  merchant_url: string;       // uri; a virtual card authorized without a checkout fills only this origin
  context: string;            // at least 100 characters
  line_items?: LinkLineItem[];
  totals?: LinkTotal[];
  metadata?: Record<string, string>;
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

// one live purchase with a KERNEL-enrolled card. authorize, then fill on merchant_url's
// origin, then submit checkout before expires_at (one hour after authorization). fill
// delivers card details for a few minutes; submit promptly. repeated fill is safe and never
// submits. supported cards without an agentic network token are authorized without a
// one-time code. Mastercard purchases need no hosted approval. a Visa purchase returns a
// spend_approval action: the cardholder approves it with a Visa passkey (the link expires
// after 30 minutes) before details are issued. amount, merchant_url and merchant_country
// are recorded but do not restrict browser payment requests. updates are not supported.
type KernelCardSpec = {
  provider: "kernel";
  wallet: string;             // KERNEL wallet item key
  amount: number;             // integer minor units; 1..50000
  currency: string;           // iso 4217 three letters
  merchant_name: string;      // 1..255 characters
  merchant_url: string;       // https merchant checkout url; fill is locked to its origin
  merchant_country?: string;  // iso 3166-1 alpha-2; required for Visa cards
  merchant_category?: string; // actual category, 1..255 characters; omit rather than invent
  merchant_category_code?: string; // actual four-digit MCC, not 0000; omit rather than invent
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

permitted domains are not configurable in the spec. Link card fill is limited to the origin
of the checkout the card was authorized at; AgentCard domains are provider-assigned.
`
