package cli

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/launch"
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
		// Phase 2 replaces this branch with the interactive picker. The
		// planner reports the bare condition; naming a CLI flag is this
		// layer's job.
		return fmt.Errorf("a model is required: pass --model <slug> (run %q to browse; "+
			"the interactive picker arrives in Phase 2)", "openrouter-launch models")
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

	if err := a.svc.Launch(plan, func(w launch.Warning) {
		renderWarning(cmd, w)
	}); err != nil {
		if isAgentExitError(err) {
			// On Windows, agent.Run waits for the child instead of replacing
			// the process (exec_windows.go), so a nonzero exit reaches here
			// as an error wrapping *exec.ExitError. The agent already
			// inherited stderr and reported its own failure, so cobra's
			// default "Error: ..." line would just be redundant noise on
			// top; main still receives the real error to extract the exit
			// code from (see exitCode in main.go).
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
// structural shape of *exec.ExitError). On Unix, agent.Run only returns an
// error when syscall.Exec itself fails to start the process, so this is
// always false there.
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
