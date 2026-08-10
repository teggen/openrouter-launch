package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/ui"
)

// filterRow is one line of the filter&sort screen.
//
// Declared as a table rather than a switch so that adding a row is a table
// entry — label, explanation, how to render it, how to advance it — and cannot
// half-land as a key binding with no explanation next to it. That explanation
// is the whole reason this screen exists: the chords it replaced were named in
// a footer and defined nowhere.
//
// The last two rows configure the ordering rather than the filtering. They
// share the shape deliberately: one cursor, one editing key, and no row that
// behaves unlike its neighbours.
type filterRow struct {
	label   string
	explain string
	value   func(filterState) string
	cycle   func(*filterState)
}

// unconstrained is the value shown for a numeric filter that is not filtering.
// It is not "0" or "free": 0 means "no constraint", and rendering a price
// ceiling of 0 through FormatPrice would say "free", which is a different
// claim entirely.
const unconstrained = "any"

var filterRows = []filterRow{{
	label:   "Tools",
	explain: "only models that can call tools",
	value:   func(f filterState) string { return onOff(f.toolsOnly) },
	cycle:   func(f *filterState) { f.toolsOnly = !f.toolsOnly },
}, {
	label:   "Free",
	explain: "only models priced at $0",
	value:   func(f filterState) string { return onOff(f.freeOnly) },
	cycle:   func(f *filterState) { f.freeOnly = !f.freeOnly },
}, {
	label:   "Min context",
	explain: "hide models with a smaller context window",
	value: func(f filterState) string {
		if f.minContext == 0 {
			return unconstrained
		}
		return openrouter.FormatContext(f.minContext)
	},
	cycle: func(f *filterState) { f.minContext = nextContext(f.minContext) },
}, {
	label:   "Max price",
	explain: "hide models above this price per million tokens",
	value: func(f filterState) string {
		if f.maxPrice == 0 {
			return unconstrained
		}
		return openrouter.FormatPrice(f.maxPrice, false) + "/M"
	},
	cycle: func(f *filterState) { f.maxPrice = nextPrice(f.maxPrice) },
}, {
	label:   "Sort by",
	explain: "order the table by this column",
	// ui.SortLabel, so the row names the column with the table's own header
	// and a rename cannot leave the two disagreeing.
	value: func(f filterState) string { return ui.SortLabel(f.sort.Key) },
	cycle: func(f *filterState) { f.sort.Key = nextSortKey(f.sort.Key) },
}, {
	// Shown even while Sort by is "relevance", where it does nothing: hiding
	// it would make the screen's row list depend on its own state, and the
	// explanation stands on its own either way.
	label:   "Direction",
	explain: "which end of the sort comes first",
	value: func(f filterState) string {
		if f.sort.Desc {
			return "descending"
		}
		return "ascending"
	},
	cycle: func(f *filterState) { f.sort.Desc = !f.sort.Desc },
}}

// onOff renders a boolean filter. The active state is capitalised so a glance
// down the column finds what is actually filtering.
func onOff(on bool) string {
	if on {
		return "ON"
	}
	return "off"
}

var filterScreenHints = []string{
	"↑/↓ move", "space toggle/cycle", "enter apply", "esc cancel",
}

type filterScreenInput struct {
	Filters filterState
	Models  []openrouter.Model
	Width   int
	Height  int
}

type filterScreenChoice struct {
	// Filters is what the picker should reopen with. On a cancel it is what
	// the screen opened with, not the live edits.
	Filters filterState
	// Applied distinguishes enter from esc.
	Applied bool
	// Cancelled is set only by ctrl+c, and ends the session rather than
	// returning to the picker — matching every other screen.
	Cancelled bool
}

type filterScreenModel struct {
	// opened is restored on cancel, so esc cannot leak the live edits.
	opened  filterState
	filters filterState
	all     []openrouter.Model
	cursor  int
	width   int
	height  int
	choice  filterScreenChoice
	done    bool
	esc     escLatch
}

func newFilterScreenModel(in filterScreenInput) filterScreenModel {
	return filterScreenModel{
		opened:  in.Filters,
		filters: in.Filters,
		all:     in.Models,
		width:   in.Width,
		height:  in.Height,
	}
}

// matches is how many models the live filters leave visible.
//
// It is the picker's own expression — Rank over Apply — rather than Apply
// alone. filterState carries the session's search, and a count that ignored
// it would disagree with the list the user just came from.
func (m filterScreenModel) matches() int {
	return len(Rank(openrouter.Apply(m.all, m.filters.catalogFilter()), m.filters.search))
}

func (m filterScreenModel) Init() tea.Cmd { return nil }

func (m filterScreenModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if handled, escaped := m.esc.step(msg); handled {
		if escaped {
			return m.cancel()
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// cancel resolves the screen as "discard": the filters it opened with, and
// Applied false.
func (m filterScreenModel) cancel() (tea.Model, tea.Cmd) {
	m.choice = filterScreenChoice{Filters: m.opened}
	m.done = true
	return m, tea.Quit
}

func (m filterScreenModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		// Deferred, like the picker's: see escLatch. It matters more here,
		// where resolving a split chord as esc would throw away edits.
		return m, m.esc.arm()

	case "ctrl+c":
		m.choice = filterScreenChoice{Filters: m.opened, Cancelled: true}
		m.done = true
		return m, tea.Quit

	case "enter":
		m.choice = filterScreenChoice{Filters: m.filters, Applied: true}
		m.done = true
		return m, tea.Quit

	case " ", "space":
		filterRows[m.cursor].cycle(&m.filters)
		return m, nil

	case "up", "ctrl+p":
		m.moveCursor(-1)
		return m, nil
	case "down", "ctrl+n":
		m.moveCursor(1)
		return m, nil
	}
	return m, nil
}

func (m *filterScreenModel) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(filterRows) {
		m.cursor = len(filterRows) - 1
	}
}

func (m filterScreenModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Filter & Sort") + "\n\n")

	// Measured rather than hard-coded, so a longer label or a wider value
	// cannot silently run into the column on its right.
	labelWidth, valueWidth := 0, 0
	for _, row := range filterRows {
		labelWidth = max(labelWidth, lipgloss.Width(row.label))
		valueWidth = max(valueWidth, lipgloss.Width(row.value(m.filters)))
	}

	for i, row := range filterRows {
		line := fmt.Sprintf("  %s %-*s  %-*s  %s",
			cursorCell(i == m.cursor),
			labelWidth, row.label,
			valueWidth, row.value(m.filters),
			dimStyle.Render(row.explain))
		b.WriteString(clampLine(line, m.width) + "\n")
	}

	// Four spaces, to start under the labels rather than under the cursor
	// column: "  " + the one-column cursor cell + " ".
	b.WriteString("\n" + clampLine(dimStyle.Render(fmt.Sprintf("    %d of %d models match",
		m.matches(), len(m.all))), m.width) + "\n\n")

	for _, line := range hintLines(filterScreenHints, m.width-2) {
		b.WriteString(dimStyle.Render("  "+line) + "\n")
	}
	return b.String()
}
