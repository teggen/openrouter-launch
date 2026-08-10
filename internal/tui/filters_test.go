package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

func TestNextContextCyclesAndWrapsToUnfiltered(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 32_000},
		{32_000, 128_000},
		{128_000, 200_000},
		{200_000, 1_000_000},
		{1_000_000, 0},
	}
	for _, c := range cases {
		if got := nextContext(c.in); got != c.want {
			t.Errorf("nextContext(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// A value that is not on the cycle comes from a hand-edited config or from
// --min-context with an arbitrary number. Advancing to the next larger entry
// means one keystroke can never silently widen a constraint the user set
// deliberately.
func TestNextContextAdvancesPastAnUnlistedValue(t *testing.T) {
	if got := nextContext(50_000); got != 128_000 {
		t.Errorf("nextContext(50000) = %d, want 128000", got)
	}
	if got := nextContext(2_000_000); got != 0 {
		t.Errorf("nextContext(2000000) = %d, want 0 (wrap)", got)
	}
}

func TestNextPriceCyclesAndWrapsToUnfiltered(t *testing.T) {
	cases := []struct{ in, want float64 }{{0, 1}, {1, 5}, {5, 15}, {15, 0}}
	for _, c := range cases {
		if got := nextPrice(c.in); got != c.want {
			t.Errorf("nextPrice(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNextPriceAdvancesPastAnUnlistedValue(t *testing.T) {
	if got := nextPrice(3); got != 5 {
		t.Errorf("nextPrice(3) = %v, want 5", got)
	}
}

func TestFilterStateRoundTripsThroughConfigButDropsSearch(t *testing.T) {
	in := filterState{
		search: "anthropic", toolsOnly: true, freeOnly: true,
		minContext: 200_000, maxPrice: 5,
	}
	out := filterStateFrom(in.persisted(), in.persistedSort())

	if out.search != "" {
		t.Errorf("search survived persistence as %q; it must not be saved", out.search)
	}
	if out.toolsOnly != true || out.freeOnly != true ||
		out.minContext != 200_000 || out.maxPrice != 5 {
		t.Errorf("round trip lost a persisted filter: %+v", out)
	}
}

// The picker applies the four catalog filters through openrouter.Apply and
// the search through Rank, which orders by match quality. If catalogFilter
// also carried Search, Apply's plain substring match would narrow the list
// before Rank ever saw it, and two different search semantics would compete
// over one list.
func TestCatalogFilterNeverCarriesSearch(t *testing.T) {
	f := filterState{search: "anthropic", toolsOnly: true}
	if got := f.catalogFilter().Search; got != "" {
		t.Errorf("catalogFilter().Search = %q, want empty", got)
	}
	if !f.catalogFilter().ToolsOnly {
		t.Error("catalogFilter() dropped ToolsOnly")
	}
}

func TestLabelNamesEveryActiveFilter(t *testing.T) {
	f := filterState{toolsOnly: true, freeOnly: true, minContext: 200_000, maxPrice: 5}
	got := f.label()
	for _, want := range []string{"tools", "free", "200k", "$5.00"} {
		if !strings.Contains(got, want) {
			t.Errorf("label() = %q, missing %q", got, want)
		}
	}
}

func TestLabelWithNothingActiveIsNotEmpty(t *testing.T) {
	if got := (filterState{}).label(); got != "no filters" {
		t.Errorf("label() = %q, want %q", got, "no filters")
	}
}

func TestLabelOmitsInactiveFilters(t *testing.T) {
	got := filterState{toolsOnly: true}.label()
	if strings.Contains(got, "free") {
		t.Errorf("label() = %q, names an inactive filter", got)
	}
}

func TestFilterStateFromConfigCarriesEveryField(t *testing.T) {
	in := config.Filters{ToolsOnly: true, FreeOnly: true, MinContext: 128_000, MaxPrice: 15}
	got := filterStateFrom(in, config.Sort{})
	if got.toolsOnly != true || got.freeOnly != true ||
		got.minContext != 128_000 || got.maxPrice != 15 {
		t.Errorf("filterStateFrom(%+v) = %+v", in, got)
	}
}

func TestLabelNamesTheSortOnlyWhenOneIsActive(t *testing.T) {
	if got := (filterState{}).label(); got != "no filters" {
		t.Errorf("idle label = %q, want %q — relevance is not worth a status line", got, "no filters")
	}

	// "no filters" has to survive next to the sort, or the line would claim
	// the list is unfiltered only while it is also unsorted.
	got := filterState{sort: openrouter.Sort{Key: openrouter.SortOutput}}.label()
	if !strings.Contains(got, "OUTPUT/M") || !strings.Contains(got, "no filters") {
		t.Errorf("label = %q, want it to keep \"no filters\" and name OUTPUT/M", got)
	}

	desc := filterState{
		toolsOnly: true,
		sort:      openrouter.Sort{Key: openrouter.SortContext, Desc: true},
	}.label()
	if !strings.Contains(desc, "tools") || !strings.Contains(desc, "CONTEXT") ||
		!strings.Contains(desc, "↓") {
		t.Errorf("label = %q, want tools, CONTEXT and a descending arrow", desc)
	}
	asc := filterState{sort: openrouter.Sort{Key: openrouter.SortContext}}.label()
	if !strings.Contains(asc, "↑") {
		t.Errorf("label = %q, want an ascending arrow", asc)
	}
}

func TestNextSortKeyCyclesEveryColumnAndReturnsToRelevance(t *testing.T) {
	var seen []openrouter.SortKey
	k := openrouter.SortNone
	for i := 0; i < len(openrouter.SortKeys)+1; i++ {
		k = nextSortKey(k)
		seen = append(seen, k)
	}
	want := append(append([]openrouter.SortKey{}, openrouter.SortKeys...), openrouter.SortNone)
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("cycle = %v, want %v", seen, want)
	}

	// A value from a hand-edited config is not on the cycle; one press must
	// still land somewhere sane rather than looping on itself.
	if got := nextSortKey(openrouter.SortKey("bogus")); got != openrouter.SortKeys[0] {
		t.Errorf("nextSortKey(bogus) = %q, want %q", got, openrouter.SortKeys[0])
	}
}

func TestSortRoundTripsThroughTheFilterState(t *testing.T) {
	in := config.Sort{Column: "input", Desc: true}
	f := filterStateFrom(config.Filters{}, in)
	if f.sort != (openrouter.Sort{Key: openrouter.SortInput, Desc: true}) {
		t.Fatalf("filterStateFrom lost the sort: %+v", f.sort)
	}
	if got := f.persistedSort(); got != in {
		t.Errorf("persistedSort = %+v, want %+v", got, in)
	}
	if got := filterStateFrom(config.Filters{}, config.Sort{Column: "prompt"}).sort; got !=
		(openrouter.Sort{}) {
		t.Errorf("an unknown persisted column became %+v, want relevance", got)
	}
}
