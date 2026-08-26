package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/teggen/openrouter-launch/internal/catalog"
	"github.com/teggen/openrouter-launch/internal/catalog/catalogtest"
	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/ui"
)

// pickerFixture opens the picker over the shared three-model catalog with no
// filters active, so every test starts from the full list.
func pickerFixture() pickerModel {
	return pickerFixtureWith(filterState{})
}

// pickerFixtureWith is pickerFixture with filters already active.
//
// Tests used to seed a filter by pressing alt+f, which stopped being possible
// when the chords were dropped. Seeding through the input is better anyway:
// it states the precondition instead of depending on a second binding's
// behaviour to establish it.
func pickerFixtureWith(f filterState) pickerModel {
	return newPickerModel(pickerInput{
		Agent:   stubSpec("claude"),
		Models:  catalogtest.Models(),
		Filters: f,
		Height:  24,
		Width:   100,
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

// The alt chords are no longer bound. A terminal that still delivers one
// must produce nothing at all — neither a filter change nor a typed letter.
// The letter half is guarded by the search-append branch's !key.Alt, which is
// exactly what TestAltKeysRenderDistinctlyFromPlainKeys pins; the filter half
// fails the moment someone re-adds a case.
func TestPickerAltChordsAreInert(t *testing.T) {
	m := press(t, pickerFixture(), altKey('t'), altKey('f'), altKey('c'), altKey('p'))

	if m.filters.search != "" {
		t.Errorf("search = %q, want empty; an alt chord was typed into the search box",
			m.filters.search)
	}
	if m.filters != (filterState{}) {
		t.Errorf("filters = %+v, want the zero state; an alt chord is still bound", m.filters)
	}
	if m.done {
		t.Error("an alt chord resolved the picker")
	}
}

func TestPickerCtrlFRequestsTheFiltersScreenForTheHighlightedModel(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyDown), typeKey(tea.KeyCtrlF))

	if !m.done || m.choice.Kind != pickFilters {
		t.Fatalf("done=%v kind=%v, want a filters request", m.done, m.choice.Kind)
	}
	// Carried so the driver can reopen the picker on the same model: applying
	// a filter must not lose the row you were comparing.
	if m.choice.ModelID != "qwen/qwen3-coder:free" {
		t.Errorf("filters request carried %q, want the highlighted model", m.choice.ModelID)
	}
}

func TestPickerCtrlFCarriesTheLiveFilters(t *testing.T) {
	m := press(t, pickerFixtureWith(filterState{freeOnly: true}), typeKey(tea.KeyCtrlF))
	if !m.choice.Filters.freeOnly {
		t.Error("ctrl+f dropped the live filter state")
	}
}

// enter and ctrl+s both bail on an empty list, and ctrl+f must NOT copy that
// guard: a filter combination matching nothing is precisely when you need the
// filters screen to undo it. Guarding here traps the user with esc as the
// only way out.
func TestPickerCtrlFOnAnEmptyListStillOpens(t *testing.T) {
	m := press(t, pickerFixture(), typeRunes("zzz-matches-nothing")...)
	if len(m.visible) != 0 {
		t.Fatalf("fixture did not produce an empty list: %v", visibleIDs(m))
	}

	m = press(t, m, typeKey(tea.KeyCtrlF))

	if !m.done || m.choice.Kind != pickFilters {
		t.Fatalf("done=%v kind=%v, want the filters screen to open anyway",
			m.done, m.choice.Kind)
	}
	if m.choice.ModelID != "" {
		t.Errorf("ModelID = %q with nothing visible, want empty", m.choice.ModelID)
	}
}

// The reported defect, at the model level: bubbletea split ESC+t across two
// reads, so the picker saw a bare esc followed by a plain t. Before the latch
// that closed the picker AND typed the letter.
func TestPickerEscFollowedByARuneSwallowsBothHalvesOfTheChord(t *testing.T) {
	m := send(t, pickerFixture(), typeKey(tea.KeyEsc), runeKey('t'))

	if m.done {
		t.Error("a split alt chord closed the picker")
	}
	if m.filters.search != "" {
		t.Errorf("search = %q, want empty; the chord's letter was typed", m.filters.search)
	}
}

// The other half of the latch: with no rune to claim it, the esc must still
// resolve. A latch that only ever swallowed would make esc dead.
func TestPickerEscAloneStillResolvesWhenTheWindowCloses(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyEsc))
	if m.done {
		t.Fatal("esc resolved immediately; the alt-chord window was never opened")
	}

	m = send(t, m, escTimeoutMsg{})

	if !m.done || m.choice.Kind != pickBack {
		t.Errorf("done=%v kind=%v after the window closed, want pickBack", m.done, m.choice.Kind)
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

// pickerChoice.Filters' doc says filters ride every exit; pickModel and
// pickBack were pinned (below), but ctrl+s was not.
func TestPickerCtrlSCarriesTheLiveFilters(t *testing.T) {
	m := press(t, pickerFixtureWith(filterState{freeOnly: true}), typeKey(tea.KeyCtrlS))
	if !m.choice.Filters.freeOnly {
		t.Error("ctrl+s dropped the live filter state")
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
	m := send(t, pickerFixtureWith(filterState{freeOnly: true}), realEsc()...)
	if m.choice.Kind != pickBack {
		t.Fatalf("kind = %v, want pickBack", m.choice.Kind)
	}
	if !m.choice.Filters.freeOnly {
		t.Error("esc dropped the filter change; the driver would persist stale state")
	}
}

// ctrl+c must be distinguishable from esc: the driver turns it into an
// immediate ErrCancelled instead of routing it like a plain back-out (see
// stepPicker). It still carries pickBack and the live filters, matching
// esc's own payload — only Cancelled differs.
func TestPickerCtrlCCancelsCarryingLiveFilters(t *testing.T) {
	m := press(t, pickerFixtureWith(filterState{freeOnly: true}), typeKey(tea.KeyCtrlC))
	if m.choice.Kind != pickBack {
		t.Fatalf("kind = %v, want pickBack", m.choice.Kind)
	}
	if !m.choice.Cancelled {
		t.Error("ctrl+c did not mark the choice as cancelled")
	}
	if !m.choice.Filters.freeOnly {
		t.Error("ctrl+c dropped the live filter state")
	}
}

func TestPickerEscIsNotCancelled(t *testing.T) {
	m := send(t, pickerFixture(), realEsc()...)
	if m.choice.Cancelled {
		t.Error("esc marked the choice as cancelled; only ctrl+c should")
	}
}

func TestPickerEnterCarriesTheLiveFilters(t *testing.T) {
	m := press(t, pickerFixtureWith(filterState{freeOnly: true}), typeKey(tea.KeyEnter))
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

// Applying a filter keeps you on the model you were looking at, if it
// survives the filter. Losing your place makes the filters unusable for
// comparison.
//
// The picker only supplies half of this now — the ModelID it hands to the
// driver on ctrl+f. The round trip that spends it lives one layer up, in
// TestSessionApplyingFiltersReopensThePickerOnTheSameModel; here the property
// is that a model reachable by preselection is actually restored, which is
// what makes that ModelID worth carrying.
func TestPickerPreselectionSurvivesAFilterThatKeepsTheModel(t *testing.T) {
	m := press(t, pickerFixture(), typeKey(tea.KeyDown)) // qwen, which is free
	before := m.visible[m.cursor].ID

	// qwen supports tools, so it survives the filter the driver is about to
	// apply, and the reopened picker must land back on it.
	reopened := newPickerModel(pickerInput{
		Agent:    stubSpec("claude"),
		Models:   catalogtest.Models(),
		Filters:  filterState{toolsOnly: true},
		Selected: before,
		Height:   24, Width: 100,
	})
	if got := reopened.visible[reopened.cursor].ID; got != before {
		t.Errorf("highlighted %q after the filter was applied, want %q", got, before)
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
		Agent: stubSpec("claude"), Models: catalogtest.Models(),
		Selected: "openai/o1-mini", Height: 24, Width: 100,
	})
	if got := m.visible[m.cursor].ID; got != "openai/o1-mini" {
		t.Errorf("highlighted %q, want the preselected model", got)
	}
}

func TestPickerPreselectingAnAbsentModelFallsBackToTheTop(t *testing.T) {
	m := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(),
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

// The whole-view blob is not safe to assert against here: this fixture's
// model IDs are short enough to render whole in every row, so "claude"
// appears regardless of the title state, and the TOOLS column header carries
// the word "tools" regardless of the filter state. Anchor on the specific
// line each fact belongs to instead.
func TestPickerViewNamesTheAgentTheFiltersAndTheCounts(t *testing.T) {
	m := pickerFixtureWith(filterState{toolsOnly: true})
	lines := strings.Split(m.View(), "\n")

	title := lines[0]
	if !strings.Contains(title, "claude") {
		t.Errorf("title line = %q, missing the agent's display name", title)
	}

	// Found by content rather than a fixed index: more robust to layout
	// changes elsewhere in the view, and "of 3" only ever appears on the
	// status line, never in the footer or a model row.
	var status string
	for _, line := range lines {
		if strings.Contains(line, "of 3") {
			status = line
			break
		}
	}
	if status == "" {
		t.Fatalf("View has no status line containing %q:\n%s", "of 3", m.View())
	}
	if !strings.Contains(status, "tools") {
		t.Errorf("status line = %q, missing the active filter label %q", status, "tools")
	}
	if !strings.Contains(status, "2 of 3") {
		t.Errorf("status line = %q, missing %q", status, "2 of 3")
	}
}

// Two distinct descriptions, on two different models, with the cursor moved
// between them: a hard-coded visible[0] lookup (instead of following
// m.cursor) would still pass a version of this test that only checked the
// description present at rest, so each half also asserts the OTHER model's
// description is absent.
func TestPickerViewShowsTheHighlightedModelsDescription(t *testing.T) {
	models := catalogtest.Models()
	models[0].Description = "Aardvark marker for the first model"
	models[1].Description = "Bumblebee marker for the second model"
	m := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: models, Height: 24, Width: 100,
	})

	view := m.View()
	if !strings.Contains(view, "Aardvark marker") {
		t.Errorf("View does not show the highlighted model's description:\n%s", view)
	}
	if strings.Contains(view, "Bumblebee marker") {
		t.Errorf("View shows the second model's description before it is highlighted:\n%s", view)
	}

	m = press(t, m, typeKey(tea.KeyDown))
	view = m.View()
	if !strings.Contains(view, "Bumblebee marker") {
		t.Errorf("View does not follow the cursor to the second model's description:\n%s", view)
	}
	if strings.Contains(view, "Aardvark marker") {
		t.Errorf("View still shows the first model's description after the cursor moved:\n%s", view)
	}
}

// The fixed-height guarantee, end to end: a one-word description and a
// several-paragraph one must produce the same number of rendered lines, or
// the list jumps as the cursor moves.
func TestPickerViewHeightIsStableAcrossCursorMoves(t *testing.T) {
	models := catalogtest.Models()
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

// The catalog table with all five columns has a floor of about 62 columns,
// which is wider than a lot of real terminals. modelTable sheds columns and
// truncates MODEL with an explicit "…" rather than leaning on the renderer,
// which would slice the right border off and leave a table that looks
// broken rather than narrow.
func TestPickerViewClampsRowsToTheAvailableWidth(t *testing.T) {
	for _, width := range []int{40, 60, 80, 100} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			m := newPickerModel(pickerInput{
				Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: width,
			})

			// EVERY line, not just the table's: the title, the description
			// pane, the status line, and the key footer are all chrome that
			// used to be written at whatever width its content happened to
			// be. The footer in particular was a fixed 85 columns.
			var checked int
			for i, line := range strings.Split(m.View(), "\n") {
				checked++
				if got := lipgloss.Width(line); got > width {
					t.Errorf("line %d is %d columns wide, want at most %d:\n%q", i, got, width, line)
				}
			}
			if checked == 0 {
				t.Fatalf("no lines rendered, so this asserted nothing")
			}
		})
	}
}

// Which columns go, and in what order. MODEL must survive every width — it
// is the thing being chosen — so a "shed everything" implementation that
// satisfied the width test above still fails here.
func TestPickerShedsCatalogColumnsOnNarrowTerminals(t *testing.T) {
	headersAt := func(width int) string {
		m := newPickerModel(pickerInput{
			Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: width,
		})
		for _, line := range strings.Split(m.View(), "\n") {
			if strings.Contains(line, "MODEL") {
				return line
			}
		}
		t.Fatalf("no header row at width %d", width)
		return ""
	}

	wide := headersAt(100)
	for _, want := range ui.ModelHeaders {
		if !strings.Contains(wide, want) {
			t.Errorf("a 100-column terminal dropped %q: %q", want, wide)
		}
	}

	narrow := headersAt(40)
	if !strings.Contains(narrow, "MODEL") {
		t.Errorf("MODEL was dropped at 40 columns, leaving nothing to choose between: %q", narrow)
	}
	if strings.Contains(narrow, "OUTPUT/M") {
		t.Errorf("a 40-column terminal kept OUTPUT/M, so nothing was shed: %q", narrow)
	}

	// 40 columns still fits two of the droppable columns, and even at 20 the
	// shedding loop stops as soon as the table fits — so neither width can
	// tell "MODEL is exempt" from "MODEL was never reached". 10 fits
	// nothing, so the loop runs the drop list to the end and only the
	// exemption keeps MODEL alive.
	tiny := headersAt(10)
	if !strings.Contains(tiny, "MODEL") {
		t.Errorf("MODEL was dropped once every other column had gone: %q", tiny)
	}
	for _, gone := range []string{"CONTEXT", "INPUT/M", "OUTPUT/M", "TOOLS"} {
		if strings.Contains(tiny, gone) {
			t.Errorf("a 10-column terminal kept %q: %q", gone, tiny)
		}
	}
}

// Truncating MODEL and shedding columns both make a table narrower, so a
// width assertion alone cannot tell them apart — deleting the truncation
// just makes the shedding loop drop one more column, and the result still
// fits. This pins which one happens: at a width that comfortably holds
// every column, an overlong id must be cut, not cost the user a column.
func TestPickerTruncatesALongModelIDRatherThanSheddingAColumn(t *testing.T) {
	m := newPickerModel(pickerInput{
		Agent:  stubSpec("claude"),
		Models: []catalog.Model{{ID: strings.Repeat("very-long-vendor/", 4) + "model", ContextLength: 200000}},
		Height: 24, Width: 100,
	})
	view := m.View()

	for _, want := range ui.ModelHeaders {
		if !strings.Contains(view, want) {
			t.Errorf("a long model id cost us the %q column instead of being truncated:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "…") {
		t.Errorf("the long model id was not truncated, so the table overflows:\n%s", view)
	}
}

// Pins chromeHeight against bubbletea's actual renderer arithmetic
// (standard_renderer.go's flush) rather than eyeballing the constant:
//
//	newLines := strings.Split(r.buf.String(), "\n")
//	if r.height > 0 && len(newLines) > r.height {
//	    newLines = newLines[len(newLines)-r.height:]
//	}
//
// r.buf.String() is View()'s output verbatim. Two properties matter: the
// split must not have more elements than the terminal is tall (or the
// renderer truncates from the TOP, discarding line 0), and — reproducing
// that same truncation here — line 0, the "Model for <agent>    search:
// <query>" line, must survive it. chromeHeight = 8 fails both: View()'s
// trailing "\n" pushes the split to height+1 elements, and the renderer
// drops exactly the title line to get back down to height. chromeHeight =
// 10 fails neither — it only wastes one row — which is why this test does
// not merely check the title survives; it also checks the raw line count,
// so an off-by-one that shrinks the list without ever truncating stays
// caught by intent even though it renders correctly today.
func TestPickerViewFitsAndKeepsTitleVisibleAtVariousHeights(t *testing.T) {
	for _, height := range []int{24, 30, 40} {
		t.Run(fmt.Sprintf("height=%d", height), func(t *testing.T) {
			m := newPickerModel(pickerInput{
				Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: height, Width: 100,
			})

			rawLines := strings.Split(m.View(), "\n")
			if len(rawLines) > height {
				t.Errorf("View() split into %d lines, want at most %d (the terminal height) — "+
					"the renderer will truncate from the top", len(rawLines), height)
			}

			// Reproduce the renderer's own truncation so the second
			// assertion checks what would actually reach the screen, not
			// just the unclamped View() output.
			visible := rawLines
			if len(visible) > height {
				visible = visible[len(visible)-height:]
			}
			frame := strings.Join(visible, "\n")
			if !strings.Contains(frame, "Model for") {
				t.Errorf("at height %d, the title/search line is not in the visible frame:\n%s",
					height, frame)
			}
		})
	}
}

func TestPickerScrollsToKeepTheCursorVisible(t *testing.T) {
	// A list far longer than the window forces the offset to move.
	var many []catalog.Model
	for _, m := range catalogtest.Models() {
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

// A long search query is user-controlled and unbounded, so the title line
// is the one piece of chrome that can overflow no matter how wide the
// terminal is. Landmine 17 is specifically about this line staying visible.
func TestPickerTitleLineStaysWithinTheTerminal(t *testing.T) {
	const width = 60
	m := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: width,
	})
	m = press(t, m, typeRunes(strings.Repeat("search-term-", 10))...)

	title := strings.Split(m.View(), "\n")[0]
	if got := lipgloss.Width(title); got > width {
		t.Errorf("title line is %d columns wide, want at most %d:\n%q", got, width, title)
	}
}

// The footer wraps rather than overflowing, and every extra line it takes
// has to come out of the list — otherwise it pushes the title off the top,
// which is Landmine 17's outcome by another route.
func TestPickerFooterWrapsAndIsPaidForOutOfTheList(t *testing.T) {
	wide := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: 100,
	})
	narrow := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: 40,
	})

	if len(wide.footer()) != 1 {
		t.Errorf("a 100-column terminal wrapped the footer into %d lines", len(wide.footer()))
	}
	if len(narrow.footer()) < 2 {
		t.Fatalf("a 40-column terminal did not wrap the footer: %q", narrow.footer())
	}
	// Same terminal height, more footer lines, so fewer model rows.
	if narrow.listHeight() >= wide.listHeight() {
		t.Errorf("listHeight = %d narrow vs %d wide: the extra footer lines were not paid for",
			narrow.listHeight(), wide.listHeight())
	}
	// And the whole view still fits, title included.
	if got := len(strings.Split(narrow.View(), "\n")); got > 24 {
		t.Errorf("View split into %d lines, want at most 24:\n%s", got, narrow.View())
	}
}

// The footer is the only place the picker advertises how to reach the
// filters, and advertising a chord that a terminal may silently swallow is
// what made the filters undiscoverable in the first place. It must name
// ctrl+f and must not name an alt chord.
func TestPickerFooterAdvertisesCtrlFAndNoAltChord(t *testing.T) {
	footer := strings.Join(pickerFixture().footer(), hintSeparator)

	if !strings.Contains(footer, "ctrl+f filter&sort") {
		t.Errorf("footer = %q, missing the filter&sort key", footer)
	}
	if strings.Contains(footer, "alt+") {
		t.Errorf("footer = %q, still advertises an alt chord", footer)
	}
}

// hintLines must never split one hint across two lines: "ctrl+s save pro" /
// "file" reads as two broken things rather than one wrapped thing.
func TestHintLinesBreakBetweenHintsNeverInsideOne(t *testing.T) {
	for _, width := range []int{20, 30, 40, 60, 100} {
		for _, line := range hintLines(pickerHints, width) {
			for _, part := range strings.Split(line, hintSeparator) {
				if !slices.Contains(pickerHints, part) {
					t.Errorf("width %d produced %q, which is not a whole hint (line %q)",
						width, part, line)
				}
			}
		}
	}
}

// modelIDList is the visible list's IDs, for failure messages.
func modelIDList(models []catalog.Model) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.ID
	}
	return out
}

// The load-bearing composition test. The fixture is chosen so relevance order
// and column order genuinely DISAGREE: searching "o" matches all three models,
// and Rank puts openai/o1-mini first (an ID prefix beats a substring) while
// cheapest-output puts the free qwen model first. A fixture where the two
// agree passes with SortModels moved inside Rank and proves nothing.
func TestPickerSortAppliesOutsideRank(t *testing.T) {
	unsorted := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(),
		Filters: filterState{search: "o"}, Width: 120, Height: 40,
	})
	if got := unsorted.visible[0].ID; got != "openai/o1-mini" {
		t.Fatalf("relevance order changed: first is %q, want openai/o1-mini "+
			"(this test cannot tell the two orders apart unless they differ)", got)
	}

	sorted := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(),
		Filters: filterState{
			search: "o",
			sort:   openrouter.Sort{Key: openrouter.SortOutput},
		},
		Width: 120, Height: 40,
	})
	want := []string{"qwen/qwen3-coder:free", "openai/o1-mini", "anthropic/claude-opus-4.6"}
	for i, id := range want {
		if sorted.visible[i].ID != id {
			t.Fatalf("sorted+searched order = %v, want %v (is SortModels inside Rank?)",
				modelIDList(sorted.visible), want)
		}
	}
}

func TestPickerKeepsTheSortAcrossASearchEdit(t *testing.T) {
	m := newPickerModel(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(),
		Filters: filterState{sort: openrouter.Sort{Key: openrouter.SortOutput, Desc: true}},
		Width:   120, Height: 40,
	})
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	p := next.(pickerModel)
	if got := p.visible[0].ID; got != "anthropic/claude-opus-4.6" {
		t.Errorf("after typing, the first row is %q, want the priciest — recompute dropped the sort", got)
	}
}
