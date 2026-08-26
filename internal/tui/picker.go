package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/catalog"
	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/ui"
)

const (
	// defaultListHeight is used before the first WindowSizeMsg arrives, so
	// the picker renders something rather than nothing on its first frame.
	defaultListHeight = 10
	// nonListChrome is the number of lines View() writes outside the model
	// table: 8 of them — the title line, the blank line under it, the blank
	// line after the table, the two description-pane lines, the blank line
	// before the status line, the status line, and the key footer — plus 1,
	// because View()'s output also ENDS in "\n", so splitting it the way
	// bubbletea's standard renderer does (strings.Split on "\n") yields one
	// more element than the newline count. bubbletea drops from the TOP of
	// that split when it has more elements than the terminal is tall — and
	// line 0 is the title/search line, so an off-by-one here makes the
	// search echo invisible at every terminal height, not just small ones.
	// Recounting the literal chrome lines in View() and stopping at 8
	// reintroduces that bug. See
	// TestPickerViewFitsAndKeepsTitleVisibleAtVariousHeights, which pins
	// this against the renderer's actual arithmetic.
	nonListChrome = 9
	// descriptionHeight is fixed on purpose. See descriptionLines.
	descriptionHeight = 2
)

// tableFrame is what a bordered table costs on top of its rows: a top
// border, a header row, a header rule, and a bottom border.
//
// MEASURED at init, not written as 4. Landmine 17 exists because the chrome
// above was counted by hand and came out one short, and this change added
// four more lines to the same budget — the same mistake, twice, would be
// nobody's fault but ours. Rendering a one-row table and subtracting that
// row cannot drift from what lipgloss actually draws.
var tableFrame = lipgloss.Height(theme.Render(ui.Table{
	Headers: append([]string{" "}, ui.ModelHeaders...),
	Rows:    [][]string{append([]string{" "}, ui.ModelCells(catalog.Model{})...)},
})) - 1

// pickerHints is the key footer, one hint per element so hintLines can
// break between them on a narrow terminal instead of overflowing.
var pickerHints = []string{
	"ctrl+f filter&sort", "ctrl+s save profile", "esc back",
}

// footer is the key hints, packed to the terminal width.
func (m pickerModel) footer() []string {
	return hintLines(pickerHints, m.width-2) // 2 for the indent View adds
}

// chromeHeight is the budget subtracted from the terminal height to get
// listHeight().
//
// It is a method, not a constant, because the footer wraps: nonListChrome
// counts ONE footer line, so every extra one costs a model row. A fixed
// budget here would let the footer push the list — and with it the title —
// off the top of the screen, which is Landmine 17's outcome by yet another
// route.
func (m pickerModel) chromeHeight() int {
	return nonListChrome + tableFrame + len(m.footer()) - 1
}

type pickerInput struct {
	Agent   *agent.Spec
	Models  []catalog.Model
	Filters filterState
	// Selected preselects a model by ID: config's last_model on first open,
	// and the previously highlighted model when the picker reopens after a
	// profile save.
	Selected string
	Width    int
	Height   int
}

type pickerChoiceKind int

const (
	pickBack pickerChoiceKind = iota
	pickModel
	pickSaveProfile
	pickFilters
)

type pickerChoice struct {
	Kind pickerChoiceKind
	// ModelID is the highlighted model, for pickModel, pickSaveProfile and
	// pickFilters. On pickFilters it is what lets the driver reopen the
	// picker on the same row, so applying a filter does not lose your place.
	ModelID string
	// Filters is the live filter state, returned on every exit — including
	// pickBack — because the driver persists filters whether or not the
	// session went on to launch.
	Filters filterState
	// Cancelled is set only by ctrl+c, never by esc. The driver checks it
	// before routing on Kind, so ctrl+c ends the session in one press
	// instead of retreating one step like esc — see stepPicker.
	Cancelled bool
}

type pickerModel struct {
	agent   *agent.Spec
	all     []catalog.Model
	filters filterState
	visible []catalog.Model
	cursor  int
	offset  int
	width   int
	height  int
	choice  pickerChoice
	done    bool
	// esc defers a lone esc long enough to tell it apart from an alt chord
	// whose two bytes landed in separate reads. See escLatch.
	esc escLatch
}

func newPickerModel(in pickerInput) pickerModel {
	m := pickerModel{
		agent:   in.Agent,
		all:     in.Models,
		filters: in.Filters,
		width:   in.Width,
		height:  in.Height,
	}
	m.recompute(in.Selected)
	return m
}

// recompute reapplies the filters and the search, then restores the cursor to
// keepID when that model is still visible.
//
// The order matters, in three steps. The four catalog filters run through
// openrouter.Apply — letting Apply also match on Search would narrow the list
// with plain substring semantics before Rank ever saw it. Then the search runs
// through Rank, which orders by match quality. Then the chosen column runs
// through SortModels, OUTSIDE Rank, so a column the user picked deliberately
// beats relevance and relevance survives only as the stable sort's tie-break.
//
// Sorting inside Rank's argument type-checks, looks identical at a glance, and
// inverts that decision — see Landmine 38.
func (m *pickerModel) recompute(keepID string) {
	m.visible = openrouter.SortModels(
		Rank(openrouter.Apply(m.all, m.filters.catalogFilter()), m.filters.search),
		m.filters.sort)
	m.cursor = indexOfModel(m.visible, keepID)
	m.clampScroll()
}

func indexOfModel(models []catalog.Model, id string) int {
	if id == "" {
		return 0
	}
	for i, m := range models {
		if m.ID == id {
			return i
		}
	}
	return 0
}

// currentID is the highlighted model's ID, or "" when nothing is visible.
func (m pickerModel) currentID() string {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return ""
	}
	return m.visible[m.cursor].ID
}

// listHeight is how many model rows fit. Before the first WindowSizeMsg the
// terminal height is unknown, so a fixed default is used rather than
// rendering an empty list.
func (m pickerModel) listHeight() int {
	if m.height <= 0 {
		return defaultListHeight
	}
	if h := m.height - m.chromeHeight(); h > 0 {
		return h
	}
	return 1
}

func (m *pickerModel) moveCursor(delta int) {
	if len(m.visible) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	m.clampScroll()
}

// clampScroll keeps the cursor inside the rendered window and the window
// inside the list.
func (m *pickerModel) clampScroll() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset > len(m.visible)-h {
		m.offset = len(m.visible) - h
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The latch runs before anything else, because while an esc is pending
	// the very next message decides what that esc meant. It consumes only
	// what it needs: a resize still reaches the case below with the esc
	// still waiting.
	if handled, escaped := m.esc.step(msg); handled {
		if escaped {
			return m.back()
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// back resolves the picker as "the user retreated", carrying the live filters
// because the driver persists them whether or not the session launched.
func (m pickerModel) back() (tea.Model, tea.Cmd) {
	m.choice = pickerChoice{Kind: pickBack, Filters: m.filters}
	m.done = true
	return m, tea.Quit
}

func (m pickerModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Switching on String() first is load-bearing: it is what keeps every
	// named key and control chord out of the search-append branch at the
	// bottom of this function, which matches on key.Type.
	//
	// The four alt chords that used to live here are gone. They were
	// unreachable on terminals that deliver Alt some other way, and on those
	// terminals the letter arrived bare and was typed into the search box —
	// the defect this screen's filters screen replaced them to fix. A
	// surviving alt+t now matches no case and is stopped by the !key.Alt
	// guard below, so it does nothing at all.
	switch key.String() {
	case "esc":
		// Deferred, not acted on: this may be the first byte of an alt chord.
		return m, m.esc.arm()

	case "ctrl+c":
		m.choice = pickerChoice{Kind: pickBack, Filters: m.filters, Cancelled: true}
		m.done = true
		return m, tea.Quit

	case "enter":
		if len(m.visible) == 0 {
			return m, nil
		}
		m.choice = pickerChoice{Kind: pickModel, ModelID: m.currentID(), Filters: m.filters}
		m.done = true
		return m, tea.Quit

	case "ctrl+s":
		if len(m.visible) == 0 {
			return m, nil
		}
		m.choice = pickerChoice{Kind: pickSaveProfile, ModelID: m.currentID(), Filters: m.filters}
		m.done = true
		return m, tea.Quit

	// Deliberately NOT guarded on len(m.visible) the way enter and ctrl+s
	// are. A filter combination that matches nothing is exactly when the
	// filters screen is needed, and a guard here would leave esc as the only
	// way out of a list the user filtered into emptiness.
	case "ctrl+f":
		m.choice = pickerChoice{Kind: pickFilters, ModelID: m.currentID(), Filters: m.filters}
		m.done = true
		return m, tea.Quit

	case "up", "ctrl+p":
		m.moveCursor(-1)
		return m, nil
	case "down", "ctrl+n":
		m.moveCursor(1)
		return m, nil
	case "pgup":
		m.moveCursor(-m.listHeight())
		return m, nil
	case "pgdown":
		m.moveCursor(m.listHeight())
		return m, nil

	case "backspace":
		if r := []rune(m.filters.search); len(r) > 0 {
			m.filters.search = string(r[:len(r)-1])
			m.recompute("")
		}
		return m, nil
	}

	// Everything else that produces text goes to the search box. Editing the
	// search re-ranks, so the cursor goes to the best match rather than
	// staying on whatever happened to be highlighted.
	switch {
	case key.Type == tea.KeyRunes && !key.Alt:
		m.filters.search += string(key.Runes)
		m.recompute("")
	case key.Type == tea.KeySpace:
		m.filters.search += " "
		m.recompute("")
	}
	return m, nil
}

const (
	// minModelWidth is the floor the MODEL column is never truncated below.
	minModelWidth = 8
	// catalogIndent is the two columns modelTable's output is indented by
	// in View, which count against the terminal width.
	catalogIndent = 2
)

// catalogDropOrder lists the catalog columns to shed on a terminal too
// narrow for all of them, least useful first.
//
// A table cannot be cut mid-line the way clampRow cut a preformatted row:
// its fixed columns impose a floor (about 62 columns with all five), and
// below that something has to give. Dropping a column is the honest
// version of "give" — the alternative is letting bubbletea slice the right
// border off and leave a table that looks broken rather than narrow.
// MODEL is never dropped: it is the thing being chosen.
var catalogDropOrder = []int{3, 2, 1, 4} // OUTPUT/M, INPUT/M, CONTEXT, TOOLS

// modelTable renders the visible window as a bordered table, shedding
// columns until it fits the terminal.
func (m pickerModel) modelTable() string {
	keep := []int{0, 1, 2, 3, 4}
	out := m.renderCatalog(keep)
	for _, drop := range catalogDropOrder {
		if m.width <= 0 || lipgloss.Width(out)+catalogIndent <= m.width {
			break
		}
		keep = without(keep, drop)
		out = m.renderCatalog(keep)
	}
	return out
}

// renderCatalog draws the visible window using the catalog columns in keep
// (indices into ui.ModelHeaders), truncating MODEL until the table fits.
//
// The table is ALWAYS listHeight rows tall, padded with blank rows, so the
// description pane and status lines below it do not move as the list
// shortens — the same fixed-height guarantee descriptionLines gives the
// pane itself.
//
// Rows must stay ONE line tall, which is why ui.Table's MaxWidth is
// deliberately NOT used here: it wraps an overlong cell, and a wrapped row
// would make the table taller than listHeight budgeted for, pushing the
// title off the top of the screen. Truncating with an explicit "…" is what
// clampRow used to do for the whole line.
func (m pickerModel) renderCatalog(keep []int) string {
	headers := []string{" "}
	for _, c := range keep {
		headers = append(headers, ui.ModelHeaders[c])
	}

	h := m.listHeight()
	rows := make([][]string, 0, h)
	for i := 0; i < h; i++ {
		idx := m.offset + i
		if idx >= len(m.visible) {
			rows = append(rows, make([]string, len(keep)+1))
			continue
		}
		cells := ui.ModelCells(m.visible[idx])
		row := []string{cursorCell(idx == m.cursor)}
		for _, c := range keep {
			row = append(row, cells[c])
		}
		rows = append(rows, row)
	}

	selected := m.cursor - m.offset
	render := func() string {
		return theme.Render(ui.Table{
			Headers:  headers,
			Rows:     rows,
			Emphasis: func(row int) bool { return row == selected },
			Role: func(_, col int) ui.Role {
				if col == 0 {
					return ui.RolePlain
				}
				return ui.ModelRole(keep[col-1])
			},
		})
	}

	out := render()
	if m.width <= 0 {
		return out
	}
	// Shrink MODEL by measuring, rather than deriving the other columns'
	// widths plus padding plus borders by hand — that arithmetic is what
	// Landmine 17 is about. MODEL (always column 1, since keep[0] is 0) is
	// the only variable-width column, so one pass is normally exact; the
	// loop is a bound, not an algorithm.
	for i := 0; i < 4; i++ {
		excess := lipgloss.Width(out) + catalogIndent - m.width
		widest := widestCell(rows, 1)
		if excess <= 0 || widest <= minModelWidth {
			break
		}
		budget := max(widest-excess, minModelWidth)
		for _, row := range rows {
			row[1] = truncate(row[1], budget)
		}
		out = render()
	}
	return out
}

// widestCell is the display width of the widest cell in column col.
func widestCell(rows [][]string, col int) int {
	n := 0
	for _, row := range rows {
		if w := lipgloss.Width(row[col]); w > n {
			n = w
		}
	}
	return n
}

// without returns keep with column drop removed, leaving keep untouched.
func without(keep []int, drop int) []int {
	out := make([]int, 0, len(keep))
	for _, c := range keep {
		if c != drop {
			out = append(out, c)
		}
	}
	return out
}

func (m pickerModel) View() string {
	var b strings.Builder

	name := ""
	if m.agent != nil {
		name = m.agent.Launcher.DisplayName()
	}
	// Clamped: the search echo grows as the user types, and Landmine 17 is
	// about this line in particular staying visible.
	b.WriteString(clampLine(titleStyle.Render("Model for "+name)+"    "+
		dimStyle.Render("search: "+m.filters.search), m.width) + "\n\n")

	b.WriteString(indent(m.modelTable()) + "\n\n")
	desc := ""
	if m.cursor < len(m.visible) {
		desc = m.visible[m.cursor].Description
	}
	width := m.width - 4
	if width <= 0 {
		width = 76
	}
	for _, line := range descriptionLines(desc, width, descriptionHeight) {
		b.WriteString("  " + dimStyle.Render(line) + "\n")
	}

	b.WriteString("\n" + clampLine("  "+m.filters.label()+"    "+
		dimStyle.Render(fmt.Sprintf("%d of %d models", len(m.visible), len(m.all))), m.width) + "\n")
	for _, line := range m.footer() {
		b.WriteString(dimStyle.Render("  "+line) + "\n")
	}

	return b.String()
}
