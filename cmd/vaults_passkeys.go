package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const vaultPasskeyOperationHelp = `passkey operations apply only when advertised by a KERNEL-hosted credential item.
they require --browser-id with a browser session ID, not a reusable browser name. the
browser must have the item's vault attached. --field selects one passkey field; arm and
register require it when more than one eligible field exists, while disarm omits it to
affect every passkey field. the positional operation supplies type.

arm_passkey lets that browser answer matching navigator.credentials.get requests with a
stored passkey. KERNEL signs on its servers, so the private key never enters the browser.
it returns immediately; there is no time limit. it stays armed until disarm_passkey, the
browser ends, the vault is detached, or the item or field is deleted. arming another
passkey for the same relying party replaces the earlier one. inspect the page to confirm
sign-in. if a new tab asks for a passkey in its first scripts before the handler attaches,
reload it. while armed, any script on the relying party's pages can obtain a sign-in
without user interaction; disarm when the task is done. signatures use counter 0 and
synced-passkey backup flags.

register_passkey makes the next navigator.credentials.create request in that browser create
a passkey and store it in the selected empty passkey field before returning it to the page.
only ES256 passkeys are created. declare an optional passkey field first. the operation
returns immediately; there is no time limit, and a later register_passkey replaces the
browser's prior pending registration. drive the site's add-passkey flow yourself, then
confirm the field has_value with items get or inspect items events for
credential_passkey_registered; use arm_passkey to test sign-in.

disarm_passkey removes this item's armed passkeys and pending registration from the browser.
with --field it affects only that field; without --field it affects every passkey field on
the item. pages already loaded keep KERNEL's WebAuthn handler but use the browser's own
behavior.`

func isPasskeyOperation(operation string) bool {
	switch operation {
	case "arm_passkey", "register_passkey", "disarm_passkey":
		return true
	default:
		return false
	}
}

func parseVaultPasskeyOperationFlags(cmd *cobra.Command, operation string) (*kernel.VaultItemPerformOperationParams, error) {
	fieldSet := cmd.Flags().Changed("field")
	if !isPasskeyOperation(operation) {
		if fieldSet {
			return nil, fmt.Errorf("--field is only supported for arm_passkey, register_passkey, and disarm_passkey")
		}
		return nil, nil
	}
	if cmd.Flags().Changed("params") || cmd.Flags().Changed("spec-file") {
		return nil, fmt.Errorf("%s uses --browser-id and optional --field, not --params or --spec-file", operation)
	}
	if cmd.Flags().Changed("page-url") {
		return nil, fmt.Errorf("--page-url is only supported for authorize; %s was not invoked", operation)
	}
	if !cmd.Flags().Changed("browser-id") {
		return nil, fmt.Errorf("%s requires --browser-id with a browser session ID, not a name", operation)
	}
	browserID, _ := cmd.Flags().GetString("browser-id")
	if strings.TrimSpace(browserID) == "" {
		return nil, fmt.Errorf("--browser-id must be a non-empty browser session ID, not a name")
	}
	field, _ := cmd.Flags().GetString("field")
	if fieldSet && (strings.TrimSpace(field) == "" || len(field) > 64) {
		return nil, fmt.Errorf("--field must be a non-empty passkey field name of at most 64 bytes")
	}

	request := &kernel.VaultItemPerformOperationParams{}
	switch operation {
	case "arm_passkey":
		body := &kernel.ArmPasskeyVaultItemOperationRequestParam{
			BrowserID: browserID,
			Type:      kernel.ArmPasskeyVaultItemOperationRequestTypeArmPasskey,
		}
		if fieldSet {
			body.Field = kernel.Opt(field)
		}
		request.OfArmPasskey = body
	case "register_passkey":
		body := &kernel.RegisterPasskeyVaultItemOperationRequestParam{
			BrowserID: browserID,
			Type:      kernel.RegisterPasskeyVaultItemOperationRequestTypeRegisterPasskey,
		}
		if fieldSet {
			body.Field = kernel.Opt(field)
		}
		request.OfRegisterPasskey = body
	case "disarm_passkey":
		body := &kernel.DisarmPasskeyVaultItemOperationRequestParam{
			BrowserID: browserID,
			Type:      kernel.DisarmPasskeyVaultItemOperationRequestTypeDisarmPasskey,
		}
		if fieldSet {
			body.Field = kernel.Opt(field)
		}
		request.OfDisarmPasskey = body
	}
	return request, nil
}

type vaultPasskeyOperationResult struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	RpID   string `json:"rp_id,omitempty"`
}

var vaultPasskeyOperationResultFields = vaultFieldsOf("type status rp_id")

func printVaultPasskeyOperationResult(response *kernel.VaultItemOperationResponseUnion, operation, output string) error {
	if response == nil {
		return fmt.Errorf("empty %s result; inspect the item and browser before invoking another passkey operation", operation)
	}
	safe, err := filterVaultJSON(json.RawMessage(response.RawJSON()), vaultPasskeyOperationResultFields)
	if err != nil {
		return fmt.Errorf("invalid %s result; inspect the item and browser before invoking another passkey operation", operation)
	}
	var result vaultPasskeyOperationResult
	if json.Unmarshal(safe, &result) != nil || result.Type != operation || (result.Status != "armed" && result.Status != "disarmed") {
		return fmt.Errorf("invalid %s result; inspect the item and browser before invoking another passkey operation", operation)
	}
	if output == "json" {
		return printVaultJSON(result)
	}
	rows := pterm.TableData{{"property", "value"}, {"operation", result.Type}, {"status", result.Status}}
	if result.RpID != "" {
		rows = append(rows, []string{"relying party", result.RpID})
	}
	PrintTableNoPad(rows, true)
	switch operation {
	case "arm_passkey":
		pterm.Info.Println("armed means the browser will answer matching passkey requests; it does not mean a page has asked yet. inspect the page, and disarm when the task is done.")
	case "register_passkey":
		pterm.Info.Println("complete the website's add-passkey flow, then confirm the field has_value with items get or inspect items events. only ES256 passkeys are created.")
	case "disarm_passkey":
		pterm.Info.Println("the browser no longer uses the selected passkey field or fields from this item; already-loaded pages use the browser's own behavior.")
	}
	return nil
}
