package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kernel/kernel-go-sdk"
)

// CleanedUpSdkError extracts a message field from the raw JSON resposne.
// This is the convention we use in the API for error response bodies (400s and 500s)
type CleanedUpSdkError struct {
	Err error
}

var _ error = CleanedUpSdkError{}

func (e CleanedUpSdkError) Error() string {
	var kerror *kernel.Error
	if errors.As(e.Err, &kerror) {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(kerror.RawJSON()), &m); err == nil {
			message, _ := m["message"].(string)
			code, _ := m["code"].(string)
			return fmt.Sprintf("%s: %s", code, message)
		} else if kerror.Response != nil && kerror.Response.Body != nil {
			// try response body as text
			body, err := io.ReadAll(kerror.Response.Body)
			if err == nil && len(body) > 0 {
				return string(body)
			}
		}
	}
	return e.Err.Error()
}

func (e CleanedUpSdkError) Unwrap() error {
	return e.Err
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
