package cli

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// runner performs the process handoff. Tests replace it to capture commands.
var runner = agent.Run

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

// checkAgentSupported reports why an agent cannot be pointed at OpenRouter.
func checkAgentSupported(spec *agent.Spec) error {
	if !spec.Status.Supported {
		return fmt.Errorf("%s cannot be pointed at OpenRouter: %s", spec.Name, spec.Status.Reason)
	}
	return nil
}

// resolveAndRun validates the request and hands off to the agent.
func resolveAndRun(cmd *cobra.Command, a *app, spec *agent.Spec, modelID string, extraArgs []string) error {
	if err := checkAgentSupported(spec); err != nil {
		return err
	}

	if platform, ok := spec.Launcher.(agent.PlatformSupported); ok {
		if err := platform.Supported(); err != nil {
			return err
		}
	}

	if modelID == "" {
		return fmt.Errorf("a model is required: pass --model <slug> (run %q to browse; the interactive picker arrives in Phase 2)", "openrouter-launch models")
	}

	if installable, ok := spec.Launcher.(agent.Installable); ok && !installable.CheckInstalled() {
		return fmt.Errorf("%s is not installed.\n%s", spec.Launcher.DisplayName(), installable.InstallHint())
	}

	snap, err := loadCatalog(cmd.Context(), a.svc, a.flags.refresh, cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	model, ok := openrouter.FindByID(snap.Models, modelID)
	if !ok {
		suggestions := openrouter.Suggest(snap.Models, modelID, 5)
		if len(suggestions) == 0 {
			return fmt.Errorf("unknown model %q", modelID)
		}
		return fmt.Errorf("unknown model %q. Did you mean:\n  %s",
			modelID, strings.Join(suggestions, "\n  "))
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	apiKey, err := config.ResolveAPIKey(cfg)
	if err != nil {
		return err
	}

	if compatible, ok := spec.Launcher.(agent.Compatible); ok {
		if err := compatible.CheckModel(model); err != nil {
			if !errors.Is(err, agent.ErrIncompatibleModel) {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
			ok, cerr := confirm(cmd, a.flags, "Launch anyway?")
			if cerr != nil {
				return cerr
			}
			if !ok {
				return errors.New("cancelled")
			}
		}
	}

	command, err := spec.Launcher.Command(agent.Request{
		Model:     model,
		APIKey:    apiKey,
		ExtraArgs: extraArgs,
	})
	if err != nil {
		return err
	}

	// Recorded before handing off, because on Unix the process is replaced
	// and nothing after runner() executes.
	cfg.LastAgent = spec.Name
	cfg.LastModel = model.ID
	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not save last selection: %v\n", err)
	}

	if err := runner(command); err != nil {
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
