package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/ui"
)

func newProfileCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage saved agent + model profiles",
	}
	cmd.AddCommand(
		newProfileListCmd(),
		newProfileAddCmd(),
		newProfileLaunchCmd(a),
		newProfileRemoveCmd(),
		newProfileRenameCmd(),
	)
	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(out, "No profiles saved. Add one with: openrouter-launch profile add --name <n> --agent <a> --model <slug>")
				return nil
			}
			_, err = fmt.Fprintln(out,
				ui.NewTheme(out).Render(profilesTable(cfg.Profiles, agent.Lookup, agent.Installed)))
			return err
		},
	}
}

// profilesTable builds the listing.
//
// lookup and installed are injected for the same reason agentsTable takes
// them: a profile naming an unregistered agent cannot be created through
// the CLI — profile add validates the name — so a test can only reach that
// row by supplying its own lookup. Without the STATUS column that failure
// stays invisible until launch time.
func profilesTable(
	profiles []config.Profile,
	lookup func(string) (*agent.Spec, error),
	installed func(*agent.Spec) bool,
) ui.Table {
	var (
		rows  [][]string
		roles []ui.Role
	)
	for _, p := range profiles {
		status, role := ui.UnknownAgentStatus()
		if spec, err := lookup(p.Agent); err == nil {
			status, role = ui.AgentStatus(spec, installed(spec))
		}
		rows = append(rows, []string{p.Name, p.Agent, status, p.Model, strings.Join(p.Args, " ")})
		roles = append(roles, role)
	}

	return ui.Table{
		Headers:  []string{"NAME", "AGENT", "STATUS", "MODEL", "ARGS"},
		Rows:     rows,
		MaxWidth: ui.MaxTableWidth,
		Role: func(row, col int) ui.Role {
			if row < 0 || row >= len(roles) {
				return ui.RolePlain
			}
			switch col {
			case 0:
				return ui.RoleAccent
			case 2:
				return roles[row]
			case 4:
				return ui.RoleDim
			default:
				return ui.RolePlain
			}
		},
	}
}

func newProfileAddCmd() *cobra.Command {
	var name, agentName, model string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Save a new profile",
		Long:  "Save a named agent + model combination. Arguments after -- are stored and passed to the agent on launch.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := agent.Lookup(agentName)
			if err != nil {
				return err
			}
			if err := launch.CheckSupported(spec); err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.AddProfile(config.Profile{
				Name:  name,
				Agent: spec.Name,
				Model: model,
				Args:  args,
			}); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Saved profile %q: %s with %s\n", name, spec.Name, model)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "profile name (required)")
	cmd.Flags().StringVar(&agentName, "agent", "", "agent to launch (required)")
	cmd.Flags().StringVar(&model, "model", "", "OpenRouter model slug (required)")
	// MarkFlagRequired's only failure is "no such flag" (pflag's
	// SetAnnotation), i.e. one of these literals drifting from the StringVar
	// names three lines above. Discarding that error is not harmless: the
	// flag would silently become optional and `profile add` would write a
	// profile with an empty name/agent/model. No test covers a missing
	// required flag, so nothing else would catch the typo. Panicking matches
	// Landmine 10's precedent for construction-time wiring errors — this
	// runs inside NewRootCmdWith, so a typo fails the binary and every test
	// at construction rather than one subcommand at runtime.
	for _, flag := range []string{"name", "agent", "model"} {
		if err := cmd.MarkFlagRequired(flag); err != nil {
			panic(fmt.Sprintf("profile add: mark %q required: %v", flag, err))
		}
	}

	return cmd
}

func newProfileLaunchCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "launch <name>",
		Short: "Launch a saved profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			profile, ok := cfg.Profile(args[0])
			if !ok {
				return fmt.Errorf("%w: %s", config.ErrProfileNotFound, args[0])
			}
			spec, err := agent.Lookup(profile.Agent)
			if err != nil {
				return err
			}
			return resolveAndRun(cmd, a, spec, profile.Model, profile.Args)
		},
	}
}

func newProfileRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rm <name>",
		Aliases: []string{"remove", "delete"},
		Short:   "Delete a saved profile",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RemoveProfile(args[0]); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted profile %q\n", args[0])
			return nil
		},
	}
}

func newProfileRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a saved profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RenameProfile(args[0], args[1]); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Renamed %q to %q\n", args[0], args[1])
			return nil
		},
	}
}
