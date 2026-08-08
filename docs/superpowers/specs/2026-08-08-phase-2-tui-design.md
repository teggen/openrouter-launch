# Phase 2 — the interactive TUI

**Status:** design approved 2026-08-08. Supersedes the `## TUI` section of
`2026-08-07-openrouter-launch-design.md` where the two disagree; that
section's key table is corrected here for a reason recorded below.

## Context

Phase 1 shipped the non-interactive spine. The Phase 2 prerequisite refactor
(`2026-08-07-phase-2-planner-refactor-design.md`) extracted `internal/launch`,
a planner that performs the launch sequence and never touches the terminal:
every condition a user must see comes back as a `Warning` or a typed error.
That package exists so a TUI can drive the same logic the CLI drives.

This spec is the TUI itself.

## Scope

In:

- Root screen: profiles and agents, `last_agent` preselected.
- Model picker: type-to-search, four filters, description pane, `ctrl+s`
  profile save.
- Filter persistence to `config.filters`.
- API-key prompt on first launch that needs one. The key is saved to config
  unconditionally, and the prompt discloses this before the user types.
- Error screens rendering the planner's typed errors.

Out, with reasons:

- **Background catalog refresh streaming into the live picker.** The
  `2026-08-07` spec describes it. The cache already carries a 24h TTL, so a
  warm cache is genuinely current and the only window it improves is the
  moment after expiry. It costs a goroutine, a channel, and re-ranking a list
  the user is actively navigating. Deferred, not rejected.
- **Provider filter key.** Unchanged from the original spec: typing
  `anthropic/` narrows to that vendor through normal search.
- **Profile editing in the TUI.** `profile rm|rename` already exist. The TUI
  adds profiles (`ctrl+s`) because that is where you discover you want one.

## Dependencies

`github.com/charmbracelet/bubbletea v1.3.10` and
`github.com/charmbracelet/lipgloss v1.1.0`.

`go.mod`'s `go` directive rises from `1.22` to `1.24`. This is a deliberate
reversal of a deliberate decision: Phase 1 recorded `go 1.22` as a
compatibility floor to be bumped only when a feature required it. bubbletea
declares `go 1.24.0` from v1.3.8 onward, `go 1.23.0` for v1.3.5–v1.3.7, and
`go 1.18` at v1.3.4 (verified against the module cache), so holding the floor
would mean pinning a year-old release. The floor moves rather than the
dependency. lipgloss v1.1.0 still declares `go 1.18` and constrains nothing.

## Package layout

```
internal/tui/tui.go      Run — driver state machine, Options, ErrCancelled
internal/tui/root.go     root screen: profiles + agents
internal/tui/picker.go   model picker: search, filters, description pane
internal/tui/prompt.go   one-line input (profile name, API key), maskable
internal/tui/confirm.go  warnings + optional question
internal/tui/notice.go   error screens
internal/tui/match.go    ranked search — pure
internal/tui/filters.go  the alt+c / alt+p cycles — pure
internal/tui/style.go    lipgloss styles and row layout
```

Import direction is one-way: `cli → tui → launch`. `tui` must never import
`cli`, because `cli` imports `tui`. This is enforced by a test, not by a
comment (see Testing).

## Entry point

```go
// Options configures one interactive session.
type Options struct {
	Service   *launch.Service
	Agent     *agent.Spec // non-nil skips the root screen
	ExtraArgs []string    // everything after --
	Refresh   bool        // --refresh
	AssumeYes bool        // --yes
}

// Run drives the session and returns an approved plan. It never launches.
func Run(ctx context.Context, opts Options) (launch.Plan, error)
```

`Run` returns `ErrCancelled` when the user backs all the way out.

**`Run` must not launch.** The caller calls `launch.Service.Launch` after
`Run` returns, so every bubbletea program has torn down and the terminal is
out of raw mode before `syscall.Exec` replaces the process. Launching from
inside a screen would hand a raw-mode terminal to the agent.

## Architecture: one program per screen

Each screen is a small `tea.Model` run to completion by its own
`tea.NewProgram(...).Run()`, sequenced by a plain Go driver. This is the
pattern `ollama/cmd/tui` uses (`SelectSingle` at `selector.go:704`,
`RunConfirm` at `confirm.go:98`, `RunSignIn` at `signin.go:295`).

Three consequences, each of which is why this shape was chosen over a single
model with a screen enum or a parent router:

1. **`Plan`'s IO never enters an event loop.** The driver calls
   `Service.Plan` *between* programs, in ordinary Go, then runs the confirm
   screen with warnings already in hand. No `tea.Cmd` wrapping, no async
   result message, no screen that can render a half-resolved plan.
2. **Navigation is a `for` loop.** "Esc goes back to root" is Go control
   flow, readable top to bottom, with no child-to-parent message protocol.
3. **Teardown before handoff is structural.** The last program has provably
   returned before the driver reaches its return statement. No other ordering
   is available to get wrong.

Screens are reached through injected function fields, never called directly:

```go
type screens struct {
	root    func(rootInput) (rootChoice, error)
	pick    func(pickerInput) (pickerChoice, error)
	prompt  func(promptInput) (string, error)
	confirm func(confirmInput) (bool, error)
	notice  func(noticeInput) error
}
```

Production wires these to bubbletea programs. Tests wire them to closures
returning canned choices, which makes the whole navigation state machine
testable with no terminal and no bubbletea program at all.

The picker runs with `tea.WithAltScreen()`; it is the one view that wants the
whole terminal and should leave no scrollback behind. The other screens run
inline, leaving their final render as a wizard trail.

## Screens

### Root

A `Profiles` section then an `Agents` section. Section headers are not
selectable and the cursor skips them. Agents render with install state.
Unsupported agents render dim with their `Status.Reason` and are skipped by
the cursor entirely, so an unselectable row can never be highlighted.

`last_agent` is preselected, so Enter alone reaches the usual picker.
Selecting a profile skips the picker and goes straight to planning.

Keys: `↑`/`↓`/`k`/`j` move, `enter` selects, `esc`/`q`/`ctrl+c` cancels.

### Picker

```
Model for Claude Code                                    search: anthro

    anthropic/claude-opus-4.6      200k   $15.00/$75.00 per M   tools
  › anthropic/claude-sonnet-4.6    200k    $3.00/$15.00 per M   tools
    anthropic/claude-haiku-4.5     200k    $1.00/$5.00 per M    tools

  Claude Sonnet 4.6 balances speed and capability, with the same 200k
  context window as Opus…

  tools · ctx≥200k                              37 of 412 models
  alt+t tools · alt+f free · alt+c ctx · alt+p price · ctrl+s save profile
```

The description pane is **fixed height (2 lines, hard-truncated)**. OpenRouter
descriptions run to several paragraphs for some models; a variable-height pane
would reflow the list on every cursor move.

The status line always names the active filters, so the current view is never
ambiguous, and shows `visible of total` so an empty list reads as
over-filtering rather than a broken catalog.

**Key map.** The original spec asked for type-to-search *and* bare `t`/`f`/`c`/`$`
filter keys. Those are the same keystrokes — typing `anthropic` starts
`a`-`n`-`t`, and that `t` would toggle the tools filter mid-word; `f` breaks
`free`, `c` breaks `claude`, and `$` is unreachable while typing a price.
Type-to-search is kept as primary because it is the better default for a
400-model list and because "typing `anthropic/` narrows to that vendor" is how
the spec eliminates a provider filter. Filters move to `alt+`:

| Key | Action |
| --- | --- |
| printable runes | append to search |
| `backspace` | delete from search |
| `alt+t` | toggle tool-calling only (on by default) |
| `alt+f` | toggle free only |
| `alt+c` | cycle min context: any → 32k → 128k → 200k → 1M → any |
| `alt+p` | cycle max completion $/M: any → 1 → 5 → 15 → any |
| `↑`/`↓`, `pgup`/`pgdn` | move, scroll |
| `enter` | select the highlighted model |
| `ctrl+s` | save agent + highlighted model as a profile |
| `esc` | back to root, or cancel when `Options.Agent` was set |
| `ctrl+c` | cancel |

The footer spells the bindings out, since `alt+` is less discoverable than a
bare letter and some terminals need "send Meta as Escape" enabled.

**Filtering and ranking are separate steps.** The picker calls
`openrouter.Apply` with `Search` empty — the four filters only — then ranks
and narrows by search with `match.Rank`. Doing both in `Apply` would mean two
different search semantics competing over the same list. `match.Rank` follows
`ollama/cmd/tui/selector.go:623`: exact match, then prefix, then substring,
then description hit, with match position and length as tiebreaks and a
stable final ordering.

### Prompt

One line of input, reused twice: the profile name after `ctrl+s`, and the API
key when planning returns `ErrNoAPIKey`. A `masked` flag renders input as
dots. Validation errors render inline and keep the user in the prompt — for a
profile name that is `config.AddProfile`'s own `ErrProfileExists`, reused
rather than re-implemented.

A profile saved with `ctrl+s` captures `Options.ExtraArgs` into
`Profile.Args`, so `openrouter-launch claude -- --resume` followed by
`ctrl+s` favorites the invocation the user is actually performing. This
matches `profile add --name … -- <args>`.

### Confirm

Lists every `Plan.Warning` message, then asks the `Warning.Question` the
planner supplied. It never invents question wording: carrying the question
text on the `Warning` rather than a bare `Confirm bool` was a deliberate
planner decision, and this screen is its reason.

When the plan carries warnings but none carries a `Question`, the screen shows
the same list with a footer reading `enter: launch · esc: back`. A footer is
not a question, so this does not violate the rule above.

`Options.AssumeYes` (`--yes`) skips this screen entirely, in both modes,
answering yes. The warnings are still rendered — `cli` writes them to stderr
regardless — so `--yes` suppresses the interruption, never the information.
That is the same contract `--yes` has on the flag-driven path today.

### Notice

Title, lines, and `enter: back`. Renders the planner's typed errors using
their fields, not their `Error()` string: `NotInstalledError.Hint` as install
instructions, `UnknownModelError.Suggestions` as a list,
`UnsupportedAgentError.Reason` as the explanation.

## Data flow

```
root ──select agent──► picker ──enter──► plan ──warnings──► confirm ──yes──► return Plan
 │                       │                │                    │
 │ select profile ───────┼────────────────┘                    │ no
 │                       │ ctrl+s → prompt(name) → save → back  ▼
 │ esc → ErrCancelled    │ esc → root            plan ◄── ErrNoAPIKey ── prompt(masked)
```

Four invariants:

**`--refresh` is consumed exactly once.** `Service.Snapshot` is called twice
per launch: once to populate the picker, once inside `Plan`. Passing
`Refresh: true` to both makes two HTTP round trips for one launch. The flag is
spent on the picker's load, and `Plan` is then always called with
`Refresh: false`, reading the now-warm cache. On the profile path there is no
picker, so `Plan` consumes it instead. This must carry a comment and a test:
it is a one-line mistake that doubles API traffic while looking correct.

**Warnings render in two places, deliberately.** The confirm screen shows them
because that is what the user is deciding on. `cli` *also* writes them to
stderr after `Run` returns and before `Launch`, because the picker runs in the
alt screen and everything it drew is gone once the alt screen tears down —
the stderr line is the only lasting trace in scrollback. This also keeps
`cli`'s launch path uniform with the flag-driven path and avoids an
`AssumeYes` special case.

**Filters save on exit, after re-reading config.** The session loads
`cfg.Filters` through `launch.FilterFrom` at start and, if the filter state
changed, re-reads the config and saves on exit — whether the session launched
or was cancelled. The re-read is not defensive boilerplate: `ctrl+s` can add a
profile during the very session whose filters are being written, and writing a
config captured at start would delete it. `recordSelection` re-reads for the
same reason.

**A profile is a stored reference that can rot.** Its model can be delisted,
its agent uninstalled, or (Phase 3+) demoted to Tier 3. So `UnknownModelError`
and `UnsupportedAgentError` are reachable on the profile path even though the
picker and root screen make them unreachable interactively. Every typed error
therefore needs a real notice screen, not an "impossible" branch.

## Error handling

| Condition | Behavior |
| --- | --- |
| Not a terminal (stdin or stdout) | `Run` returns an error naming `--model`, rather than letting bubbletea hang or fail obscurely |
| `esc` from root, `ctrl+c` anywhere | `ErrCancelled`; `cli` exits 0 silently — backing out is not a failure |
| Catalog load fails, no cache | `Run` returns the error; there is nothing to pick from |
| Catalog stale | Picker opens on cached data; the stale `Warning` reaches the confirm screen and stderr |
| `ErrNoAPIKey` at plan time | Masked prompt, disclosing that the key is saved unconditionally to config before the user types, then retry the plan. No offer/decline: `config.ResolveAPIKey` reads the key from config or the environment only, so a decline-and-still-launch path would need a new override threaded through `internal/launch` |
| `NotInstalledError` | Notice screen with `Hint`; `esc` returns to **root**, not the picker — a different model cannot fix a missing binary, but a different agent can. `Plan` deliberately checks the empty model before the install so the picker is still reachable with nothing installed |
| `UnknownModelError` (profile path) | Notice screen listing `Suggestions`; `esc` returns to root |
| `UnsupportedAgentError` (profile path) | Notice screen with `Reason`; `esc` returns to root |
| Incompatible pairing | Confirm screen with the planner's question; declining returns to the picker |
| Config save fails (filters, profile, key) | Notice screen; the session continues. Persistence is a convenience, not a precondition |

## CLI integration

1. **Root gains `RunE`** for bare invocation, calling `tui.Run` with
   `Agent: nil`. It must also gain `Args: cobra.NoArgs` — once root has a
   `RunE`, cobra routes an unrecognized subcommand into it instead of
   erroring, and `NoArgs` restores `unknown command "bogus"`.
2. **`resolveAndRun`'s `launch.ErrNoModel` branch** becomes `tui.Run` with
   `Agent: spec`, replacing the "the interactive picker arrives in Phase 2"
   message.
3. **A shared `launchPlan` helper** carries the tail both paths need: render
   `plan.Warnings` to stderr, call `Service.Launch`, and apply the
   `isAgentExitError` / `SilenceErrors` handling. Confirmation stays in
   `resolveAndRun` for the flag path and in the confirm screen for the TUI
   path; only the tail is shared.
4. **`ErrCancelled` maps to exit 0** with no error output.

`openrouter-launch <agent> -m <slug>` is unchanged and never opens the TUI.

## Targeted refactors of existing code

Two duplications that this phase would otherwise worsen, both already flagged
in `HANDOFF.md`:

- **`formatPrice` / `formatContext` move to `internal/openrouter`** as
  `FormatPrice` / `FormatContext`. They are unexported in `cli` today and the
  picker needs the identical rendering; both packages already import
  `openrouter`, and these are pure formatters over that package's own types.
  `cli/models.go` delegates. This resolves the open decision recorded as
  Phase 2 note 5 rather than duplicating under time pressure.
- **The `fakeModels()` fixture moves to `internal/openrouter/ortest`.** It is
  currently duplicated in `internal/cli` and `internal/launch` and kept in
  sync by hand; several tests in both packages depend on `openai/o1-mini`
  being the only entry without tool support. A third copy in `tui` makes a
  known hazard worse.

No other refactoring. Notably, Phase 2 note 4 — reconsidering the shared
mutable `&Claude{}` in the registry — **does not apply**: it was conditional
on a background refresh goroutine, which this phase does not add.

## Testing

- **Screen models:** table tests driving `Update` with injected `tea.KeyMsg`
  values, asserting on model state (cursor, filter state, search text,
  resulting choice), not on rendered output. No program is started.
- **Driver:** the `screens` struct is populated with closures returning canned
  choices, so navigation — esc→root, decline→picker, ctrl+s→prompt→picker,
  no-key→prompt→retry, refresh-consumed-once — is tested with no terminal.
- **`match.Rank` and the filter cycles:** pure function table tests.
- **`View`:** substring assertions only, kept tolerant. lipgloss degrades to
  no-color when stdout is not a TTY, which `go test` guarantees, so output is
  stable — but styling is not the behavior under test.
- **Import boundary:** a test using `go/build` (stdlib) to read
  `internal/tui`'s `Imports` and `TestImports` and assert neither contains
  `internal/cli`. The architecture's central claim gets a check, not a comment.

Every test must answer: *would this fail if the behavior it names were
broken?* Both prior phases found that most Important review findings were
defects in plan-authored test code — assertions that passed for the wrong
reason — rather than in implementation.

## New landmines

To be added to `HANDOFF.md` on completion:

- **`tui.Run` must never launch.** The terminal must be out of raw mode
  before `syscall.Exec`. `Run` returns a plan; only the caller launches.
- **`--refresh` is consumed once.** `Plan` is called with `Refresh: false`
  after the picker has already refreshed.
- **`internal/tui` must not import `internal/cli`.** `cli` imports `tui`.
  Enforced by test.
- **Root's `RunE` requires `Args: cobra.NoArgs`**, or unknown subcommands
  silently open the TUI instead of erroring.
