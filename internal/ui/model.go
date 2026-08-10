package ui

import "github.com/teggen/openrouter-launch/internal/openrouter"

// ModelHeaders are the catalog columns. Shared by `orl models` and the TUI
// picker for the same reason AgentStatus is shared: two renderings of the
// same data that drift are worse than one that is slightly less convenient
// to build.
//
// INPUT/OUTPUT rather than PROMPT/COMPLETION: those are OpenRouter's wire
// names (pricing.prompt / pricing.completion) and they stay on the Model
// fields, but the columns say what a user pays for.
var ModelHeaders = []string{"MODEL", "CONTEXT", "INPUT/M", "OUTPUT/M", "TOOLS"}

// SortLabel is the display name of a sort key: the table's own header for a
// column, "relevance" for SortNone and for anything unrecognised.
//
// Positional by design — openrouter.SortKeys is declared in ModelHeaders order
// and TestSortLabelMatchesTheTable pins that, so a renamed or reordered column
// cannot leave the filter&sort screen naming a different one.
func SortLabel(k openrouter.SortKey) string {
	for i, key := range openrouter.SortKeys {
		if key == k && i < len(ModelHeaders) {
			return ModelHeaders[i]
		}
	}
	return "relevance"
}

// ModelCells renders one catalog row, in ModelHeaders' order.
func ModelCells(m openrouter.Model) []string {
	tools := ""
	if m.SupportsTools {
		tools = glyphOK
	}
	return []string{
		m.ID,
		openrouter.FormatContext(m.ContextLength),
		// Landmine 4: unknown pricing is never free. FormatPrice renders
		// "?" when PricingUnknown is set, so dropping that argument would
		// be an actively wrong claim about what a launch costs.
		openrouter.FormatPrice(m.PromptPricePerM, m.PricingUnknown),
		openrouter.FormatPrice(m.CompletionPricePerM, m.PricingUnknown),
		tools,
	}
}

// ModelRole is the role of catalog column col, so both surfaces colour the
// catalog identically.
func ModelRole(col int) Role {
	switch col {
	case 0:
		return RoleAccent
	case 4:
		return RoleOK
	default:
		return RolePlain
	}
}
