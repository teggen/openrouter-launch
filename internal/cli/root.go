// Package cli wires the openrouter-launch commands together.
package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/tui"
)

// globalFlags holds values shared by every subcommand.
type globalFlags struct {
	refresh bool
	yes     bool
}

// tuiFunc opens the interactive session. It is a field on app rather than a
// direct call to tui.Run so tests can drive the CLI's wiring without a
// terminal — the same reasoning that made Service.Catalog and Service.Run
// fields rather than package globals.
type tuiFunc func(context.Context, tui.Options) (launch.Plan, error)

// app is what every subcommand needs: the shared launch service and the
// global flag values.
type app struct {
	svc     *launch.Service
	flags   *globalFlags
	openTUI tuiFunc
}

// NewRootCmd builds the command tree against the live OpenRouter API.
func NewRootCmd() *cobra.Command {
	return NewRootCmdWith(&launch.Service{})
}

// NewRootCmdWith builds the command tree against the given service, so the
// CLI and the TUI session it opens (see runTUI's tui.Options.Service) share
// one *launch.Service instance rather than each constructing its own. It is
// a constructor rather than a package-level variable so tests get an
// isolated command tree, wired to their own service, on every call.
func NewRootCmdWith(svc *launch.Service) *cobra.Command {
	return newRootCmd(svc, tui.Run)
}

func newRootCmd(svc *launch.Service, openTUI tuiFunc) *cobra.Command {
	if svc == nil {
		// A nil Service would build a full command tree that panics later,
		// on first use, when Snapshot dereferences a nil receiver. Fail at
		// construction instead, naming the problem, matching the idiom in
		// agent.buildIndex for a nil Launcher.
		panic("cli: NewRootCmdWith requires a non-nil *launch.Service")
	}
	a := &app{svc: svc, flags: &globalFlags{}, openTUI: openTUI}

	root := &cobra.Command{
		Use:   "openrouter-launch",
		Short: "Launch coding agents against OpenRouter models",
		Long: "openrouter-launch picks an OpenRouter model and starts a coding " +
			"agent configured to use it, without modifying the agent's own configuration.",
		SilenceUsage: true,
		// NoArgs states the constraint explicitly: root takes no positional
		// arguments. Cobra's legacyArgs fallback happens to reject an
		// unrecognized subcommand here too (root has subcommands and no
		// parent), but that fallback's applicability depends on those
		// incidental properties, so this guard does not rely on it.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd, a, nil, nil)
		},
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
