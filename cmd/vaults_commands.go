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
		Use: "vaults", Aliases: []string{"vault"}, Short: "Prepare and observe project-owned payment credentials",
		Long: `Prepare and observe payment credentials and stored logins; vault commands do not submit merchant payments.

Optionally select a project with --project <id-or-name> or KERNEL_PROJECT.
Otherwise, the API resolves the project from your credentials and its defaults.
Vault names, item keys, and project ownership are immutable.

1. Create/select a vault, then create a provider wallet and follow its returned action.
2. For Link, list wallet payment methods and select an ID explicitly.
3. Create a card request with --provider and --spec JSON.
4. Inspect items get, then use items invoke <vault> <key> <operation> only when advertised.
   Follow the operation description and any returned provider action.
5. Attach the vault with browsers create --vault <id-or-name>. For ready Link cards,
   use advertised fill with --params to bind checkout fields. Returned non-secret
   aliases are an alternative for explicitly chosen egress-substitution integrations,
   not a fallback after fill. Inspect items get/events for payment outcomes.

For logins and other non-payment credentials, use credentials create instead of a
wallet and card: declare the fields, supply any known values with --values-file,
and hand the returned collection URL to whoever holds the credential. Attach the
vault with browsers create --vault, then use advertised fill to bind its fields.
Credential items must never hold card numbers, security codes, or expiration dates.

Permitted checkout domains are provider-assigned and displayed when returned;
there is no domain-setting API.
Never supply card data, OAuth codes, ciphertext, or secrets in shell arguments.
Use vault-provider-configs for client credentials and wallets create --tokens-file
for imported Link grants; both accept protected files or stdin.
Never retry failed, timed-out, rejected, or indeterminate payments.
JSON output preserves returned public fields but omits unknown/opaque provider data.`,
		Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
	}

	create := &cobra.Command{Use: "create --name <name>", Short: "Create or retrieve a vault by immutable name", Args: cobra.NoArgs, PreRunE: vaultPreRun,
		Long: "Create or retrieve a vault by immutable name.\nFree organizations can store up to 3 non-deleted vaults across all projects; paid plans and active trials have no vault cap.\nRetrieving an existing vault by name succeeds even at the limit.\nSee kernel org limits get for the current cap and usage.",
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

	items := &cobra.Command{Use: "items", Short: "Inspect vault item state, actions, aliases, and outcomes"}
	itemList := &cobra.Command{Use: "list <vault>", Short: "List items by vault ID or name", Args: cobra.ExactArgs(1), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getVaultsHandler(cmd).ListItems(cmd.Context(), args[0], vaultOutput(cmd))
		}}
	addVaultJSONOutputFlag(itemList)
	itemGet := &cobra.Command{Use: "get <vault> <key>", Short: "Get item state and any required action", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "Get item state, available operations, provider actions, and returned checkout aliases.\n--wait is a single bounded server-side observation, not a retry or a guarantee of readiness.\nAn item still pending after the wait is returned as-is; ready does not mean paid.\nrecovery_required stops waiting and means unresolved, not declined or expired.\nA known authorization ID must be reconciled with the provider or support; do not retry, delete, or replace the payment.\nAn AgentCard checkout that returned no authorization ID may be abandoned by deleting that card explicitly, which is not proof that the payment did not occur.",
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
Read its description with items get before invoking; follow any approval requirements.
Authorize sends {"type":"authorize"} without --params and returns an updated item;
--open opens its returned HTTPS action URL.
Collect is advertised by ready and pending_collection credential items. It takes no
--params and returns the item with a time-scoped hosted form URL, reusing an active
session or renewing an expired one; --open opens it. Opening the form clears no
values and changes neither readiness nor the item version. Treat the URL as a secret.
Prepare_checkout is advertised by eligible unused AgentCard cards before the first
Square Pay action. It requires --params with browser_id (session ID of a browser
created with this vault attached), merchant_origin (canonical origin of the
top-level merchant document, not the Square iframe; http only for localhost), and
environment (production or sandbox, describing Square and not the AgentCard
credential mode). Deliver the returned approval URL and keep that page open;
--open opens it. Then poll with items get --wait 60 until ready_to_submit and
submit native Pay before the preparation deadline; readiness lasts at most 30
seconds and polling never extends it. Unused preparations expire automatically.
Every preparation is single-use, including after failure or expiry: do not
automatically retry, and reconcile uncertain outcomes with the merchant.
Fill requires --params JSON with browser_id (session ID, not name) and 1-32 fields.
Each binding has field and selector. Optional timeout_ms is 1-30000 (default 10000).
Do not include type, values, or frame IDs in --params.
For cards, page_url is a required exact HTTPS URL and each field is one of number,
cvc, exp_month (MM), exp_year (YYYY), billing_name, billing_line1, billing_line2,
billing_city, billing_state, billing_postal_code, billing_country, or the combined
expiration, which also requires format MM/YY or MM/YYYY. Fill is available only when
advertised by a ready Link card, not AgentCard.
For credentials, each field is a declared field name with a stored value, format is
not accepted, and page_url may be omitted to require exactly one open page. A totp
field fills a freshly generated code; its seed never enters the browser.
The API searches the selected page and descendant frames, including payment iframes.
Fill returns value-free per-field outcomes, not an updated item. Completed exits 0;
failed/unknown exit nonzero while preserving the result in -o json.
Fill is not atomic: earlier writes are not rolled back. Transport errors do not
prove no writes occurred. No automatic retries, alias fallback, or form submission.
Inspect the browser before deciding what to do next; completed does not mean paid.`,
		Example: `  kernel vaults items get checkout order-1
  kernel vaults items invoke checkout order-1 authorize --open
  kernel vaults items invoke checkout order-1 prepare_checkout --params '{"browser_id":"browser-session-id","merchant_origin":"https://shop.example.com","environment":"production"}' --open
  kernel vaults items invoke checkout order-1 fill --params '{"browser_id":"browser-session-id","page_url":"https://shop.example/checkout","fields":[{"field":"number","selector":"#card-number"},{"field":"expiration","format":"MM/YY","selector":"#expiry"},{"field":"cvc","selector":"#security-code"}],"timeout_ms":10000}' -o json
  kernel vaults items invoke logins hacker-news collect --open
  kernel vaults items invoke logins hacker-news fill --params '{"browser_id":"browser-session-id","fields":[{"field":"username","selector":"#login"},{"field":"password","selector":"#password"}]}' -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			open, _ := cmd.Flags().GetBool("open")
			raw, _ := cmd.Flags().GetString("params")
			params, err := parseVaultOperationParams(args[2], raw, cmd.Flags().Changed("params"), cmd.Flags().Changed("open"))
			if err != nil {
				return err
			}
			return getVaultsHandler(cmd).Invoke(cmd.Context(), args[0], args[1], args[2], params, vaultOutput(cmd), open)
		}}
	invoke.Flags().String("params", "", "Operation-specific JSON object for fill and prepare_checkout; omit type (supplied by <operation>)")
	invoke.Flags().Bool("open", false, "Open a returned HTTPS action or approval URL for authorize, collect, and prepare_checkout")
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

	cards := &cobra.Command{Use: "cards", Short: "Configure card requests"}
	cards.AddCommand(newVaultCardCommand(false), newVaultCardCommand(true))

	credentials := &cobra.Command{Use: "credentials", Aliases: []string{"credential"}, Short: "Store logins and other non-payment credentials"}
	credentials.AddCommand(newVaultCredentialCreateCommand(), newVaultCredentialUpdateCommand())
	cmd.AddCommand(items, wallets, cards, credentials)
	return cmd
}

func newVaultCredentialCreateCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "create <vault> <key> --spec '<json>'", Short: "Declare a credential item and optionally seed its values", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: `Create a credential item at an immutable key, without a wallet or provider.
Repeating the original creation request returns the current item without
overwriting later edits; a different request at the same key returns 409.
Use vaults credentials update for changes.
` + vaultCredentialSpecHelp,
		Example: `  kernel vaults credentials create logins hacker-news     --spec '{
      "description": "Hacker News",
      "fields": {
        "username": {"type": "text", "sensitive": false},
        "password": {"type": "password"}
      }
    }' --values-file ./values.json --open`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := vaultCredentialSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).CreateCredential(cmd.Context(), args[0], args[1], spec, vaultOutput(cmd), open)
		}}
	cmd.Flags().String("spec", "", "Credential specification JSON with fields and an optional description (required)")
	_ = cmd.MarkFlagRequired("spec")
	addVaultCredentialValuesFlag(cmd)
	cmd.Flags().Bool("open", false, "Open a returned HTTPS collection URL in your browser")
	addVaultJSONOutputFlag(cmd)
	return cmd
}

func newVaultCredentialUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "update <vault> <key> --version <n>", Short: "Set or clear credential values and the description", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: `Atomically update the description and selected values; omitted properties are preserved.
--version is the expected current item version from the latest read, so a
concurrent edit returns 409 instead of being overwritten. Read it with items get.
Field names, types, required flags, and sensitivity cannot change, and unknown
field names return 400. A successful update increments the version and invalidates
outstanding hosted collection sessions.

--values-file sets values; a JSON null or empty string clears one immediately.
Clearing a required field reopens collection and returns a fresh collection action;
clearing a required totp field returns 400 because no form can collect it.
--description "" clears the description.`,
		Example: `  kernel vaults credentials update logins hacker-news --version 3 --values-file ./values.json
  kernel vaults credentials update logins hacker-news --version 3 --description "Hacker News"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			version, _ := cmd.Flags().GetInt64("version")
			if version < 1 {
				return fmt.Errorf("--version must be the expected current item version (1 or greater)")
			}
			expectedItemID, _ := cmd.Flags().GetString("expected-item-id")
			spec, err := vaultCredentialUpdateSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).UpdateCredential(cmd.Context(), args[0], args[1], version, expectedItemID, spec, vaultOutput(cmd), open)
		}}
	cmd.Flags().Int64("version", 0, "Expected current item version from the latest read (required)")
	_ = cmd.MarkFlagRequired("version")
	cmd.Flags().String("description", "", "Replacement form title; an empty string clears it")
	cmd.Flags().String("expected-item-id", "", "Immutable item ID precondition; returns 409 if the key now identifies a different item")
	addVaultCredentialValuesFlag(cmd)
	cmd.Flags().Bool("open", false, "Open a returned HTTPS collection URL in your browser")
	addVaultJSONOutputFlag(cmd)
	return cmd
}

func addVaultCredentialValuesFlag(cmd *cobra.Command) {
	cmd.Flags().String("values-file", "", "JSON object of field names to values, read from a file (use '-' for stdin); never pass values as shell arguments")
}

func newVaultDeleteCommand(item bool) *cobra.Command {
	use, short, nargs := "delete <vault>", "Delete a vault and invalidate all its items", 1
	long := short + ".\nUnresolved payment operations block deletion, including operations on child cards of a wallet."
	if item {
		use, short, nargs = "delete <vault> <key>", "Delete an item and invalidate its credential", 2
		long = short + `.
Unresolved payment operations normally block deletion. An AgentCard checkout whose
create response returned no authorization ID may be abandoned by deleting that card
directly, so a replacement can be created; deleting its wallet or vault stays blocked.
Deleting or recreating an item is not proof that a payment did not occur.`
	}
	cmd := &cobra.Command{Use: use, Short: short, Long: long, Args: cobra.ExactArgs(nargs), PreRunE: vaultPreRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			if item {
				key = args[1]
			}
			yes, _ := cmd.Flags().GetBool("yes")
			return getVaultsHandler(cmd).Delete(cmd.Context(), args[0], key, yes)
		}}
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func newVaultCardCommand(update bool) *cobra.Command {
	use, short := "create", "Create a card request without authorizing it"
	if update {
		use, short = "update", "Update a card spec when the API permits configuration"
	}
	cmd := &cobra.Command{Use: use + " <vault> <key> --provider <link|agentcard> --spec '<json>'", Short: short, Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: short + `. Neither create nor update authorizes a Link card.
Requested cards accept a replacement spec. Pending issuance updates preserve omitted
optional fields; explicit empty lists clear them. The API restricts fields after
authorization starts; wallet/provider bindings cannot change. An uncertain update
enters recovery_required and must not be retried. Checkout cards can be edited
between authorizations. Identical creates return existing state without resetting it.
Never reconfigure to retry a failed, timed-out, rejected, or indeterminate payment.
` + vaultSpecHelp + vaultCardSpecHelp,
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
			return getVaultsHandler(cmd).SaveCard(cmd.Context(), args[0], args[1], param.Override[kernel.CardVaultItemSpecUnionParam](spec), update, vaultOutput(cmd))
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
