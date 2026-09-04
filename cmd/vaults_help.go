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
  user_id?: string; // usr_...; already enrolled in this organization
};

AgentCard sandbox/live mode is deployment-controlled, not a per-item flag.
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
  test: boolean;             // true = test; false = live; required
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

AgentCard sandbox/live mode is deployment-controlled; its spec has no test field.
Permitted domains are provider-assigned, not configurable in the spec.
`
