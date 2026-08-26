# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Landmines

The `.go` comments cite invariants as `Landmine N` — numbered up to 38, of
which twelve are referenced in code. Each one cost real debugging to
establish. Treat every one as binding: several plausible-looking "cleanups"
(unifying the two base URLs, merging `Catalog` into `Cache`, simplifying the
one-program-per-screen TUI) are explicitly things that broke before.

**Most of those twelve citations now live in
`github.com/teggen/agentlaunch`**, not here — the launchers, the exec handoff
and the planner moved there. Landmines 1, 2, 3, 18 and 24 are enforced in that
module; this repository holds the values they constrain
(`internal/provider/openrouter.go`) and the tests that pin them.

The numbering came from a project-state document, `HANDOFF.md`, that is no
longer tracked here. **A full lookup table does still exist**: all 38
landmines are stated in prose in
`.superpowers/sdd/2026-08-15-external-review-fixes/HANDOFF-snapshot-2026-08-16.md`
(table at roughly lines 200–962), which supersedes the older advice to run
`git show 582c6a9:HANDOFF.md`. Read it before touching anything a citation
guards. Even without it the cost is lower than it sounds: every citation sits
beside a comment that states the invariant and why it holds, which is where
the reasoning was always load-bearing — the number is a cross-reference, not
the explanation.

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
make ci                                          # everything CI runs (GOWORK=off — see below)
make tools                                       # install pinned lint/security tools
go test ./internal/tui/ -run TestName -v         # single test
make test-race                                   # race check (TUI package)
make cross                                       # cross-compile check
make test-isolated                               # machine-independence run (Landmine 8)
make snapshot                                    # all six release artifacts, published nowhere
```

`make tools` installs analysis binaries with the local Go toolchain
deliberately: prebuilt ones abort once your Go moves ahead of theirs.

**Two repositories, one workspace.** The launch layer is
`github.com/teggen/agentlaunch`, developed alongside this one at
`~/projects/agentlaunch`. An uncommitted `go.work` **in this repository's
root** (gitignored) lists `.` and `../agentlaunch`, so the local checkout
shadows the published module for every `go` command run here — which is what
lets the two be edited together, and means a plain `go test ./...` can pass
against source that exists on no other machine.

It must NOT live at `~/projects/`. `go` discovers a workspace by walking UP
from the working directory, so one placed in the general projects folder
applies to every Go module beneath it — and a module absent from the `use`
list then fails outright: `go build ./...` in `~/projects/ollama` died with
"directory prefix . does not contain modules listed in go.work". A workspace,
once found, is authoritative. Keep it scoped to the repo that needs it.

`make ci` therefore sets `GOWORK=off` (target-specific, so it reaches every
prerequisite), which is what makes a green run here the same claim CI makes:
resolved from the proxy, at the tagged version. Everything else keeps the
workspace on purpose. **Never commit a `replace` directive** — it passes
`tidy-check` locally and fails CI. Changing the module means tagging it there
first, then `go get github.com/teggen/agentlaunch@vX.Y.Z` here.

`./orl models` and any launch command hit the live OpenRouter API (catalog
endpoint is public; launches need a key). The interactive screens refuse
without a TTY, so they cannot be exercised from this harness — headless
bubbletea program tests exist instead (`internal/tui/program_test.go`).

## Architecture

Launch pipeline, one direction, no cycles:

```
                        ┌──────────────── github.com/teggen/agentlaunch ────┐
main.go → internal/cli →│ launch (planner) ──────────────→ agent (exec)     │
             │  │       │      ↕                               ↓            │
             │  │       │  (guards)                         catalog         │
             │  │       │                        (Model, Catalog, Snapshot) │
             │  │       └───────────────────────────────────↑───────────────┘
             │  │                                           │
             │  └→ internal/provider (the OpenRouter Provider/Host + Registry)
             │                                              │
             └───→ internal/tui (screens) → internal/openrouter
                                            (client, cache, filter/sort/format)
```

Arrows point from importer to imported. **The box is a module boundary, not a
directory**: `agent`, `launch` and `catalog` are a separate repository, and
nothing inside it may import anything belonging to one tool in particular.
That is enforced from inside by `TestAgentDependsOnNothingButTheCatalog` and
`TestLaunchDependsOnNothingButTheAgentsAndTheCatalog`, which check
`pkg.TestImports` as well as `pkg.Imports` — a test reaching for a config
package to seed a real settings file would compile, pass, and make the
package unmovable — and additionally refuse **any** import outside the
standard library, since the module's zero-dependency surface is what lets a
tool adopt it without inheriting a supply chain.

`catalog` is provider-neutral and travels with the launch layer because both
halves need `Model`. `internal/openrouter` stays here and holds everything
vendor-specific — the `/models` wire format, the HTTP client, the on-disk
cache, the presentation helpers — and depends on `catalog` rather than the
other way round. `internal/provider` is the only place a provider is named:
the OpenRouter descriptor, the frozen host marker, and `Registry()`, the one
binding this tool ships.

- **`agent`** (in the module) — declarative registry of agents. `Launcher` is the
  only required interface and its `Command(Request) (Command, error)` MUST
  be pure (no writes, no network, no spawning): purity is what lets every
  agent be tested by comparing a struct. Everything else is opt-in
  capability interfaces detected by type assertion — the whole set is
  `Installable`, `Installer`, `Compatible`, `PlatformSupported`,
  `ConfigWriter`, `CredentialShadowCheck`, and `Staged`
  (`agent/agent.go`). The registry is a **value**, not package
  state: `Builtins()` returns provider-independent `Definition`s and
  `NewRegistry(Binding, []Definition)` resolves them against one `Provider`
  + `Host`, so a second tool binds the same eleven recipes to its own
  provider. A definition whose `New` returns `ErrUnsupportedProvider` stays
  registered with a placeholder launcher and its reason, which is how the
  three desktop apps are refused; every other construction error fails the
  whole registry. `MustRegistry` is the composition-root wrapper that turns
  that into a startup panic; this tool's one call is
  `provider.Registry()` (`internal/provider/openrouter.go`), and it is the
  only place a provider is named. `ExecArgs` dedupes the environment so our env
  always beats the user's stray exports (Landmine 3); on Unix the handoff is
  `syscall.Exec`, so nothing after it runs.
- **`launch`** (in the module) — the terminal-free planner. `Service` is nothing but
  function fields — `LoadCatalog`, `APIKey`, `RecordSelection`, `StageDir` —
  because a planner shared by more than one launcher tool cannot know which
  endpoint to fetch from, where the cache lives, what the settings file looks
  like, or which directory is "ours" to stage into. `cli.newService` is the
  one place all four are named, and `TestNewServiceWiresEverySeam` fails if a
  new one is added and left nil there. `Run`/`RunWait` keep defaults: a
  process handoff is a syscall, not a policy. Guards run in a fixed
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
- **`catalog`** (in the module) — the normalized `Model`, the narrow `Catalog`
  interface, and `Snapshot` (a read plus its provenance). Unknown pricing is
  never treated as free, and `Model` carries **no json struct tags**: the
  cache marshals it directly, so existing files store Go field names and
  adding tags would keep decoding (`encoding/json` matches
  case-insensitively) while zeroing every price — a $75/M model rendering
  free, Landmine 4 by a new route. `TestModelHasNoJSONTags` and the
  `CacheSchema` version both guard that. `catalog/catalogtest` is the shared
  three-model fixture seven test files depend on.
- **`internal/provider`** — the OpenRouter `Provider` and `Host` literals and
  `Registry()`, the single binding this tool ships. Both base URLs live here
  in one struct, which is what lets Landmine 1 be stated as a relationship
  (`AnthropicBaseURL + "/v1" == BaseURL`) rather than as two unrelated
  constants. `Host.Marker` is **persisted user data**, not a label: droid's
  restore recognises our entries in `~/.factory/settings.local.json` by exact
  `displayName` match, so changing it orphans every entry a previous version
  wrote. The golden launch surface for all eleven agents is pinned here too,
  against `Registry()` rather than a re-derived binding.
- **`internal/openrouter`** — hand-rolled catalog client implementing
  `catalog.Catalog`; `Cache` wraps a `catalog.Catalog` and adds provenance —
  deliberately not merged. `Filter`/`SortModels`/`FormatPrice` are
  presentation and stay here on purpose.
- **Zero-touch principle (the design's central claim):** agents are
  configured only via env vars, inline-config env content, CLI overrides,
  or — where nothing else reaches the agent — a key on argv; never by
  writing an agent's own config files. Across BOTH modules there are exactly
  **five** write sites, and all five are sanctioned — the table below is that
  enumeration, and the two allowlists in `writesites_test.go` are its
  machine-checked form. This is Landmine 6, and the pair is now its whole
  statement.

  The **Module** column matters: the extraction split the claim in two, and
  neither repository can see the other's half by walking its own tree.

  | # | Module | File | What it writes |
  |---|---|---|---|
  | 1 | this repo | `internal/openrouter/cache.go` | `$XDG_CACHE_HOME/openrouter-launch/models.json` |
  | 2 | this repo | `internal/config/config.go` | `$XDG_CONFIG_HOME/openrouter-launch/config.json` — 0600, it holds the API key |
  | 3 | agentlaunch | `launch/handoff.go` | the `Staged` materializer, launcher-owned files under our own config dir (openclaw) |
  | 4 | agentlaunch | `agent/droid.go` | **agent-owned** write: `ConfigWriter.Apply`, marker-owned entries in `~/.factory/settings.local.json`, restored on exit |
  | 5 | agentlaunch | `agent/cline.go` | **agent-owned restore only**: `Apply` snapshots `~/.cline/data/settings/providers.json` and writes nothing; cline itself persists the key `-k` supplies, and restore removes it (Landmine 36) |

  Sites 3, 4, and 5 are not violations of zero-touch; the principle is
  "never write an agent's files *except* through the capability-gated,
  restoring `ConfigWriter`". Two tests hold the enumeration here:
  `TestWriteSitesAreExhaustivelyEnumerated` walks this tree against the
  two-entry allowlist, and
  `TestDependencyWriteSitesAreExhaustivelyEnumerated` resolves the
  dependency's source through `go list -m` and walks that against the
  three-entry one — because a sixth site introduced upstream, or pulled in by
  a version bump, would otherwise leave every test in both repositories green
  while making this table false. The module runs its own copy of the walk as
  well. A sixth write site anywhere, or any write inside `agent` outside
  `droid.go` and `cline.go`, is a Critical defect. Claude Code and codex are
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
- Tests that need a binary to look absent must redirect the home directory —
  real installs exist on this machine and `findPath` has home-dir fallbacks
  (Landmine 8). `t.Setenv("HOME", …)` alone is NOT enough: `os.UserHomeDir`
  reads `USERPROFILE` on Windows, and `APPDATA`/`LOCALAPPDATA` feed hermes's
  and qwen's lookups. Use the `testHome(t)` helper — the module's copy in
  `agent/home_test.go`, this repo's in
  `internal/provider/goldenlaunch_test.go`. They are duplicated because a
  `_test.go` helper cannot be imported across packages, and exporting one
  from production code to serve tests would put it in the module's public API.
- **Launcher behavior is pinned by a PAIR of tests, and neither half is
  sufficient.** Every launcher test in the module runs against a *synthetic*
  provider (`acme`, a `/v9` root, `ACME_API_KEY`), which is what proves a
  launcher reads its `Provider` field rather than a constant — a fixture that
  merely looked like OpenRouter would be satisfied by a hardcoded value. The
  golden test here (`internal/provider/goldenlaunch_test.go`) pins the real
  OpenRouter argv and environment, because a launcher can read its field
  correctly and still be wired to the *wrong* value. Falsifying codex's
  `wire_api` to `"chat"` (Landmine 18) fails **only** the golden half. Keep
  both for anything added later.
- **Mutation checks now cross a module boundary.** To break behavior that
  lives in `agentlaunch`, edit `~/projects/agentlaunch` and run this repo's
  test — the workspace makes the change visible immediately, with no tag and
  no `go get`. Copy the file to a scratch directory first: `git checkout` is
  the wrong way to undo a mutation (on an untracked file it restores nothing;
  on a tracked one it silently reverts to HEAD, discarding work in progress).

## Workflow

`develop` is the working branch; `main` holds released code. Stable tags
(`vX.Y.Z`) are cut on `main`, prerelease tags (`vX.Y.Z-beta.N`) on `develop`,
and `.github/scripts/check-tag-branch.sh` refuses a tag cut on the wrong
branch. (Through Phase 4b this was direct-to-main with no branches at all;
that changed when CI landed.)

`github.com/teggen/agentlaunch` is a second repository with its own CI and its
own release cycle; it currently has a single `main` branch and no tag-branch
guard. **A change that spans both is two commits and a tag, in order**: land
and tag the module first, then `go get github.com/teggen/agentlaunch@vX.Y.Z`
here and commit the `go.mod`/`go.sum` bump. The reverse order leaves this
repo's CI red, because CI has no workspace and can only resolve a published
version. An untagged module change is invisible to everyone but this
machine.

The project is built spec → plan → subagent-driven execution: specs in
`docs/superpowers/specs/`, plans in `docs/superpowers/plans/`; read the
relevant spec for *why* before changing *what*.
