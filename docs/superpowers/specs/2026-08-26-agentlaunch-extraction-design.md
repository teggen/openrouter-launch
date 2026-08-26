# Extracting the launch layer into `agentlaunch` — 2026-08-26

This records *why* the module boundary moved, for the benefit of whoever later
wonders whether it could be moved back. It is the spec half of a change whose
plan lived at `~/.claude/plans/we-will-soon-build-streamed-nygaard.md`; the
build ledger under `.superpowers/sdd/2026-08-26-agentlaunch-extraction/`
carries the captures and the harness.

A boundary change of this size is the class that gets owner approval at spec
review — the one previous instance was the Landmine 6 amendment — and it had
it before Phase A began.

## The problem

More launcher tools are coming: the same job as `orl` — configure a coding
agent and hand off to it — pointed at other providers (locally served LLMs,
direct vendor APIs, other aggregators, self-hosted gateways). The asset worth
reusing is not the catalog code. It is the eleven agent recipes and the
planner around them:

- codex needs `wire_api="responses"` and rejects `"chat"` outright (Landmine 18)
- Claude Code needs a base URL *without* `/v1` while the catalog needs one
  *with* it (Landmine 1)
- `ANTHROPIC_AUTH_TOKEN` must be present-but-empty or Claude Code silently
  authenticates against Anthropic directly, running the session on the user's
  own account (Landmine 2)
- `execve` does not dedupe `envp`, so a user's stray export beats ours unless
  `ExecArgs` strips it (Landmine 3)
- cline's key cannot travel in the environment, because its hub daemon
  resolves credentials from whatever shell first started it (Landmine 36)
- a `ConfigWriter` agent must never take the `syscall.Exec` handoff (Landmine 24)

Each cost real debugging. Rewriting them per tool means rediscovering them per
tool — and the failure mode of getting one wrong is not a crash. It is a
launch that silently bills the wrong account, or runs a different model than
the one the user picked.

That capability was welded to OpenRouter and sealed inside `internal/`, where
the `internal/` prefix makes it unimportable by anything outside this module
as a matter of language rule, not policy.

## What was decided

A standalone module, `github.com/teggen/agentlaunch`, owning the agent
registry, the eleven launchers parameterized by a `Provider` descriptor, the
exec handoff and the terminal-free planner. `openrouter-launch` becomes its
first consumer with **no change in observable behavior**.

Four owner decisions, made before work started and not re-litigated since:

1. **A new standalone repository**, developed locally alongside this one with
   an uncommitted `go.work`.
2. **Parameterize by `Provider`**, so the module ships the concrete launchers
   and a future `ollama-launch` supplies a different descriptor and gets every
   compatible agent — rather than each tool writing its own eleven launchers.
3. **Restructure in place first, then move.** Phase A decoupled inside this
   repo with the full suite green at every step; Phase B was the physical move.
4. Target provider families: local OpenAI-compatible, direct vendor APIs,
   other aggregators, self-hosted gateways.

## Why parameterize at construction rather than per-request

An earlier draft put `Provider` and `App` on `agent.Request`. That does not
work, and the reason is worth recording because it looks like a free choice.

`CredentialShadowCheck` is `ShadowedCredential() string` — it takes no
request — and four launchers key it on the provider (pi and hermes read
`store["openrouter"]`, openclaw matches on the provider name inside a key,
hermes parses `OPENROUTER_API_KEY=`). There is no request to read a provider
from at that call site. So the provider must live on the launcher, which means
launchers are **constructed against a binding** — which is what makes the
registry a value rather than package state.

That in turn subsumes provider incompatibility into machinery that already
existed. A launcher that cannot be pointed at the bound provider returns an
error wrapping `ErrUnsupportedProvider` from its `New`, and the registry
records it as `Spec.Status{Supported: false, Reason: …}` — which the planner's
`CheckSupported` already guarded, `agents --all` already rendered, and the TUI
already skipped. No new guard, no new typed error, no CLI or TUI plumbing. The
three desktop stubs became the degenerate case of the same rule.

## Why the OpenRouter descriptor did not come along

`provider_openrouter.go` carried its own instruction: it "lives here for now
and moves out to the consuming tool once the registry takes a provider rather
than defaulting to one". The registry became a value in Phase A, so the
condition was met.

The split that resulted is the one worth preserving:

- The **module** keeps the general rule. `Provider.Validate` refuses any
  `AnthropicBaseURL` ending in a version segment — Landmine 1 as a property of
  all providers, machine-checked for the first time, since before this the two
  URLs lived in packages that did not import each other and nothing anywhere
  stated their relationship.
- This **repository** keeps the values that rule constrains, in
  `internal/provider`, along with the tests that pin them: that OpenRouter's
  two roots differ by exactly `/v1`, that the droid marker is frozen, and the
  golden argv-and-environment surface for all eleven agents.

Only the tool that names a provider can know what the right values are.

## The pair of tests, and why neither half suffices

This is the single most important thing to preserve, because it is the control
that makes "nothing changed" a fact rather than a hope.

Every launcher test in the module runs against a **synthetic** provider
(`acme`, a `/v9` root, `ACME_API_KEY`). That is what proves a launcher reads
its `Provider` field rather than a constant of its own — a fixture that merely
*looked* like OpenRouter would be satisfied by a hardcoded value.

The **golden** test here pins the real OpenRouter values, because a launcher
can read its field correctly and still be wired to the *wrong* value, and no
synthetic-provider test can see that.

Verified during the move: falsifying codex's `wire_api` to `"chat"` fails
**only** the golden half; unifying the two base URLs now panics at registry
construction. Keep both halves for anything added later.

The golden test asserts against `provider.Registry()` rather than building a
binding of its own — a re-derived binding would keep passing if the
composition root started building a different one.

## What the extraction let the tests say that they could not before

Two claims became expressible only once the code was a module:

- **Zero third-party dependencies.** Both boundary tests now refuse any import
  outside the standard library, not merely imports of this tool's packages.
  `go.mod` staying empty is a *consequence* of that property, not a check on
  it: it would grow a `require` line silently on the first stray import. A
  consuming tool adopting this module inherits no supply chain, and that is
  now enforced rather than observed.
- **The distinction between a bound registry and the recipes.** In-repo, the
  test fixture was "the registry this tool ships", which conflated the two.
  In the module the fixture is synthetic, and `TestRegistriesAreIndependent`
  needed a *second* synthetic provider to stay meaningful — built from one
  provider twice it would have passed with the `Binding` thrown away entirely.

## Landmine 6 becomes a two-module claim

Three of the five sanctioned write sites moved (`launch/handoff.go`,
`agent/droid.go`, `agent/cline.go`); two stayed
(`internal/openrouter/cache.go`, `internal/config/config.go`). The existing
test walks `.` and checks its allowlist in both directions, so it failed
loudly at the move rather than silently — the good outcome, but it cannot
follow the three that left.

The resolution is deliberately not "copy the test and hope". The module runs
its own copy over its own tree, and this repository additionally runs
`TestDependencyWriteSitesAreExhaustivelyEnumerated`, which resolves the
dependency's source through `go list -m` and walks *that*. Without it, a sixth
write site introduced upstream — or pulled in by a version bump here — would
leave every test in both repositories green while making README's and
CLAUDE.md's enumeration false.

That test fails rather than skips when the dependency cannot be resolved. A
skip there fails **open**: the enumeration would go unchecked on precisely the
runs where it could not be verified, and report success having checked
nothing.

`writeSitePattern` was copied verbatim, including the `os.Create(` paren
anchor. The looser `\bos\.Create\b` form silently missed every `CreateTemp`
hit — which is the atomic-write shape three of the five sites use — so the
pattern that reads as more permissive in fact saw fewer write sites.

## Two build-level traps the split creates

- **`.golangci.yml`'s noctx exclusion.** Its path was
  `internal/agent/exec_(wait|windows)\.go`. Once the package is `agent/`, that
  matches nothing — and matching nothing re-enables a `noctx` failure on
  `RunWait`, the exact site Landmine 24 protects. Confirmed by restoring the
  stale path: two findings under `GOOS=windows`, one under Linux. The
  asymmetry is itself the point — lint is GOOS-sensitive, `exec_windows.go` is
  invisible on Linux, and `make lint-cross` is what closes that.
- **The workspace shadows the published module.** With `~/projects/go.work`
  active, every `go` command resolves `agentlaunch` from disk. A plain
  `make ci` could therefore pass against source that exists on no other
  machine. `ci` sets `GOWORK=off` target-specifically so a green run locally
  is the same claim CI makes. A `replace` directive would produce the same
  divergence permanently and must never be committed: it passes `tidy-check`
  locally and fails CI.

## Verification

- 616 tests before, 621 after (424 here + 197 in the module); nothing lost,
  the five additions are the new guards.
- Coverage: this repo 90.0%, the module 86.8%, both against an unchanged 80%
  floor.
- `make ci` green in both repositories, this one with `GOWORK=off` against the
  tagged `v0.1.0` resolved from the proxy.
- `make snapshot` still produces all six release artifacts.
- Fourteen terminal-reachable commands — `agents`, `agents --all`, `--help`,
  `models --help`, `profile list` (seeded with both a real and an unknown
  agent, so the registry lookup and the unknown-agent row are exercised),
  `profile --help`, `claude --help`, `chatgpt -m x/y`, `profile add`,
  `profile launch nope`, `version`, and three more agent `--help`s — diffed
  byte-for-byte against a binary built from the pre-extraction commit.
  Identical.

## Still open, and deliberately not decided here

Four agents (cline, pi, hermes, omp) select a provider from a list compiled
into the agent, so they cannot be pointed at a relocated endpoint without
writing the agent's own config — which zero-touch forbids outside a restoring
`ConfigWriter`. The options are to ship them unsupported for such providers
(what the registry's `Status`/`Reason` mechanism already renders) or to add
opt-in `ConfigWriter` variants, which would take Landmine 6 from five write
sites to nine.

**Decide it when the second tool exists, not before — and do not add the
enabling flag early, which would decide it implicitly.**
