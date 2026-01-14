package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for Kernel CLI.

To load completions:

Bash:
  $ source <(kernel completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ kernel completion bash > /etc/bash_completion.d/kernel
  # macOS:
  $ kernel completion bash > $(brew --prefix)/etc/bash_completion.d/kernel

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ kernel completion zsh > "${fpath[1]}/_kernel"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ kernel completion fish | source

  # To load completions for each session, execute once:
  $ kernel completion fish > ~/.config/fish/completions/kernel.fish
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			return cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			return cmd.Root().GenFishCompletion(os.Stdout, true)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
