# Listing Tables Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render every listing this tool prints — `orl agents` (with and without `--all`), `orl profile list`, `orl models`, and the TUI root screen — as bordered tables with a dedicated status column, shared styling, and color auto-detected from the destination.

**Architecture:** A new `internal/ui` package owns the border style, the palette, and the status vocabulary; `internal/cli` and `internal/tui` both render through it so they cannot drift. `ui.Render` is a pure function from `[][]string` to a string, which is what lets tests feed adversarial content instead of whatever the registry happens to hold. The TUI root screen gains a measured (not predicted) scroll window, because boxing takes it past a 24-line terminal.

**Tech Stack:** Go 1.25, `lipgloss v1.1.0` + its `lipgloss/table` subpackage (already required — no new module), `github.com/muesli/termenv` promoted from indirect to direct, cobra, bubbletea.

**Spec:** `docs/superpowers/specs/2026-08-09-listing-tables-design.md` — read it for *why*.

## Global Constraints

- **Read `HANDOFF.md`'s Landmines before changing anything.** Landmines 13 (tui imports), 16 (screen-closure tests must drive a real program), and 17 (never hand-count chrome lines) are all live in this change.
- **No new module dependencies.** `lipgloss/table` ships inside `lipgloss v1.1.0`. `github.com/muesli/termenv` moves from the indirect to the direct block in `go.mod` — it is already in the tree.
- **`golang.org/x/term` must NOT be added.** `internal/tui/program.go` declined it by name; the CLI table cap is the constant `ui.MaxTableWidth = 100`, not the terminal's width.
- **`internal/tui` must not import `internal/cli`, cobra, or pflag** (Landmine 13, pinned by `TestTUIDependsOnNeitherCLINorCobra`). `internal/ui` must not either.
- **No new write sites.** This change writes no files. `TestWriteSitesAreExhaustivelyEnumerated` must stay green untouched (Landmine 6).
- **Every test gets its mutation check**: break the behavior, watch *that named test* fail, revert. This project's recurring defect is tests that pass for the wrong reason.
- **Status is never carried by color alone.** The `✓` / `✗` / `⚠` glyph is always emitted.
- Run `make pre-commit` before each commit; `make ci` once at the end.

## File Structure

| File | Responsibility |
|---|---|
| `internal/ui/ui.go` (new) | `Role`, `Theme`, `Table`, `Render`, `MaxTableWidth` — the border style and palette |
| `internal/ui/ui_test.go` (new) | Render behavior: borders, color detection, the width cap, row rules |
| `internal/ui/status.go` (new) | `AgentStatus`, `UnknownAgentStatus` — the one status vocabulary |
| `internal/ui/status_test.go` (new) | One case per vocabulary row |
| `internal/cli/agents.go` | `agentsTable` (pure) + the command |
| `internal/cli/profile.go` | `profilesTable` (pure) + the `list` command |
| `internal/cli/models.go` | `modelsTable` (pure) + the command |
| `internal/cli/harness_test.go` | `tableRows` helper — parses a rendered table back into cells |
| `internal/tui/style.go` | package `theme`, existing styles re-sourced from `ui` |
| `internal/tui/root.go` | `View` renders tables; `Update` handles `tea.WindowSizeMsg`; scroll window |
| `internal/tui/root_test.go` | Updated label assertions + new layout/scroll tests |

---

### Task 1: `internal/ui` — Theme, Role, Table, Render

**Files:**
- Create: `internal/ui/ui.go`
- Test: `internal/ui/ui_test.go`
- Modify: `go.mod` (promote `github.com/muesli/termenv` to the direct require block)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `ui.MaxTableWidth` (untyped const `100`); `type Role int` with `RolePlain, RoleAccent, RoleDim, RoleOK, RoleWarn, RoleBad`; `func NewTheme(w io.Writer) *Theme`; `func (*Theme) Style(Role) lipgloss.Style`; `type Table struct { Headers []string; Rows [][]string; Role func(row, col int) Role; Emphasis func(row int) bool; MaxWidth int }`; `func (*Theme) Render(Table) string`.

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/ui_test.go`:

```go
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
	if lineWith(got, "claude") == "" || strings.Contains(lineWith(got, "claude"), bold) {
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

	under := simple()
	under.MaxWidth = 100
	if strings.Contains(th.Render(under), "├") && strings.Count(th.Render(under), "├") > 1 {
		t.Errorf("row rules drawn on a table under its cap:\n%s", th.Render(under))
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/ -v`
Expected: FAIL — the package does not exist (`no Go files in .../internal/ui`).

- [ ] **Step 3: Write the implementation**

Create `internal/ui/ui.go`:

```go
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
```

- [ ] **Step 4: Promote termenv to a direct dependency**

Run: `go mod tidy && go build ./...`
Expected: `github.com/muesli/termenv v0.16.0` moves out of the `// indirect` block. `git diff go.mod` must show **only** that move — no version changes, no new modules.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, 7 tests.

- [ ] **Step 6: Run the mutation checks**

Each of these must fail the *named* test. Apply it, run, confirm the failure, revert.

| Mutation | Must fail |
|---|---|
| In `newTheme`, give `RoleBad` the same `Foreground` as `RoleOK` | `TestRenderPaintsDistinctRolesDistinctly` |
| In `Render`, replace the body with `return th.build(t, false).Width(t.MaxWidth).String()` | `TestMaxWidthDoesNotExpandATableUnderTheCap` |
| In `Render`, drop the `MaxWidth` branch and always return `natural` | `TestMaxWidthCapsAnOverflowingTable` |
| In `Render`, pass `true` for `rules` on both paths | `TestRowRulesAppearOnlyWhenTheCapBinds` |
| In `build`, drop the `Emphasis` branch | `TestEmphasisBoldsOnlyTheChosenRow` |
| In `NewTheme`, use `lipgloss.DefaultRenderer()` instead of one bound to `w` | `TestRenderEmitsNoEscapesWhenTheDestinationIsNotATerminal` — **note:** under `go test` stdout is already a pipe, so confirm this one fails by temporarily forcing `TrueColor` in `NewTheme`; record in the commit message that the binding is what the test pins |

- [ ] **Step 7: Commit**

```bash
git add internal/ui/ go.mod go.sum
git commit -m "feat(ui): add the shared table renderer

Border style, palette, and a pure Render so both surfaces draw the same
tables. MaxWidth is applied in two passes because lipgloss's table.Width
is a target, not a maximum, and row rules appear exactly when the cap
binds, which is when cells wrap."
```

---

### Task 2: `internal/ui` — the status vocabulary

**Files:**
- Create: `internal/ui/status.go`
- Test: `internal/ui/status_test.go`

**Interfaces:**
- Consumes: `Role`, `RoleOK`, `RoleWarn`, `RoleBad` from Task 1.
- Produces: `func AgentStatus(spec *agent.Spec, installed bool) (string, Role)`; `func UnknownAgentStatus() (string, Role)`.

- [ ] **Step 1: Write the failing test**

Create `internal/ui/status_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
)

// stubLauncher satisfies agent.Launcher so a test can build a Spec.
// Spec.Launcher must never be nil (Landmine 10).
type stubLauncher struct{ name string }

func (s stubLauncher) Name() string        { return s.name }
func (s stubLauncher) DisplayName() string { return s.name }
func (s stubLauncher) Command(agent.Request) (agent.Command, error) {
	return agent.Command{}, nil
}

func spec(supported bool) *agent.Spec {
	return &agent.Spec{
		Name:     "x",
		Launcher: stubLauncher{name: "X"},
		Status:   agent.Status{Supported: supported, Reason: "because"},
	}
}

func TestAgentStatusVocabulary(t *testing.T) {
	cases := []struct {
		name      string
		spec      *agent.Spec
		installed bool
		wantText  string
		wantRole  Role
	}{
		{"installed", spec(true), true, "✓ installed", RoleOK},
		{"not installed", spec(true), false, "✗ not installed", RoleBad},
		// Unsupported wins over installed-ness: the binary may well be on
		// the machine, but it still cannot be pointed at OpenRouter.
		{"unsupported beats installed", spec(false), true, "⚠ unsupported", RoleWarn},
		{"unsupported", spec(false), false, "⚠ unsupported", RoleWarn},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, role := AgentStatus(c.spec, c.installed)
			if text != c.wantText {
				t.Errorf("text = %q, want %q", text, c.wantText)
			}
			if role != c.wantRole {
				t.Errorf("role = %v, want %v", role, c.wantRole)
			}
		})
	}
}

func TestUnknownAgentStatus(t *testing.T) {
	text, role := UnknownAgentStatus()
	if text != "⚠ unknown agent" {
		t.Errorf("text = %q, want %q", text, "⚠ unknown agent")
	}
	if role != RoleWarn {
		t.Errorf("role = %v, want RoleWarn", role)
	}
}

// Status must never depend on color alone: piped output, NO_COLOR, a dumb
// terminal, and a red/green-colorblind reader all lose the color and keep
// the glyph.
func TestEveryStatusCarriesAGlyph(t *testing.T) {
	texts := []string{}
	for _, s := range []*agent.Spec{spec(true), spec(false)} {
		for _, installed := range []bool{true, false} {
			text, _ := AgentStatus(s, installed)
			texts = append(texts, text)
		}
	}
	unknown, _ := UnknownAgentStatus()
	texts = append(texts, unknown)

	for _, text := range texts {
		if !strings.ContainsAny(text, "✓✗⚠") {
			t.Errorf("status %q carries no glyph, so it depends on color alone", text)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ui/ -run 'Status|Glyph' -v`
Expected: FAIL — `undefined: AgentStatus`, `undefined: UnknownAgentStatus`.

- [ ] **Step 3: Write the implementation**

Create `internal/ui/status.go`:

```go
package ui

import "github.com/teggen/openrouter-launch/internal/agent"

// The glyph is ALWAYS emitted, so a status never depends on color alone —
// it survives NO_COLOR, a pipe, a dumb terminal, and a reader who cannot
// tell red from green.
const (
	glyphOK   = "✓"
	glyphBad  = "✗"
	glyphWarn = "⚠"
)

// AgentStatus is the single source of the status vocabulary. Both the CLI
// listing and the TUI root screen call it, so the two cannot disagree about
// what a state is called or how it is colored.
//
// Unsupported outranks installed-ness on purpose: the binary may be present,
// but an agent that cannot be pointed at OpenRouter still cannot be launched
// by this tool, and reporting it as "installed" would be a wrong claim.
func AgentStatus(spec *agent.Spec, installed bool) (string, Role) {
	switch {
	case !spec.Status.Supported:
		return glyphWarn + " unsupported", RoleWarn
	case installed:
		return glyphOK + " installed", RoleOK
	default:
		return glyphBad + " not installed", RoleBad
	}
}

// UnknownAgentStatus is the cell for a saved profile naming an agent that is
// not in the registry. It is reachable only from a hand-edited config or an
// agent dropped between releases — `profile add` validates the name — but
// without it that failure stays invisible until launch time.
func UnknownAgentStatus() (string, Role) { return glyphWarn + " unknown agent", RoleWarn }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, 10 tests.

- [ ] **Step 5: Run the mutation checks**

| Mutation | Must fail |
|---|---|
| Swap `RoleOK` and `RoleBad` in `AgentStatus` | `TestAgentStatusVocabulary/installed` |
| Move the `!spec.Status.Supported` case below `case installed` | `TestAgentStatusVocabulary/unsupported_beats_installed` |
| Drop the glyph from any one return | `TestEveryStatusCarriesAGlyph` |

- [ ] **Step 6: Commit**

```bash
git add internal/ui/status.go internal/ui/status_test.go
git commit -m "feat(ui): add the shared agent status vocabulary

One place decides what a state is called and colored, so the CLI listing
and the TUI root screen cannot drift. Unsupported outranks installed-ness:
the binary can be present and still not be launchable against OpenRouter."
```

---

### Task 3: `orl agents` and `orl agents --all`

**Files:**
- Modify: `internal/cli/agents.go` (whole file)
- Modify: `internal/cli/harness_test.go` (add `tableRows`)
- Test: `internal/cli/agents_test.go`

**Interfaces:**
- Consumes: `ui.NewTheme`, `ui.Table`, `ui.Render`, `ui.MaxTableWidth`, `ui.AgentStatus`, `ui.Role*` from Tasks 1–2.
- Produces: `func agentsTable(specs []*agent.Spec, installed func(*agent.Spec) bool, all bool) ui.Table`; test helper `func tableRows(t *testing.T, out string) [][]string`.

- [ ] **Step 1: Add the `tableRows` test helper**

Append to `internal/cli/harness_test.go` (and add `"strings"` to its imports):

```go
// tableRows parses a rendered ui.Table back into its cells, rejoining the
// lines of a wrapped cell. Row 0 is the header.
//
// Assertions go through this rather than matching substrings against the
// raw output. Once a cell can wrap, "does the output contain X" depends on
// where the wrap landed — which is not a property any of these tests means
// to assert, and which would make them fail for a cosmetic reason or, worse,
// pass because a phrase happened to survive intact.
//
// It assumes the first column of a row is never legitimately empty, which
// holds for every table here (NAME and MODEL are always set).
func tableRows(t *testing.T, out string) [][]string {
	t.Helper()

	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "│") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "│"), "│")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if len(rows) > 0 && cells[0] == "" {
			// A continuation line of a wrapped row.
			last := rows[len(rows)-1]
			for i := range cells {
				if i < len(last) && cells[i] != "" {
					last[i] = strings.TrimSpace(last[i] + " " + cells[i])
				}
			}
			continue
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		t.Fatalf("no table rows in output:\n%s", out)
	}
	return rows
}

// tableRow returns the body row whose first cell is name.
func tableRow(t *testing.T, out, name string) []string {
	t.Helper()
	for _, row := range tableRows(t, out)[1:] {
		if row[0] == name {
			return row
		}
	}
	t.Fatalf("no row named %q in output:\n%s", name, out)
	return nil
}
```

- [ ] **Step 2: Write the failing tests**

Replace `claudeStatusField` in `internal/cli/agents_test.go` with a call to `tableRow`, and update/add these tests. The imports become `strings`, `testing`, `github.com/teggen/openrouter-launch/internal/agent`, `github.com/teggen/openrouter-launch/internal/ui` (drop `regexp`).

```go
// wideLauncher is a stub whose description is far longer than any real
// agent's. Landmine 10: Spec.Launcher must never be nil.
type wideLauncher struct{}

func (wideLauncher) Name() string        { return "wide" }
func (wideLauncher) DisplayName() string { return "Wide Agent" }
func (wideLauncher) Command(agent.Request) (agent.Command, error) {
	return agent.Command{}, nil
}

func claudeStatusField(t *testing.T, out string) string {
	t.Helper()
	return tableRow(t, out, "claude")[2]
}

// TestAgentsOutputStaysNarrow pins the width cap, and it must be fed a
// SYNTHETIC spec to do so.
//
// The previous version rendered the live registry, whose longest
// description leaves the table at 94 columns. Once ui.Table carries a
// MaxWidth, deleting that cap would not have widened the real listing at
// all, so the test would have passed while testing nothing. A 200-character
// description is what makes the cap the only reason this passes.
func TestAgentsOutputStaysNarrow(t *testing.T) {
	specs := []*agent.Spec{{
		Name:        "wide",
		Launcher:    wideLauncher{},
		Description: strings.Repeat("extremely verbose description ", 7),
		Status:      agent.Status{Supported: true},
	}}

	out := ui.NewTheme(new(strings.Builder)).Render(
		agentsTable(specs, func(*agent.Spec) bool { return true }, false))

	for _, line := range strings.Split(out, "\n") {
		if n := len([]rune(line)); n > ui.MaxTableWidth {
			t.Errorf("line is %d columns, want <= %d:\n%s", n, ui.MaxTableWidth, line)
		}
	}
	// The cap must not have swallowed the row entirely.
	if !strings.Contains(out, "wide") {
		t.Errorf("capped table lost its row:\n%s", out)
	}
}

func TestAgentsCommandShowsInstalledWhenBinaryFound(t *testing.T) {
	spec, err := agent.Lookup("claude")
	if err != nil {
		t.Fatalf("lookup claude: %v", err)
	}
	claude := spec.Launcher.(*agent.Claude)
	prev := claude.LookPath
	claude.LookPath = func(string) (string, error) { return "/usr/local/bin/claude", nil }
	t.Cleanup(func() { claude.LookPath = prev })

	h := newHarness(t)
	if got := claudeStatusField(t, h.run(t, "agents")); got != "✓ installed" {
		t.Errorf("status = %q, want %q", got, "✓ installed")
	}
}

func TestAgentsCommandShowsNotInstalledWhenBinaryNotFound(t *testing.T) {
	spec, err := agent.Lookup("claude")
	if err != nil {
		t.Fatalf("lookup claude: %v", err)
	}
	claude := spec.Launcher.(*agent.Claude)
	prev := claude.LookPath
	claude.LookPath = func(string) (string, error) { return "", agent.ErrUnknownAgent }
	t.Cleanup(func() { claude.LookPath = prev })
	t.Setenv("HOME", t.TempDir()) // Landmine 8

	h := newHarness(t)
	if got := claudeStatusField(t, h.run(t, "agents")); got != "✗ not installed" {
		t.Errorf("status = %q, want %q", got, "✗ not installed")
	}
}

// The reason wraps across several lines inside its cell, so this asserts on
// the REASON column reconstructed by tableRows, not on a raw substring that
// only survives at one particular column width.
func TestAgentsAllShowsUnsupportedWithReason(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "agents", "--all")

	for _, name := range desktopApps {
		row := tableRow(t, out, name)
		if row[2] != "⚠ unsupported" {
			t.Errorf("%s status = %q, want %q", name, row[2], "⚠ unsupported")
		}
		if !strings.Contains(row[3], "desktop app authenticates through its own account") {
			t.Errorf("%s reason = %q, want the full reason", name, row[3])
		}
	}
}

// --all is where the 227-column line came from. It must now wrap instead.
func TestAgentsAllWrapsRatherThanWidening(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "agents", "--all")

	for _, line := range strings.Split(out, "\n") {
		if n := len([]rune(line)); n > ui.MaxTableWidth {
			t.Errorf("line is %d columns, want <= %d:\n%s", n, ui.MaxTableWidth, line)
		}
	}
	// A wrapped table must carry row rules, or the multi-line rows read as
	// one block. Two supported agents plus the header rule is the floor.
	if got := strings.Count(out, "├"); got < 2 {
		t.Errorf("wrapped table has %d row rules, want rules between rows:\n%s", got, out)
	}
}

// Every CLI test writes to a buffer, and so does every pipe. Escapes there
// would land in files and greps.
func TestListingsEmitNoEscapesWhenNotATerminal(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{"agents"}, {"agents", "--all"}} {
		if got := h.run(t, args...); strings.Contains(got, "\x1b") {
			t.Errorf("%v emitted ANSI escapes to a buffer:\n%q", args, got)
		}
	}
}
```

`TestAgentsCommandListsClaude` and `TestAgentsHidesUnsupportedByDefault` stay exactly as they are — box characters sit outside the cell text, so their substring assertions still hold.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run Agents -v`
Expected: FAIL — `undefined: agentsTable`, and the status assertions fail with `"installed"` where `"✓ installed"` is wanted.

- [ ] **Step 4: Write the implementation**

Replace `internal/cli/agents.go`:

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/ui"
)

func newAgentsCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "agents",
		Short: "List the agents this tool can launch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			_, err := fmt.Fprintln(out,
				ui.NewTheme(out).Render(agentsTable(agent.List(), agent.Installed, all)))
			return err
		},
	}

	cmd.Flags().BoolVar(&all, "all", false,
		"include agents that cannot be pointed at OpenRouter, with the reason")
	return cmd
}

// agentsTable builds the listing.
//
// It takes the specs and an installed-ness probe rather than reaching for
// agent.List/agent.Installed itself, so a test can render an adversarial
// spec — a 200-character description no real agent has — and watch the
// width cap do its job. Without that seam TestAgentsOutputStaysNarrow can
// only ever measure whatever the registry happens to contain, which is
// comfortably under the cap and would leave the test unable to fail.
func agentsTable(specs []*agent.Spec, installed func(*agent.Spec) bool, all bool) ui.Table {
	headers := []string{"NAME", "AGENT", "STATUS", "DESCRIPTION"}
	if all {
		headers = []string{"NAME", "AGENT", "STATUS", "REASON", "DESCRIPTION"}
	}

	var (
		rows  [][]string
		roles []ui.Role
	)
	for _, spec := range specs {
		// Unsupported agents (the Tier 3 desktop apps) are hidden by
		// default — the Phase 4a decision recorded in HANDOFF.md. They stay
		// registered and `openrouter-launch <agent>` still reports the
		// reason; this hides them from the listing, it does not
		// un-register them.
		if !spec.Status.Supported && !all {
			continue
		}
		status, role := ui.AgentStatus(spec, installed(spec))

		row := []string{spec.Name, spec.Launcher.DisplayName(), status, spec.Description}
		if all {
			row = []string{
				spec.Name, spec.Launcher.DisplayName(), status,
				spec.Status.Reason, spec.Description,
			}
		}
		rows = append(rows, row)
		roles = append(roles, role)
	}

	const statusCol = 2
	return ui.Table{
		Headers:  headers,
		Rows:     rows,
		MaxWidth: ui.MaxTableWidth,
		Role: func(row, col int) ui.Role {
			if row < 0 || row >= len(roles) {
				return ui.RolePlain
			}
			switch col {
			case 0:
				return ui.RoleAccent
			case statusCol:
				return roles[row]
			case 1:
				return ui.RolePlain
			default: // REASON and DESCRIPTION
				return ui.RoleDim
			}
		},
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -v`
Expected: PASS. Then eyeball the real thing:

```bash
go run . agents
go run . agents --all
go run . agents | cat        # must contain no escape codes
NO_COLOR=1 go run . agents
```

- [ ] **Step 6: Run the mutation checks**

| Mutation | Must fail |
|---|---|
| Drop `MaxWidth` from `agentsTable` | `TestAgentsOutputStaysNarrow` **and** `TestAgentsAllWrapsRatherThanWidening` |
| Return `ui.RolePlain` for `statusCol` | `TestRenderPaintsDistinctRolesDistinctly` will not catch this — instead confirm by eye that `agents` loses its green/red, then revert |
| Emit `spec.Description` in place of `spec.Status.Reason` in the `all` row | `TestAgentsAllShowsUnsupportedWithReason` |
| Drop the `!spec.Status.Supported && !all` filter | `TestAgentsHidesUnsupportedByDefault` |

- [ ] **Step 7: Commit**

```bash
git add internal/cli/agents.go internal/cli/agents_test.go internal/cli/harness_test.go
git commit -m "feat(cli): render the agents listing as a table

Adds a REASON column to --all and caps the table at 100 columns, which
replaces the 227-column line the three desktop-app reasons used to
produce. agentsTable takes its specs and install probe as arguments so
TestAgentsOutputStaysNarrow can feed a 200-character description; against
the live registry that test could not have failed."
```

---

### Task 4: `orl profile list`

**Files:**
- Modify: `internal/cli/profile.go` (`newProfileListCmd` + new `profilesTable`)
- Test: `internal/cli/profile_test.go`

**Interfaces:**
- Consumes: `ui.*` from Tasks 1–2, `tableRow` from Task 3.
- Produces: `func profilesTable(profiles []config.Profile, lookup func(string) (*agent.Spec, error), installed func(*agent.Spec) bool) ui.Table`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/cli/profile_test.go`:

```go
func TestProfileListShowsAgentInstallState(t *testing.T) {
	profiles := []config.Profile{{Name: "p1", Agent: "claude", Model: "anthropic/x"}}
	lookup := func(string) (*agent.Spec, error) {
		return &agent.Spec{
			Name: "claude", Launcher: wideLauncher{},
			Status: agent.Status{Supported: true},
		}, nil
	}

	out := ui.NewTheme(new(strings.Builder)).Render(
		profilesTable(profiles, lookup, func(*agent.Spec) bool { return false }))

	if got := tableRow(t, out, "p1")[2]; got != "✗ not installed" {
		t.Errorf("status = %q, want %q", got, "✗ not installed")
	}
}

// A profile naming an agent that is no longer registered. `profile add`
// validates the name, so this arrives only from a hand-edited config or an
// agent dropped between releases — and without this column the failure is
// invisible until you try to launch it.
func TestProfileListFlagsAnUnknownAgent(t *testing.T) {
	profiles := []config.Profile{{Name: "old", Agent: "vscode", Model: "openai/x"}}
	lookup := func(string) (*agent.Spec, error) { return nil, agent.ErrUnknownAgent }

	out := ui.NewTheme(new(strings.Builder)).Render(
		profilesTable(profiles, lookup, func(*agent.Spec) bool { return true }))

	if got := tableRow(t, out, "old")[2]; got != "⚠ unknown agent" {
		t.Errorf("status = %q, want %q", got, "⚠ unknown agent")
	}
}

func TestProfileListRendersNameAgentModelAndArgs(t *testing.T) {
	h := newHarness(t)
	h.run(t, "profile", "add", "--name", "opus-cc", "--agent", "claude",
		"--model", "anthropic/claude-opus-4.6", "--", "--resume")

	row := tableRow(t, h.run(t, "profile", "list"), "opus-cc")
	for i, want := range map[int]string{1: "claude", 3: "anthropic/claude-opus-4.6", 4: "--resume"} {
		if row[i] != want {
			t.Errorf("column %d = %q, want %q", i, row[i], want)
		}
	}
}

func TestProfileListEmptyStateIsUnchanged(t *testing.T) {
	h := newHarness(t)
	if got := h.run(t, "profile", "list"); !strings.Contains(got, "No profiles saved.") {
		t.Errorf("empty state = %q, want the add hint", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run Profile -v`
Expected: FAIL — `undefined: profilesTable`.

- [ ] **Step 3: Write the implementation**

In `internal/cli/profile.go`, replace `newProfileListCmd` and add `profilesTable`. Imports become `fmt`, `strings`, cobra, `internal/agent`, `internal/config`, `internal/launch`, `internal/ui` (drop `text/tabwriter`).

```go
func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(out, "No profiles saved. Add one with: openrouter-launch profile add --name <n> --agent <a> --model <slug>")
				return nil
			}
			_, err = fmt.Fprintln(out,
				ui.NewTheme(out).Render(profilesTable(cfg.Profiles, agent.Lookup, agent.Installed)))
			return err
		},
	}
}

// profilesTable builds the listing.
//
// lookup and installed are injected for the same reason agentsTable takes
// them: a profile naming an unregistered agent cannot be created through
// the CLI (profile add validates the name), so a test can only reach that
// row by supplying its own lookup.
func profilesTable(
	profiles []config.Profile,
	lookup func(string) (*agent.Spec, error),
	installed func(*agent.Spec) bool,
) ui.Table {
	var (
		rows  [][]string
		roles []ui.Role
	)
	for _, p := range profiles {
		status, role := ui.UnknownAgentStatus()
		if spec, err := lookup(p.Agent); err == nil {
			status, role = ui.AgentStatus(spec, installed(spec))
		}
		rows = append(rows, []string{p.Name, p.Agent, status, p.Model, strings.Join(p.Args, " ")})
		roles = append(roles, role)
	}

	return ui.Table{
		Headers:  []string{"NAME", "AGENT", "STATUS", "MODEL", "ARGS"},
		Rows:     rows,
		MaxWidth: ui.MaxTableWidth,
		Role: func(row, col int) ui.Role {
			if row < 0 || row >= len(roles) {
				return ui.RolePlain
			}
			switch col {
			case 0:
				return ui.RoleAccent
			case 2:
				return roles[row]
			case 4:
				return ui.RoleDim
			default:
				return ui.RolePlain
			}
		},
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -v`
Expected: PASS.

- [ ] **Step 5: Run the mutation checks**

| Mutation | Must fail |
|---|---|
| Ignore `lookup`'s error and always call `ui.AgentStatus` (nil deref → panic is also a failure, but make it return a blank cell instead) | `TestProfileListFlagsAnUnknownAgent` |
| Drop the STATUS column from `Headers` and the rows | `TestProfileListShowsAgentInstallState` |
| Return early before the empty-state check | `TestProfileListEmptyStateIsUnchanged` |

- [ ] **Step 6: Commit**

```bash
git add internal/cli/profile.go internal/cli/profile_test.go
git commit -m "feat(cli): render profile list as a table with a status column

A saved profile pointing at an uninstalled — or since-removed — agent used
to look identical to a working one until launch time. lookup is injected
because profile add validates the agent name, so the unknown-agent row is
otherwise unreachable from a test."
```

---

### Task 5: `orl models`

**Files:**
- Modify: `internal/cli/models.go`
- Test: `internal/cli/models_test.go`

**Interfaces:**
- Consumes: `ui.*`, `tableRow`.
- Produces: `func modelsTable(models []openrouter.Model) ui.Table`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/cli/models_test.go`:

```go
func TestModelsTableMarksToolSupport(t *testing.T) {
	out := ui.NewTheme(new(strings.Builder)).Render(modelsTable([]openrouter.Model{
		{ID: "a/tools", ContextLength: 200000, SupportsTools: true},
		{ID: "a/plain", ContextLength: 128000},
	}))

	if got := tableRow(t, out, "a/tools")[4]; got != "✓" {
		t.Errorf("tools cell = %q, want %q", got, "✓")
	}
	if got := tableRow(t, out, "a/plain")[4]; got != "" {
		t.Errorf("tools cell = %q, want empty for a tool-less model", got)
	}
}

// Landmine 4 at the render layer: a model whose price failed to parse is
// not free, and rendering it as free is an actively wrong claim about cost.
func TestModelsTableNeverRendersUnknownPricingAsFree(t *testing.T) {
	out := ui.NewTheme(new(strings.Builder)).Render(modelsTable([]openrouter.Model{
		{ID: "x/y", ContextLength: 1000, PricingUnknown: true},
	}))

	row := tableRow(t, out, "x/y")
	if strings.Contains(row[2], "0.00") || strings.Contains(row[3], "0.00") {
		t.Errorf("unknown pricing rendered as free: %q", row)
	}
	if row[2] != "?" || row[3] != "?" {
		t.Errorf("price cells = %q/%q, want %q", row[2], row[3], "?")
	}
}

func TestModelsListingEmitsNoEscapesWhenNotATerminal(t *testing.T) {
	h := newHarness(t)
	if got := h.run(t, "models"); strings.Contains(got, "\x1b") {
		t.Errorf("models emitted ANSI escapes to a buffer:\n%q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run Models -v`
Expected: FAIL — `undefined: modelsTable`.

- [ ] **Step 3: Write the implementation**

In `internal/cli/models.go`, replace the `tabwriter` block at the end of `RunE` with:

```go
			out := cmd.OutOrStdout()
			_, err = fmt.Fprintln(out, ui.NewTheme(out).Render(modelsTable(models)))
			return err
```

and add:

```go
// modelsTable builds the catalog listing. Deliberately uncapped: its widest
// cell is a 50-character model id, already under ui.MaxTableWidth, and
// truncating a model slug would make it un-copy-pasteable.
func modelsTable(models []openrouter.Model) ui.Table {
	rows := make([][]string, 0, len(models))
	for _, m := range models {
		tools := ""
		if m.SupportsTools {
			tools = "✓"
		}
		rows = append(rows, []string{
			m.ID,
			openrouter.FormatContext(m.ContextLength),
			openrouter.FormatPrice(m.PromptPricePerM, m.PricingUnknown),
			openrouter.FormatPrice(m.CompletionPricePerM, m.PricingUnknown),
			tools,
		})
	}

	return ui.Table{
		Headers: []string{"MODEL", "CONTEXT", "PROMPT/M", "COMPLETION/M", "TOOLS"},
		Rows:    rows,
		Role: func(_, col int) ui.Role {
			switch col {
			case 0:
				return ui.RoleAccent
			case 4:
				return ui.RoleOK
			default:
				return ui.RolePlain
			}
		},
	}
}
```

Remove the now-unused `text/tabwriter` import and add `internal/ui`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -v && go run . models --free | head -20`
Expected: PASS, and a bordered catalog table.

- [ ] **Step 5: Run the mutation checks**

| Mutation | Must fail |
|---|---|
| Set `tools = "✓"` unconditionally | `TestModelsTableMarksToolSupport` |
| Pass `false` for `PricingUnknown` to `FormatPrice` | `TestModelsTableNeverRendersUnknownPricingAsFree` |

- [ ] **Step 6: Commit**

```bash
git add internal/cli/models.go internal/cli/models_test.go
git commit -m "feat(cli): render the models listing as a table

Uncapped on purpose: the widest cell is a 50-character model id, and a
truncated slug cannot be copy-pasted into -m."
```

---

### Task 6: TUI root screen — tables

**Files:**
- Modify: `internal/tui/style.go` (add `theme`, re-source `dimStyle`/`warnStyle`)
- Modify: `internal/tui/root.go` (`View` only)
- Test: `internal/tui/root_test.go`

**Interfaces:**
- Consumes: `ui.*` from Tasks 1–2.
- Produces: package-level `theme *ui.Theme` in `internal/tui`; `func (m rootModel) render(start, end int) string`.

- [ ] **Step 1: Write the failing tests**

In `internal/tui/root_test.go`, update the two label assertions and add the layout tests:

```go
// The section labels are uppercase to match the table headers.
//
// TestRootOmitsTheProfilesHeaderWhenThereAreNone asserts an ABSENCE, so it
// has to be updated in lockstep: left looking for "Profiles" it would pass
// against a screen that says "PROFILES", testing nothing at all.
func TestRootListsProfilesBeforeAgents(t *testing.T) {
	m := rootFixture([]config.Profile{{Name: "opus-cc", Agent: "claude", Model: "anthropic/x"}}, "")
	got := m.View()

	pi, ai := strings.Index(got, "PROFILES"), strings.Index(got, "AGENTS")
	if pi < 0 || ai < 0 {
		t.Fatalf("View = %q, missing a section label", got)
	}
	if pi > ai {
		t.Error("the Agents section renders before Profiles")
	}
}

func TestRootOmitsTheProfilesHeaderWhenThereAreNone(t *testing.T) {
	if got := rootFixture(nil, "").View(); strings.Contains(got, "PROFILES") {
		t.Errorf("View = %q, shows an empty Profiles section", got)
	}
}

func TestRootRendersRowsInsideABorderedTable(t *testing.T) {
	got := rootFixture(nil, "").View()
	for _, want := range []string{"╭", "│", "├", "╰", "NAME", "STATUS"} {
		if !strings.Contains(got, want) {
			t.Errorf("View is missing %q:\n%s", want, got)
		}
	}
}

// The cursor marker must live in the first column of the SELECTED row and
// nowhere else. Asserting only that "›" appears somewhere would pass for a
// marker stapled to a fixed row.
func TestRootCursorMarkerTracksTheSelection(t *testing.T) {
	m := rootFixture(nil, "")
	marked := markedRow(t, m.View())
	if marked != "claude" {
		t.Errorf("marker on %q, want claude", marked)
	}

	m.move(1)
	if marked := markedRow(t, m.View()); marked != "codex" {
		t.Errorf("after move the marker is on %q, want codex", marked)
	}
}

// markedRow returns the NAME cell of the row carrying the cursor marker,
// failing if zero or more than one row carries it.
func markedRow(t *testing.T, view string) string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(view, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "│") {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "│"), "│")
		if len(cells) > 1 && strings.TrimSpace(cells[0]) == "›" {
			found = append(found, strings.TrimSpace(cells[1]))
		}
	}
	if len(found) != 1 {
		t.Fatalf("found %d marked rows (%v), want exactly 1:\n%s", len(found), found, view)
	}
	return found[0]
}
```

`TestRootUninstalledAgentIsStillSelectable` keeps its `strings.Contains(got, "not installed")` assertion — `✗ not installed` still contains it.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/ -run Root -v`
Expected: FAIL — no border characters, no `PROFILES`, no marker column.

- [ ] **Step 3: Add the theme to `internal/tui/style.go`**

```go
var (
	// theme is bound to os.Stdout because that is where bubbletea renders.
	// Under `go test` stdout is a pipe, so views come back free of escapes
	// and string assertions keep working.
	theme = ui.NewTheme(os.Stdout)

	titleStyle    = lipgloss.NewStyle().Bold(true)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "249"})
	selectedStyle = lipgloss.NewStyle().Bold(true)
	// Sourced from the shared palette so the screens and the tables cannot
	// drift apart. Same values as before, now declared once.
	dimStyle  = theme.Style(ui.RoleDim)
	warnStyle = theme.Style(ui.RoleWarn).Bold(true)
)
```

Add `"os"` and `internal/ui` to the imports.

- [ ] **Step 4: Rewrite `View` in `internal/tui/root.go`**

```go
// cursorCell is the marker column's content. Both branches are one column
// wide so rows do not shift as the cursor moves.
func cursorCell(selected bool) string {
	if selected {
		return "›"
	}
	return " "
}

func (m rootModel) View() string {
	return m.render(0, len(m.rows))
}

// render draws rows[start:end].
//
// buildRootRows stays the single row list, headers included, so
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
			b.WriteString(headerStyle.Render("  "+strings.ToUpper(m.rows[i].label)) + "\n")
			i++
			continue
		}
		run := i
		for end > i && m.rows[i].kind == m.rows[run].kind {
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
			rows = append(rows, []string{marker, row.profile.Name, row.profile.Agent, status, row.profile.Model})
			roles = append(roles, role)
			continue
		}
		status, role := ui.AgentStatus(row.agent, row.installed)
		rows = append(rows, []string{marker, row.agent.Name, row.agent.Launcher.DisplayName(), status})
		roles = append(roles, role)
	}

	selected := m.cursor - from
	return theme.Render(ui.Table{
		Headers:  headers,
		Rows:     rows,
		Emphasis: func(row int) bool { return row == selected },
		MaxWidth: m.width,
		Role: func(row, col int) ui.Role {
			if row < 0 || row >= len(roles) {
				return ui.RolePlain
			}
			switch col {
			case 1:
				return ui.RoleAccent
			case 3:
				return roles[row]
			default:
				return ui.RolePlain
			}
		},
	})
}

// indent shifts a rendered table right by two columns, matching the title
// and footer.
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}
```

`rootRow` gains two fields so `sectionTable` does not have to re-probe the
filesystem while rendering (and so a profile's agent is resolved once):

```go
type rootRow struct {
	kind   rowKind
	label  string
	detail string
	// selectable is false for section headers, so the cursor can never land
	// on a row that would do nothing.
	selectable bool
	profile    config.Profile
	agent      *agent.Spec
	// spec is the profile's agent, nil when the profile names one that is
	// not registered; installed is that agent's install state. Both are
	// resolved in buildRootRows so View stays free of IO — it is called on
	// every keystroke.
	spec      *agent.Spec
	installed bool
}
```

In `buildRootRows`, fill them: for profile rows, `spec, err := agent.Lookup(p.Agent)` (nil on error) and `installed` via `in.Installed`; for agent rows, `row.installed = in.Installed == nil || in.Installed(spec)`. Keep setting `detail` — nothing reads it any more, so **delete the field and its assignments** rather than leaving dead state.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -v`
Expected: PASS.

- [ ] **Step 6: Run the mutation checks**

| Mutation | Must fail |
|---|---|
| `Emphasis: func(row int) bool { return row == 0 }` | `TestRootCursorMarkerTracksTheSelection` — no; it pins the marker. Instead make `cursorCell` always return `"›"` | `TestRootCursorMarkerTracksTheSelection` (two marked rows) |
| Render the agents run before the profiles run | `TestRootListsProfilesBeforeAgents` |
| Emit the section label for an empty profiles section | `TestRootOmitsTheProfilesHeaderWhenThereAreNone` |
| Drop `strings.ToUpper` from the label | `TestRootListsProfilesBeforeAgents` |

- [ ] **Step 7: Commit**

```bash
git add internal/tui/style.go internal/tui/root.go internal/tui/root_test.go
git commit -m "feat(tui): render the root screen as bordered tables

The cursor moves into a marker column inside the border and the selected
row is bolded, so the selection survives NO_COLOR. buildRootRows is
unchanged — only View — so cursor movement and every key-handling test are
untouched. Section labels are uppercase now; the absence assertion in
TestRootOmitsTheProfilesHeaderWhenThereAreNone was updated in lockstep,
since left alone it would have passed vacuously."
```

---

### Task 7: TUI root screen — the scroll window

**Files:**
- Modify: `internal/tui/root.go` (`rootModel`, `Update`, `View`)
- Test: `internal/tui/root_test.go`

**Interfaces:**
- Consumes: `render(start, end int) string` from Task 6.
- Produces: `rootModel.width`, `rootModel.height`; `func (m rootModel) windowStart(n int) int`.

- [ ] **Step 1: Write the failing tests**

```go
// manyAgents overflows any realistic terminal: 15 agents plus chrome and
// two table frames is well past 24 lines.
func manyAgents() []*agent.Spec {
	var specs []*agent.Spec
	for _, n := range []string{
		"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8",
		"a9", "a10", "a11", "a12", "a13", "a14", "a15",
	} {
		specs = append(specs, stubSpec(n))
	}
	return specs
}

func tallFixture(height int) rootModel {
	m := newRootModel(rootInput{
		Profiles:  []config.Profile{{Name: "p1", Agent: "claude", Model: "m"}},
		Agents:    manyAgents(),
		Installed: allInstalled,
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: height})
	return next.(rootModel)
}

// Landmine 17's failure mode, reached by a new route: bubbletea's renderer
// drops lines from the TOP when the view is taller than the terminal, and
// line 0 is the title. Asserting against lipgloss.Height rather than a
// hand-counted chrome constant is the point — recounting chrome by hand is
// exactly what produced Landmine 17.
func TestRootViewFitsTheTerminalHeight(t *testing.T) {
	for _, height := range []int{12, 20, 24, 40} {
		for _, cursor := range []int{0, 8, 15} {
			m := tallFixture(height)
			for i := 0; i < cursor; i++ {
				m.move(1)
			}
			view := m.View()
			if got := lipgloss.Height(view); got > height {
				t.Errorf("height=%d cursor=%d: view is %d lines:\n%s", height, cursor, got, view)
			}
			if !strings.Contains(view, "openrouter-launch") {
				t.Errorf("height=%d cursor=%d: the title was dropped:\n%s", height, cursor, view)
			}
		}
	}
}

func TestRootScrollWindowKeepsTheCursorVisible(t *testing.T) {
	m := tallFixture(20)
	for i := 0; i < 14; i++ {
		m.move(1)
	}
	if got := markedRow(t, m.View()); got != "a15" {
		t.Errorf("marked row = %q, want the cursor row a15", got)
	}
}

// Before the first WindowSizeMsg the height is unknown, so everything
// renders rather than nothing — the picker's defaultListHeight posture.
func TestRootRendersEverythingBeforeTheFirstWindowSize(t *testing.T) {
	m := newRootModel(rootInput{Agents: manyAgents(), Installed: allInstalled})
	if got := m.View(); !strings.Contains(got, "a15") {
		t.Errorf("View dropped rows before the first WindowSizeMsg:\n%s", got)
	}
}

func TestRootRangeIndicatorAppearsOnlyWhenScrolled(t *testing.T) {
	if got := tallFixture(12).View(); !strings.Contains(got, " of ") {
		t.Errorf("a scrolled view has no range indicator:\n%s", got)
	}
	if got := tallFixture(60).View(); strings.Contains(got, " of ") {
		t.Errorf("an unscrolled view shows a range indicator:\n%s", got)
	}
}
```

Add `"github.com/charmbracelet/lipgloss"` to the test imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/ -run 'RootView|RootScroll|RootRange|RootRenders' -v`
Expected: FAIL — `tea.WindowSizeMsg` is ignored, so the view is its full height at every terminal size.

- [ ] **Step 3: Write the implementation**

Add the fields and the size handler:

```go
type rootModel struct {
	rows   []rootRow
	cursor int
	width  int
	height int
	choice rootChoice
	done   bool
}
```

In `Update`, before the `tea.KeyMsg` switch:

```go
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}
```

Replace `View`:

```go
// View renders as many rows as fit, centered on the cursor.
//
// It MEASURES rather than predicts. The rendered height depends on how many
// table frames land inside the window — each costs a top border, a header
// row, and a header rule — and that depends on where the window starts, so
// a hand-computed chrome constant would have to encode a number that
// changes with the cursor position. Landmine 17 exists because someone
// recounted the picker's chrome by hand and got 8 where the renderer's
// arithmetic says 9; this shrinks the window until the real output fits.
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
// list. The window is derived from the cursor alone — there is no stored
// scroll offset — so View stays a pure function of the model and no second
// piece of state can desynchronize from the cursor.
func (m rootModel) windowStart(n int) int {
	start := m.cursor - (n-1)/2
	if max := len(m.rows) - n; start > max {
		start = max
	}
	if start < 0 {
		start = 0
	}
	return start
}
```

And in `render`, append the range indicator to the footer:

```go
	footer := "  ↑/↓ move · enter select · esc quit"
	if start > 0 || end < len(m.rows) {
		footer += fmt.Sprintf("    %d-%d of %d",
			countSelectable(m.rows[:start])+1,
			countSelectable(m.rows[:end]),
			countSelectable(m.rows))
	}
	b.WriteString(dimStyle.Render(footer) + "\n")
```

```go
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
```

Add `"fmt"` and `"github.com/charmbracelet/lipgloss"` to `root.go`'s imports.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -v && go test ./internal/tui/ -race`
Expected: PASS.

- [ ] **Step 5: Run the mutation checks**

| Mutation | Must fail |
|---|---|
| `View` returns `m.render(0, len(m.rows))` unconditionally | `TestRootViewFitsTheTerminalHeight` |
| `windowStart` returns `0` always | `TestRootScrollWindowKeepsTheCursorVisible` |
| Drop the `start > 0 \|\| end < len(m.rows)` guard on the indicator | `TestRootRangeIndicatorAppearsOnlyWhenScrolled` |
| `Update` ignores `tea.WindowSizeMsg` | `TestRootViewFitsTheTerminalHeight` |
| `View` skips the `m.height <= 0` branch | `TestRootRendersEverythingBeforeTheFirstWindowSize` |

- [ ] **Step 6: Drive the real program headlessly (Landmine 16)**

The root closure in `liveScreens` must still work end to end. Confirm
`internal/tui/program_test.go`'s existing root-closure test passes
unmodified; if it drives the screen with Down/Enter it now exercises the
scroll path too. Do **not** add a nil-check test.

Run: `go test ./internal/tui/ -run Program -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "feat(tui): scroll the root screen to fit the terminal

Boxing took the screen from ~22 to ~28 lines, and bubbletea drops from the
TOP, so an 80x24 terminal would have silently eaten the title. View shrinks
the window until the real rendered output fits rather than subtracting a
hand-counted chrome constant — Landmine 17 is exactly that mistake. The
window is derived from the cursor with no stored offset, so View stays pure."
```

---

### Task 8: Documentation and the full gate

**Files:**
- Modify: `README.md` (any sample listing output)
- Modify: `HANDOFF.md` (state row, test count, a new landmine)
- Modify: `CLAUDE.md` (only if a claim it makes is now stale)

- [ ] **Step 1: Refresh the README's sample output**

Run the commands and paste the real output:

```bash
go run . agents
go run . profile list
go run . models --free | head -12
```

Check `grep -n 'NAME\|STATUS\|tabwriter' README.md` for stale blocks.

- [ ] **Step 2: Update HANDOFF.md**

- The **State** line and the **Tests** row: recount with
  `go test ./... -list '.*' | grep -c '^Test'` and record the delta rather
  than guessing.
- Add `internal/ui/` to the "Where things are" tree.
- Add a landmine, numbered next in sequence:

> **N. `ui.Table.MaxWidth` is a cap applied in two passes — do not pass it
> straight to `table.Width`.** lipgloss's `table.Width` is a TARGET, not a
> maximum: it *expands* a narrower table to exactly that width, so a
> one-line `Width(MaxTableWidth)` would stretch every listing to 100
> columns. `Theme.Render` builds the table naturally, measures, and
> re-renders capped only on overflow. `TestMaxWidthDoesNotExpandATableUnderTheCap`
> pins it. Row rules are drawn iff the cap binds, because that is when
> cells wrap and two multi-line rows otherwise read as one block.
>
> Related: `TestAgentsOutputStaysNarrow` renders a **synthetic** 200-character
> description, not the live registry. Against the registry the widest table
> is 94 columns, so the test would pass with the cap deleted — which is why
> `agentsTable` takes its specs as an argument.

- [ ] **Step 3: Run the full gate**

```bash
make ci
```

Expected: green — fmt, vet, lint on 3 GOOS, actionlint, tidy, cross-build,
security, race, coverage above the 80% floor, and the Landmine 8 isolated
run. `make security` still prints the 14 triaged gosec findings; that is
expected (Landmine 29), and **no new finding may appear** — the new code
adds no file access, no exec, and no credential handling.

- [ ] **Step 4: Verify the write-site invariant is untouched**

```bash
go test ./internal/agent/ -run TestWriteSitesAreExhaustivelyEnumerated -v
```

Expected: PASS unmodified. This change writes no files; if this test needed
editing, something is wrong.

- [ ] **Step 5: Commit**

```bash
git add README.md HANDOFF.md CLAUDE.md
git commit -m "docs: record the listing-tables change

Refreshes the README samples, adds internal/ui to the tree, and adds the
landmine for lipgloss's table.Width being a target rather than a maximum."
```

---

## Self-Review

**Spec coverage.** Every section maps to a task: `internal/ui` → Tasks 1–2;
color detection → Task 1 Step 1; the status vocabulary table → Task 2; the
four CLI tables → Tasks 3–5; the TUI columns, cursor column, and section
labels → Task 6; the measured scroll window and range indicator → Task 7;
the spec's Testing table → distributed across the tasks that own each
behavior, with `TestAgentsOutputStaysNarrow`'s synthetic fixture called out
in Task 3. `termenv` promotion → Task 1 Step 4.

**Type consistency.** `ui.Table`'s field set (`Headers`, `Rows`, `Role`,
`Emphasis`, `MaxWidth`) is identical in Tasks 1, 3, 4, 5, 6. `Role` is
`func(row, col int) Role` everywhere; `Emphasis` is `func(row int) bool`
everywhere. `AgentStatus(spec, installed) (string, Role)` and
`UnknownAgentStatus() (string, Role)` match between Task 2's definition and
every call site. `agentsTable`/`profilesTable`/`modelsTable` all return
`ui.Table`. `tableRow` (Task 3) is used in Tasks 4 and 5.

**One correction found and fixed inline:** Task 6's mutation table
originally listed an `Emphasis` mutation against
`TestRootCursorMarkerTracksTheSelection`, which pins the *marker*, not the
bolding. The `Emphasis` mutation belongs to Task 1's
`TestEmphasisBoldsOnlyTheChosenRow`; Task 6's row now mutates `cursorCell`
instead.
