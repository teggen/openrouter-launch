# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Read HANDOFF.md first

`HANDOFF.md` is the canonical project state: current phase, working commands,
and — most importantly — the **Landmines** section, a numbered list of
invariants that each cost real debugging to establish. Treat every landmine
as binding; several plausible-looking "cleanups" (unifying the two base
URLs, merging Catalog into Cache, simplifying the one-program-per-screen
TUI) are explicitly things that broke before. The build ledger under
`.superpowers/sdd/` (gitignored but persistent, cited from HANDOFF.md)
records why past decisions and deferrals were made — read it before
re-litigating one.

## Commands

```bash
go build -o orl .                                # build the binary
go test ./... -count=1                           # full suite
go test ./internal/tui/ -run TestName -v         # single test
go test ./internal/tui/ -race -count=1           # race check (TUI package)
go vet ./... && gofmt -l .                       # lint; gofmt output must be empty
GOOS=windows go build ./... && GOOS=darwin go build ./...   # cross-compile check

# Machine-independence run (Landmine 8): claude/codex/opencode are really
# installed on this machine; the suite must stay green with them invisible.
HOME=$(mktemp -d) PATH="/usr/local/go/bin:/usr/bin:/bin" go test ./... -count=1
```

`./orl models` and any launch command hit the live OpenRouter API (catalog
endpoint is public; launches need a key). The interactive screens refuse
without a TTY, so they cannot be exercised from this harness — headless
bubbletea program tests exist instead (`internal/tui/program_test.go`).

## Architecture

Launch pipeline, one direction, no cycles:

```
main.go → internal/cli (cobra) → internal/launch (planner) → internal/agent (exec)
                                       ↕                          ↑
                                 internal/tui (screens)     internal/openrouter
                                                            (catalog + cache)
```

- **`internal/agent`** — declarative registry of agents. `Launcher` is the
  only required interface and its `Command(Request) (Command, error)` MUST
  be pure (no writes, no network, no spawning): purity is what lets every
  agent be tested by comparing a struct. Everything else is opt-in
  capability interfaces detected by type assertion (`Installable`,
  `Compatible`, `PlatformSupported`, `ConfigWriter`…). Unsupported agents
  stay registered with a stub launcher and a reason — `Spec.Launcher` nil
  panics at init. `ExecArgs` dedupes the environment so our env always
  beats the user's stray exports (Landmine 3); on Unix the handoff is
  `syscall.Exec`, so nothing after it runs.
- **`internal/launch`** — the terminal-free planner. Guards run in a fixed
  order (supported → platform → model → installed → …) returning typed
  condition errors (`UnsupportedAgentError`, `NotInstalledError`, …) that
  the CLI and TUI each render their own way. Side effects (recording the
  last selection, then process handoff) live in one function so their
  order cannot drift.
- **`internal/tui`** — bubbletea, one program per screen; the session
  driver (`tui.Run`) returns an approved `launch.Plan` and never launches,
  so every program tears down before `syscall.Exec`. Must not import
  `internal/cli`, cobra, or pflag (test-enforced). Screen-closure wiring in
  `program.go` must be tested by driving a real headless program, never by
  nil-checks (Landmine 16).
- **`internal/openrouter`** — hand-rolled catalog client behind the narrow
  `Catalog` interface; `Cache` wraps a `Catalog` and adds provenance —
  deliberately not merged. Unknown pricing is never treated as free.
- **Zero-touch principle (the design's central claim):** agents are
  configured only via env vars, inline-config env content, or CLI
  overrides — never by writing an agent's own config files. The tree has
  exactly two write sites (XDG cache + XDG config), verified by grep, and
  the config file is 0600 (it holds an API key). Claude Code and codex are
  pointed at *different* OpenRouter base URLs on purpose (Landmine 1), and
  codex requires `wire_api="responses"` (Landmine 18).

## Testing conventions

- This project's recurring review finding is **tests that pass for the
  wrong reason** (substring assertions satisfiable by unrelated output,
  guards whose deletion still errors differently, fixtures that cannot
  distinguish a property from its negation). Before trusting a new test,
  run its mutation check: break the behavior, watch the named test fail,
  revert.
- Tests that need a binary to look absent must set
  `t.Setenv("HOME", t.TempDir())` — real installs exist on this machine
  and `findPath` has home-dir fallbacks (Landmine 8).

## Workflow

Direct commits to `main` (owner's choice — no feature branches). The
project is built spec → plan → subagent-driven execution: specs in
`docs/superpowers/specs/`, plans in `docs/superpowers/plans/`; read the
relevant spec for *why* before changing *what*.
