package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
)

// Shared by vaults and vaults credentials help so both paths are always presented together.
const vaultCredentialPathsHelp = `Credential vaults have two sign-in paths. Before creating anything:
1. Look for an existing credential with items list <vault> -o json. Reuse a credential
   whose description or requested website matches the site, and invoke only its
   advertised operations. A different spec at an existing key returns 409.
2. Otherwise ask the user where this login lives and wait for the answer; do not
   choose for them:
   - Kernel-hosted collection (spec provider "kernel", the default): you define the
     site's fields, the user types values into a Kernel-hosted form at the returned
     collection URL, Kernel stores them encrypted, and items invoke fill writes them
     into a vault-bound browser without submitting.
   - 1Password brokered approval (spec provider "1password", preview): requires the
     login to be saved in a 1Password vault only the user can access (such as their
     Personal, Private, or Employee vault). Items in shared vaults cannot be approved,
     and passkeys are not supported. State this requirement in the question itself,
     not as a footnote, and ask the user to confirm the login is in such a vault; if it
     is in a shared vault, they must move it first or choose Kernel-hosted collection.
     The account owner connects the account once and approves each access request in
     the 1Password app. 1pw_fill fills and submits through the 1Password extension.
3. If 1Password is unavailable in this deployment, the account cannot be connected,
   or the owner declines, tell the user and offer Kernel-hosted collection; do not
   switch paths without asking. If the user wants neither, stop.
Neither path returns secret values through the API or CLI. Never ask the user to paste
passwords, OAuth codes, tokens, or keys into the terminal or chat; share only returned
URLs. Give each URL to the user as bare text on its own line, never inside backticks,
code blocks, or quotes, so their chat client can make it clickable; this includes
onepassword:// approval links. An agent controlling the browser can still read filled pages. A filled or
submitted form is not proof of sign-in: inspect the page afterward.`

const vaultOnePasswordCredentialHelp = `1Password flow:
1. Ask whose 1Password account holds the login and confirm it is saved in a vault
   only that owner can access, not a shared vault. Reuse that owner's connected
   credential_account from items list. Otherwise run credentials connect <vault>
   <account-key> --provider 1password, share the returned authorization URL with
   that owner, and poll items get <vault> <account-key> --wait 60 until connected.
2. credentials create <vault> <key> --spec-file with provider "1password", account
   (the credential_account key), and requests: version 2 with 1-5 login entries, each
   with the HTTPS website of a login page. Use one entry unless the user needs several
   logins, such as separate accounts or sign-in origins; per-entry reason and
   keywords go in the spec. No field definitions, selectors, or values are accepted.
3. Invoke 1pw_create_access_request once with a vault-bound browser_id. Present the
   returned onepassword:// approval link to the account owner unchanged, as bare text
   on its own line (not in backticks or a code block), with the returned instructions.
4. Invoke 1pw_access_request_status (timeout_seconds 0-120) until the credential is
   ready, declined, or failed. Ready means approved, not signed in.
5. Open the login page and invoke 1pw_fill with browser_id and the exact page_url. When
   several approved entries share that page's origin, pass entry_id from the
   state.access_request entries in items get -o json. fill_submitted means the form
   was submitted, not that sign-in succeeded.
Invoke only advertised operations. Never automatically retry an access request, fill,
or recovery. After an uncertain outcome (timeout, HTTP 5xx, fill_unknown, or a pending
item with no approval link or advertised operation), stop and tell the user; do not
delete and recreate the item to reset it. Declined means the owner refused; do
not ask again unless the user requests it. After a confirmed failed status, ask the
user before deleting and recreating the credential for at most one new request.
Account links: if a credential_account advertises 1pw_recover, invoke it and share the
new link with the owner, then run credentials connect again with the same key. For
reconnect_required or declined without 1pw_recover, ask the owner before running
credentials connect again with the same key for a new link.`

const vaultOnePasswordStoredTokenHelp = `Stored-token 1Password credentials (developer integrations only, separate from the
flow above): a developer who already holds a 1Password broker access token and its
matching integration key may create the credential with access_token, integration_key,
and requests instead of account. Supply either account or both secrets, never both. Put them only in a protected --spec-file
or stdin; they are write-only and never displayed. Never ask an end user for them.
Replace the token with items invoke 1pw_update_access_token --spec-file.`

const vaultCredentialHelp = `Create credentials for a website.

` + vaultCredentialPathsHelp + `

Kernel-hosted flow:
Do not use credential items to store, collect, or fill credit card data, including
card numbers (PANs), security codes (CVV/CVC), or expiration dates. Use wallet and
card item types for credit cards and payment checkout instead.

First create a vault for the end user and attach it with browsers create --vault.
Use a protected JSON file or stdin, never secret values in shell arguments.
The spec contains an optional provider ("kernel", the default), description, and
fields as an ordered array of named definitions.
Inspect the website and list fields in its natural top-to-bottom order because the
user-facing collection form renders that order unchanged. Definitions accept a stable
name and an optional non-secret human-readable label; forms fall back to name. Updates,
state, and browser fills always use name. Field types are text, email, password, and
totp; definitions also accept required, sensitive, and value.
Set description to the recognizable site name only, e.g. "Hacker News", not
"Hacker News sign-in credentials". This text is the user-facing form title.
Set sensitive:false explicitly for ordinary usernames and email addresses.
Reserve sensitive:true for secrets such as passwords, API tokens, and TOTP seeds.
Password and totp must be sensitive. Omitted sensitive defaults to true for safety.
Omit required values to receive a collection URL to present to the user.
Poll items get --wait 60 until state.status is ready, then use items invoke fill.
Ready means populated, not a successful login. An agent controlling the browser
can read filled values. TOTP seeds must not be collected through the hosted form.
Get/list output includes definitions, has_value, and explicitly non-sensitive text/email values.
Sensitive values and TOTP seeds are omitted.
Collection URLs are bearer credentials: share only with the intended user.

` + vaultOnePasswordCredentialHelp + `

` + vaultOnePasswordStoredTokenHelp

func newVaultCredentialsCommand() *cobra.Command {
	group := &cobra.Command{Use: "credentials", Short: "Collect, update, and fill user credentials", Long: vaultCredentialHelp}
	for _, update := range []bool{false, true} {
		name, short := "create", "Create a credential and return its collection URL"
		if update {
			name, short = "update", "Update credential values or description using an expected version"
		}
		cmd := &cobra.Command{Use: name + " <vault> <key> --spec-file <path|->", Short: short, Args: cobra.ExactArgs(2), PreRunE: vaultPreRun, Long: vaultCredentialHelp,
			RunE: func(cmd *cobra.Command, args []string) error {
				data, err := readVaultSpecFile(cmd)
				if err != nil {
					return err
				}
				version, _ := cmd.Flags().GetInt64("version")
				open, _ := cmd.Flags().GetBool("open")
				expectedID, _ := cmd.Flags().GetString("expected-item-id")
				if cmd.Flags().Changed("expected-item-id") && strings.TrimSpace(expectedID) == "" {
					return fmt.Errorf("--expected-item-id must not be empty")
				}
				return getVaultsHandler(cmd).saveCredential(cmd.Context(), args[0], args[1], data, update, version, expectedID, vaultOutput(cmd), open)
			},
		}
		if update {
			cmd.Long += "\nUpdate applies to Kernel-hosted credentials only. It preserves omitted fields, replaces nonempty string values, and clears supported values with null or an empty string. Clearing a required text/email/password field returns pending_collection; form submissions still require a nonempty value.\nField definitions are immutable. Do not automatically retry version conflicts."
			cmd.Flags().Int64("version", 0, "Expected version from items get (required; never auto-refreshed)")
			_ = cmd.MarkFlagRequired("version")
			cmd.Flags().String("expected-item-id", "", "Immutable item ID from the original read; reject an update if the key now refers to a replacement item")
			cmd.Example = "  kernel vaults credentials update user-vault login --version 2 --spec-file changes.json"
		} else {
			cmd.Example = `  # Kernel-hosted collection
  kernel vaults credentials create user-vault login --spec-file - <<'JSON'
{"description":"Hacker News","fields":[{"name":"username","label":"Username","type":"text","required":true,"sensitive":false},{"name":"password","label":"Password","type":"password","required":true,"sensitive":true}]}
JSON

  # 1Password brokered approval (account is the connected credential_account key)
  kernel vaults credentials create user-vault github --spec-file - <<'JSON'
{"provider":"1password","account":"onepassword","requests":{"version":2,"entries":[{"type":"login","parameters":{"website":"https://github.com"}}]}}
JSON`
		}
		cmd.Flags().String("spec-file", "", "Credential spec JSON file (use '-' for stdin; maximum 128 KiB)")
		_ = cmd.MarkFlagRequired("spec-file")
		cmd.Flags().Bool("open", false, "Open the returned HTTPS collection URL")
		addVaultJSONOutputFlag(cmd)
		group.AddCommand(cmd)
	}
	connect := &cobra.Command{Use: "connect <vault> <account-key> --provider 1password", Short: "Connect a 1Password account and return its authorization URL", Args: cobra.ExactArgs(2), PreRunE: vaultPreRun,
		Long: `Create a credential_account item that connects the user's 1Password account.
Only use this after the user chose 1Password brokered approval over Kernel-hosted collection.
Share the returned 1Password authorization URL with the account owner; they sign in and
consent at 1Password, and Kernel receives the grant. No tokens or keys are displayed.
Poll items get --wait 60 until the account state is connected, then set account to
this key in credentials create. Repeating the request returns a connected or
still-pending account unchanged; otherwise it starts a new authorization with a new URL.

` + vaultOnePasswordCredentialHelp,
		Example: "  kernel vaults credentials connect user-vault onepassword --provider 1password",
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, _ := cmd.Flags().GetString("provider")
			if provider != "1password" {
				return fmt.Errorf("--provider must be 1password")
			}
			open, _ := cmd.Flags().GetBool("open")
			return getVaultsHandler(cmd).connectCredentialAccount(cmd.Context(), args[0], args[1], vaultOutput(cmd), open)
		},
	}
	connect.Flags().String("provider", "", "Credential account provider: 1password (required)")
	_ = connect.MarkFlagRequired("provider")
	connect.Flags().Bool("open", false, "Open the returned HTTPS authorization URL")
	addVaultJSONOutputFlag(connect)
	group.AddCommand(connect)
	return group
}

func readVaultSpecFile(cmd *cobra.Command) ([]byte, error) {
	path, _ := cmd.Flags().GetString("spec-file")
	if path == "" {
		return nil, fmt.Errorf("--spec-file is required (use '-' for stdin)")
	}
	var reader io.Reader = cmd.InOrStdin()
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("could not open --spec-file")
		}
		defer f.Close()
		reader = f
	}
	const limit = 128 * 1024
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || len(data) > limit {
		return nil, fmt.Errorf("could not read --spec-file (maximum 128 KiB)")
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return nil, fmt.Errorf("--spec-file must contain a JSON object")
	}
	return data, nil
}

func (c VaultsCmd) saveCredential(ctx context.Context, vault, key string, data []byte, update bool, version int64, expectedID, output string, open bool) error {
	var item *kernel.VaultItemUnion
	var err error
	if update {
		if version < 1 {
			return fmt.Errorf("--version must be positive")
		}
		var spec kernel.CredentialVaultItemSpecUpdateParam
		if json.Unmarshal(data, &spec) != nil {
			return fmt.Errorf("invalid credential update spec")
		}
		request := kernel.CredentialVaultItemUpdateRequestParam{Type: "credential", Version: version, Spec: spec}
		if expectedID != "" {
			request.ExpectedItemID = kernel.String(expectedID)
		}
		item, err = c.vaults.Items.Update(ctx, key, kernel.VaultItemUpdateParams{IDOrName: vault, OfCredentialVaultItemUpdateRequest: &request}, option.WithMaxRetries(0))
	} else {
		spec, specErr := credentialSpecInput(data)
		if specErr != nil {
			return specErr
		}
		item, err = c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{IDOrName: vault, OfCredential: &kernel.CredentialVaultItemRequestParam{Type: "credential", Spec: spec}}, option.WithMaxRetries(0))
	}
	if err != nil {
		return vaultCredentialError(err)
	}
	return c.showItem(item, output, open)
}

// Specs without a provider predate 1Password support and remain Kernel-hosted.
func credentialSpecInput(data []byte) (kernel.CredentialVaultItemSpecInputUnionParam, error) {
	var header struct {
		Provider *string `json:"provider"`
	}
	if json.Unmarshal(data, &header) != nil {
		return kernel.CredentialVaultItemSpecInputUnionParam{}, fmt.Errorf("invalid credential spec")
	}
	provider := "kernel"
	if header.Provider != nil {
		provider = *header.Provider
	}
	switch provider {
	case "kernel":
		var spec kernel.KernelCredentialVaultItemSpecInputParam
		if json.Unmarshal(data, &spec) != nil || len(spec.Fields) == 0 {
			return kernel.CredentialVaultItemSpecInputUnionParam{}, fmt.Errorf("credential spec requires fields")
		}
		spec.Provider = kernel.KernelCredentialVaultItemSpecInputProviderKernel
		return kernel.CredentialVaultItemSpecInputUnionParam{OfKernel: &spec}, nil
	case "1password":
		var spec kernel.OnePasswordCredentialVaultItemSpecInputParam
		if json.Unmarshal(data, &spec) != nil {
			return kernel.CredentialVaultItemSpecInputUnionParam{}, fmt.Errorf("invalid 1Password credential spec")
		}
		accountBacked := spec.Account.Valid() && strings.TrimSpace(spec.Account.Value) != ""
		storedToken := spec.AccessToken.Valid() || spec.IntegrationKey.Valid()
		if accountBacked == storedToken || (storedToken && (!spec.AccessToken.Valid() || !spec.IntegrationKey.Valid())) {
			return kernel.CredentialVaultItemSpecInputUnionParam{}, fmt.Errorf("1Password credential spec requires either account (a credential_account key) or both access_token and integration_key, never both")
		}
		if entries := len(spec.Requests.Entries); entries > 5 || (entries == 0 && (storedToken || !spec.Website.Valid())) {
			return kernel.CredentialVaultItemSpecInputUnionParam{}, fmt.Errorf("1Password credential spec requires requests with 1-5 login entries")
		}
		return kernel.CredentialVaultItemSpecInputUnionParam{Of1password: &spec}, nil
	default:
		return kernel.CredentialVaultItemSpecInputUnionParam{}, fmt.Errorf("credential spec provider must be kernel or 1password")
	}
}

func (c VaultsCmd) connectCredentialAccount(ctx context.Context, vault, key, output string, open bool) error {
	request := kernel.CredentialAccountVaultItemRequestParam{
		Type: kernel.CredentialAccountVaultItemRequestTypeCredentialAccount,
		Spec: kernel.OnePasswordCredentialAccountSpecParam{
			Provider: kernel.OnePasswordCredentialAccountSpecProvider1password,
			Authorization: kernel.OnePasswordCredentialAccountSpecAuthorizationParam{
				Method: "oauth",
				Client: kernel.OnePasswordCredentialAccountSpecAuthorizationClientParam{Type: "kernel_managed"},
			},
		},
	}
	item, err := c.vaults.Items.Upsert(ctx, key, kernel.VaultItemUpsertParams{IDOrName: vault, OfCredentialAccount: &request}, option.WithMaxRetries(0))
	if err != nil {
		return onePasswordConnectError(err)
	}
	return c.showItem(item, output, open)
}
