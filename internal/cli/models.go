package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/ui"
)

func newModelsCmd(a *app) *cobra.Command {
	var flagFilter openrouter.Filter

	cmd := &cobra.Command{
		Use:   "models [search]",
		Short: "List OpenRouter models",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// Persisted filters are the baseline; a flag the user actually
			// typed wins. Changed() is what makes an explicit --tools=false
			// distinguishable from an absent --tools.
			filter := launch.MergeFilters(cfg.Filters, flagFilter, cmd.Flags().Changed)
			if len(args) == 1 {
				filter.Search = args[0]
			}

			snap, err := a.svc.Snapshot(cmd.Context(), a.flags.refresh)
			if err != nil {
				return err
			}
			if w, ok := launch.StaleWarning(snap, time.Now()); ok {
				renderWarning(cmd, w)
			}

			models := openrouter.Apply(snap.Models, filter)
			if len(models) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No models match those filters.")
				return nil
			}

			out := cmd.OutOrStdout()
			_, err = fmt.Fprintln(out, ui.NewTheme(out).Render(modelsTable(models)))
			return err
		},
	}

	cmd.Flags().BoolVar(&flagFilter.ToolsOnly, launch.FlagTools, false,
		"only models supporting tool calling (defaults to the saved filter)")
	cmd.Flags().BoolVar(&flagFilter.FreeOnly, launch.FlagFree, false, "only free models")
	cmd.Flags().StringVar(&flagFilter.Provider, "provider", "",
		"only models from this provider (e.g. anthropic)")
	cmd.Flags().IntVar(&flagFilter.MinContext, launch.FlagMinContext, 0,
		"minimum context window in tokens")
	cmd.Flags().Float64Var(&flagFilter.MaxPrice, launch.FlagMaxPrice, 0,
		"maximum USD per million completion tokens")

	return cmd
}

// modelsTable builds the catalog listing.
//
// Deliberately uncapped, unlike the other listings: its widest cell is a
// 50-character model id, already comfortably under ui.MaxTableWidth, and
// truncating a model slug would make it impossible to copy-paste into -m.
func modelsTable(models []openrouter.Model) ui.Table {
	rows := make([][]string, 0, len(models))
	for _, m := range models {
		tools := ""
		if m.SupportsTools {
			tools = "✓"
		}
		rows = append(rows, []string{
			m.ID,
			openrouter.FormatContext(m.ContextLength),
			// Landmine 4: unknown pricing is never free. FormatPrice
			// renders it as "?" when PricingUnknown is set, so dropping
			// that argument would claim a model costs nothing.
			openrouter.FormatPrice(m.PromptPricePerM, m.PricingUnknown),
			openrouter.FormatPrice(m.CompletionPricePerM, m.PricingUnknown),
			tools,
		})
	}

	return ui.Table{
		Headers: []string{"MODEL", "CONTEXT", "PROMPT/M", "COMPLETION/M", "TOOLS"},
		Rows:    rows,
		Role: func(_, col int) ui.Role {
			switch col {
			case 0:
				return ui.RoleAccent
			case 4:
				return ui.RoleOK
			default:
				return ui.RolePlain
			}
		},
	}
}
