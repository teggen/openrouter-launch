// Package cli wires the openrouter-launch commands together.
package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/tui"
	"github.com/teggen/openrouter-launch/internal/version"
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

// app is what every subcommand needs: the shared launch service, the agent
// registry the tree is built from, and the global flag values.
type app struct {
	svc     *launch.Service
	reg     *agent.Registry
	flags   *globalFlags
	openTUI tuiFunc
}

// openRouterRegistry binds the built-in agents to OpenRouter. This is the
// composition root: it is the one place in the tree that names a provider,
// which is what a second tool built on the same agents would replace.
//
// MustRegistry is right here for the same reason NewRootCmdWith panics on a
// nil Service — a malformed registry is a programmer error in a literal, and
// the whole binary is unusable, so failing at construction beats failing at
// the first launch. agent.NewRegistry returns an error instead, because a
// library must not panic on a caller's slice.
func openRouterRegistry() *agent.Registry {
	return agent.MustRegistry(agent.Binding{
		Provider: agent.OpenRouter,
		Host:     agent.OpenRouterHost,
	}, agent.Builtins())
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
	return newRootCmd(svc, openRouterRegistry(), tui.Run)
}

func newRootCmd(svc *launch.Service, reg *agent.Registry, openTUI tuiFunc) *cobra.Command {
	if svc == nil {
		// A nil Service would build a full command tree that panics later,
		// on first use, when Snapshot dereferences a nil receiver. Fail at
		// construction instead, naming the problem, matching the idiom in
		// agent.MustRegistry for a malformed registry.
		panic("cli: NewRootCmdWith requires a non-nil *launch.Service")
	}
	if reg == nil {
		// Same reasoning as the nil Service above: newLaunchCmds dereferences
		// the registry while the tree is still being built, so a nil one
		// panics anyway — obscurely, inside the agent package.
		panic("cli: newRootCmd requires a non-nil *agent.Registry")
	}
	a := &app{svc: svc, reg: reg, flags: &globalFlags{}, openTUI: openTUI}

	root := &cobra.Command{
		Use:   "openrouter-launch",
		Short: "Launch coding agents against OpenRouter models",
		// Setting Version is what makes cobra synthesise --version.
		Version: version.String(),
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

	root.AddCommand(newAgentsCmd(a))
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
