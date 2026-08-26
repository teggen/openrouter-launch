package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/teggen/agentlaunch/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/ui"
)

// rootInput is what the root screen renders.
type rootInput struct {
	Profiles []config.Profile
	Agents   []*agent.Spec
	// Installed reports whether an agent's binary is present. It is injected
	// rather than calling the registry directly so tests never depend on
	// what is installed on the machine running them.
	Installed func(*agent.Spec) bool
	// Lookup resolves the agent a profile row names. It is injected for the
	// same reason Installed is, and it used to be missing: this screen called
	// agent.Lookup on the package-level registry while the very next line
	// consulted the injected Installed, so a test that supplied its own
	// agents still resolved profile rows against whatever the real registry
	// held. nil resolves nothing, leaving profile rows without a status.
	Lookup func(string) (*agent.Spec, error)
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
			if in.Lookup != nil {
				if spec, err := in.Lookup(p.Agent); err == nil {
					row.spec, row.installed = spec, installed(spec)
				}
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
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}

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

// View renders as many rows as fit, centered on the cursor.
//
// It MEASURES rather than predicts. The rendered height depends on how many
// table frames land inside the window — each costs a top border, a header
// row, and a header rule — and that depends on where the window starts, so
// a hand-computed chrome constant would have to encode a number that
// changes as the cursor moves. Landmine 17 exists because someone recounted
// the picker's chrome lines by hand and got 8 where the renderer's
// arithmetic says 9; this shrinks the window until the real output fits.
//
// Before the first WindowSizeMsg the height is unknown, so everything
// renders rather than nothing — the picker's defaultListHeight posture.
func (m rootModel) View() string {
	if m.height <= 0 {
		return m.render(0, len(m.rows))
	}
	for n := len(m.rows); n >= 1; n-- {
		start := m.windowStart(n)
		if out := m.render(start, start+n); lipgloss.Height(out) <= m.height {
			return out
		}
	}
	return m.render(m.cursor, m.cursor+1)
}

// windowStart centers an n-row window on the cursor and clamps it to the
// list.
//
// The window is derived from the cursor alone — there is no stored scroll
// offset — so View stays a pure function of the model and no second piece
// of state can desynchronize from it. The cost is that the list scrolls by
// one on every move once the cursor is past the middle, rather than only
// when it would leave the window; for a screen this size that is not worth
// a state variable.
func (m rootModel) windowStart(n int) int {
	start := m.cursor - (n-1)/2
	if last := len(m.rows) - n; start > last {
		start = last
	}
	if start < 0 {
		start = 0
	}
	return start
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

	hints := []string{"↑/↓ move", "enter select", "esc quit"}
	if start > 0 || end < len(m.rows) {
		// Only when something is off screen, so a scrolled screen is never
		// silently truncated — and an unscrolled one carries no noise.
		hints = append(hints, fmt.Sprintf("%d-%d of %d",
			countSelectable(m.rows[:start])+1,
			countSelectable(m.rows[:end]),
			countSelectable(m.rows)))
	}
	// A wrapped footer costs rows, but View measures its own output and
	// shrinks the window until it fits, so nothing here needs a budget.
	for _, line := range hintLines(hints, m.width-2) {
		b.WriteString(dimStyle.Render("  "+line) + "\n")
	}
	return b.String()
}

// countSelectable counts the rows a user can land on, so the range
// indicator counts items rather than including section headers.
func countSelectable(rows []rootRow) int {
	n := 0
	for _, r := range rows {
		if r.selectable {
			n++
		}
	}
	return n
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
