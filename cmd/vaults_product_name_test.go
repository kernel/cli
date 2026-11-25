package cmd

import (
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var capitalizedProductName = regexp.MustCompile(`\bKernel\b`)

// Vault help writes the product name as KERNEL.
func TestVaultHelpWritesProductNameAsKERNEL(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for field, text := range map[string]string{"Short": cmd.Short, "Long": cmd.Long, "Example": cmd.Example} {
			if capitalizedProductName.MatchString(text) {
				t.Errorf("%q %s writes Kernel; use KERNEL", cmd.CommandPath(), field)
			}
		}
		for _, flags := range []*pflag.FlagSet{cmd.LocalFlags(), cmd.PersistentFlags()} {
			flags.VisitAll(func(flag *pflag.Flag) {
				if capitalizedProductName.MatchString(flag.Usage) {
					t.Errorf("%q --%s usage writes Kernel; use KERNEL", cmd.CommandPath(), flag.Name)
				}
			})
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(newVaultsCommand())
	walk(newVaultProviderConfigsCommand())
}
