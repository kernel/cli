package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/spf13/cobra"
)

// Do not wrap SDK errors here: response bodies and transport errors can echo
// write-only credentials, and the root command unwraps SDK errors for display.
func vaultCardError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) {
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(apiErr.RawJSON()), &body) == nil {
			discoveryFailure := false
			switch body.Code {
			case "ambiguous_page", "timeout":
				discoveryFailure = apiErr.StatusCode == 400
			case "destination_denied":
				discoveryFailure = apiErr.StatusCode == 403
			case "browser_not_found":
				discoveryFailure = apiErr.StatusCode == 404
			case "browser_unavailable":
				discoveryFailure = apiErr.StatusCode == 409
			case "browser_error":
				discoveryFailure = apiErr.StatusCode == 500
			}
			if discoveryFailure {
				return fmt.Errorf("%s: Link checkout inspection failed before a card was created; correct the browser or page and retry", body.Code)
			}
			if apiErr.StatusCode == 409 && body.Code == "conflict" {
				return fmt.Errorf("vault conflict (HTTP 409); inspect current state and immutable bindings")
			}
		}
	}
	return vaultCredentialError(err)
}

func vaultCredentialError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 400:
			return fmt.Errorf("vault request rejected (HTTP 400); check the input and credential validity")
		case 403:
			return fmt.Errorf("vault request forbidden (HTTP 403); check authentication scope and permissions")
		case 404:
			return fmt.Errorf("vault resource not found (HTTP 404)")
		case 409:
			return fmt.Errorf("vault conflict (HTTP 409); inspect current version, state, and immutable bindings before retrying")
		default:
			return fmt.Errorf("vault request failed (HTTP %d); outcome may be unresolved, inspect existing state before taking further action", apiErr.StatusCode)
		}
	}
	return fmt.Errorf("vault request failed; details withheld to protect credentials; inspect existing state before taking further action")
}

func readVaultSecrets(cmd *cobra.Command, flag string, fields ...string) (map[string]string, error) {
	path, _ := cmd.Flags().GetString(flag)
	if path == "" {
		return nil, fmt.Errorf("--%s requires a file path or '-' for stdin", flag)
	}
	var reader io.Reader = cmd.InOrStdin()
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("could not open --%s file", flag)
		}
		defer file.Close()
		reader = file
	}
	const maxBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		return nil, fmt.Errorf("could not read --%s (maximum 1 MiB)", flag)
	}
	var values map[string]string
	if json.Unmarshal(data, &values) != nil || len(values) != len(fields) {
		return nil, fmt.Errorf("--%s must contain only the documented non-empty JSON string fields", flag)
	}
	for _, field := range fields {
		if values[field] == "" {
			return nil, fmt.Errorf("--%s requires %s as a non-empty string", flag, field)
		}
	}
	return values, nil
}

func vaultSpecHasSecrets(value json.RawMessage) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(value, &object) != nil {
		return false
	}
	for key, child := range object {
		switch strings.ToLower(key) {
		case "tokens", "access_token", "refresh_token", "link_pay_token", "client_secret", "credentials":
			return true
		case "authorization", "client", "provider_config":
			if vaultSpecHasSecrets(child) {
				return true
			}
		}
	}
	return false
}
