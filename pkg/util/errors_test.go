package util

import (
	"io"
	"net/http"
	"strings"
	"testing"

	kernel "github.com/kernel/kernel-go-sdk"
)

func TestCleanedUpSDKErrorIncludesStatusForPlainTextResponses(t *testing.T) {
	err := CleanedUpSdkError{Err: &kernel.Error{
		StatusCode: http.StatusForbidden,
		Response: &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("Credential is scoped to a different project\n")),
		},
	}}

	if got, want := err.Error(), "403: Credential is scoped to a different project"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
