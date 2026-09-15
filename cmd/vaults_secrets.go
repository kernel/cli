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
			return fmt.Errorf("vault request forbidden (HTTP 403); configuration writes require organization-scoped authentication")
		case 404:
			return fmt.Errorf("vault resource not found (HTTP 404)")
		case 409:
			return fmt.Errorf("vault conflict (HTTP 409); names and bindings must match, grants cannot be replaced, and referenced configurations cannot be deleted")
		default:
			return fmt.Errorf("vault request failed (HTTP %d); outcome may be unresolved, inspect existing state before taking further action", apiErr.StatusCode)
		}
	}
	return fmt.Errorf("vault request failed; details withheld to protect credentials; inspect existing state before taking further action")
}

// Credential item writes fail for reasons a wallet or provider configuration
// cannot, so map their conflicts to what the caller must actually reconcile.
// Details stay withheld: bodies and transport errors can echo submitted values.
func vaultCredentialItemError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 400:
			return fmt.Errorf("credential request rejected (HTTP 400); field names must be declared, values must satisfy their declared type, and a required totp field needs a valid Base32 seed that no form can collect")
		case 409:
			return fmt.Errorf("credential conflict (HTTP 409); the item changed since your last read or is not a credential item. Re-read it with items get and retry with the version it returns")
		}
	}
	return vaultCredentialError(err)
}

func readVaultSecretFile(cmd *cobra.Command, flag string) ([]byte, error) {
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
	return data, nil
}

func readVaultSecrets(cmd *cobra.Command, flag string, fields ...string) (map[string]string, error) {
	data, err := readVaultSecretFile(cmd, flag)
	if err != nil {
		return nil, err
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

// Credential values are write-only secrets, so they arrive through a protected
// file or stdin rather than shell arguments. A null value clears a stored value
// on update; creation rejects null and empty values separately.
func readVaultFieldValues(cmd *cobra.Command, flag string) (map[string]*string, error) {
	data, err := readVaultSecretFile(cmd, flag)
	if err != nil {
		return nil, err
	}
	var values map[string]*string
	if json.Unmarshal(data, &values) != nil || len(values) < 1 || len(values) > 32 {
		return nil, fmt.Errorf("--%s must be a JSON object mapping 1-32 field names to string or null values", flag)
	}
	for name := range values {
		if !vaultFieldNamePattern.MatchString(name) {
			return nil, fmt.Errorf("--%s field names must match [a-zA-Z][a-zA-Z0-9_]{0,63}", flag)
		}
	}
	return values, nil
}
