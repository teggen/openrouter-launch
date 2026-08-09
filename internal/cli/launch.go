package cli

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/tui"
)

// newLaunchCmds builds one subcommand per registered agent.
func newLaunchCmds(a *app) []*cobra.Command {
	specs := agent.List()
	cmds := make([]*cobra.Command, 0, len(specs))

	for _, spec := range specs {
		spec := spec
		var modelID string

		cmd := &cobra.Command{
			Use:     spec.Name,
			Short:   "Launch " + spec.Launcher.DisplayName(),
			Aliases: spec.Aliases,
			Args:    cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return resolveAndRun(cmd, a, spec, modelID, args)
			},
		}
		cmd.Flags().StringVarP(&modelID, "model", "m", "", "OpenRouter model slug (required)")
		cmds = append(cmds, cmd)
	}
	return cmds
}

// resolveAndRun plans the launch, renders whatever the plan reports, and
// hands off. All of the decision-making lives in launch.Service; this
// function is the cobra-flavored rendering of it.
func resolveAndRun(cmd *cobra.Command, a *app, spec *agent.Spec, modelID string, extraArgs []string) error {
	plan, err := a.svc.Plan(cmd.Context(), launch.Request{
		Spec:      spec,
		ModelID:   modelID,
		ExtraArgs: extraArgs,
		Refresh:   a.flags.refresh,
	})

	// Printed before the error is inspected: the planner returns whatever it
	// had accumulated when a guard failed, and a stale catalog is often the
	// explanation for that failure.
	for _, w := range plan.Warnings {
		renderWarning(cmd, w)
	}

	if errors.Is(err, launch.ErrNoModel) {
		// No model named, so open the picker. ErrNoModel is returned by
		// Plan's third guard, before the catalog is loaded, so plan.Warnings
		// is always empty here and nothing is rendered twice.
		return runTUI(cmd, a, spec, extraArgs)
	}
	if err != nil {
		return err
	}

	// Confirmation is a second pass: there is nothing to confirm on a path
	// that is about to fail, and printing every warning before asking gives
	// the user the full picture at the prompt.
	for _, w := range plan.Warnings {
		if w.Question == "" {
			continue
		}
		ok, cerr := confirm(cmd, a.flags, w.Question)
		if cerr != nil {
			return cerr
		}
		if !ok {
			return errors.New("cancelled")
		}
	}

	return handoff(cmd, a, plan)
}

// runTUI opens the interactive session and launches whatever it approves.
//
// tui.Run has already returned by the time handoff runs, so every bubbletea
// program has torn down and the terminal is out of raw mode before
// syscall.Exec replaces the process.
func runTUI(cmd *cobra.Command, a *app, spec *agent.Spec, extraArgs []string) error {
	plan, err := a.openTUI(cmd.Context(), tui.Options{
		Service:   a.svc,
		Agent:     spec,
		ExtraArgs: extraArgs,
		Refresh:   a.flags.refresh,
		AssumeYes: a.flags.yes,
	})
	if errors.Is(err, tui.ErrCancelled) {
		// Backing out of the picker is not a failure.
		return nil
	}
	if err != nil {
		return err
	}

	// Rendered here as well as on the confirm screen: the picker runs in the
	// alt screen, so everything it drew is gone once that tears down, and
	// this line is the only lasting trace in scrollback.
	for _, w := range plan.Warnings {
		renderWarning(cmd, w)
	}
	return handoff(cmd, a, plan)
}

// handoff is the tail both launch paths share.
func handoff(cmd *cobra.Command, a *app, plan launch.Plan) error {
	if err := a.svc.Launch(plan, func(w launch.Warning) {
		renderWarning(cmd, w)
	}); err != nil {
		if isAgentExitError(err) {
			// On Windows, agent.Run waits for the child instead of replacing
			// the process, so a nonzero exit arrives here wrapping an
			// *exec.ExitError. The agent already inherited stderr and
			// reported its own failure, so cobra's "Error: ..." line would be
			// redundant noise; main still receives the error to extract the
			// exit code from.
			cmd.SilenceErrors = true
		}
		return err
	}
	return nil
}

// renderWarning prints a diagnostic through cobra's IO, so it honors the
// same redirection as every other CLI message.
func renderWarning(cmd *cobra.Command, w launch.Warning) {
	fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
}

// isAgentExitError reports whether err carries the launched agent's own
// exit code, i.e. it wraps a value with an ExitCode() int method (the
// structural shape of *exec.ExitError). This used to be always false on
// Unix, because agent.Run's syscall.Exec only ever errors when it fails to
// start the process. That is no longer the whole story: ConfigWriter agents
// take the fork-and-wait path (agent.RunWait), which waits on the child
// instead of replacing the process, so a nonzero exit reaches here wrapping
// a real *exec.ExitError on Unix too.
func isAgentExitError(err error) bool {
	var ec interface{ ExitCode() int }
	return errors.As(err, &ec)
}

// confirm asks a yes/no question, defaulting to no. --yes answers yes.
func confirm(cmd *cobra.Command, global *globalFlags, question string) (bool, error) {
	if global.yes {
		return true, nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", question)
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}

	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
