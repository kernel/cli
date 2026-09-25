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
	cmd.Flags().Lookup("output").Usage = "Output format: json for display-safe API fields"
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
		label := "vault ID or name"
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
		Use: "vaults", Aliases: []string{"vault"}, Short: "Collect user credentials and manage payment credentials",
		Long: `Collect user credentials and manage payment credentials; fill never submits website forms.

Do not use credential items to store, collect, or fill credit card data.
Use wallet and card item types for credit cards and payment checkout instead.

User credential flow:
1. Create a vault per end user and create a browser with --vault <id-or-name>.
2. Navigate to a sensitive form and define its fields with credentials create --spec-file.
3. Present the returned collection URL to the user. Poll items get --wait 60 for ready.
4. Use items invoke <vault> <key> fill --spec-file with browser_id and field selectors.
Use credentials update --version for edits, or items invoke collect to reopen the form.
Credential values belong in protected files/stdin, never command-line arguments.
See credentials --help and items invoke --help for examples.

Payment credential flow:

Optionally select a project with --project <id-or-name> or KERNEL_PROJECT.
Otherwise, the API resolves the project from your credentials and its defaults.
Vault names, item keys, and project ownership are immutable.

1. Create/select a vault, then create a provider wallet and follow its returned action.
2. For Link, list wallet payment methods and select an ID explicitly. Create a browser
   with --vault <id-or-name>, navigate to final checkout, and gather final spend details.
3. Create one Link card with that browser ID and exact page URL. Kernel inspects the
   checkout and internally selects a Link payment token or virtual card. Creation starts approval.
4. Share the returned approval URL and retrieve the item until fill is advertised.
5. Invoke fill with the parameters described by the advertised operation; browser-vault
   attachment is required for fill. Ready Link cards use only advertised fill. Link cards
   do not expose aliases or support egress substitution. AgentCard-only checkout aliases
   support egress substitution with checkout hold, approval, and replay.
6. Fill never submits payment; inspect the checkout and submit separately when ready.
   Virtual-card selection is creation-time fallback, not a fallback after fill. Inspect
   items get/events for the outcome.

Permitted checkout domains are provider-assigned and displayed when returned;
there is no domain-setting API.
Never supply card data, OAuth codes, ciphertext, or secrets in shell arguments.
Use vault-provider-configs for client credentials and wallets create --tokens-file
for imported Link grants; both accept protected files or stdin.
Never automatically retry failed, timed-out, rejected, or indeterminate payments.
When an item explicitly permits user-confirmed abandonment, delete that card before creating a replacement.
JSON output preserves returned public fields but omits unknown/opaque provider data.`,
		Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
	}

	create := &cobra.Command{Use: "create --name <name>", Short: "Create or retrieve a vault by immutable name", Args: cobra.NoArgs, PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			return getVaultsHandler(cmd).Create(cmd.Context(), name, vaultOutput(cmd))
		}}
	create.Flags().String("name", "", "Immutable vault name (required)")
	_ = create.MarkFlagRequired("name")
	addVaultJSONOutputFlag(create)

	list := &cobra.Command{Use: "list", Short: "List vaults in the effective project", Args: cobra.NoArgs, PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, _ := cmd.Flags().GetInt64("limit")
			offset, _ := cmd.Flags().GetInt64("offset")
			project, _ := cmd.Flags().GetString("project")
			return getVaultsHandler(cmd).List(cmd.Context(), limit, offset, resolveProjectSelection(project), vaultOutput(cmd))
		}}
	list.Flags().Int64("limit", 20, "Maximum vaults to return (1-100)")
	list.Flags().Int64("offset", 0, "Number of vaults to skip")
	addVaultJSONOutputFlag(list)

	get := &cobra.Command{Use: "get <vault>", Short: "Get a vault by ID or name", Args: cobra.ExactArgs(1), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getVaultsHandler(cmd).Get(cmd.Context(), args[0], vaultOutput(cmd))
		}}
	addVaultJSONOutputFlag(get)
	cmd.AddCommand(create, list, get, newVaultDeleteCommand(false))

	items := &cobra.Command{Use: "items", Short: "Inspect readiness and collection URLs, or invoke collect/fill", Long: "Use get --wait 60 to observe readiness and get -o json for schema/version/presence.\nUse invoke collect to obtain a collection URL, or invoke fill --spec-file to fill a browser.\nCreate and edit credentials with vaults credentials; payment items use wallets/cards."}
	itemList := &cobra.Command{Use: "list <vault>", Short: "List items by vault ID or name", Args: cobra.ExactArgs(1), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getVaultsHandler(cmd).ListItems(cmd.Context(), args[0], vaultOutput(cmd))
		}}
	addVaultJSONOutputFlag(itemList)
	itemGet := &cobra.Command{Use: "get <vault> <key>", Short: "Get item state and any required action", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "Get item state, available operations, provider actions, and returned AgentCard checkout aliases.\n--wait is a single bounded server-side observation, not a retry or a guarantee of readiness.\nAn item still pending after the wait is returned as-is; ready means populated for credentials, not logged in or paid.\nFor credential edits on an already-ready item, compare versions without --wait. Explicitly non-sensitive text/email values are returned; sensitive values and TOTP seeds are omitted.\nrecovery_required stops waiting and means unresolved, not declined or expired.\nReconcile with the provider or support; do not retry, delete, or replace the payment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			wait, _ := cmd.Flags().GetInt64("wait")
			expand, _ := cmd.Flags().GetStringSlice("expand")
			open, _ := cmd.Flags().GetBool("open")
			project, _ := cmd.Flags().GetString("project")
			return getVaultsHandler(cmd).GetItem(cmd.Context(), args[0], args[1], wait, expand, resolveProjectSelection(project), vaultOutput(cmd), open)
		}}
	itemGet.Flags().Int64("wait", 0, "Hold while pending for up to this many seconds (0-60); observe only")
	itemGet.Flags().StringSlice("expand", nil, "Advertised live data to fetch: payment_methods")
	itemGet.Flags().Bool("open", false, "Open a returned HTTPS action URL in your browser")
	addVaultJSONOutputFlag(itemGet)
	itemEvents := &cobra.Command{Use: "events <vault> <key>", Short: "Read immutable item events without retrying payments", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			after, _ := cmd.Flags().GetString("after")
			wait, _ := cmd.Flags().GetInt64("wait")
			return getVaultsHandler(cmd).Events(cmd.Context(), args[0], args[1], after, wait, vaultOutput(cmd))
		}}
	itemEvents.Flags().String("after", "", "Return events after this event ID (use the last ID from the previous response)")
	itemEvents.Flags().Int64("wait", 0, "Long-poll once for new events (0-60 seconds)")
	addVaultJSONOutputFlag(itemEvents)
	invoke := &cobra.Command{Use: "invoke <vault> <key> <operation>", Short: "Invoke an operation advertised by an item", Args: cobra.ExactArgs(3), PreRunE: vaultPreRun,
		Long: `Retrieve the item and invoke only an operation listed in available_operations.
collect returns a time-scoped URL for the full credential form without clearing values.
Each operation's description lists the inputs that item needs; read it before invoking.
fill requires --params JSON or --spec-file <path|-> with browser_id (session ID, not name).
Credentials require 1-32 ordered fields (field, selector). Link cards require an exact
page_url; include fields only when the item's advertised fill description asks for them.
Do not include type, values, or frame IDs.
The vault must already be attached to the browser. page_url selects an existing page;
fill never navigates. Credentials use declared field names, must omit format, and may
omit page_url only when the API can resolve a unique page. TOTP codes stay server-generated.
Cards require an exact HTTPS page_url. Stored fields: number, cvc, exp_month (MM),
exp_year (YYYY), billing_name, billing_line1, billing_line2, billing_city,
billing_state, billing_postal_code, billing_country. expiration requires format MM/YY
or MM/YYYY. Optional timeout_ms is 1-30000 (default 10000).
The API searches the page and descendant frames, including payment iframes.
Fill is available for credential items and ready Link cards when advertised, not AgentCard.
Link cards do not expose aliases or support egress substitution.
Fill writes real values into the browser; unrestricted browser/CDP access can read them.
Fill never explicitly submits forms or clicks buttons, but input/change events may trigger site behavior.
completed means credentials were supplied, not website acceptance, login, or payment success.
failed may leave partial writes; unknown quarantines the browser. Never automatically
retry or fall back to aliases. Requests are not automatically retried.
API validation errors (400/403/404/409) include HTTP status, recognized error codes,
and corrective guidance; no fields were written by that request. Inspect and correct
the cause before deciding on a new fill. Transport loss remains an uncertain outcome.
prepare_checkout requires checkout.browser_id, checkout.merchant_origin (canonical HTTPS
origin of the top-level merchant page), and checkout.environment (production or sandbox).
Use only when advertised for an AgentCard card. Keep the returned approval page open,
poll until ready_to_submit, then submit native Pay before preparation.expires_at.
Preparations are single-use, including after failure or expiry; never retry automatically.
collect/prepare_checkout may use --open. Fill returns value-free per-field outcomes;
completed exits 0, failed/unknown exit nonzero with valid JSON retained on stdout in -o json.`,
		Example: `  kernel vaults items invoke user-vault login collect
  kernel vaults items invoke user-vault login fill --spec-file - <<'JSON'
{"browser_id":"<browser-id>","fields":[{"field":"username","selector":"#username"},{"field":"password","selector":"#password"}]}
JSON
  kernel vaults items invoke checkout order-1 fill --params '{"browser_id":"browser-session-id","page_url":"https://shop.example/checkout","fields":[{"field":"number","selector":"#card-number"}]}' -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			open, _ := cmd.Flags().GetBool("open")
			raw, _ := cmd.Flags().GetString("params")
			paramsSet := cmd.Flags().Changed("params")
			if cmd.Flags().Changed("spec-file") {
				if args[2] != "fill" && args[2] != "prepare_checkout" {
					return fmt.Errorf("--spec-file is only supported for fill and prepare_checkout")
				}
				data, err := readVaultSpecFile(cmd)
				if err != nil {
					return err
				}
				raw, paramsSet = string(data), true
			}
			params, err := parseVaultOperationParams(args[2], raw, paramsSet, cmd.Flags().Changed("open"))
			if err != nil {
				return err
			}
			return getVaultsHandler(cmd).Invoke(cmd.Context(), args[0], args[1], args[2], params, vaultOutput(cmd), open)
		}}
	invoke.Flags().String("params", "", "Fill or prepare_checkout parameters JSON (maximum 128 KiB); omit type and credential values")
	invoke.Flags().String("spec-file", "", "Fill or prepare_checkout parameters JSON file (use '-' for stdin; maximum 128 KiB)")
	invoke.MarkFlagsMutuallyExclusive("params", "spec-file")
	invoke.Flags().Bool("open", false, "Open a returned HTTPS action URL in your browser")
	addVaultJSONOutputFlag(invoke)
	items.AddCommand(itemList, itemGet, itemEvents, invoke, newVaultDeleteCommand(true))

	wallets := &cobra.Command{Use: "wallets", Short: "Connect provider wallets and inspect funding methods"}
	walletCreate := &cobra.Command{Use: "create <vault> <key> --provider <link|agentcard> --spec '<json>'", Short: "Create a wallet and display its connection or enrollment action", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "Create a wallet at an immutable key and follow the returned provider action.\n" + vaultSpecHelp + vaultWalletSpecHelp,
		Example: `  kernel vaults wallets create checkout wallet-1 \
    --provider link --spec '{
      "authorization": {
        "method": "oauth",
        "client": {"type": "kernel_managed"}
      }
    }' --open

  kernel vaults wallets create checkout wallet-1 \
    --provider agentcard --spec '{}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := vaultWalletSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).CreateWallet(cmd.Context(), args[0], args[1], spec, vaultOutput(cmd), open)
		}}
	addVaultSpecFlags(walletCreate)
	walletCreate.Flags().String("provider-config-id", "", "Bind the new wallet to this provider configuration ID (immutable)")
	walletCreate.Flags().String("provider-config-name", "", "Bind the new wallet to this provider configuration name (immutable)")
	walletCreate.MarkFlagsMutuallyExclusive("provider-config-id", "provider-config-name")
	walletCreate.Flags().String("tokens-file", "", "Import Link access_token and refresh_token JSON from a file (use '-' for stdin)")
	walletCreate.Flags().Bool("open", false, "Open the returned HTTPS connection/enrollment URL")
	addVaultJSONOutputFlag(walletCreate)
	methods := &cobra.Command{Use: "payment-methods <vault> <key>", Short: "Fetch advertised live wallet payment methods", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "Fetch payment_methods through the item's GET expansion. The wallet must advertise this expansion.\nDisplays selectable IDs and advisory capabilities; never automatically chooses a funding method.\nJSON returns the item with expanded.payment_methods, like items get --expand payment_methods.",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, _ := cmd.Flags().GetString("project")
			return getVaultsHandler(cmd).GetItem(cmd.Context(), args[0], args[1], 0, []string{"payment_methods"}, resolveProjectSelection(project), vaultOutput(cmd), false)
		}}
	addVaultJSONOutputFlag(methods)
	wallets.AddCommand(walletCreate, methods)

	cards := &cobra.Command{Use: "cards", Short: "Create immutable card requests at final checkout"}
	cards.AddCommand(newVaultCardCommand())
	cmd.AddCommand(items, wallets, cards, newVaultCredentialsCommand())
	return cmd
}

func newVaultDeleteCommand(item bool) *cobra.Command {
	use, short, nargs := "delete <vault>", "Delete a vault and invalidate all its items", 1
	if item {
		use, short, nargs = "delete <vault> <key>", "Delete an item and invalidate its credential", 2
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
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt; abandon recovery only after explicit user confirmation")
	return cmd
}

func newVaultCardCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "create <vault> <key> --provider <link|agentcard> --spec '<json>'", Short: "Create an immutable card request and start approval", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "Create a card after reaching final checkout. For Link, Kernel inspects the checkout and internally selects a Link payment token or virtual card. Creation starts human approval; share the returned URL and retrieve the item until fill appears.\n" + vaultSpecHelp + vaultCardSpecHelp + vaultLinkPurchaseTypesHelp,
		Example: "  kernel vaults cards create" + ` checkout order-1 \
    --provider link --spec '{
      "wallet": "wallet-1",
      "browser_id": "browser-session-id",
      "page_url": "https://shop.example/checkout",
      "payment_method_id": "pm-1",
      "amount": 1234,
      "currency": "usd",
      "merchant_name": "Example Shop",
      "context": "Final checkout for one item totaling USD 12.34. This is a new purchase and not a retry of an uncertain payment."
    }'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := vaultSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			return getVaultsHandler(cmd).SaveCard(cmd.Context(), args[0], args[1], param.Override[kernel.CardVaultItemSpecUnionParam](spec), vaultOutput(cmd))
		}}
	addVaultSpecFlags(cmd)
	addVaultJSONOutputFlag(cmd)
	return cmd
}

func addVaultSpecFlags(cmd *cobra.Command) {
	cmd.Flags().String("provider", "", "Provider: link or agentcard (required)")
	cmd.Flags().String("spec", "", "Raw JSON specification object (required); see types and examples above")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("spec")
}

func vaultSpecFromFlags(cmd *cobra.Command) (map[string]json.RawMessage, error) {
	provider, _ := cmd.Flags().GetString("provider")
	if provider != "link" && provider != "agentcard" {
		return nil, fmt.Errorf("--provider must be link or agentcard")
	}
	raw, _ := cmd.Flags().GetString("spec")
	var spec map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &spec); err != nil || spec == nil {
		return nil, fmt.Errorf("--spec must be a JSON object")
	}
	if provider == "link" {
		if _, exists := spec["merchant_account_id"]; exists {
			return nil, fmt.Errorf("omit merchant_account_id; Kernel discovers it from the checkout")
		}
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
	spec["provider"], _ = json.Marshal(provider)
	return spec, nil
}
