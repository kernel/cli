package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
)

// Environments and checkout processors accepted by prepare_checkout. Square,
// Braintree, Worldpay and Adyen use production or sandbox; Bambora and Mercado Pago
// use shared. Pairing is enforced by the API, which owns processor enablement.
var vaultCheckoutEnvironments = []kernel.VaultCheckoutContextEnvironment{
	kernel.VaultCheckoutContextEnvironmentProduction,
	kernel.VaultCheckoutContextEnvironmentSandbox,
	kernel.VaultCheckoutContextEnvironmentShared,
}

var vaultCheckoutProcessors = []kernel.AgentcardPreparedProcessor{
	kernel.AgentcardPreparedProcessorSquare,
	kernel.AgentcardPreparedProcessorBraintree,
	kernel.AgentcardPreparedProcessorWorldpay,
	kernel.AgentcardPreparedProcessorBambora,
	kernel.AgentcardPreparedProcessorMercadoPago,
	kernel.AgentcardPreparedProcessorAdyen,
}

func vaultCheckoutProcessorNames() []string {
	names := make([]string, 0, len(vaultCheckoutProcessors))
	for _, p := range vaultCheckoutProcessors {
		names = append(names, string(p))
	}
	return names
}

func parseVaultCheckoutParams(raw string) (*kernel.VaultCheckoutContextParam, error) {
	object, err := vaultParamsObject(raw, "checkout")
	if err != nil {
		return nil, err
	}
	checkout, err := vaultParamsObject(string(object["checkout"]), "browser_id merchant_origin environment psp")
	if err != nil {
		return nil, fmt.Errorf("invalid checkout: %w", err)
	}
	var params kernel.VaultCheckoutContextParam
	if json.Unmarshal(checkout["browser_id"], &params.BrowserID) != nil || strings.TrimSpace(params.BrowserID) == "" {
		return nil, fmt.Errorf("checkout.browser_id must be a non-empty browser session ID")
	}
	if json.Unmarshal(checkout["environment"], &params.Environment) != nil || !slices.Contains(vaultCheckoutEnvironments, params.Environment) {
		return nil, fmt.Errorf("checkout.environment must be production, sandbox, or shared")
	}
	// The processor is optional; omitting it keeps Square compatibility. Non-Square
	// processors require multi-processor preparation enablement on the project.
	if psp, present := checkout["psp"]; present {
		if json.Unmarshal(psp, &params.Psp) != nil || !slices.Contains(vaultCheckoutProcessors, params.Psp) {
			return nil, fmt.Errorf("checkout.psp must be one of %s", strings.Join(vaultCheckoutProcessorNames(), ", "))
		}
	}
	if json.Unmarshal(checkout["merchant_origin"], &params.MerchantOrigin) != nil {
		return nil, fmt.Errorf("checkout.merchant_origin must be a canonical HTTPS origin (HTTP localhost is allowed for tests)")
	}
	u, err := url.Parse(params.MerchantOrigin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" ||
		u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		params.MerchantOrigin != u.Scheme+"://"+u.Host || strings.ContainsAny(u.Host, "*\\") ||
		(u.Scheme != "https" && !(u.Scheme == "http" && u.Hostname() == "localhost")) {
		return nil, fmt.Errorf("checkout.merchant_origin must be a canonical HTTPS origin (HTTP localhost is allowed for tests)")
	}
	return &params, nil
}
