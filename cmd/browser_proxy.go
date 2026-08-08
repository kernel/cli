package cmd

import (
	"fmt"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
)

// proxyModes are the values accepted by --proxy-mode. direct forces egress
// without a proxy regardless of stealth; default restores the browser's
// stealth-derived default (Kernel's stealth proxy when stealth is on, direct
// egress otherwise).
var proxyModes = []string{
	string(kernel.BrowserProxyModeDirect),
	string(kernel.BrowserProxyModeDefault),
}

// proxySelection holds the mutually exclusive proxy flags shared by the browser
// and auth-connection commands. The API accepts exactly one of id, name, or
// mode, so the CLI validates that before building the param.
type proxySelection struct {
	ID   string
	Name string
	Mode string
}

// set reports whether any proxy flag was provided.
func (p proxySelection) set() bool {
	return p.ID != "" || p.Name != "" || p.Mode != ""
}

// buildProxyConfigParam converts the proxy flags to the API's proxy config.
// Callers should only use the result when set() is true: an empty config is
// rejected by the API rather than treated as "leave unchanged".
func buildProxyConfigParam(sel proxySelection) (kernel.BrowserProxyConfigParam, error) {
	p := kernel.BrowserProxyConfigParam{}

	provided := make([]string, 0, 3)
	if sel.ID != "" {
		provided = append(provided, "--proxy-id")
	}
	if sel.Name != "" {
		provided = append(provided, "--proxy-name")
	}
	if sel.Mode != "" {
		provided = append(provided, "--proxy-mode")
	}
	if len(provided) > 1 {
		return p, fmt.Errorf("must specify at most one of %s", strings.Join(provided, ", "))
	}

	switch {
	case sel.ID != "":
		p.ID = kernel.Opt(sel.ID)
	case sel.Name != "":
		p.Name = kernel.Opt(sel.Name)
	case sel.Mode != "":
		mode := strings.TrimSpace(strings.ToLower(sel.Mode))
		switch kernel.BrowserProxyMode(mode) {
		case kernel.BrowserProxyModeDirect, kernel.BrowserProxyModeDefault:
			p.Mode = kernel.BrowserProxyMode(mode)
		default:
			return p, fmt.Errorf("unknown proxy mode %q: must be one of %s", sel.Mode, strings.Join(proxyModes, ", "))
		}
	}
	return p, nil
}

// formatBrowserProxy renders a session's resolved proxy configuration for a
// details table, or "" when the API reported none.
func formatBrowserProxy(p kernel.BrowserProxy) string {
	return formatProxyRef(p.ID, p.Name, p.Mode)
}

// formatBrowserProxyConfig renders a stored proxy configuration (such as an auth
// connection's browser default) for a details table, or "" when there is none.
func formatBrowserProxyConfig(p kernel.BrowserProxyConfig) string {
	return formatProxyRef(p.ID, p.Name, p.Mode)
}

// formatProxyRef shows a selected proxy by name (falling back to its ID), or the
// egress mode when no proxy is selected.
func formatProxyRef(id, name string, mode kernel.BrowserProxyMode) string {
	switch {
	case name != "" && id != "":
		return fmt.Sprintf("%s (%s)", name, id)
	case name != "":
		return name
	case id != "":
		return id
	case mode != "":
		return string(mode)
	default:
		return ""
	}
}
