package openrouter

import "strings"

// Filter narrows the catalog. Zero values mean "no constraint".
type Filter struct {
	// Search matches case-insensitively against ID and Name.
	Search string
	// ToolsOnly keeps only models supporting tool calling.
	ToolsOnly bool
	// FreeOnly keeps only zero-cost models.
	FreeOnly bool
	// Provider matches the slug prefix before "/".
	Provider string
	// MinContext is the minimum context window in tokens.
	MinContext int
	// MaxPrice is the ceiling on USD per million completion tokens.
	MaxPrice float64
}

// Apply returns the models matching every set constraint, preserving order.
func Apply(models []Model, f Filter) []Model {
	search := strings.ToLower(f.Search)
	provider := strings.ToLower(f.Provider)

	out := make([]Model, 0, len(models))
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
		if search != "" && !matches(m, search) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func matches(m Model, lowerSearch string) bool {
	return strings.Contains(strings.ToLower(m.ID), lowerSearch) ||
		strings.Contains(strings.ToLower(m.Name), lowerSearch)
}

// FindByID returns the model with exactly this ID.
func FindByID(models []Model, id string) (Model, bool) {
	for _, m := range models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// Suggest returns up to limit model IDs loosely matching query, for use in
// "did you mean" messages after an unknown slug.
func Suggest(models []Model, query string, limit int) []string {
	lower := strings.ToLower(query)
	out := make([]string, 0, limit)
	for _, m := range models {
		if len(out) >= limit {
			break
		}
		if lower == "" || matches(m, lower) {
			out = append(out, m.ID)
		}
	}
	return out
}
