package openrouter

import (
	"strings"

	"github.com/teggen/agentlaunch/catalog"
)

// Filter narrows the catalog. Zero values mean "no constraint".
type Filter struct {
	// Search matches case-insensitively against ID and Name.
	Search string
	// ToolsOnly keeps only models supporting tool calling.
	ToolsOnly bool
	// FreeOnly keeps only zero-cost models.
	FreeOnly bool
	// Provider matches Model.Provider case-insensitively.
	Provider string
	// MinContext is the minimum context window in tokens.
	MinContext int
	// MaxPrice is the ceiling on USD per million completion tokens.
	MaxPrice float64
}

// Apply returns the models matching every set constraint, preserving order.
func Apply(models []catalog.Model, f Filter) []catalog.Model {
	provider := strings.ToLower(f.Provider)

	out := make([]catalog.Model, 0, len(models))
	for _, m := range models {
		if f.ToolsOnly && !m.SupportsTools {
			continue
		}
		if f.FreeOnly && !m.IsFree() {
			continue
		}
		if provider != "" && strings.ToLower(m.Provider) != provider {
			continue
		}
		if f.MinContext > 0 && m.ContextLength < f.MinContext {
			continue
		}
		// Unknown pricing cannot be confirmed under the ceiling, so exclude it.
		if f.MaxPrice > 0 && (m.PricingUnknown || m.CompletionPricePerM > f.MaxPrice) {
			continue
		}
		if f.Search != "" && !m.Matches(f.Search) {
			continue
		}
		out = append(out, m)
	}
	return out
}
