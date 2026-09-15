package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
)

func parseVaultCheckoutParams(raw string) (*kernel.VaultCheckoutContextParam, error) {
	object, err := vaultParamsObject(raw, "checkout")
	if err != nil {
		return nil, err
	}
	checkout, err := vaultParamsObject(string(object["checkout"]), "browser_id merchant_origin environment")
	if err != nil {
		return nil, fmt.Errorf("invalid checkout: %w", err)
	}
	var params kernel.VaultCheckoutContextParam
	if json.Unmarshal(checkout["browser_id"], &params.BrowserID) != nil || strings.TrimSpace(params.BrowserID) == "" {
		return nil, fmt.Errorf("checkout.browser_id must be a non-empty browser session ID")
	}
	if json.Unmarshal(checkout["environment"], &params.Environment) != nil || (params.Environment != "production" && params.Environment != "sandbox") {
		return nil, fmt.Errorf("checkout.environment must be production or sandbox")
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
