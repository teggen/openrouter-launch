package tui

import (
	"strings"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// filterState is the picker's live filter state: the four persisted filters
// plus the search text, which is deliberately session-only.
type filterState struct {
	search     string
	toolsOnly  bool
	freeOnly   bool
	minContext int
	maxPrice   float64
}

// contextCycle and priceCycle are the values the filters screen's Min context
// and Max price rows step through. 0 means "no constraint" and is the first
// entry, so a full cycle always returns to unfiltered rather than trapping the
// user in a narrow view.
var (
	contextCycle = []int{0, 32_000, 128_000, 200_000, 1_000_000}
	priceCycle   = []float64{0, 1, 5, 15}
)

func filterStateFrom(f config.Filters) filterState {
	return filterState{
		toolsOnly:  f.ToolsOnly,
		freeOnly:   f.FreeOnly,
		minContext: f.MinContext,
		maxPrice:   f.MaxPrice,
	}
}

// persisted returns the part of the state that survives the session. Search
// is excluded by design: a saved search would silently hide most of the
// catalog on the next run with no visible cause.
func (f filterState) persisted() config.Filters {
	return config.Filters{
		ToolsOnly:  f.toolsOnly,
		FreeOnly:   f.freeOnly,
		MinContext: f.minContext,
		MaxPrice:   f.maxPrice,
	}
}

// catalogFilter returns the openrouter filter for everything except search.
// Search is applied separately by Rank, which orders by match quality;
// letting openrouter.Apply also match on Search would put two different
// search semantics in series over the same list.
func (f filterState) catalogFilter() openrouter.Filter {
	return openrouter.Filter{
		ToolsOnly:  f.toolsOnly,
		FreeOnly:   f.freeOnly,
		MinContext: f.minContext,
		MaxPrice:   f.maxPrice,
	}
}

// nextContext returns the cycle entry after cur. A value not on the cycle —
// a hand-edited config, or --min-context with an arbitrary number — advances
// to the next larger entry, so one keystroke never widens a constraint the
// user set deliberately.
func nextContext(cur int) int {
	for _, v := range contextCycle {
		if v > cur {
			return v
		}
	}
	return contextCycle[0]
}

// nextPrice returns the cycle entry after cur, with the same
// never-silently-widen rule as nextContext.
func nextPrice(cur float64) float64 {
	for _, v := range priceCycle {
		if v > cur {
			return v
		}
	}
	return priceCycle[0]
}

// label renders the active filters for the picker's status line. It returns
// "no filters" rather than an empty string so the line never looks broken.
func (f filterState) label() string {
	var parts []string
	if f.toolsOnly {
		parts = append(parts, "tools")
	}
	if f.freeOnly {
		parts = append(parts, "free")
	}
	if f.minContext > 0 {
		parts = append(parts, "ctx≥"+openrouter.FormatContext(f.minContext))
	}
	if f.maxPrice > 0 {
		parts = append(parts, "≤"+openrouter.FormatPrice(f.maxPrice, false)+"/M")
	}
	if len(parts) == 0 {
		return "no filters"
	}
	return strings.Join(parts, " · ")
}
