package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/ui"
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
	kind  rowKind
	label string
	// selectable is false for section headers and for agents that cannot be
	// pointed at OpenRouter, so the cursor can never land on a row that would
	// do nothing.
	selectable bool
	profile    config.Profile
	agent      *agent.Spec
	// spec is a profile row's agent, nil when the profile names one that is
	// no longer registered; installed is that agent's install state. Both
	// are resolved once in buildRootRows rather than in View, which runs on
	// every keystroke and must stay free of IO.
	spec      *agent.Spec
	installed bool
}

type rootModel struct {
	rows   []rootRow
	cursor int
	// width and height come from tea.WindowSizeMsg. Both are 0 until the
	// first one arrives; see View for what that means.
	width  int
	height int
	choice rootChoice
	done   bool
}

func newRootModel(in rootInput) rootModel {
	rows := buildRootRows(in)
	return rootModel{rows: rows, cursor: initialCursor(rows, in.LastAgent)}
}

func buildRootRows(in rootInput) []rootRow {
	installed := func(s *agent.Spec) bool { return in.Installed == nil || in.Installed(s) }

	var rows []rootRow

	if len(in.Profiles) > 0 {
		rows = append(rows, rootRow{kind: rowHeader, label: "Profiles"})
		for _, p := range in.Profiles {
			row := rootRow{kind: rowProfile, label: p.Name, selectable: true, profile: p}
			// A profile can name an agent that is no longer registered —
			// only from a hand-edited config, since profile add validates
			// the name, but the status column is the only place that
			// failure surfaces before launch time.
			if spec, err := agent.Lookup(p.Agent); err == nil {
				row.spec, row.installed = spec, installed(spec)
			}
			rows = append(rows, row)
		}
	}

	rows = append(rows, rootRow{kind: rowHeader, label: "Agents"})
	for _, spec := range in.Agents {
		// An agent that cannot be pointed at OpenRouter is not listed at
		// all. This screen exists to pick something to launch, so an
		// unselectable row carrying a long reason was noise; the reason is
		// still reported by `openrouter-launch <agent>`, which reaches the
		// UnsupportedAgentError notice through a path this does not touch.
		if !spec.Status.Supported {
			continue
		}
		// Uninstalled agents stay selectable: Plan checks the empty model
		// before the install guard, so a user with nothing installed can
		// still browse the catalog and see what they would be launching.
		rows = append(rows, rootRow{
			kind: rowAgent, label: spec.Launcher.DisplayName(), agent: spec,
			selectable: true, installed: installed(spec),
		})
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
	return m.render(0, len(m.rows))
}

// render draws rows[start:end].
//
// buildRootRows stays the single row list, section headers included, so
// initialCursor, move, and every key-handling test are untouched. render
// walks the window, emits a label when it meets a header row, and collects
// each run of consecutive profile or agent rows into one table — so a
// window starting mid-agents simply draws neither the profiles table nor
// its label.
func (m rootModel) render(start, end int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("openrouter-launch") + "\n\n")

	for i := start; i < end; {
		if m.rows[i].kind == rowHeader {
			// A blank line before every label but the first, so a table's
			// bottom border does not butt straight into the next heading.
			if i > start {
				b.WriteString("\n")
			}
			b.WriteString(headerStyle.Render("  "+strings.ToUpper(m.rows[i].label)) + "\n")
			i++
			continue
		}
		run := i
		for i < end && m.rows[i].kind == m.rows[run].kind {
			i++
		}
		b.WriteString(indent(m.sectionTable(run, i)) + "\n")
	}

	b.WriteString(dimStyle.Render("  ↑/↓ move · enter select · esc quit") + "\n")
	return b.String()
}

// sectionTable renders rows[from:to], which are all of one kind.
func (m rootModel) sectionTable(from, to int) string {
	profiles := m.rows[from].kind == rowProfile

	headers := []string{" ", "NAME", "AGENT", "STATUS"}
	if profiles {
		headers = []string{" ", "NAME", "AGENT", "STATUS", "MODEL"}
	}

	var (
		rows  [][]string
		roles []ui.Role
	)
	for i := from; i < to; i++ {
		row := m.rows[i]
		marker := cursorCell(i == m.cursor)

		if profiles {
			status, role := ui.UnknownAgentStatus()
			if row.spec != nil {
				status, role = ui.AgentStatus(row.spec, row.installed)
			}
			rows = append(rows, []string{
				marker, row.profile.Name, row.profile.Agent, status, row.profile.Model,
			})
			roles = append(roles, role)
			continue
		}

		status, role := ui.AgentStatus(row.agent, row.installed)
		rows = append(rows, []string{
			marker, row.agent.Name, row.agent.Launcher.DisplayName(), status,
		})
		roles = append(roles, role)
	}

	selected := m.cursor - from
	const statusCol = 3
	return theme.Render(ui.Table{
		Headers:  headers,
		Rows:     rows,
		MaxWidth: m.width,
		Emphasis: func(row int) bool { return row == selected },
		Role: func(row, col int) ui.Role {
			if row < 0 || row >= len(roles) {
				return ui.RolePlain
			}
			switch col {
			case 1:
				return ui.RoleAccent
			case statusCol:
				return roles[row]
			default:
				return ui.RolePlain
			}
		},
	})
}

// indent shifts a rendered table right by two columns, lining it up with
// the title, the section labels, and the footer.
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}
