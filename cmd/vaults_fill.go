package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// Keep these limits in sync with https://api.onkernel.com/spec.yaml.
const (
	vaultFillOperation       = "fill"
	vaultFillExpirationField = "expiration"
	vaultFillMaxFields       = 32
	vaultFillMaxTimeoutMs    = 30000
)

var vaultFillStoredFields = []string{
	"number", "exp_month", "exp_year", "cvc", "billing_name", "billing_line1",
	"billing_line2", "billing_city", "billing_state", "billing_postal_code", "billing_country",
}

var vaultFillExpirationFormats = []string{"MM/YY", "MM/YYYY"}

var vaultFillFlags = []string{"browser-id", "page-url", "field", "timeout-ms"}

// vaultFillField is one parsed --field binding. Bindings keep their command-line
// order because the API fills in request order and stops at the first failure.
type vaultFillField struct {
	Field    string
	Format   string
	Selector string
}

type vaultFillRequest struct {
	BrowserID string
	PageURL   string
	TimeoutMs int64
	Fields    []vaultFillField
}

// vaultFillFromFlags returns the fill request for the fill operation and nil for
// every other advertised operation, which takes no parameters.
func vaultFillFromFlags(cmd *cobra.Command, operation string) (*vaultFillRequest, error) {
	if operation != vaultFillOperation {
		for _, name := range vaultFillFlags {
			if cmd.Flags().Changed(name) {
				return nil, fmt.Errorf("--%s applies only to the fill operation", name)
			}
		}
		return nil, nil
	}
	browserID, _ := cmd.Flags().GetString("browser-id")
	pageURL, _ := cmd.Flags().GetString("page-url")
	raw, _ := cmd.Flags().GetStringArray("field")
	timeout, _ := cmd.Flags().GetInt64("timeout-ms")
	request := &vaultFillRequest{BrowserID: strings.TrimSpace(browserID), PageURL: strings.TrimSpace(pageURL)}
	// A zero value means "unset": the API applies its own default deadline.
	if cmd.Flags().Changed("timeout-ms") {
		if timeout < 1 {
			return nil, vaultFillTimeoutError()
		}
		request.TimeoutMs = timeout
	}
	if err := request.validate(raw); err != nil {
		return nil, err
	}
	return request, nil
}

func vaultFillTimeoutError() error {
	return fmt.Errorf("--timeout-ms must be between 1 and %d milliseconds for the whole operation", vaultFillMaxTimeoutMs)
}

func (r *vaultFillRequest) validate(raw []string) error {
	if r.BrowserID == "" {
		return fmt.Errorf("fill requires --browser-id with a browser session ID, not a reusable browser name")
	}
	if err := validateVaultFillPageURL(r.PageURL); err != nil {
		return err
	}
	if len(raw) == 0 {
		return fmt.Errorf("fill requires at least one --field <field>=<css-selector> binding")
	}
	if len(raw) > vaultFillMaxFields {
		return fmt.Errorf("fill accepts at most %d --field bindings", vaultFillMaxFields)
	}
	if r.TimeoutMs < 0 || r.TimeoutMs > vaultFillMaxTimeoutMs {
		return vaultFillTimeoutError()
	}
	seen := make(map[string]bool, len(raw))
	for _, value := range raw {
		field, err := parseVaultFillField(value)
		if err != nil {
			return err
		}
		if seen[field.Selector] {
			return fmt.Errorf("each --field must use a distinct selector; %q is repeated and no two bindings may resolve to the same element", field.Selector)
		}
		seen[field.Selector] = true
		r.Fields = append(r.Fields, field)
	}
	return nil
}

// The API requires an exact HTTPS page URL without embedded credentials and
// matches it against exactly one open page; prefixes and globs never match.
func validateVaultFillPageURL(value string) error {
	invalid := fmt.Errorf("--page-url must be the exact current HTTPS page URL without embedded credentials")
	if value == "" {
		return fmt.Errorf("fill requires --page-url with the exact current top-level page URL")
	}
	if strings.ContainsAny(value, " \t\r\n*") {
		return invalid
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host == "" {
		return invalid
	}
	return nil
}

func parseVaultFillField(value string) (vaultFillField, error) {
	// Selectors may contain '=' (for example input[name=card]), so only the first
	// separator delimits the card field from its selector.
	name, selector, found := strings.Cut(value, "=")
	selector = strings.TrimSpace(selector)
	if !found || selector == "" {
		return vaultFillField{}, fmt.Errorf("--field must be <field>=<css-selector>, for example --field number='#card-number'")
	}
	name, format, hasFormat := strings.Cut(strings.TrimSpace(name), ":")
	field := vaultFillField{Field: name, Format: format, Selector: selector}
	if name == vaultFillExpirationField {
		if !hasFormat {
			return vaultFillField{}, fmt.Errorf("expiration requires a format: use --field expiration:<%s>=<css-selector>", strings.Join(vaultFillExpirationFormats, "|"))
		}
		if !containsVaultFillValue(vaultFillExpirationFormats, format) {
			return vaultFillField{}, fmt.Errorf("expiration format must be one of: %s", strings.Join(vaultFillExpirationFormats, ", "))
		}
		return field, nil
	}
	if hasFormat {
		return vaultFillField{}, fmt.Errorf("only expiration takes a format; drop %q from --field %s", format, name)
	}
	if !containsVaultFillValue(vaultFillStoredFields, name) {
		return vaultFillField{}, fmt.Errorf("--field %q is not a card field; use one of: %s, %s:<%s>", name,
			strings.Join(vaultFillStoredFields, ", "), vaultFillExpirationField, strings.Join(vaultFillExpirationFormats, "|"))
	}
	return field, nil
}

func containsVaultFillValue(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (r vaultFillRequest) params() kernel.FillVaultItemOperationRequestParam {
	body := kernel.FillVaultItemOperationRequestParam{
		BrowserID: r.BrowserID,
		PageURL:   r.PageURL,
		Type:      kernel.FillVaultItemOperationRequestTypeFill,
	}
	if r.TimeoutMs != 0 {
		body.TimeoutMs = kernel.Opt(r.TimeoutMs)
	}
	for _, field := range r.Fields {
		if field.Field == vaultFillExpirationField {
			body.Fields = append(body.Fields, kernel.VaultCardFillFieldParamOfVaultCardFillFieldVaultCardExpirationFillField(field.Field, field.Format, field.Selector))
			continue
		}
		body.Fields = append(body.Fields, kernel.VaultCardFillFieldParamOfVaultCardFillFieldVaultCardStoredFillField(field.Field, field.Selector))
	}
	return body
}

var vaultFillResultFields = vaultOutputFields{
	"type": nil, "status": nil, "fields": vaultFieldsOf("index status error_code"),
}

// Fill returns a value-free execution result instead of the item, so it is
// printed on its own. Request bindings are local and label each result row.
func printVaultFillResult(result kernel.FillVaultItemOperationResult, request *vaultFillRequest, output string) error {
	raw, err := filterVaultJSON(json.RawMessage(result.RawJSON()), vaultFillResultFields)
	if err != nil {
		return err
	}
	if output == "json" {
		return printVaultJSON(raw)
	}
	var safe kernel.FillVaultItemOperationResult
	if err := json.Unmarshal(raw, &safe); err != nil {
		return fmt.Errorf("invalid vault fill response")
	}
	rows := pterm.TableData{{"#", "Field", "Selector", "Status", "Error"}}
	for _, field := range safe.Fields {
		name, selector := "-", "-"
		if request != nil && field.Index >= 0 && int(field.Index) < len(request.Fields) {
			binding := request.Fields[int(field.Index)]
			name, selector = binding.Field, binding.Selector
			if binding.Format != "" {
				name += " (" + binding.Format + ")"
			}
		}
		rows = append(rows, []string{fmt.Sprint(field.Index), name, selector, string(field.Status), util.OrDash(string(field.ErrorCode))})
	}
	PrintTableNoPad(rows, true)
	printVaultFillGuidance(safe.Status)
	return nil
}

func printVaultFillGuidance(status kernel.FillVaultItemOperationResultStatus) {
	switch status {
	case kernel.FillVaultItemOperationResultStatusCompleted:
		pterm.Success.Println("Fill completed: every requested field was written. Filling is not payment and does not confirm merchant acceptance; the form was not submitted.")
	case kernel.FillVaultItemOperationResultStatusFailed:
		pterm.Warning.Println("Fill failed: execution stopped at the first failed field and earlier fields were not rolled back. Do not automatically retry or fall back to aliases; inspect the page and items events before any explicit new attempt.")
	default:
		pterm.Warning.Println("Fill outcome unknown: at least one field's result could not be determined, which is not a retry signal. Do not automatically retry or fall back to aliases; reconcile with items events before any explicit new attempt.")
	}
	pterm.Info.Println("Secret values are never returned. An agent with unrestricted browser access can still read filled values from the page.")
}
