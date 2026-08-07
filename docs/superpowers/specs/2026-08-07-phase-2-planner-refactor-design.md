# Phase 2 prerequisite — planner refactor

**Date:** 2026-08-07
**Status:** Approved for planning
**Supersedes nothing.** Extends `2026-08-07-openrouter-launch-design.md`.

## Summary

Phase 2 is a bubbletea TUI. Before any TUI code is written, the launch logic has
to stop owning the terminal. Today `resolveAndRun` (`internal/cli/launch.go`)
takes a `*cobra.Command`, writes diagnostics to `cmd.ErrOrStderr()`, prompts
interactively, and returns errors for cobra to format. A bubbletea program
cannot call it: it owns the terminal, and a mid-flight `warning: ...` plus
`[y/N] ` written to stderr corrupts the frame.

This refactor extracts the launch sequence into a new `internal/launch` package
that performs IO but never touches the terminal. Every condition a user must see
comes back as a value — a `Warning` or a typed error — and cobra and bubbletea
each render and confirm in their own idiom.

It also lands two things Phase 2 needs and that do not exist yet: the
`config.Filters` ↔ `openrouter.Filter` bridge, and dependency injection for the
catalog source and the process handoff.

Doing this after the TUI exists would mean rewriting both call sites, which is
why it is step one.

## Goals

- `internal/launch` resolves a launch request without terminal IO.
- Advisory conditions (stale catalog, model incompatibility, unsaved selection)
  are returned as `Warning` values carrying their own confirmation prompt.
- Hard stops are typed errors carrying their data, not pre-formatted sentences.
- The catalog source and the process handoff become injected dependencies.
- `models` honors persisted `cfg.Filters`, with explicit flags overriding.
- Existing CLI behavior is preserved byte-for-byte, with the two documented
  exceptions below.

## Non-goals

- Writing any bubbletea code. No `internal/tui` in this change.
- Opening a picker when `--model` is omitted. The branch is pre-wired; the
  behavior stays "error and tell the user to pass `--model`".
- A root `RunE` for bare invocation.
- De-sharing the registry's mutable `&Claude{}` singleton (see Deferred).
- Any new agent.

## Why a new package

`cli` will import the Phase 2 TUI, because root's bare `RunE` and the
`--model`-less picker branch both construct it. If the planner stayed in
`internal/cli`, the TUI would have to import `cli` to reach it, and that is an
import cycle. The planner therefore has to leave `cli` regardless of taste.

It lands in a new `internal/launch` rather than in `internal/agent` so that
`agent` keeps depending only on `openrouter`. Folding the planner into `agent`
would give it imports of `config` (API key resolution) and the catalog cache,
widening the narrowest package in the tree.

```
              internal/launch
               ↑           ↑
        internal/cli   internal/tui   (Phase 2)
               └──────────→┘          cli opens the TUI; tui never imports cli

internal/agent    unchanged — still imports only openrouter
internal/config   unchanged
internal/openrouter unchanged
```

## The launch package

### Service

```go
// Package launch resolves a launch request into a runnable command without
// touching the terminal, so both the CLI and the TUI can drive it.
package launch

// Service resolves launch requests and hands off to agents.
// The zero value is usable and talks to the live OpenRouter API.
type Service struct {
	// Catalog is the model source. nil means the live OpenRouter client.
	Catalog openrouter.Catalog
	// Run performs the process handoff. nil means agent.Run.
	Run func(agent.Command) error
}

type Request struct {
	Spec      *agent.Spec
	ModelID   string
	ExtraArgs []string
	Refresh   bool // bypass the cached catalog
}

type Plan struct {
	Spec     *agent.Spec
	Model    openrouter.Model
	Command  agent.Command
	Warnings []Warning
}
```

`Catalog` and `Run` being nilable fields with live defaults replaces the mutable
package globals `cli.catalogSource` and `cli.runner`. Tests construct a
`Service` with fakes; the TUI receives the same `Service` the CLI built.

### Warnings

```go
type WarningKind int

const (
	WarnStaleCatalog      WarningKind = iota // refresh failed, cached data served
	WarnIncompatibleModel                    // agent may not fully support this model
	WarnSelectionNotSaved                    // last selection could not be persisted
)

type Warning struct {
	Kind WarningKind
	// Message is the diagnostic text, rendered after the caller's own
	// "warning: " prefix.
	Message string
	// Question is non-empty when the caller must get the user's approval
	// before launching. It is the prompt to put to the user.
	Question string
}
```

`Question` carries the prompt rather than a separate `Confirm bool` so a caller
cannot ask "Launch anyway?" about a warning that is not the compatibility one.
Today that string is hardcoded next to the check; making it data means a future
confirm-worthy warning brings its own wording.

`Plan` appends warnings in guard order, so the slice reproduces today's stderr
ordering (stale catalog before compatibility) without the caller knowing why.

### Typed errors

Hard stops carry their data so the TUI can render an install hint as a panel and
suggestions as a selectable list, rather than parsing a sentence.

```go
type UnsupportedAgentError struct{ Agent, Reason string }
type UnsupportedPlatformError struct {
	Agent string
	Err   error // Unwrap
}
type NotInstalledError struct{ Agent, DisplayName, Hint string }
type UnknownModelError struct {
	ModelID     string
	Suggestions []string
}

// ErrNoModel reports that no model was selected. The CLI maps it to the
// "pass --model" message; Phase 2 makes this the open-the-picker branch.
var ErrNoModel = errors.New("no model selected")

// CheckSupported reports why an agent cannot be pointed at OpenRouter.
// Plan's first guard, and also called directly by `profile add`, which
// refuses to save a profile for an unsupported agent without planning a
// launch.
func CheckSupported(spec *agent.Spec) error
```

`cli.checkAgentSupported` is deleted in favor of `CheckSupported`, and
`TestCheckAgentSupported` moves to `launch` with it. `profile add` is the reason
this is exported rather than folded into `Plan`.

Each `Error()` returns exactly the string `resolveAndRun` produces today, so
cobra output is unchanged. `UnsupportedPlatformError` unwraps to the launcher's
own error, preserving `errors.Is` for callers of `PlatformSupported.Supported()`.

`ErrNoModel` is deliberately bare: the message the CLI prints names a CLI flag
and the binary, which the planner has no business knowing.

### Methods

```go
func (s *Service) Snapshot(ctx context.Context, refresh bool) (openrouter.Snapshot, error)
func (s *Service) Plan(ctx context.Context, req Request) (Plan, error)
func (s *Service) Launch(p Plan, warn func(Warning)) error

// StaleWarning returns the warning for a snapshot served from a failed
// refresh, and false if the snapshot is fresh. now is a parameter so the
// function is pure and testable.
func StaleWarning(snap openrouter.Snapshot, now time.Time) (Warning, bool)
```

`Plan` performs IO — catalog fetch, config read — but never touches the
terminal. `loadCatalog` loses its `io.Writer` outright rather than trading it for
a return value: `Snapshot` already carries `Stale`, `StaleErr`, and `Age`, so the
warning was always derivable from what the caller had.

**`Launch`'s `warn` callback is not incidental.** The save-failure warning cannot
be a return value: on Unix the handoff is `syscall.Exec`, nothing after it runs,
and a returned warning would never be printed. It has to be delivered before the
handoff or not at all. `warn` is called synchronously, must not block, and may be
nil. A non-blocking callback is safe for bubbletea because it only pushes a
message — unlike a blocking `Confirm`, which would force the whole launch onto a
goroutine pumping messages back.

Keeping save and handoff inside one function is also what protects the
save-before-handoff ordering from both call sites.

### Guard sequence

`Plan` preserves the existing order, minus the handoff:

1. agent supported → `UnsupportedAgentError`
2. platform supported → `UnsupportedPlatformError`
3. model ID non-empty → `ErrNoModel`
4. agent installed → `NotInstalledError`
5. catalog load → error, or `WarnStaleCatalog`
6. model found → `UnknownModelError` with suggestions
7. config load, API key resolve → error
8. `CheckModel` → `WarnIncompatibleModel` for an `ErrIncompatibleModel`-wrapped
   error; any other error propagates unchanged
9. `Launcher.Command` → the built `agent.Command`

`Launch` then does: save last selection (failure → `WarnSelectionNotSaved` via
`warn`) → handoff.

**Documented deviation.** Confirmation used to sit between steps 8 and 9; it now
happens in the caller, after `Plan` returns. The only observable consequence is
that a `Launcher.Command` build error surfaces without a preceding compatibility
prompt. This is intentional: it removes a prompt for a launch that was going to
fail regardless, and returning a fully-built `agent.Command` is what lets the TUI
show the command before the user commits to it.

`Launch` re-reads config before setting `LastAgent`/`LastModel` rather than
reusing the `*Config` that `Plan` loaded for the API key. In the CLI the two are
equivalent; in a TUI where a profile may be added between planning and launching,
re-reading is the correct one.

## The filters bridge

`config` does not import `openrouter` and should not start; `openrouter` does not
import `config`. `launch` is the only package that already knows both types, so
the bridge lives there.

```go
// Flag names shared by the CLI and the merge, so "was --tools set?" does not
// depend on a string literal duplicated across files.
const (
	FlagTools      = "tools"
	FlagFree       = "free"
	FlagMinContext = "min-context"
	FlagMaxPrice   = "max-price"
)

// FilterFrom converts persisted filter state into a catalog filter.
func FilterFrom(f config.Filters) openrouter.Filter

// MergeFilters returns the persisted filters, overridden by each flag the user
// explicitly set. changed reports whether a flag was provided;
// cmd.Flags().Changed satisfies it directly.
func MergeFilters(persisted config.Filters, flags openrouter.Filter,
	changed func(string) bool) openrouter.Filter
```

`Provider` and `Search` have no persisted counterpart in `config.Filters`, so
they always come straight from flags.

The CLI reads filters and never writes them. `models --free` is a one-shot, not a
setting; the TUI is the only writer, in Phase 2.

## Call sites

`resolveAndRun` shrinks to rendering:

```go
plan, err := a.svc.Plan(cmd.Context(), launch.Request{
	Spec: spec, ModelID: modelID, ExtraArgs: extraArgs, Refresh: a.flags.refresh,
})
if errors.Is(err, launch.ErrNoModel) {
	// Phase 2: this branch opens the picker instead.
	return fmt.Errorf("a model is required: pass --model <slug> (run %q to browse; "+
		"the interactive picker arrives in Phase 2)", "openrouter-launch models")
}
if err != nil {
	return err
}

for _, w := range plan.Warnings {
	fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
	if w.Question == "" {
		continue
	}
	ok, cerr := confirm(cmd, a.flags, w.Question)
	if cerr != nil {
		return cerr
	}
	if !ok {
		return errors.New("cancelled")
	}
}

if err := a.svc.Launch(plan, func(w launch.Warning) {
	fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
}); err != nil {
	if isAgentExitError(err) {
		cmd.SilenceErrors = true
	}
	return err
}
return nil
```

`isAgentExitError` and `confirm` stay in `cli` — `SilenceErrors` is a cobra
concern and `confirm` is a terminal concern.

Wiring in `root.go`:

```go
// app holds what every subcommand needs.
type app struct {
	svc   *launch.Service
	flags *globalFlags
}

func NewRootCmd() *cobra.Command { return NewRootCmdWith(&launch.Service{}) }
func NewRootCmdWith(svc *launch.Service) *cobra.Command
```

`newModelsCmd`, `newProfileCmd`, and `newLaunchCmds` take `*app` instead of
`*globalFlags`. `newProfileAddCmd` swaps `checkAgentSupported` for
`launch.CheckSupported`.

`models` gains a config read:

```go
cfg, err := config.Load()
if err != nil {
	return err
}
f := launch.MergeFilters(cfg.Filters, flagFilter, cmd.Flags().Changed)
if len(args) == 1 {
	f.Search = args[0]
}
```

## Behavior changes

Two, both intended:

1. **Bare `openrouter-launch models` becomes tools-only.** `config.defaults()`
   sets `ToolsOnly: true` ("a coding agent without tool calling is unusable"),
   and `models` now consults it. `--tools=false` restores the full listing.
   `TestModelsCommandListsAll` asserts the old behavior and will fail; it becomes
   a `--tools=false` test, joined by one pinning the new default and one covering
   the explicit override.
2. **A `Launcher.Command` build error no longer follows a compatibility prompt**
   — see the documented deviation above.

Everything else is byte-identical, including every error string and the order in
which warnings reach stderr.

## Testing

`internal/launch` gets its own suite:

- Guard-sequence ordering. Each guard is asserted to fire while the *later*
  guards' preconditions are also unmet, so reordering the sequence fails a test
  rather than passing for the wrong reason.
- The payload on each typed error, not just that an error occurred.
- Warning order and `Question` presence per kind.
- Save-before-handoff. `TestLaunchSavesSelectionBeforeHandoff` moves down from
  `cli` to where the ordering now lives, keeping its trick of inspecting the
  config from inside the `Run` stub.
- `MergeFilters` as a table: persisted-only, flag-only, explicit `--tools=false`
  over a persisted `true`, and `Provider`/`Search` passthrough.
- `StaleWarning` fresh and stale.

`internal/cli` tests replace the two globals with a harness that builds a
`*launch.Service` with a fake catalog and a capturing `Run`, then calls
`NewRootCmdWith`. The harness exposes the root command so the existing tests that
set stdin or assert on errors keep working. `cli.runner` and `cli.catalogSource`
are deleted. Exit-code-suppression tests stay in `cli`.

Every new test must answer: *would this fail if the behavior it names were
broken?* Nine of the ten Important findings in Phase 1 were tests that passed for
the wrong reason.

Landmine 6 is re-verified by exhaustive grep after the move: the only write sites
in the tree remain the catalog cache and `config.Save`.

## Deferred

- **De-sharing the registry's `&Claude{}`.** Tests patch `LookPath` on a shared
  package-level instance. No production code mutates it, so it is a test-only
  hazard; fixing it touches the registry, the agents listing, and every
  launcher-related test. Revisit if the TUI's background refresh ever races it.
- **Root `RunE` and the picker branch.** Pre-wired here, implemented with the TUI.
- **`config.Filters` writes.** The TUI is the only writer.

## Verification

```bash
go test ./... -count=1
go vet ./... && gofmt -l .
GOOS=windows go build ./... && GOOS=darwin go build ./...
go build -o /tmp/orl . && /tmp/orl agents
/tmp/orl models --tools=false   # o1-mini present
/tmp/orl models                 # o1-mini absent
```
