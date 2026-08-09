package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
)

func newAgentsCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "agents",
		Short: "List the agents this tool can launch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tAGENT\tSTATUS\tDESCRIPTION")

			for _, spec := range agent.List() {
				// Unsupported agents (the Tier 3 desktop apps) are hidden by
				// default. Not for tidiness: tabwriter pads every column to
				// its widest cell, so their ~99-character reason widened the
				// STATUS column of all 14 rows and pushed the table to 227
				// columns. They stay registered, and `openrouter-launch
				// <agent>` still reports the reason — this hides them from
				// the listing, it does not un-register them.
				if !spec.Status.Supported && !all {
					continue
				}
				status := "not installed"
				switch {
				case !spec.Status.Supported:
					status = "unsupported: " + spec.Status.Reason
				case agent.Installed(spec):
					status = "installed"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					spec.Name, spec.Launcher.DisplayName(), status, spec.Description)
			}
			return w.Flush()
		},
	}

	cmd.Flags().BoolVar(&all, "all", false,
		"include agents that cannot be pointed at OpenRouter, with the reason")
	return cmd
}
