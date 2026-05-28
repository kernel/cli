package cmd

import (
	"github.com/kernel/cli/pkg/util"
	"github.com/spf13/cobra"
)

func validateJSONOutput(output string) error {
	return util.ValidateJSONOutput(output)
}

func addJSONOutputFlag(cmd *cobra.Command) {
	util.AddJSONOutputFlag(cmd)
}
