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
make help                                        # every target
make build                                       # ./orl with version info
make test                                        # full suite
make pre-commit                                  # clean, fmt-check, vet, lint, security, test
make ci                                          # everything CI runs
make tools                                       # install pinned lint/security tools
go test ./internal/tui/ -run TestName -v         # single test
make test-race                                   # race check (TUI package)
make cross                                       # cross-compile check
make test-isolated                               # machine-independence run (Landmine 8)
make snapshot                                    # all six release artifacts, published nowhere
```

`make tools` installs analysis binaries with the local Go toolchain
deliberately: prebuilt ones abort once your Go moves ahead of theirs.

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
  capability interfaces detected by type assertion — the whole set is
  `Installable`, `Installer`, `Compatible`, `PlatformSupported`,
  `ConfigWriter`, `CredentialShadowCheck`, and `Staged`
  (`internal/agent/agent.go`). Unsupported agents
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
  configured only via env vars, inline-config env content, CLI overrides,
  or — where nothing else reaches the agent — a key on argv; never by
  writing an agent's own config files. The tree has exactly **five** write
  sites, and all five are sanctioned — see Landmine 6's table in
  `HANDOFF.md` for the authoritative version:

  | # | File | What it writes |
  |---|---|---|
  | 1 | `internal/openrouter/cache.go` | `$XDG_CACHE_HOME/openrouter-launch/models.json` |
  | 2 | `internal/config/config.go` | `$XDG_CONFIG_HOME/openrouter-launch/config.json` — 0600, it holds the API key |
  | 3 | `internal/launch/handoff.go` | the `Staged` materializer, launcher-owned files under our own config dir (openclaw) |
  | 4 | `internal/agent/droid.go` | **agent-owned** write: `ConfigWriter.Apply`, marker-owned entries in `~/.factory/settings.local.json`, restored on exit |
  | 5 | `internal/agent/cline.go` | **agent-owned restore only**: `Apply` snapshots `~/.cline/data/settings/providers.json` and writes nothing; cline itself persists the key `-k` supplies, and restore removes it (Landmine 36) |

  Sites 3, 4, and 5 are not violations of zero-touch; the principle is
  "never write an agent's files *except* through the capability-gated,
  restoring `ConfigWriter`". The enumeration is enforced by grep and pinned
  by `TestWriteSitesAreExhaustivelyEnumerated` (`writesites_test.go`), whose
  allowlist is the machine-checked source of truth — a sixth write site
  anywhere, or any write inside `internal/agent` outside `droid.go` and
  `cline.go`, is a Critical defect. Claude Code and codex are
  pointed at *different* OpenRouter base URLs on purpose (Landmine 1),
  codex requires `wire_api="responses"` (Landmine 18), and cline's key must
  travel on argv because its hub daemon makes env delivery inert
  (Landmine 36).

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

`develop` is the working branch; `main` holds released code. Stable tags
(`vX.Y.Z`) are cut on `main`, prerelease tags (`vX.Y.Z-beta.N`) on `develop`,
and `.github/scripts/check-tag-branch.sh` refuses a tag cut on the wrong
branch. (Through Phase 4b this was direct-to-main with no branches at all;
that changed when CI landed.)

The project is built spec → plan → subagent-driven execution: specs in
`docs/superpowers/specs/`, plans in `docs/superpowers/plans/`; read the
relevant spec for *why* before changing *what*.
