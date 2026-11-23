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
			if code == "outcome_unknown" {
				if invocationID, _ := m["invocation_id"].(string); invocationID != "" {
					return fmt.Sprintf("%s (invocation_id: %s): %s", code, invocationID, message)
				}
			}
			if rawFields, ok := m["unsupported_fields"].([]interface{}); ok {
				fields := make([]string, 0, len(rawFields))
				for _, rawField := range rawFields {
					if field, ok := rawField.(string); ok && field != "" {
						fields = append(fields, field)
					}
				}
				if len(fields) > 0 {
					return fmt.Sprintf("%s: %s (unsupported fields: %s)", code, message, strings.Join(fields, ", "))
				}
			}
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
