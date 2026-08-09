package ui

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// colorTheme forces a color profile onto a buffer-bound renderer. Without
// this the role-to-color mapping is untestable: every buffer resolves to
// termenv.Ascii, so a theme that painted every role identically would look
// exactly like a correct one.
func colorTheme(w io.Writer) *Theme {
	r := lipgloss.NewRenderer(w)
	r.SetColorProfile(termenv.TrueColor)
	r.SetHasDarkBackground(true)
	return newTheme(r)
}

func simple() Table {
	return Table{
		Headers: []string{"NAME", "STATUS"},
		Rows:    [][]string{{"claude", "installed"}, {"droid", "not installed"}},
	}
}

func TestRenderDrawsARoundedBorderedTable(t *testing.T) {
	got := NewTheme(&bytes.Buffer{}).Render(simple())

	for _, want := range []string{"╭", "╮", "╰", "╯", "│", "├", "┼", "NAME", "claude"} {
		if !strings.Contains(got, want) {
			t.Errorf("Render is missing %q:\n%s", want, got)
		}
	}
}

// Every CLI test writes into a *bytes.Buffer, and every piped invocation
// writes into a pipe. Both must come out clean, or the tool spews escape
// codes into files and greps.
func TestRenderEmitsNoEscapesWhenTheDestinationIsNotATerminal(t *testing.T) {
	tbl := simple()
	tbl.Role = func(int, int) Role { return RoleOK }
	tbl.Emphasis = func(int) bool { return true }

	if got := NewTheme(&bytes.Buffer{}).Render(tbl); strings.Contains(got, "\x1b") {
		t.Errorf("Render emitted ANSI escapes to a non-terminal:\n%q", got)
	}
}

// The complement: when color IS available, different roles must actually
// look different. Mutation check — point RoleOK and RoleBad at the same
// style and this fails.
func TestRenderPaintsDistinctRolesDistinctly(t *testing.T) {
	tbl := simple()
	tbl.Role = func(row, col int) Role {
		if row == 0 {
			return RoleOK
		}
		return RoleBad
	}

	got := colorTheme(&bytes.Buffer{}).Render(tbl)
	if !strings.Contains(got, "\x1b") {
		t.Fatalf("forced color profile produced no escapes:\n%q", got)
	}

	ok, bad := escapesOnLineWith(got, "claude"), escapesOnLineWith(got, "droid")
	if ok == "" || bad == "" {
		t.Fatalf("a role produced no escape at all: ok=%q bad=%q", ok, bad)
	}
	if ok == bad {
		t.Errorf("RoleOK and RoleBad rendered identically (%q)", ok)
	}
}

func TestEmphasisBoldsOnlyTheChosenRow(t *testing.T) {
	tbl := simple()
	tbl.Emphasis = func(row int) bool { return row == 1 }

	got := colorTheme(&bytes.Buffer{}).Render(tbl)
	const bold = "\x1b[1m"
	if strings.Contains(lineWith(got, "claude"), bold) {
		t.Errorf("unemphasized row is bold:\n%q", lineWith(got, "claude"))
	}
	if !strings.Contains(lineWith(got, "droid"), bold) {
		t.Errorf("emphasized row is not bold:\n%q", lineWith(got, "droid"))
	}
}

// The cap must bind on overflow...
func TestMaxWidthCapsAnOverflowingTable(t *testing.T) {
	tbl := simple()
	tbl.Rows = [][]string{{"claude", strings.Repeat("verylongword ", 20)}}
	tbl.MaxWidth = 60

	for _, line := range strings.Split(NewTheme(&bytes.Buffer{}).Render(tbl), "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line is %d columns, want <= 60:\n%s", w, line)
		}
	}
}

// ...and must NOT expand a table that is already under it. table.Width is a
// TARGET in lipgloss, not a maximum: passing MaxWidth straight through
// stretches a 30-column table to 100. Measured, not assumed.
func TestMaxWidthDoesNotExpandATableUnderTheCap(t *testing.T) {
	th := NewTheme(&bytes.Buffer{})
	natural := lipgloss.Width(th.Render(simple()))

	tbl := simple()
	tbl.MaxWidth = 100
	if got := lipgloss.Width(th.Render(tbl)); got != natural {
		t.Errorf("capped width = %d, want the natural %d — MaxWidth expanded the table", got, natural)
	}
}

// Row rules exist because a wrapped cell makes two rows one visual block.
// They must appear exactly when the cap binds, and not otherwise.
func TestRowRulesAppearOnlyWhenTheCapBinds(t *testing.T) {
	th := NewTheme(&bytes.Buffer{})

	// One "├" is the rule under the header; more than one means rules
	// between body rows.
	under := simple()
	under.MaxWidth = 100
	if got := strings.Count(th.Render(under), "├"); got > 1 {
		t.Errorf("row rules drawn on a table under its cap (%d rules):\n%s", got, th.Render(under))
	}

	over := simple()
	over.Rows = [][]string{{"a", strings.Repeat("word ", 40)}, {"b", strings.Repeat("word ", 40)}}
	over.MaxWidth = 50
	if got := strings.Count(th.Render(over), "├"); got < 2 {
		t.Errorf("row rules missing on a wrapped table (%d rules):\n%s", got, th.Render(over))
	}
}

// lineWith returns the first line containing want, or "".
func lineWith(out, want string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	return ""
}

// escapesOnLineWith returns the escape sequences on the first line
// containing want, with the visible text removed.
func escapesOnLineWith(out, want string) string {
	line := lineWith(out, want)
	var b strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] != '\x1b' {
			continue
		}
		j := i
		for j < len(line) && line[j] != 'm' {
			j++
		}
		b.WriteString(line[i:min(j+1, len(line))])
		i = j
	}
	return b.String()
}
