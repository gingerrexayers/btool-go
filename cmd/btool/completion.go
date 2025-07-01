package main

import (
	"os"

	"github.com/spf13/cobra"
)

// NewCompletionCommand creates the 'completion' command, which is a standard
// feature in Cobra applications to generate shell completion scripts.
func NewCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate completion script",
		Long: `To load completions:

Bash:

  To load completions for the current session, run:
  $ source <(btool completion bash)

  To load completions for all new sessions, run once:
  # macOS (using Homebrew):
  $ btool completion bash > $(brew --prefix)/etc/bash_completion.d/btool
  # Linux:
  $ sudo btool completion bash > /etc/bash_completion.d/btool

Zsh:

  If shell completion is not already enabled in your environment,
  you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  To load completions for all new sessions, run once:
  $ btool completion zsh > "${fpath[1]}/_btool"

  You will need to start a new shell for this setup to take effect.

Fish:

  To load completions for the current session, run:
  $ btool completion fish | source

  To load completions for all new sessions, run once:
  $ btool completion fish > ~/.config/fish/completions/btool.fish

Powershell:

  To load completions for the current session, run:
  PS> btool completion powershell | Out-String | Invoke-Expression

  To load completions for all new sessions, add the following
  to your PowerShell profile file:
  PS> Invoke-Expression (& btool completion powershell | Out-String)

  If the profile file doesn't exist, you can create it by running:
  PS> New-Item -Path $PROFILE -Type File -Force
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.ExactValidArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			switch args[0] {
			case "bash":
				cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
		},
	}
}
