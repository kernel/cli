package cmd

// Keep these help types in sync with https://api.onkernel.com/spec.yaml.
const vaultSpecHelp = `
--spec takes the specification object, not the {type, spec} request envelope.
--provider supplies spec.provider; omit it from JSON or supply the same value.
The API validates provider-specific fields. Values are forwarded without defaults
or normalization. Never include card data, OAuth tokens, or provider secrets in --spec.

// Keep these types in sync with https://api.onkernel.com/spec.yaml.
// TypeScript notation: ? means optional. Other fields are required.
// The provider field below is supplied by --provider.
`

const vaultWalletSpecHelp = `
Omit config selection to preserve Kernel-managed defaults.
--provider-config-id and --provider-config-name are mutually exclusive.
For Link with selection flags, use --spec '{}' and --tokens-file <path|->.
Alternatively, set the customer_managed client and provider_config in --spec,
with tokens still supplied ONLY through --tokens-file. Its JSON object must have
access_token and refresh_token strings from the same currently valid grant.
Config client credentials belong in vault-provider-configs, not in a wallet.

Your backend completes OAuth before import and must stop refreshing the grant
after import: Kernel owns subsequent rotation. Duplicate creation never replaces
tokens. Bindings are immutable; renaming a config preserves the resolved ID.
If an imported wallet becomes degraded, obtain a fresh grant in your backend and
import it under a NEW wallet key for NEW work. This does not rebind existing cards
or resolve uncertain payments. Retain old items for reconciliation; do not repeat
an uncertain payment through the new wallet. There is no in-place reauthorization.

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
  provider_config?: ProviderConfigReference; // omit for Kernel-managed credentials
  user_id?: string; // usr_...; enrolled in this organization under the SAME config
};
`

const vaultCardSpecHelp = `
Create the card only after reaching final checkout and gathering the final spend details.
Creation starts human approval. Card requests are immutable; changed purchase details
require cancellation and a new item. Never replace an uncertain payment.

type LinkCardSpec = {
  provider: "link";
  wallet: string;             // wallet item key
  payment_method_id: string;  // from wallets payment-methods
  amount: number;             // integer minor units; 1..500000
  currency: string;           // three letters
  merchant_name: string;      // 1..255 characters
  merchant_url: string;       // URI
  context: string;            // at least 100 characters
  line_items?: LinkLineItem[];
  totals?: LinkTotal[];
  metadata?: Record<string, string>;
  expires_at?: number;        // int64
};

type AgentCardCardSpec = {
  provider: "agentcard";
  wallet: string;             // wallet item key
  merchant: string;           // approval-screen name; 1..120 characters
  amount: number;             // integer minor units; 1..9007199254740991
  currency: string;           // three letters
  card_id?: string;           // vc_...; otherwise chosen at approval
};

Permitted domains are provider-assigned, not configurable in the spec.
`

const vaultLinkPurchaseTypesHelp = `
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
`

const vaultPaymentTokenSpecHelp = `
Create a Link payment token only after reaching final checkout. Kernel inspects the
vault-linked browser, reveals Link's agent controls, and binds the request to the
observed Stripe merchant. Do not inspect hidden controls or provide merchant_account_id.
If creation returns lpt_not_supported, create a card instead. No other error is a
fallback signal. Creation starts human approval and requests are immutable.
After approval, invoke fill; filling authenticates the checkout but does not submit it.

type LinkPaymentTokenSpec = {
  provider: "link";
  wallet: string;             // connected wallet item key
  browser_id: string;         // active vault-linked browser session ID
  page_url: string;           // exact final checkout page URL
  payment_method_id: string;  // from wallets payment-methods
  amount: number;             // integer minor units; 1..500000
  currency: string;           // three letters
  context: string;            // at least 100 characters
  line_items?: LinkLineItem[];
  totals?: LinkTotal[];
  metadata?: Record<string, string>;
  expires_at?: number;        // int64
};
`
