package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrintResultShowsStatusReasonOnFailure(t *testing.T) {
	buf := capturePtermOutput(t)

	printResult(false, `{"error":"domain is required"}`, "Invocation failed. See output for details.")

	out := buf.String()
	require.Contains(t, out, "Reason: Invocation failed. See output for details.")
	require.Contains(t, out, `"error": "domain is required"`)
	require.Less(t, strings.Index(out, "Reason:"), strings.Index(out, "Result:"),
		"the customer-safe reason should precede the raw output")
}

func TestPrintResultOmitsStatusReasonWhenEmpty(t *testing.T) {
	buf := capturePtermOutput(t)

	printResult(false, "boom", "")

	out := buf.String()
	require.NotContains(t, out, "Reason:")
	require.Contains(t, out, "boom")
}

func TestPrintResultIgnoresStatusReasonOnSuccess(t *testing.T) {
	buf := capturePtermOutput(t)

	// status_reason is omitted for non-failed invocations, but a stale value must
	// never be presented as a failure summary.
	printResult(true, `{"ok":true}`, "should not appear")

	out := buf.String()
	require.NotContains(t, out, "should not appear")
	require.Contains(t, out, `"ok": true`)
}
