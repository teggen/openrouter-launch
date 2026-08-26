# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Landmines

The `.go` comments cite invariants as `Landmine N` — numbered up to 38, of
which twelve are referenced in code. Each one cost real debugging to
establish. Treat every one as binding: several plausible-looking "cleanups"
(unifying the two base URLs, merging `Catalog` into `Cache`, simplifying the
one-program-per-screen TUI) are explicitly things that broke before.

The numbering came from a project-state document, `HANDOFF.md`, that is no
longer part of this repository, so a bare `Landmine N` has no lookup table to
resolve against. That costs less than it sounds: every citation sits beside a
comment that states the invariant and why it holds, which is where the
reasoning was always load-bearing — the number is a cross-reference, not the
explanation. Where you do want the original text, `git show 582c6a9:HANDOFF.md`
still produces it as of 2026-08-15.

Two tracked records outlive it, and both are worth reading before
re-litigating a past decision: the specs and plans under
`docs/superpowers/`, and the build ledger under `.superpowers/sdd/`
(gitignored but persistent), which records why decisions and deferrals were
made and what was tried.

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
                                       ↕                          ↓
                                 internal/tui (screens)     internal/catalog
                                                            (Model, Catalog, Snapshot)
                                                                  ↑
                                                            internal/openrouter
                                                            (client, cache, filter/sort/format)
```

Arrows point from importer to imported. The split at the bottom is the one
that matters. `internal/catalog` is provider-neutral and imports nothing else
in this repo; `internal/openrouter` holds everything vendor-specific — the
`/models` wire format, the HTTP client, the on-disk cache, the presentation
helpers — and depends on `catalog` rather than the other way round.
`internal/agent`'s only in-repo import is `internal/catalog`, enforced by
`TestAgentDependsOnNothingButTheCatalog`. `internal/launch` still reaches
`internal/openrouter` and `internal/config` for its cache and settings; those
two edges become injected fields next.

- **`internal/agent`** — declarative registry of agents. `Launcher` is the
  only required interface and its `Command(Request) (Command, error)` MUST
  be pure (no writes, no network, no spawning): purity is what lets every
  agent be tested by comparing a struct. Everything else is opt-in
  capability interfaces detected by type assertion — the whole set is
  `Installable`, `Installer`, `Compatible`, `PlatformSupported`,
  `ConfigWriter`, `CredentialShadowCheck`, and `Staged`
  (`internal/agent/agent.go`). The registry is a **value**, not package
  state: `Builtins()` returns provider-independent `Definition`s and
  `NewRegistry(Binding, []Definition)` resolves them against one `Provider`
  + `Host`, so a second tool binds the same eleven recipes to its own
  provider. A definition whose `New` returns `ErrUnsupportedProvider` stays
  registered with a placeholder launcher and its reason, which is how the
  three desktop apps are refused; every other construction error fails the
  whole registry. `MustRegistry` is the composition-root wrapper that turns
  that into a startup panic (`internal/cli/root.go`), and it is the only
  place a provider is named. `ExecArgs` dedupes the environment so our env
  always beats the user's stray exports (Landmine 3); on Unix the handoff is
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
- **`internal/catalog`** — the normalized `Model`, the narrow `Catalog`
  interface, and `Snapshot` (a read plus its provenance). Unknown pricing is
  never treated as free, and `Model` carries **no json struct tags**: the
  cache marshals it directly, so existing files store Go field names and
  adding tags would keep decoding (`encoding/json` matches
  case-insensitively) while zeroing every price — a $75/M model rendering
  free, Landmine 4 by a new route. `TestModelHasNoJSONTags` and the
  `CacheSchema` version both guard that. `catalog/catalogtest` is the shared
  three-model fixture seven test files depend on.
- **`internal/openrouter`** — hand-rolled catalog client implementing
  `catalog.Catalog`; `Cache` wraps a `catalog.Catalog` and adds provenance —
  deliberately not merged. `Filter`/`SortModels`/`FormatPrice` are
  presentation and stay here on purpose.
- **Zero-touch principle (the design's central claim):** agents are
  configured only via env vars, inline-config env content, CLI overrides,
  or — where nothing else reaches the agent — a key on argv; never by
  writing an agent's own config files. The tree has exactly **five** write
  sites, and all five are sanctioned — the table below is that enumeration,
  and `writeSiteAllowlist` in `writesites_test.go` is its machine-checked
  form. This is Landmine 6, and the pair is now its whole statement.

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
