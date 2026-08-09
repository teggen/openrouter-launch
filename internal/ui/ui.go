// Package ui renders the tables the CLI prints and the TUI draws. It exists
// so the two surfaces cannot drift apart: one border style, one palette,
// and (in status.go) one status vocabulary, in one place.
package ui

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// MaxTableWidth is the width the CLI listings are capped at.
//
// A constant, not the terminal's real width: reading that would mean a
// direct golang.org/x/term dependency, which internal/tui/program.go
// already declined by name. text/tabwriter never queried the width either,
// so a terminal narrower than this soft-wraps exactly as it did before.
const MaxTableWidth = 100

// Role is what a cell MEANS. Callers pick roles and Theme decides how a
// role looks, so the palette lives in exactly one place.
type Role int

const (
	RolePlain Role = iota
	RoleAccent
	RoleDim
	RoleOK
	RoleWarn
	RoleBad
)

// Theme carries a lipgloss renderer bound to one destination writer.
//
// Binding to the destination rather than to os.Stdout is what makes color
// correct in every case at once: a *bytes.Buffer (every test) and a pipe
// both resolve to termenv.Ascii and emit nothing, a real terminal gets its
// own profile, and NO_COLOR is honored by termenv underneath.
type Theme struct {
	styles map[Role]lipgloss.Style
	header lipgloss.Style
}

// NewTheme builds a theme whose color is detected from w.
func NewTheme(w io.Writer) *Theme { return newTheme(lipgloss.NewRenderer(w)) }

// newTheme is the seam tests use to force a color profile; see colorTheme
// in ui_test.go. A buffer always resolves to Ascii, so without it the
// role-to-color mapping could not be observed at all.
func newTheme(r *lipgloss.Renderer) *Theme {
	return &Theme{
		header: r.NewStyle().Bold(true),
		styles: map[Role]lipgloss.Style{
			RolePlain:  r.NewStyle(),
			RoleAccent: r.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "39"}),
			RoleDim:    r.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "242", Dark: "246"}),
			RoleOK:     r.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "42"}),
			RoleWarn:   r.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"}),
			RoleBad:    r.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"}),
		},
	}
}

// Style exposes a role's style so callers rendering something other than a
// table cell — the TUI's title, footer, and notices — draw from the same
// palette instead of redeclaring colors.
func (th *Theme) Style(r Role) lipgloss.Style { return th.styles[r] }

// Table is a data-only description of a table.
type Table struct {
	Headers []string
	Rows    [][]string
	// Role reports a body cell's role. lipgloss calls it more than once per
	// cell (a measure pass and a render pass), so it MUST have no side
	// effects. nil means RolePlain everywhere.
	Role func(row, col int) Role
	// Emphasis bolds a whole row on top of each cell's own role, rather
	// than replacing it — that is how the TUI's selected row keeps its
	// status color while still standing out.
	Emphasis func(row int) bool
	// MaxWidth caps the rendered width. 0 means no cap.
	MaxWidth int
}

// Render draws t.
//
// Applying MaxWidth takes two passes on purpose. lipgloss's table.Width is
// a TARGET, not a maximum — it expands a narrower table to exactly that
// width — so the table is built at its natural width first and only
// re-rendered capped when it genuinely overflows.
//
// Row rules appear exactly when the cap binds. That is when cells wrap to
// several lines, and without a rule two multi-line rows read as one block.
// Tying the rule to the condition that motivates it removes a knob that
// could only ever be set one way.
func (th *Theme) Render(t Table) string {
	natural := th.build(t, false).String()
	if t.MaxWidth <= 0 || lipgloss.Width(natural) <= t.MaxWidth {
		return natural
	}
	return th.build(t, true).Width(t.MaxWidth).String()
}

func (th *Theme) build(t Table, rules bool) *table.Table {
	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(th.styles[RoleDim]).
		BorderRow(rules).
		Headers(t.Headers...).
		Rows(t.Rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return th.header.Padding(0, 1)
			}
			role := RolePlain
			if t.Role != nil {
				role = t.Role(row, col)
			}
			style := th.styles[role].Padding(0, 1)
			if t.Emphasis != nil && t.Emphasis(row) {
				style = style.Bold(true)
			}
			return style
		})
}
