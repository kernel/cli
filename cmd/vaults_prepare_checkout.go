package cmd

import (
	"errors"
	"fmt"

	kernel "github.com/kernel/kernel-go-sdk"
)

// A preparation is single-use even after failure or expiry, so a failed request
// is not a retry signal: the attempt may already have consumed one.
const vaultPrepareCheckoutUncertain = "a single-use preparation may still have been created; inspect the item and its events, and do not automatically retry"

func vaultPrepareCheckoutRequestError(err error) error {
	var apiErr *kernel.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf("prepare_checkout failed (HTTP %d); %s", apiErr.StatusCode, vaultPrepareCheckoutUncertain)
	}
	// Do not wrap SDK/transport errors: they can contain request or response data,
	// and the root error handler extracts raw SDK error messages through Unwrap.
	return fmt.Errorf("prepare_checkout result unavailable; %s", vaultPrepareCheckoutUncertain)
}
