// Package cli wires the openrouter-launch commands together.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/launch"
)

// globalFlags holds values shared by every subcommand.
type globalFlags struct {
	refresh bool
	yes     bool
}

// app is what every subcommand needs: the shared launch service and the
// global flag values.
type app struct {
	svc   *launch.Service
	flags *globalFlags
}

// NewRootCmd builds the command tree against the live OpenRouter API.
func NewRootCmd() *cobra.Command {
	return NewRootCmdWith(&launch.Service{})
}

// NewRootCmdWith builds the command tree against the given service. It is a
// constructor rather than a package-level variable so tests get an isolated
// tree per run, and it takes the service as an argument rather than reading
// a package global so that a Phase 2 TUI can share the same instance.
func NewRootCmdWith(svc *launch.Service) *cobra.Command {
	if svc == nil {
		// A nil Service would build a full command tree that panics later,
		// on first use, when Snapshot dereferences a nil receiver. Fail at
		// construction instead, naming the problem, matching the idiom in
		// agent.buildIndex for a nil Launcher.
		panic("cli: NewRootCmdWith requires a non-nil *launch.Service")
	}
	a := &app{svc: svc, flags: &globalFlags{}}

	root := &cobra.Command{
		Use:   "openrouter-launch",
		Short: "Launch coding agents against OpenRouter models",
		Long: "openrouter-launch picks an OpenRouter model and starts a coding " +
			"agent configured to use it, without modifying the agent's own configuration.",
		SilenceUsage: true,
	}

	root.PersistentFlags().BoolVar(&a.flags.refresh, "refresh", false,
		"bypass the cached model catalog and fetch a fresh copy")
	root.PersistentFlags().BoolVarP(&a.flags.yes, "yes", "y", false,
		"skip confirmation prompts")

	root.AddCommand(newAgentsCmd())
	root.AddCommand(newModelsCmd(a))
	root.AddCommand(newProfileCmd(a))
	for _, cmd := range newLaunchCmds(a) {
		root.AddCommand(cmd)
	}

	return root
}

// Execute runs the CLI, returning a non-nil error on failure. Cobra has
// already printed the error, so main only needs the exit code.
func Execute() error {
	return NewRootCmd().Execute()
}
