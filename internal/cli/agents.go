package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/teggen/agentlaunch/agent"
	"github.com/teggen/openrouter-launch/internal/ui"
)

func newAgentsCmd(a *app) *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "agents",
		Short: "List the agents this tool can launch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			_, err := fmt.Fprintln(out,
				ui.NewTheme(out).Render(agentsTable(a.reg.List(), a.reg.Installed, all)))
			return err
		},
	}

	cmd.Flags().BoolVar(&all, "all", false,
		"include agents that cannot be pointed at OpenRouter, with the reason")
	return cmd
}

// agentsTable builds the listing.
//
// It takes the specs and an installed-ness probe rather than reaching for the
// registry itself, so a test can render an adversarial spec — a 200-character
// description no real agent has — and watch the width cap do its job. Without
// that seam TestAgentsOutputStaysNarrow can only ever measure whatever the
// registry happens to contain, which sits comfortably under the cap and would
// leave the test unable to fail.
func agentsTable(specs []*agent.Spec, installed func(*agent.Spec) bool, all bool) ui.Table {
	// The last column carries the reason for an unsupported agent and the
	// description for everyone else, rather than the two living in separate
	// columns.
	//
	// Measured, not preferred: a fifth column leaves each of REASON and
	// DESCRIPTION about 20 columns once NAME, AGENT, and STATUS have taken
	// their share of the 100-column cap, and lipgloss hard-breaks a word
	// that cannot fit — the reason came out as "desktop app authentic /
	// ates through its own account". Folding them into one column gives it
	// ~40 and word wrapping works. Nothing is lost: an unsupported agent's
	// description ("OpenAI's desktop app") only repeats its AGENT cell,
	// while the reason is the entire point of --all.
	lastHeader := "DESCRIPTION"
	if all {
		lastHeader = "DESCRIPTION / REASON"
	}
	headers := []string{"NAME", "AGENT", "STATUS", lastHeader}

	var (
		rows  [][]string
		roles []ui.Role
	)
	for _, spec := range specs {
		// Unsupported agents (the Tier 3 desktop apps) are hidden by
		// default — a deliberate Phase 4a decision, pinned by
		// TestAgentsHidesUnsupportedByDefault and its --all counterpart.
		// They stay registered, and `openrouter-launch <agent>` still
		// reports the reason; this hides them from the listing, it does not
		// un-register them.
		if !spec.Status.Supported && !all {
			continue
		}
		status, role := ui.AgentStatus(spec, installed(spec))

		last := spec.Description
		if !spec.Status.Supported {
			last = spec.Status.Reason
		}
		rows = append(rows, []string{spec.Name, spec.Launcher.DisplayName(), status, last})
		roles = append(roles, role)
	}

	const statusCol = 2
	return ui.Table{
		Headers:  headers,
		Rows:     rows,
		MaxWidth: ui.MaxTableWidth,
		Role: func(row, col int) ui.Role {
			if row < 0 || row >= len(roles) {
				return ui.RolePlain
			}
			switch col {
			case 0:
				return ui.RoleAccent
			case 1:
				return ui.RolePlain
			case statusCol:
				return roles[row]
			default: // the description / reason column
				return ui.RoleDim
			}
		},
	}
}
