package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

const (
	// defaultListHeight is used before the first WindowSizeMsg arrives, so
	// the picker renders something rather than nothing on its first frame.
	defaultListHeight = 10
	// chromeHeight is the non-list part of the view: title, blank lines, the
	// fixed description pane, the status line, and the key footer.
	chromeHeight = 10
	// descriptionHeight is fixed on purpose. See descriptionLines.
	descriptionHeight = 2
)

type pickerInput struct {
	Agent   *agent.Spec
	Models  []openrouter.Model
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
)

type pickerChoice struct {
	Kind pickerChoiceKind
	// ModelID is the highlighted model, for pickModel and pickSaveProfile.
	ModelID string
	// Filters is the live filter state, returned on every exit — including
	// pickBack — because the driver persists filters whether or not the
	// session went on to launch.
	Filters filterState
}

type pickerModel struct {
	agent   *agent.Spec
	all     []openrouter.Model
	filters filterState
	visible []openrouter.Model
	cursor  int
	offset  int
	width   int
	height  int
	choice  pickerChoice
	done    bool
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
// The order matters: the four catalog filters run through openrouter.Apply,
// and only then does the search run through Rank, which orders by match
// quality. Letting Apply also match on Search would narrow the list with
// plain substring semantics before Rank ever saw it.
func (m *pickerModel) recompute(keepID string) {
	m.visible = Rank(openrouter.Apply(m.all, m.filters.catalogFilter()), m.filters.search)
	m.cursor = indexOfModel(m.visible, keepID)
	m.clampScroll()
}

func indexOfModel(models []openrouter.Model, id string) int {
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
	if h := m.height - chromeHeight; h > 0 {
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

func (m pickerModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Switching on String() first is load-bearing. bubbletea renders an
	// alt-modified rune as "alt+t", a distinct case from "t", so a filter
	// chord can never fall through to the search-append branch at the bottom
	// of this function. Matching on key.Type == tea.KeyRunes first would type
	// a "t" into the search box on every alt+t.
	switch key.String() {
	case "esc", "ctrl+c":
		m.choice = pickerChoice{Kind: pickBack, Filters: m.filters}
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

	// Filter chords keep the highlighted model when it survives the change,
	// so toggling a filter to compare two models does not lose your place.
	case "alt+t":
		m.filters.toolsOnly = !m.filters.toolsOnly
		m.recompute(m.currentID())
		return m, nil
	case "alt+f":
		m.filters.freeOnly = !m.filters.freeOnly
		m.recompute(m.currentID())
		return m, nil
	case "alt+c":
		m.filters.minContext = nextContext(m.filters.minContext)
		m.recompute(m.currentID())
		return m, nil
	case "alt+p":
		m.filters.maxPrice = nextPrice(m.filters.maxPrice)
		m.recompute(m.currentID())
		return m, nil

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

func (m pickerModel) View() string {
	var b strings.Builder

	name := ""
	if m.agent != nil {
		name = m.agent.Launcher.DisplayName()
	}
	b.WriteString(titleStyle.Render("Model for "+name) + "    " +
		dimStyle.Render("search: "+m.filters.search) + "\n\n")

	// The window is always listHeight rows, padded with blanks, so the panes
	// below it never move.
	h := m.listHeight()
	for i := 0; i < h; i++ {
		idx := m.offset + i
		if idx >= len(m.visible) {
			b.WriteString("\n")
			continue
		}
		row := modelRow(m.visible[idx])
		if idx == m.cursor {
			b.WriteString("  " + cursorGutter(true) + selectedStyle.Render(row) + "\n")
		} else {
			b.WriteString("  " + cursorGutter(false) + row + "\n")
		}
	}

	b.WriteString("\n")
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

	b.WriteString("\n  " + m.filters.label() + "    " +
		dimStyle.Render(fmt.Sprintf("%d of %d models", len(m.visible), len(m.all))) + "\n")
	b.WriteString(dimStyle.Render(
		"  alt+t tools · alt+f free · alt+c ctx · alt+p price · ctrl+s save profile · esc back") + "\n")

	return b.String()
}
