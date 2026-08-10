package tui

import (
	"strings"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/ui"
)

// filterState is the picker's live filter state: the four persisted filters
// plus the search text, which is deliberately session-only.
type filterState struct {
	search     string
	toolsOnly  bool
	freeOnly   bool
	minContext int
	maxPrice   float64
	// sort orders the visible list. The zero value is "relevance", which is
	// the ordering the picker had before sorting existed: catalog order, or
	// best-match-first while the search box has text. Unlike search — and like
	// the four filters — it persists.
	sort openrouter.Sort
}

// contextCycle and priceCycle are the values the filters screen's Min context
// and Max price rows step through. 0 means "no constraint" and is the first
// entry, so a full cycle always returns to unfiltered rather than trapping the
// user in a narrow view.
var (
	contextCycle = []int{0, 32_000, 128_000, 200_000, 1_000_000}
	priceCycle   = []float64{0, 1, 5, 15}
)

func filterStateFrom(f config.Filters, s config.Sort) filterState {
	return filterState{
		toolsOnly:  f.ToolsOnly,
		freeOnly:   f.FreeOnly,
		minContext: f.MinContext,
		maxPrice:   f.MaxPrice,
		// launch.SortFrom, not a local parse: an unrecognised persisted column
		// must degrade to relevance in exactly one place, shared with the CLI.
		sort: launch.SortFrom(s),
	}
}

// persistedSort is the sort's persisted form. Unlike the search box the sort
// survives the session: a user who prefers cheapest-first should not have to
// say so on every run.
func (f filterState) persistedSort() config.Sort {
	return config.Sort{Column: string(f.sort.Key), Desc: f.sort.Desc}
}

// nextSortKey advances the filter&sort screen's "Sort by" row: relevance, then
// each catalog column in header order, then back to relevance.
//
// Unlike nextContext and nextPrice there is no never-silently-widen rule to
// honour — columns have no ordering — so a value that is not on the cycle (a
// hand-edited config) lands on the first column rather than on the next
// larger one.
func nextSortKey(cur openrouter.SortKey) openrouter.SortKey {
	for i, k := range openrouter.SortKeys {
		if k == cur {
			if i+1 < len(openrouter.SortKeys) {
				return openrouter.SortKeys[i+1]
			}
			return openrouter.SortNone
		}
	}
	return openrouter.SortKeys[0]
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
	label := "no filters"
	if len(parts) > 0 {
		label = strings.Join(parts, " · ")
	}
	// The sort is appended rather than folded into parts: it is not a filter,
	// and "no filters" has to survive next to it — otherwise the line would
	// claim the list is unfiltered only while it is also unsorted.
	if f.sort.Key != openrouter.SortNone {
		arrow := "↑"
		if f.sort.Desc {
			arrow = "↓"
		}
		label += " · sort:" + ui.SortLabel(f.sort.Key) + " " + arrow
	}
	return label
}
