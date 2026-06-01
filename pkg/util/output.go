package util

import (
	"fmt"

	"github.com/spf13/cobra"
)

const JSONOutputFlagDescription = "Output format: json for raw API response"

func ValidateJSONOutput(output string) error {
	if output == "" || output == "json" {
		return nil
	}
	return fmt.Errorf("unsupported --output value %q; use \"json\" or omit --output for human-readable output", output)
}

func AddJSONOutputFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "", JSONOutputFlagDescription)
}
