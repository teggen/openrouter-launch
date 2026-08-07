// Package cli wires the openrouter-launch commands together.
package cli

import (
	"github.com/spf13/cobra"
)

// globalFlags holds values shared by every subcommand.
type globalFlags struct {
	refresh bool
	yes     bool
}

// NewRootCmd builds the command tree. It is a constructor rather than a
// package-level variable so tests get an isolated tree per run.
func NewRootCmd() *cobra.Command {
	flags := &globalFlags{}

	root := &cobra.Command{
		Use:   "openrouter-launch",
		Short: "Launch coding agents against OpenRouter models",
		Long: "openrouter-launch picks an OpenRouter model and starts a coding " +
			"agent configured to use it, without modifying the agent's own configuration.",
		SilenceUsage: true,
	}

	root.PersistentFlags().BoolVar(&flags.refresh, "refresh", false,
		"bypass the cached model catalog and fetch a fresh copy")
	root.PersistentFlags().BoolVarP(&flags.yes, "yes", "y", false,
		"skip confirmation prompts")

	root.AddCommand(newAgentsCmd())
	root.AddCommand(newModelsCmd(flags))
	root.AddCommand(newProfileCmd(flags))
	for _, cmd := range newLaunchCmds(flags) {
		root.AddCommand(cmd)
	}

	return root
}

// Execute runs the CLI, returning a non-nil error on failure. Cobra has
// already printed the error, so main only needs the exit code.
func Execute() error {
	return NewRootCmd().Execute()
}
