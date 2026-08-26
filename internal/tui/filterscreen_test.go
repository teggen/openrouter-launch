package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teggen/openrouter-launch/internal/catalog/catalogtest"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

func filterScreenFixture() filterScreenModel {
	return filterScreenFixtureWith(filterState{})
}

func filterScreenFixtureWith(f filterState) filterScreenModel {
	return newFilterScreenModel(filterScreenInput{
		Filters: f,
		Models:  catalogtest.Models(),
		Width:   100,
		Height:  24,
	})
}

// down presses the down arrow n times, to land the cursor on row n.
func down(n int) []tea.KeyMsg {
	keys := make([]tea.KeyMsg, n)
	for i := range keys {
		keys[i] = typeKey(tea.KeyDown)
	}
	return keys
}

// Every row, from the same zero state, so a cycle func wired to the wrong
// field shows up as the wrong filterState rather than merely "something
// changed". Asserting the WHOLE state is the point: a Min context row that
// flipped toolsOnly would still look busy under a field-by-field check of
// only the field it was supposed to touch.
func TestFilterScreenSpaceChangesExactlyItsOwnRow(t *testing.T) {
	cases := []struct {
		name string
		row  int
		want filterState
	}{
		{"tools", 0, filterState{toolsOnly: true}},
		{"free", 1, filterState{freeOnly: true}},
		{"min context", 2, filterState{minContext: 32_000}},
		{"max price", 3, filterState{maxPrice: 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := press(t, filterScreenFixture(), down(tc.row)...)
			m = press(t, m, typeKey(tea.KeySpace))

			if m.filters != tc.want {
				t.Errorf("filters = %+v, want %+v", m.filters, tc.want)
			}
		})
	}
}

// The two numeric rows cycle rather than toggle, and the cycle is the one
// nextContext/nextPrice define — including its never-silently-widen rule.
func TestFilterScreenSpaceCyclesTheNumericRows(t *testing.T) {
	m := press(t, filterScreenFixture(), down(2)...)
	m = press(t, m, typeKey(tea.KeySpace), typeKey(tea.KeySpace))
	if m.filters.minContext != 128_000 {
		t.Errorf("minContext = %d after two presses, want 128000", m.filters.minContext)
	}

	m = press(t, filterScreenFixture(), down(3)...)
	m = press(t, m, typeKey(tea.KeySpace), typeKey(tea.KeySpace))
	if m.filters.maxPrice != 5 {
		t.Errorf("maxPrice = %v after two presses, want 5", m.filters.maxPrice)
	}
}

func TestFilterScreenCursorStopsAtBothEnds(t *testing.T) {
	m := press(t, filterScreenFixture(), typeKey(tea.KeyUp), typeKey(tea.KeyUp))
	if m.cursor != 0 {
		t.Errorf("cursor = %d after up past the top, want 0", m.cursor)
	}

	m = press(t, m, down(len(filterRows)+2)...)
	if m.cursor != len(filterRows)-1 {
		t.Errorf("cursor = %d after down past the bottom, want %d", m.cursor, len(filterRows)-1)
	}
}

func TestFilterScreenEnterAppliesTheEditedFilters(t *testing.T) {
	m := press(t, filterScreenFixture(), typeKey(tea.KeySpace), typeKey(tea.KeyEnter))

	if !m.done || !m.choice.Applied {
		t.Fatalf("done=%v applied=%v, want an applied choice", m.done, m.choice.Applied)
	}
	if !m.choice.Filters.toolsOnly {
		t.Error("enter returned filters without the edit")
	}
}

// Cancel must restore, not merely decline to mark Applied: the driver would
// still be free to read Filters, and a screen that returned its live edits
// there makes "cancel" a lie the moment anyone does.
func TestFilterScreenEscRestoresTheFiltersItOpenedWith(t *testing.T) {
	opened := filterState{freeOnly: true}

	m := press(t, filterScreenFixtureWith(opened), typeKey(tea.KeySpace)) // tools on
	if !m.filters.toolsOnly {
		t.Fatal("the edit under test did not happen")
	}
	m = send(t, m, realEsc()...)

	if !m.done || m.choice.Applied {
		t.Fatalf("done=%v applied=%v, want an unapplied choice", m.done, m.choice.Applied)
	}
	if m.choice.Filters != opened {
		t.Errorf("cancel returned %+v, want the filters it opened with, %+v",
			m.choice.Filters, opened)
	}
}

func TestFilterScreenCtrlCCancelsTheSession(t *testing.T) {
	m := press(t, filterScreenFixture(), typeKey(tea.KeyCtrlC))
	if !m.choice.Cancelled {
		t.Error("ctrl+c did not mark the choice cancelled")
	}
}

// The same split-chord defect the picker has, on the screen where esc throws
// away edits — so a stray chord here costs more than a stray letter.
func TestFilterScreenEscFollowedByARuneSwallowsBothHalvesOfTheChord(t *testing.T) {
	m := send(t, filterScreenFixture(), typeKey(tea.KeyEsc), runeKey('t'))
	if m.done {
		t.Error("a split alt chord cancelled the filters screen")
	}
}

// Defect 2, pinned: the screen exists to say what each filter MEANS, not just
// to name it. Losing an explanation is the regression.
func TestFilterScreenViewExplainsEveryFilter(t *testing.T) {
	view := filterScreenFixture().View()

	for _, row := range filterRows {
		if !strings.Contains(view, row.label) {
			t.Errorf("view is missing the label %q:\n%s", row.label, view)
		}
		if !strings.Contains(view, row.explain) {
			t.Errorf("view is missing %q's explanation %q:\n%s", row.label, row.explain, view)
		}
	}
}

// Values must be on screen too, or the panel names four filters without
// saying what any of them is currently set to.
func TestFilterScreenViewShowsEachFiltersCurrentValue(t *testing.T) {
	view := filterScreenFixtureWith(filterState{toolsOnly: true, minContext: 128_000}).View()

	for _, want := range []string{"128k", "any"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing the value %q:\n%s", want, view)
		}
	}
}

// The count is what makes a filter's effect visible before you commit to it,
// so it must follow the edits. The $1 boundary is kept from the picker's old
// alt+p test: a free model clears any positive ceiling, which the plan got
// wrong once.
func TestFilterScreenMatchCountTracksTheEdits(t *testing.T) {
	m := filterScreenFixture()
	if got := m.matches(); got != 3 {
		t.Fatalf("matches = %d on open, want the whole fixture catalog, 3", got)
	}

	// Max price → $1: only the free model qualifies ($75 opus and $4.40
	// o1-mini are both over).
	m = press(t, m, down(3)...)
	m = press(t, m, typeKey(tea.KeySpace))
	if got := m.matches(); got != 1 {
		t.Errorf("matches = %d at a $1 ceiling, want 1 (the free model)", got)
	}

	// → $5: o1-mini joins it, opus still does not.
	m = press(t, m, typeKey(tea.KeySpace))
	if got := m.matches(); got != 2 {
		t.Errorf("matches = %d at a $5 ceiling, want 2", got)
	}
}

// The count must come from the same expression the picker's own status line
// uses — Rank over Apply — not from Apply alone. filterState carries the
// session's search, and a count that ignored it would contradict the list the
// user is looking at.
func TestFilterScreenMatchCountHonoursTheSearch(t *testing.T) {
	m := filterScreenFixtureWith(filterState{search: "o1"})

	if got := m.matches(); got != 1 {
		t.Errorf("matches = %d with the search %q active, want 1; the count ignores the search",
			got, "o1")
	}
}

func TestFilterScreenTitleAndRowsCoverSorting(t *testing.T) {
	view := filterScreenFixture().View()
	for _, want := range []string{
		"Filter & Sort",
		"Sort by", "relevance", "order the table by this column",
		"Direction", "ascending", "which end of the sort comes first",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

// The two sort rows must be wired to DIFFERENT fields of the same struct — a
// cycle func pointing at the wrong one is the mistake this screen's table
// shape makes easy.
func TestFilterScreenCyclesTheSortRows(t *testing.T) {
	m := filterScreenFixture()

	// Down to "Sort by", then space: relevance -> MODEL.
	for i := 0; i < 4; i++ {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(filterScreenModel)
	}
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = next.(filterScreenModel)
	if m.filters.sort.Key != openrouter.SortModel {
		t.Fatalf("space on Sort by gave %q, want model", m.filters.sort.Key)
	}

	// Down to "Direction", then space: ascending -> descending.
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(filterScreenModel)
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = next.(filterScreenModel)
	if !m.filters.sort.Desc {
		t.Fatal("space on Direction did not flip to descending")
	}
	if m.filters.sort.Key != openrouter.SortModel {
		t.Errorf("Direction moved the column to %q — both rows edit the same field",
			m.filters.sort.Key)
	}

	// enter carries both edits back.
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(filterScreenModel)
	if !m.choice.Applied ||
		m.choice.Filters.sort != (openrouter.Sort{Key: openrouter.SortModel, Desc: true}) {
		t.Errorf("enter returned sort %+v, applied=%v", m.choice.Filters.sort, m.choice.Applied)
	}
}

func TestFilterScreenCancelDiscardsSortEdits(t *testing.T) {
	opened := filterState{sort: openrouter.Sort{Key: openrouter.SortContext}}
	m := filterScreenFixtureWith(opened)
	for i := 0; i < 4; i++ {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(filterScreenModel)
	}
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = next.(filterScreenModel)

	final, _ := m.cancel()
	if got := final.(filterScreenModel).choice.Filters.sort; got != opened.sort {
		t.Errorf("cancel leaked the sort edit: %+v, want %+v", got, opened.sort)
	}
}
