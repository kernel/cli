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

const onePasswordUnavailable = "1Password is not available in this deployment; tell the user and offer Kernel-hosted collection instead"

// A 5xx or transport failure after dispatch leaves the 1Password outcome unknown;
// there is no reset operation, so agents must stop rather than retry or recreate.
func onePasswordOperationError(err error, operation string) error {
	const unknown = "outcome is unknown; inspect items get and tell the user; do not retry, delete, or recreate the item"
	var apiErr *kernel.Error
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%s %s", operation, unknown)
	}
	switch apiErr.StatusCode {
	case 400, 403, 404:
		return fmt.Errorf("%s rejected (HTTP %d); correct the parameters or item before deciding on a new request; do not retry automatically", operation, apiErr.StatusCode)
	case 409:
		return fmt.Errorf("%s is not currently available (HTTP 409); inspect items get for the current state; do not retry, delete, or recreate the item to reset it", operation)
	case 503:
		return fmt.Errorf("%s unavailable (HTTP 503): %s", operation, onePasswordUnavailable)
	}
	return fmt.Errorf("%s request failed (HTTP %d); %s", operation, apiErr.StatusCode, unknown)
}

func onePasswordConnectError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 500 {
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(apiErr.RawJSON()), &body) == nil && body.Code == "provider_unavailable" {
			return fmt.Errorf("1Password account linking unavailable (HTTP %d): %s", apiErr.StatusCode, onePasswordUnavailable)
		}
	}
	return vaultCredentialError(err)
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
		case "tokens", "access_token", "refresh_token", "client_secret", "credentials":
			return true
		case "authorization", "client", "provider_config":
			if vaultSpecHasSecrets(child) {
				return true
			}
		}
	}
	return false
}
