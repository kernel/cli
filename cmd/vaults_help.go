package cmd

// Keep these help types in sync with https://api.onkernel.com/spec.yaml.
const vaultSpecHelp = `
--spec takes the specification object, not the {type, spec} request envelope.
--provider supplies spec.provider; omit it from JSON or supply the same value.
The API validates provider-specific fields. Values are forwarded without defaults
or normalization. Never include card data, OAuth tokens, or provider secrets.

// Keep these types in sync with https://api.onkernel.com/spec.yaml.
// TypeScript notation: ? means optional. Other fields are required.
// The provider field below is supplied by --provider.
`

const vaultWalletSpecHelp = `
type LinkWalletSpec = {
  provider: "link";
  authorization: {
    method: "oauth";
    client: { type: "kernel_managed" };
  };
};

type AgentCardWalletSpec = {
  provider: "agentcard";
  user_id?: string;           // usr_...; already enrolled under the same configuration
  provider_config?: {         // register with vaults provider-configs; select by
    id?: string;              // either id or name. Omit provider_config to use
    name?: string;            // Kernel-managed credentials
  };
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
