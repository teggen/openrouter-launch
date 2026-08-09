# Listing tables — design

**Date:** 2026-08-09 · **Status:** approved, ready to plan

Turns every place this tool renders a list of things — the three CLI
listings (`orl agents`, with and without `--all`; `orl profile list`;
`orl models`) and the TUI root screen — into bordered tables with a
dedicated status column, shared styling, and terminal-aware color.

## Why

Three separate problems, one shared cause: every listing rolls its own
formatting.

- **Status is not a column where it matters most.** The TUI root screen
  glues `installed` / `not installed` onto the end of a variable-width
  label, so it lands at a different offset on every row and cannot be
  scanned. `profile list` does not show installed-ness at all — a profile
  pointing at an uninstalled or since-removed agent looks identical to a
  working one until you try to launch it.
- **`agents --all` is unreadable.** `tabwriter` pads every column to its
  widest cell, so the three desktop apps' ~99-character reason produces
  227-column lines. The Phase 4a decision to hide unsupported agents by
  default contained the damage for `agents` but left `--all` — the one
  command whose entire purpose is showing those reasons — unusable.
- **Nothing is shared.** `agents.go`, `profile.go`, and `models.go` each
  construct their own `tabwriter`; the TUI builds strings by hand. A
  vocabulary change ("installed" → "✓ installed") would have to be made
  and kept in sync in four places.

## Decisions taken (and the alternatives rejected)

| Decision | Chosen | Rejected, and why |
|---|---|---|
| Table style | **Rounded box, column separators, one rule under the header, no per-row rules** | Header-rule-only (`gh`/`docker` look): narrower and pipes cleanly, but the owner wants the listings to read unmistakably as tables. Full grid: doubles the line count for no added information |
| Scope | **All four listings + the TUI root screen** | CLI-only: leaves the two surfaces looking unrelated, which is the thing this change exists to fix |
| Table engine | **`lipgloss/table`** | Hand-rolled box drawing: reimplements width distribution, cell wrapping, and ANSI-aware measurement — the parts most likely to carry a quiet bug. `lipgloss/table` ships inside the already-required `lipgloss v1.1.0`, so this adds **no** new module dependency |
| Color | **Status column + accents**, auto-detected from the destination writer | Icons-only: nothing to configure, but wastes the signal. Full-row background highlight for the TUI cursor: degrades to invisible under `NO_COLOR` |
| Color control | **`NO_COLOR` / TTY detection only** | A `--no-color` flag: `NO_COLOR` is the cross-tool standard and termenv honors it for free; a flag is a second mechanism to keep working |
| Icon meaning | **Icons always emitted, color never the only carrier** | Color-only status: unreadable when piped, in a dumb terminal, or for a red/green-colorblind reader |
| Shared code | **New `internal/ui` package** | Duplicating the palette and status vocabulary in `cli` and `tui`: the drift this change exists to prevent. `cli` importing `tui` for helpers: puts bubbletea screen concerns in the command layer |
| `profile list` status | **Own STATUS column**, including `⚠ unknown agent` | Leaving it out: `orl agents` answers the question for agents, but not "is *this saved profile* still launchable" |
| `agents --all` reason | **Own REASON column, width-capped, wrapped, row separators on** | Folding the reason into STATUS: the 227-column problem, restored |
| Root screen height | **Scroll window over the combined row list** | Accepting the clipping: boxing takes the screen from ~22 to ~28 lines, and bubbletea drops from the *top*, so the title disappears silently on any 80×24 terminal — Landmine 17's failure mode reached by a new route |

## Architecture

### `internal/ui` — the one place the look is defined

```go
package ui

type Role int
const (
    RolePlain Role = iota
    RoleAccent
    RoleDim
    RoleOK      // green  — installed
    RoleWarn    // yellow — unsupported / unknown
    RoleBad     // red    — not installed
)

// Theme carries a lipgloss renderer bound to a specific destination, so
// color is decided by where the output actually goes.
type Theme struct{ ... }
func NewTheme(w io.Writer) *Theme

// Table is data only. Render is pure: same Table in, same string out.
type Table struct {
    Headers  []string
    Rows     [][]string
    Role     func(row, col int) Role // nil means RolePlain everywhere
    Emphasis func(row int) bool      // bold on top of the cell's own role
    MaxWidth int                     // 0 means no cap
}
func (th *Theme) Render(t Table) string

// AgentStatus is the single source of the status vocabulary. Both the CLI
// and the TUI call it, so the two can never disagree about what an agent's
// state is called or colored.
func AgentStatus(spec *agent.Spec, installed bool) (text string, role Role)
```

Dependency edges: `cli → ui`, `tui → ui`, `ui → agent`. No cycles, and
`TestTUIDependsOnNeitherCLINorCobra` (Landmine 13) is unaffected — `ui`
imports neither cobra nor pflag, and never will.

**`Render` being pure and taking `[][]string` is load-bearing, not
stylistic.** It is what lets a width test feed a synthetic 200-character
description instead of whatever the registry happens to contain today. See
Testing.

Two `lipgloss/table` behaviors were measured, and both shape the contract:

- **`Width(n)` is a target, not a cap** — it *expands* a narrower table to
  exactly `n`. So `MaxWidth` cannot be passed straight through: `Render`
  builds the table content-sized, measures it, and only re-renders with
  `Width(MaxWidth)` when the natural width actually exceeds the cap. A
  table under its cap keeps its natural width.
- **`StyleFunc` is called more than once per cell** (a measure pass and a
  render pass), so `Table.Role` must be free of side effects. A test that
  records calls will see each cell twice; that is the library, not a bug.

There is no `Separators` knob. Rules between body rows are drawn **iff the
width cap binds**, because that is exactly when cells wrap to multiple
lines and adjacent rows become indistinguishable — measured: two 3-line
rows under `BorderRow(false)` render as six visually identical lines. Tying
the rule to the condition that motivates it removes a caller decision that
could only ever be set one way.

### Color detection

`NewTheme` builds a `lipgloss.NewRenderer(w)` bound to the writer the table
is actually going to. Measured, not assumed:

| Destination | Profile | ANSI escapes emitted |
|---|---|---|
| `*bytes.Buffer` (every CLI test) | `Ascii` | none |
| `os.Stdout` piped to another process | `Ascii` | none |
| `os.Stdout` on a terminal | terminal's own | yes |
| any, with `NO_COLOR` set | `Ascii` | none |

Two consequences worth stating: every existing substring assertion in
`internal/cli` keeps working unchanged, and no test can accidentally start
depending on whether the machine running it has a TTY.

CLI commands pass `cmd.OutOrStdout()`, so `--help`-style redirection and
test harness buffers are both handled by the same mechanism.

### Status vocabulary

| Condition | Cell | Role |
|---|---|---|
| supported, binary found | `✓ installed` | `RoleOK` |
| supported, binary absent | `✗ not installed` | `RoleBad` |
| `!Status.Supported` | `⚠ unsupported` | `RoleWarn` |
| profile names an unregistered agent | `⚠ unknown agent` | `RoleWarn` |

The last row is reachable only from a hand-edited config or an agent
removed from the registry between releases — `profile add` calls
`launch.CheckSupported` and `agent.Lookup`, so it cannot be created through
the CLI. It is included because the failure is otherwise invisible until
launch time.

### The four CLI tables

| Command | Columns | Width | Notes |
|---|---|---|---|
| Command | Columns | Width | Notes |
|---|---|---|---|
| `agents` | NAME · AGENT · STATUS · DESCRIPTION | capped at 100; renders at 94 today, so the cap does not bind | Unsupported agents stay hidden (Phase 4a decision, unchanged) |
| `agents --all` | NAME · AGENT · STATUS · REASON · DESCRIPTION | capped at 100, which **does** bind | REASON is blank for supported agents and wraps for the three desktop apps, so row rules appear automatically |
| `profile list` | NAME · AGENT · STATUS · MODEL · ARGS | capped at 100 | Empty-state sentence unchanged |
| `models` | MODEL · CONTEXT · PROMPT/M · COMPLETION/M · TOOLS | no cap | TOOLS becomes `✓` / blank; 334 rows render fine |

`agents` carries the cap even though it renders at 94 today. That is the
point: the cap is what a future long description hits instead of blowing
the table out, and `TestAgentsOutputStaysNarrow` exists to fail if the cap
is removed. A content-sized `agents` table would leave that test with
nothing to pin.

**The cap is a constant, not the terminal's width.** Querying the terminal
would mean `golang.org/x/term`, which `internal/tui/program.go` explicitly
declined — "golang.org/x/term would do the same thing at the cost of a
direct dependency" — and `tabwriter` never queried the width either, so
this is not a regression. A terminal narrower than the table soft-wraps it,
exactly as it does today. The TUI is unaffected: it gets a real width from
`tea.WindowSizeMsg`.

Roles: NAME/MODEL accent, STATUS per the table above, DESCRIPTION dim,
borders dim, headers bold.

`models` is deliberately **not** width-capped. Its widest cell is a
50-character model id, which already fits under 100; capping it would
truncate model ids in redirected output, and a truncated slug is not
copy-pasteable.

### The TUI root screen

Columns match the CLI so the two surfaces read the same:

- Profiles table: cursor · NAME · AGENT · STATUS · MODEL
- Agents table: cursor · NAME · AGENT · STATUS

ARGS is omitted from the TUI profiles table — the root screen is
width-constrained and args are not a selection criterion. This is the one
deliberate column difference between the surfaces.

The cursor is a narrow leading column **inside** the border holding `›`,
and the selected row is bolded (`Table.Emphasis`, which composes with the
cell's own role rather than replacing it). Both carry the selection, so it
survives `NO_COLOR`.

`buildRootRows` is **unchanged** — section-header rows stay in the row
list, so `initialCursor`, `move`, and every existing key-handling test keep
working. Only `View` changes: it walks the visible window, emits a section
label when it meets a header row, and collects each run of consecutive
profile or agent rows into one table. A window that starts mid-agents
simply does not draw the profiles table or its label.

Section labels become uppercase (`PROFILES`, `AGENTS`) to match the table
headers. Two existing tests reference them by name and must be updated —
`TestRootOmitsTheProfilesHeaderWhenThereAreNone` especially, because it
asserts an *absence*: left alone it would pass vacuously against
`PROFILES` while testing nothing.

### Root screen scrolling

`rootModel` gains `width`, `height`, and `offset`, and `Update` handles
`tea.WindowSizeMsg` — which it ignores entirely today.

**`View` measures its own output rather than predicting it.** The rendered
height depends on how many table frames fall inside the window (each costs
a top border, a header row, and a header rule), which in turn depends on
where the window starts — so a hand-computed `chromeHeight` constant would
have to encode a value that changes with the cursor position. Instead:

```
for n := len(rows); n >= 1; n-- {
    start := clamp(cursor-(n-1)/2, 0, len(rows)-n)
    out := render(rows[start : start+n])
    if lipgloss.Height(out) <= height {
        return out
    }
}
```

The window is derived from the cursor alone — centered, then clamped — with
no stored scroll offset. `View` therefore stays a pure function of the
model, and there is no second piece of state that can desynchronize from
the cursor. The cost is that the list scrolls by one on every move once the
cursor passes the middle, rather than only when it would leave the window;
for a screen this size that is not worth a state variable.

This is Landmine 17's lesson applied directly: that landmine exists because
someone recounted the picker's chrome lines by hand and got 8 where the
renderer's arithmetic says 9. Measuring costs a handful of extra renders
per frame and cannot drift from what bubbletea will do.

The footer gains a range indicator (`4-9 of 13`) shown only when the window
does not hold every row, so a scrolled screen is never silently truncated.

Before the first `WindowSizeMsg`, `height` is 0 and every row renders —
matching the picker's `defaultListHeight` posture of drawing something
rather than nothing on the first frame.

## Testing

The project's recurring finding is tests that pass for the wrong reason, so
each of these names the mutation that must break it.

| Test | Mutation that must fail it |
|---|---|
| `TestAgentsOutputStaysNarrow` — rebuilt to render a **synthetic** spec with a 200-char description through `ui.Render` | Remove the `MaxWidth` cap. *Today's version cannot catch this: no registered agent has a description long enough, so the test would pass with the cap deleted.* |
| No-escapes-when-not-a-TTY, all four listings | Bind the renderer to `os.Stdout` instead of the destination writer |
| Status cell content and role, one case per vocabulary row | Swap `RoleOK`/`RoleBad`, or drop the glyph |
| `--all` reason wraps and stays ≤100 columns, with row rules | Remove `MaxWidth`, or stop drawing rules when the cap binds (rows run together) |
| `MaxWidth` does not *expand* a table under its cap | Pass `MaxWidth` straight to `table.Width` instead of measuring first |
| `profile list` unknown-agent row | Make `agent.Lookup` failure render as blank instead of `⚠ unknown agent` |
| Root cursor column tracks the cursor | Render `›` on a fixed row index |
| Root view height ≤ terminal height, at 20/24/40 lines and at first/middle/last cursor positions | Delete the measure-and-shrink loop; assert against `lipgloss.Height`, not a recounted constant |
| Root range indicator appears only when scrolled | Always show it, or never |

`claudeStatusField` in `agents_test.go` splits rows on runs of two-or-more
spaces; borders break that. It is rewritten to split on `│`. This is a test
helper change, not a contract change — the assertions it feeds are
unchanged.

Existing tests that assert substrings (`"claude"`, `"Claude Code"`,
`"desktop app authenticates through its own account"`) keep passing
unmodified, because buffered output carries no escapes and the box
characters sit outside the cell text.

## Out of scope

- **Machine-readable output** (`--json`, `--no-headers`). Boxed tables are
  worse for scripting than `tabwriter` was; if that turns out to matter, a
  `--json` flag is the right answer and is its own change.
- **Sorting or filtering the agents listing.** Registry order is preserved.
- **Changing which agents are listed.** The Phase 4a hide-unsupported
  decision stands untouched; `--all` still reveals them.
- **Alt-screen for the root screen.** It stays inline so its final render
  remains as a wizard trail, as designed in Phase 2.

## Risks

- **Root screen scrolling is new interactive behavior** and cannot be
  exercised from a headless harness by hand. It is covered by driving a
  real bubbletea program headlessly, the way Landmine 16 requires — never
  by nil-checks.
- **Unicode box characters and `✓`/`✗`/`⚠` assume a UTF-8 terminal.** Every
  agent this tool launches is itself a modern TUI, so the assumption is
  already made everywhere else in the product; `lipgloss` measures with
  `go-runewidth`, so alignment holds for wide glyphs.
- **A terminal narrower than 100 columns soft-wraps the CLI tables.**
  `tabwriter` had the same property and the 227-column `--all` line was far
  worse; the alternative costs a direct `golang.org/x/term` dependency this
  project has already declined once.
- **`github.com/muesli/termenv` becomes a direct dependency.** It is
  already in the tree as an indirect dependency of lipgloss, so nothing new
  is downloaded. It is needed so tests can force a color profile on a
  buffer-bound renderer — without it the role-to-color mapping is
  untestable, since a buffer always resolves to `Ascii`.
