package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

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
