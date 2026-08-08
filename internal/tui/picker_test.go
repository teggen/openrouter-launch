package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/openrouter/ortest"
)

// pickerFixture opens the picker over the shared three-model catalog with no
// filters active, so every test starts from the full list.
func pickerFixture() pickerModel {
	return newPickerModel(pickerInput{
		Agent:  stubSpec("claude"),
		Models: ortest.Models(),
		Height: 24,
		Width:  100,
	})
}

func visibleIDs(m pickerModel) []string {
	out := make([]string, len(m.visible))
	for i, v := range m.visible {
		out[i] = v.ID
	}
	return out
}

func TestPickerOpensOnTheWholeCatalog(t *testing.T) {
	if got := len(pickerFixture().visible); got != 3 {
		t.Errorf("visible = %d models, want 3", got)
	}
}

func TestPickerTypingNarrowsTheList(t *testing.T) {
	m := press(t, pickerFixture(), typeRunes("o1")...)
	if got := visibleIDs(m); len(got) != 1 || got[0] != "openai/o1-mini" {
		t.Errorf("visible = %v, want only openai/o1-mini", got)
	}
	if m.filters.search != "o1" {
		t.Errorf("search = %q, want %q", m.filters.search, "o1")
	}
}

// THE key-conflict test. The spec's original key table put bare t/f/c/$ on
// the filters, which collides with type-to-search: "anthropic" contains a
// t. Handling must switch on msg.String() so "alt+t" is a distinct case from
// "t"; matching on msg.Type == KeyRunes first would type the letter.
func TestPickerAltChordsAreNeverTypedIntoSearch(t *testing.T) {
	m := press(t, pickerFixture(), altKey('t'), altKey('f'), altKey('c'), altKey('p'))
	if m.filters.search != "" {
		t.Errorf("search = %q, want empty; an alt chord was typed into the search box",
			m.filters.search)
	}
}

func TestPickerAltTTogglesToolsFilter(t *testing.T) {
	m := press(t, pickerFixture(), altKey('t'))
	if !m.filters.toolsOnly {
		t.Fatal("alt+t did not enable the tools filter")
	}
	// o1-mini is the only fixture entry without tool support.
	if got := visibleIDs(m); len(got) != 2 {
		t.Errorf("visible = %v, want the two tool-capable models", got)
	}
	if m2 := press(t, m, altKey('t')); m2.filters.toolsOnly {
		t.Error("alt+t did not toggle back off")
	}
}

func TestPickerAltFTogglesFreeFilter(t *testing.T) {
	m := press(t, pickerFixture(), altKey('f'))
	if !m.filters.freeOnly {
		t.Fatal("alt+f did not enable the free filter")
	}
	if got := visibleIDs(m); len(got) != 1 || got[0] != "qwen/qwen3-coder:free" {
		t.Errorf("visible = %v, want only the free model", got)
	}
}

func TestPickerAltCCyclesTheContextFloor(t *testing.T) {
	m := press(t, pickerFixture(), altKey('c'))
	if m.filters.minContext != 32_000 {
		t.Errorf("minContext = %d, want 32000", m.filters.minContext)
	}
	m = press(t, m, altKey('c'))
	if m.filters.minContext != 128_000 {
		t.Errorf("minContext = %d after two presses, want 128000", m.filters.minContext)
	}
}

func TestPickerAltPCyclesThePriceCeiling(t *testing.T) {
	m := press(t, pickerFixture(), altKey('p'))
	if m.filters.maxPrice != 1 {
		t.Errorf("maxPrice = %v, want 1", m.filters.maxPrice)
	}
	// At $5 the free model and o1-mini both qualify: free pricing is $0,
	// which clears any positive ceiling, so only the $75 opus is excluded.
	// (openrouter.Apply's MaxPrice and FreeOnly are independent constraints;
	// see TestApplyMaxPriceUsesCompletionPrice, pinned on this same fixture.)
	m = press(t, m, altKey('p'))
	if got := visibleIDs(m); len(got) != 2 || got[0] != "qwen/qwen3-coder:free" || got[1] != "openai/o1-mini" {
		t.Errorf("visible at $5 = %v, want the free model and o1-mini", got)
	}
}

func TestPickerBackspaceEditsTheSearch(t *testing.T) {
	m := press(t, pickerFixture(), typeRunes("abc")...)
	m = press(t, m, typeKey(tea.KeyBackspace))
	if m.filters.search != "ab" {
		t.Errorf("search = %q, want %q", m.filters.search, "ab")
	}
}

func TestPickerBackspaceOnAnEmptySearchIsANoop(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyBackspace))
	if m.filters.search != "" || len(m.visible) != 3 {
		t.Errorf("search=%q visible=%d, want unchanged", m.filters.search, len(m.visible))
	}
}

func TestPickerEnterReturnsTheHighlightedModel(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyDown), typeKey(tea.KeyEnter))
	if !m.done || m.choice.Kind != pickModel {
		t.Fatalf("done=%v kind=%v, want a model choice", m.done, m.choice.Kind)
	}
	if m.choice.ModelID != "qwen/qwen3-coder:free" {
		t.Errorf("chose %q, want the second model", m.choice.ModelID)
	}
}

// Without the guard this indexes an empty slice and panics.
func TestPickerEnterOnAnEmptyListIsANoop(t *testing.T) {
	m := press(t, pickerFixture(), typeRunes("zzz-matches-nothing")...)
	if len(m.visible) != 0 {
		t.Fatalf("fixture did not produce an empty list: %v", visibleIDs(m))
	}
	if m2 := press(t, m, typeKey(tea.KeyEnter)); m2.done {
		t.Error("enter resolved the picker with nothing highlighted")
	}
}

func TestPickerCtrlSRequestsAProfileSaveForTheHighlightedModel(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyCtrlS))
	if m.choice.Kind != pickSaveProfile {
		t.Fatalf("kind = %v, want a save request", m.choice.Kind)
	}
	if m.choice.ModelID != "anthropic/claude-opus-4.6" {
		t.Errorf("save carried %q, want the highlighted model", m.choice.ModelID)
	}
}

func TestPickerCtrlSOnAnEmptyListIsANoop(t *testing.T) {
	m := press(t, pickerFixture(), typeRunes("zzz-matches-nothing")...)
	if m2 := press(t, m, typeKey(tea.KeyCtrlS)); m2.done {
		t.Error("ctrl+s resolved the picker with nothing highlighted")
	}
}

// Filters are returned on EVERY exit, including backing out, because the
// driver persists them whether or not the session launched.
func TestPickerEscReturnsBackCarryingTheLiveFilters(t *testing.T) {
	m := press(t, pickerFixture(), altKey('f'), typeKey(tea.KeyEsc))
	if m.choice.Kind != pickBack {
		t.Fatalf("kind = %v, want pickBack", m.choice.Kind)
	}
	if !m.choice.Filters.freeOnly {
		t.Error("esc dropped the filter change; the driver would persist stale state")
	}
}

func TestPickerEnterCarriesTheLiveFilters(t *testing.T) {
	m := press(t, pickerFixture(), altKey('f'), typeKey(tea.KeyEnter))
	if !m.choice.Filters.freeOnly {
		t.Error("enter dropped the filter change")
	}
}

// A search that narrows the list must not leave the cursor past the end.
func TestPickerCursorClampsWhenTheListNarrows(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyDown), typeKey(tea.KeyDown))
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want 2 before narrowing", m.cursor)
	}
	m = press(t, m, typeRunes("o1")...)
	if m.cursor >= len(m.visible) {
		t.Errorf("cursor = %d with %d visible; out of range", m.cursor, len(m.visible))
	}
}

// Toggling a filter keeps you on the model you were looking at, if it
// survives the filter. Losing your place on every chord makes the filters
// unusable for comparison.
func TestPickerFilterToggleKeepsTheHighlightedModel(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyDown)) // qwen, which is free
	before := m.visible[m.cursor].ID
	m = press(t, m, altKey('t')) // qwen supports tools, so it survives
	if got := m.visible[m.cursor].ID; got != before {
		t.Errorf("highlighted %q after the toggle, want %q", got, before)
	}
}

// Typing is different: a new search re-ranks, and the best match should be
// under the cursor rather than whatever was there before.
func TestPickerTypingMovesToTheBestMatch(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyDown), typeKey(tea.KeyDown))
	m = press(t, m, typeRunes("a")...)
	if m.cursor != 0 {
		t.Errorf("cursor = %d after typing, want 0 (the best match)", m.cursor)
	}
}

func TestPickerPreselectsTheGivenModel(t *testing.T) {
	m := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: ortest.Models(),
		Selected: "openai/o1-mini", Height: 24, Width: 100,
	})
	if got := m.visible[m.cursor].ID; got != "openai/o1-mini" {
		t.Errorf("highlighted %q, want the preselected model", got)
	}
}

func TestPickerPreselectingAnAbsentModelFallsBackToTheTop(t *testing.T) {
	m := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: ortest.Models(),
		Selected: "no/such-model", Height: 24, Width: 100,
	})
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestPickerCursorStopsAtBothEnds(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyUp), typeKey(tea.KeyUp))
	if m.cursor != 0 {
		t.Errorf("cursor = %d after up past the top, want 0", m.cursor)
	}
	m = press(t, m, typeKey(tea.KeyDown), typeKey(tea.KeyDown), typeKey(tea.KeyDown), typeKey(tea.KeyDown))
	if m.cursor != len(m.visible)-1 {
		t.Errorf("cursor = %d after down past the bottom, want %d", m.cursor, len(m.visible)-1)
	}
}

func TestPickerViewNamesTheAgentTheFiltersAndTheCounts(t *testing.T) {
	m := press(t, pickerFixture(), altKey('t'))
	got := m.View()
	for _, want := range []string{"claude", "tools", "2 of 3"} {
		if !strings.Contains(got, want) {
			t.Errorf("View = %q, missing %q", got, want)
		}
	}
}

func TestPickerViewShowsTheHighlightedModelsDescription(t *testing.T) {
	models := ortest.Models()
	models[0].Description = "A distinctive description string"
	m := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: models, Height: 24, Width: 100,
	})
	if !strings.Contains(m.View(), "A distinctive description") {
		t.Errorf("View does not show the highlighted description:\n%s", m.View())
	}
}

// The fixed-height guarantee, end to end: a one-word description and a
// several-paragraph one must produce the same number of rendered lines, or
// the list jumps as the cursor moves.
func TestPickerViewHeightIsStableAcrossCursorMoves(t *testing.T) {
	models := ortest.Models()
	models[0].Description = "Short."
	models[1].Description = strings.Repeat("A very long description. ", 100)

	m := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: models, Height: 24, Width: 100,
	})
	first := strings.Count(m.View(), "\n")
	second := strings.Count(press(t, m, typeKey(tea.KeyDown)).View(), "\n")

	if first != second {
		t.Errorf("View height changed with the cursor: %d then %d lines", first, second)
	}
}

func TestPickerViewSaysSoWhenNothingMatches(t *testing.T) {
	m := press(t, pickerFixture(), typeRunes("zzz-matches-nothing")...)
	got := m.View()
	if !strings.Contains(got, "0 of 3") {
		t.Errorf("View = %q, does not show that filtering emptied the list", got)
	}
}

func TestPickerScrollsToKeepTheCursorVisible(t *testing.T) {
	// A list far longer than the window forces the offset to move.
	var many []openrouter.Model
	for _, m := range ortest.Models() {
		for i := 0; i < 20; i++ {
			c := m
			c.ID = m.ID + string(rune('a'+i))
			many = append(many, c)
		}
	}
	m := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: many, Height: 12, Width: 100,
	})
	for i := 0; i < 30; i++ {
		m = press(t, m, typeKey(tea.KeyDown))
	}
	if m.cursor < m.offset || m.cursor >= m.offset+m.listHeight() {
		t.Errorf("cursor %d outside the window [%d, %d)",
			m.cursor, m.offset, m.offset+m.listHeight())
	}
}
