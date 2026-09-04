package cmd

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/kernel/cli/pkg/interactive"
	kernel "github.com/kernel/kernel-go-sdk"
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
	project, _ := cmd.Flags().GetString("project")
	if err := requireVaultProject(resolveProjectSelection(project)); err != nil {
		return err
	}
	if err := validateJSONOutput(vaultOutput(cmd)); err != nil {
		return err
	}
	for i, arg := range args {
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
		Long: `Prepare and observe payment credentials; vault commands do not submit merchant payments.

Select the project with --project <id-or-name> or KERNEL_PROJECT. The API assigns
immutable project ownership from that scope. Vault names and item keys are immutable.

1. Create/select a vault, then create a provider wallet and follow its returned action.
2. For Link, list wallet payment methods and select an ID explicitly.
3. Create a card request. Link requires explicit --test or --live intent.
4. For a requested Link card, use cards authorize only when advertised. Follow the
   returned approval action. AgentCard authorizes at checkout, not through this command.
5. Attach the vault with browsers create --vault <id-or-name>. Use only returned
   non-secret aliases in that browser. Inspect items get/events for the outcome.

Permitted checkout domains are provider-assigned and displayed when returned;
there is no domain-setting API. AgentCard mode is deployment-controlled, not per item.
Never supply card data, OAuth codes/tokens, ciphertext, or provider secrets to the CLI.
Never retry failed, timed-out, rejected, or indeterminate payments.
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

	list := &cobra.Command{Use: "list", Short: "List vaults in the selected project", Args: cobra.NoArgs, PreRunE: vaultPreRun,
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
		Long: "Get item state, available operations, provider actions, and returned checkout aliases.\n--wait is a single bounded server-side observation, not a retry or a guarantee of readiness.\nAn item still pending after the wait is returned as-is; ready does not mean paid.",
		RunE: func(cmd *cobra.Command, args []string) error {
			wait, _ := cmd.Flags().GetInt64("wait")
			expand, _ := cmd.Flags().GetStringSlice("expand")
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).GetItem(cmd.Context(), args[0], args[1], wait, expand, vaultOutput(cmd), open)
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
	items.AddCommand(itemList, itemGet, itemEvents, newVaultDeleteCommand(true))

	wallets := &cobra.Command{Use: "wallets", Short: "Connect provider wallets and inspect funding methods"}
	walletCreate := &cobra.Command{Use: "create <vault> <key> --provider <link|agentcard>", Short: "Create a wallet and display its connection or enrollment action", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "Create a wallet at an immutable key. Link uses Kernel-managed OAuth; complete the returned URL outside the CLI.\nAgentCard returns a card-enrollment action, or may reference an already enrolled user in this organization.\nAgentCard sandbox/live mode is fixed by the deployment; there is no per-item test flag.",
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, _ := cmd.Flags().GetString("provider")
			userID, _ := cmd.Flags().GetString("user-id")
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).CreateWallet(cmd.Context(), args[0], args[1], provider, userID, vaultOutput(cmd), open)
		}}
	walletCreate.Flags().String("provider", "", "Wallet provider: link or agentcard (required)")
	_ = walletCreate.MarkFlagRequired("provider")
	walletCreate.Flags().String("user-id", "", "Already enrolled AgentCard user ID in this organization (optional)")
	walletCreate.Flags().Bool("open", false, "Open the returned HTTPS connection/enrollment URL")
	addVaultJSONOutputFlag(walletCreate)
	methods := &cobra.Command{Use: "payment-methods <vault> <key>", Short: "Fetch advertised live wallet payment methods", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "Fetch payment_methods through the item's GET expansion. The wallet must advertise this expansion.\nDisplays selectable IDs and advisory capabilities; never automatically chooses a funding method.\nJSON returns the item with expanded.payment_methods, like items get --expand payment_methods.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return getVaultsHandler(cmd).GetItem(cmd.Context(), args[0], args[1], 0, []string{"payment_methods"}, vaultOutput(cmd), false)
		}}
	addVaultJSONOutputFlag(methods)
	wallets.AddCommand(walletCreate, methods)

	cards := &cobra.Command{Use: "cards", Short: "Configure card requests and explicitly authorize requested Link cards"}
	authorize := &cobra.Command{Use: "authorize <vault> <key>", Short: "Invoke advertised authorization for a requested Link card", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: "Use only after explicit user approval. Retrieve the item and invoke authorize only if advertised and still requested.\nThis can obtain a payment credential but does not submit a merchant payment.\nPending/terminal authorizations are never resumed or retried by this command.\nFollow any returned approval URL, then observe with items get --wait 60.",
		RunE: func(cmd *cobra.Command, args []string) error {
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).Authorize(cmd.Context(), args[0], args[1], vaultOutput(cmd), open)
		}}
	authorize.Flags().Bool("open", false, "Open the returned HTTPS approval URL")
	addVaultJSONOutputFlag(authorize)
	cards.AddCommand(newVaultCardCommand(false), newVaultCardCommand(true), authorize)
	cmd.AddCommand(items, wallets, cards)
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
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func newVaultCardCommand(update bool) *cobra.Command {
	use, short := "create", "Create a card request without authorizing it"
	if update {
		use, short = "update", "Replace a card spec when the API permits configuration"
	}
	cmd := &cobra.Command{Use: use + " <vault> <key>", Short: short, Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: short + `. Supply the complete specification, including all required flags.
Link requires --payment-method-id, --merchant-url, --context (at least 100 characters),
and exactly one of --test or --live. Amount is an integer in minor currency units.
AgentCard uses --merchant as its approval-screen name; --card-id is optional.
AgentCard mode is deployment-controlled; --test/--live are not supported for it.
Permitted domains come from the provider and cannot be configured by this API.
Neither create nor update authorizes a Link card. The API enforces update eligibility,
provider/wallet invariants, and immutable item keys. Update replaces the entire spec;
optional purchase details set outside the CLI are removed when omitted.
Never reconfigure to retry a failed, timed-out, rejected, or indeterminate payment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := vaultCardSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			return getVaultsHandler(cmd).SaveCard(cmd.Context(), args[0], args[1], spec, update, vaultOutput(cmd))
		}}
	cmd.Flags().String("provider", "", "Card provider: link or agentcard (required)")
	cmd.Flags().String("wallet", "", "Wallet item key in this vault (required)")
	cmd.Flags().Int64("amount", 0, "Amount in minor currency units, not a decimal (required)")
	cmd.Flags().String("currency", "", "Three-letter currency code (required)")
	cmd.Flags().String("merchant", "", "Merchant name for approval (required)")
	for _, flag := range []string{"provider", "wallet", "amount", "currency", "merchant"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	cmd.Flags().String("payment-method-id", "", "Explicit ID from wallets payment-methods (required for Link)")
	cmd.Flags().String("merchant-url", "", "Absolute HTTP(S) merchant URL (required for Link)")
	cmd.Flags().String("context", "", "Purchase purpose, at least 100 characters (required for Link); no secrets")
	cmd.Flags().Bool("test", false, "Request Link test credentials (explicitly choose --test or --live)")
	cmd.Flags().Bool("live", false, "Request a live Link payment credential (explicit opt-in)")
	cmd.MarkFlagsMutuallyExclusive("test", "live")
	cmd.Flags().String("card-id", "", "AgentCard vaulted card ID; omit to let the cardholder select during approval")
	addVaultJSONOutputFlag(cmd)
	return cmd
}

func vaultCardSpecFromFlags(cmd *cobra.Command) (kernel.CardVaultItemSpecUnionParam, error) {
	var spec kernel.CardVaultItemSpecUnionParam
	provider, _ := cmd.Flags().GetString("provider")
	wallet, _ := cmd.Flags().GetString("wallet")
	amount, _ := cmd.Flags().GetInt64("amount")
	currency, _ := cmd.Flags().GetString("currency")
	merchant, _ := cmd.Flags().GetString("merchant")
	if err := validateVaultName(wallet, "--wallet"); err != nil {
		return spec, err
	}
	if !regexp.MustCompile(`^[A-Za-z]{3}$`).MatchString(currency) {
		return spec, fmt.Errorf("--currency must be a three-letter currency code")
	}
	currency = strings.ToLower(currency)
	if amount < 1 {
		return spec, fmt.Errorf("--amount must be positive, in minor currency units")
	}
	if strings.TrimSpace(merchant) == "" {
		return spec, fmt.Errorf("--merchant is required")
	}
	switch provider {
	case "link":
		if cmd.Flags().Changed("card-id") {
			return spec, fmt.Errorf("--card-id is only supported by agentcard")
		}
		if amount > 500000 || utf8.RuneCountInString(merchant) > 255 {
			return spec, fmt.Errorf("link requires --amount <= 500000 and --merchant <= 255 characters")
		}
		test, _ := cmd.Flags().GetBool("test")
		live, _ := cmd.Flags().GetBool("live")
		if test == live || (cmd.Flags().Changed("test") && cmd.Flags().Changed("live")) {
			return spec, fmt.Errorf("link requires exactly one of --test or --live (set to true)")
		}
		method, _ := cmd.Flags().GetString("payment-method-id")
		merchantURL, _ := cmd.Flags().GetString("merchant-url")
		contextText, _ := cmd.Flags().GetString("context")
		if strings.TrimSpace(method) == "" {
			return spec, fmt.Errorf("--payment-method-id is required; select an ID from wallets payment-methods")
		}
		u, err := url.Parse(merchantURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
			return spec, fmt.Errorf("--merchant-url must be an absolute HTTP(S) URL without credentials")
		}
		if utf8.RuneCountInString(strings.TrimSpace(contextText)) < 100 {
			return spec, fmt.Errorf("--context must describe the purchase in at least 100 characters")
		}
		spec.OfLink = &kernel.CardVaultItemSpecLinkParam{Wallet: wallet, Amount: amount, Currency: currency, MerchantName: merchant, MerchantURL: merchantURL, PaymentMethodID: method, Context: contextText, Test: test}
	case "agentcard":
		for _, flag := range []string{"test", "live", "payment-method-id", "merchant-url", "context"} {
			if cmd.Flags().Changed(flag) {
				return spec, fmt.Errorf("--%s is only supported by Link; AgentCard mode is deployment-controlled", flag)
			}
		}
		if amount > 9007199254740991 || utf8.RuneCountInString(merchant) > 120 {
			return spec, fmt.Errorf("agentcard requires --amount <= 9007199254740991 and --merchant <= 120 characters")
		}
		spec.OfAgentcard = &kernel.CardVaultItemSpecAgentcardParam{Wallet: wallet, Amount: amount, Currency: currency, Merchant: merchant}
		cardID, _ := cmd.Flags().GetString("card-id")
		if cmd.Flags().Changed("card-id") {
			if !regexp.MustCompile(`^vc_[A-Za-z0-9_]+$`).MatchString(cardID) {
				return spec, fmt.Errorf("--card-id must be an AgentCard vaulted card ID (vc_...)")
			}
			spec.OfAgentcard.CardID = kernel.Opt(cardID)
		}
	default:
		return spec, fmt.Errorf("--provider must be link or agentcard")
	}
	return spec, nil
}
