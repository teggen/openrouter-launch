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
	var flagSort string
	var flagDesc bool

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

			// A typo on the command line is fatal, unlike the same value in
			// the config (launch.SortFrom degrades that one): the user is
			// standing right here, and printing catalog order would look like
			// the sort was applied.
			key, err := openrouter.ParseSortKey(flagSort)
			if err != nil {
				return err
			}
			sortBy := launch.MergeSort(cfg.Sort,
				openrouter.Sort{Key: key, Desc: flagDesc}, cmd.Flags().Changed)

			// --desc alone cannot do anything: SortModels returns an
			// unchanged copy for SortNone whatever Desc says. Same call as
			// the ParseSortKey typo above — the user is standing right here,
			// and printing catalog order would look like the flag applied.
			// Checked against the MERGED key, so a saved column still counts.
			// The trailing sortBy.Desc matters on its own: Changed(FlagDesc)
			// is true for the explicit --desc=false form too, and a user
			// turning descending OFF is not asking for anything a sort
			// column could satisfy — only a merged Desc of true is a request
			// this guard needs to reject.
			if sortBy.Key == openrouter.SortNone && cmd.Flags().Changed(launch.FlagDesc) && sortBy.Desc {
				return fmt.Errorf("--%s needs a sort column: add --%s (model, context, input, output, tools)",
					launch.FlagDesc, launch.FlagSort)
			}

			snap, err := a.svc.Snapshot(cmd.Context(), a.flags.refresh)
			if err != nil {
				return err
			}
			if w, ok := launch.StaleWarning(snap, time.Now()); ok {
				renderWarning(cmd, w)
			}

			models := openrouter.SortModels(openrouter.Apply(snap.Models, filter), sortBy)
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
		"maximum USD per million output tokens")
	cmd.Flags().StringVar(&flagSort, launch.FlagSort, "",
		"sort by column: model, context, input, output, tools")
	cmd.Flags().BoolVar(&flagDesc, launch.FlagDesc, false,
		"reverse the sort (largest, priciest, or Z-A first)")

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
		rows = append(rows, ui.ModelCells(m))
	}

	return ui.Table{
		Headers: ui.ModelHeaders,
		Rows:    rows,
		Role:    func(_, col int) ui.Role { return ui.ModelRole(col) },
	}
}
