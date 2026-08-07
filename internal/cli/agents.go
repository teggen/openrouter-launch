package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
)

func newAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "List the agents this tool can launch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tAGENT\tSTATUS\tDESCRIPTION")

			for _, spec := range agent.List() {
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
}
