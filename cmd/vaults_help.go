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

A wallet's configuration and provider binding are fixed at creation: it cannot be
moved to a different configuration later, and renaming one does not rebind it.
Omitting user_id returns a hosted enrollment action for the user to complete.

Link wallets on your own OAuth client are created by importing an existing grant's
access and refresh tokens. Those tokens must never be passed to the CLI; create
such wallets from your backend instead. Only the kernel_managed client shown above
is supported here.
`

const vaultCardSpecHelp = `
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

Permitted domains are provider-assigned, not configurable in the spec.
`
