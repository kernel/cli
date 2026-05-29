package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kernel/kernel-go-sdk"
)

// CleanedUpSdkError extracts a message field from the raw JSON response.
// This is the convention we use in the API for error response bodies (400s and 500s)
type CleanedUpSdkError struct {
	Err error
}

var _ error = CleanedUpSdkError{}

func (e CleanedUpSdkError) Error() string {
	if kerror, ok := e.Err.(*kernel.Error); ok {
		return cleanSdkError(kerror)
	}

	var kerror *kernel.Error
	if errors.As(e.Err, &kerror) {
		raw := kerror.Error()
		cleaned := cleanSdkError(kerror)
		if raw != "" && cleaned != raw {
			return strings.Replace(e.Err.Error(), raw, cleaned, 1)
		}
	}
	if cleaned, ok := cleanNonJSONAPIResponseError(e.Err.Error()); ok {
		return cleaned
	}
	return e.Err.Error()
}

func (e CleanedUpSdkError) Unwrap() error {
	return e.Err
}

func cleanSdkError(kerror *kernel.Error) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(kerror.RawJSON()), &m); err == nil {
		message, _ := m["message"].(string)
		code, _ := m["code"].(string)
		return fmt.Sprintf("%s: %s", code, message)
	} else if cleaned, ok := cleanNonJSONAPIResponseError(kerror.Error()); ok {
		return cleaned
	} else if kerror.Response != nil && kerror.Response.Body != nil {
		// try response body as text
		body, err := io.ReadAll(kerror.Response.Body)
		if err == nil && len(body) > 0 {
			return string(body)
		}
	}
	return kerror.Error()
}

func cleanNonJSONAPIResponseError(message string) (string, bool) {
	if !strings.Contains(message, "not 'application/json'") ||
		!strings.Contains(message, "content-type 'text/html") {
		return "", false
	}

	guidance := fmt.Sprintf(
		"server returned HTML instead of Kernel API JSON; KERNEL_BASE_URL resolves to %s. Use an API base URL, not the dashboard URL. For production, unset KERNEL_BASE_URL or set it to https://api.onkernel.com.",
		GetBaseURL(),
	)

	const sdkDecodeError = ": expected destination type"
	if idx := strings.LastIndex(message, sdkDecodeError); idx >= 0 {
		prefix := strings.TrimSuffix(message[:idx], "; check your auth and retry")
		return prefix + ": " + guidance, true
	}

	return guidance, true
}

func RequiredFlag(flag, valueHint string) error {
	if valueHint == "" {
		return fmt.Errorf("%s is required; add %s", flag, flag)
	}
	return fmt.Errorf("%s is required; add %s %s", flag, flag, valueHint)
}

func RequiredArg(name, usage string) error {
	return fmt.Errorf("missing %s; use: %s", name, usage)
}

func ChooseOne(flags ...string) error {
	return fmt.Errorf("choose one of %s", joinOptions(flags))
}

func ChooseOnlyOne(flags ...string) error {
	return fmt.Errorf("choose only one of %s", joinOptions(flags))
}

func SetAtLeastOne(flags ...string) error {
	return fmt.Errorf("set at least one of %s", joinOptions(flags))
}

func InvalidChoice(flag, value string, choices ...string) error {
	return fmt.Errorf("invalid %s %q; use one of: %s", flag, value, strings.Join(choices, ", "))
}

func NotFound(resource, id, listCommand string) error {
	if listCommand == "" {
		return fmt.Errorf("%s %q not found", resource, id)
	}
	return fmt.Errorf("%s %q not found; run `%s` to find valid IDs", resource, id, listCommand)
}

func joinOptions(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " or " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", or " + items[len(items)-1]
	}
}
