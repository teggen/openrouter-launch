package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
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
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
			}

			models := openrouter.Apply(snap.Models, filter)
			if len(models) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No models match those filters.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "MODEL\tCONTEXT\tPROMPT/M\tCOMPLETION/M\tTOOLS")
			for _, m := range models {
				tools := ""
				if m.SupportsTools {
					tools = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					m.ID, formatContext(m.ContextLength),
					formatPrice(m.PromptPricePerM, m.PricingUnknown),
					formatPrice(m.CompletionPricePerM, m.PricingUnknown), tools)
			}
			return w.Flush()
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

// formatPrice renders a USD-per-million-tokens price for display. Unknown
// pricing renders as "?" so it is never mistaken for free.
func formatPrice(usdPerM float64, unknown bool) string {
	if unknown {
		return "?"
	}
	if usdPerM == 0 {
		return "free"
	}
	return fmt.Sprintf("$%.2f", usdPerM)
}

// formatContext renders a context window in thousands of tokens.
func formatContext(tokens int) string {
	if tokens <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dk", tokens/1000)
}
