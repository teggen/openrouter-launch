package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/launch"
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
			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No profiles saved. Add one with: openrouter-launch profile add --name <n> --agent <a> --model <slug>")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tAGENT\tMODEL\tARGS")
			for _, p := range cfg.Profiles {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Agent, p.Model, strings.Join(p.Args, " "))
			}
			return w.Flush()
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
	cmd.MarkFlagRequired("name")
	cmd.MarkFlagRequired("agent")
	cmd.MarkFlagRequired("model")

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
