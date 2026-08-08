package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
)

// rootInput is what the root screen renders.
type rootInput struct {
	Profiles []config.Profile
	Agents   []*agent.Spec
	// Installed reports whether an agent's binary is present. It is injected
	// rather than calling agent.Installed directly so tests never depend on
	// what is installed on the machine running them.
	Installed func(*agent.Spec) bool
	// LastAgent preselects a row.
	LastAgent string
}

type choiceKind int

const (
	choiceCancel choiceKind = iota
	choiceProfile
	choiceAgent
)

type rootChoice struct {
	Kind    choiceKind
	Profile config.Profile
	Agent   *agent.Spec
}

type rowKind int

const (
	rowHeader rowKind = iota
	rowProfile
	rowAgent
)

type rootRow struct {
	kind   rowKind
	label  string
	detail string
	// selectable is false for section headers and for agents that cannot be
	// pointed at OpenRouter, so the cursor can never land on a row that would
	// do nothing.
	selectable bool
	profile    config.Profile
	agent      *agent.Spec
}

type rootModel struct {
	rows   []rootRow
	cursor int
	choice rootChoice
	done   bool
}

func newRootModel(in rootInput) rootModel {
	rows := buildRootRows(in)
	return rootModel{rows: rows, cursor: initialCursor(rows, in.LastAgent)}
}

func buildRootRows(in rootInput) []rootRow {
	var rows []rootRow

	if len(in.Profiles) > 0 {
		rows = append(rows, rootRow{kind: rowHeader, label: "Profiles"})
		for _, p := range in.Profiles {
			rows = append(rows, rootRow{
				kind: rowProfile, label: p.Name,
				detail: p.Agent + " · " + p.Model, selectable: true, profile: p,
			})
		}
	}

	rows = append(rows, rootRow{kind: rowHeader, label: "Agents"})
	for _, spec := range in.Agents {
		row := rootRow{kind: rowAgent, label: spec.Launcher.DisplayName(), agent: spec}
		switch {
		case !spec.Status.Supported:
			row.detail = "unsupported: " + spec.Status.Reason
		case in.Installed != nil && !in.Installed(spec):
			// Still selectable: Plan checks the empty model before the
			// install guard so a user with nothing installed can still
			// browse the catalog and see what they would be launching.
			row.detail = "not installed"
			row.selectable = true
		default:
			row.detail = "installed"
			row.selectable = true
		}
		rows = append(rows, row)
	}
	return rows
}

// initialCursor preselects last_agent, falling back to the first selectable
// row. A profile is never auto-selected over a named last agent: selecting a
// profile launches immediately, so preselecting one would put an irreversible
// action one keystroke away from a screen the user just opened.
func initialCursor(rows []rootRow, lastAgent string) int {
	if lastAgent != "" {
		for i, r := range rows {
			if r.kind == rowAgent && r.selectable && r.agent.Name == lastAgent {
				return i
			}
		}
	}
	for i, r := range rows {
		if r.selectable {
			return i
		}
	}
	return 0
}

func (m rootModel) Init() tea.Cmd { return nil }

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "esc", "ctrl+c", "q":
		m.choice = rootChoice{Kind: choiceCancel}
		m.done = true
		return m, tea.Quit
	case "up", "k", "ctrl+p":
		m.move(-1)
		return m, nil
	case "down", "j", "ctrl+n":
		m.move(1)
		return m, nil
	case "enter":
		if m.cursor < 0 || m.cursor >= len(m.rows) {
			return m, nil
		}
		row := m.rows[m.cursor]
		if !row.selectable {
			return m, nil
		}
		if row.kind == rowProfile {
			m.choice = rootChoice{Kind: choiceProfile, Profile: row.profile}
		} else {
			m.choice = rootChoice{Kind: choiceAgent, Agent: row.agent}
		}
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

// move advances the cursor by delta, skipping headers and unselectable rows
// and stopping at both ends rather than wrapping.
func (m *rootModel) move(delta int) {
	for i := m.cursor + delta; i >= 0 && i < len(m.rows); i += delta {
		if m.rows[i].selectable {
			m.cursor = i
			return
		}
	}
}

func (m rootModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("openrouter-launch") + "\n\n")

	for i, row := range m.rows {
		if row.kind == rowHeader {
			b.WriteString("\n" + headerStyle.Render("  "+row.label) + "\n")
			continue
		}

		line := row.label
		if row.detail != "" {
			line += "  " + row.detail
		}

		switch {
		case !row.selectable:
			b.WriteString("    " + dimStyle.Render(line) + "\n")
		case i == m.cursor:
			b.WriteString("  " + cursorGutter(true) + selectedStyle.Render(line) + "\n")
		default:
			b.WriteString("  " + cursorGutter(false) + line + "\n")
		}
	}

	b.WriteString("\n" + dimStyle.Render("  ↑/↓ move · enter select · esc quit") + "\n")
	return b.String()
}
