# External review triage — 2026-08-15

Three independent model-written reviews of `develop` @ `582c6a9` landed the
same day:

- `review-15-08-2026-qwen3.8-max.md`
- `review-15-08-2026-deepseek-v4-flash.md`
- `review-15-08-2026-glm-5-2.md`

This document records which of their claims survived verification against the
tree, which did not, and the evidence for each verdict. It exists so that a
rejected claim is not re-litigated later from the review file alone: the
reviews are untracked scratch, this is the ledger.

Severity uses the project's own vocabulary: **Critical** (unsanctioned write
site / wrong-model-silently), **Important** (a defect a user can meet, or a
control that no longer does what it claims), **Minor** (edge case, cosmetic,
or docs).

---

## Verified real — acted on

### T1. `make security` was red (qwen §2.1) — **Important, already resolved**

Reproduced exactly: `make security` exited 3 with four *reachable* stdlib
vulnerabilities against the then-local go1.26.5 — GO-2026-6218 (`net/url`),
GO-2026-6090 (`crypto/tls`), GO-2026-5972 (`encoding/asn1`), GO-2026-5026
(`net/http`) — all published 2026-08-13, all traced through the two real IO
paths (`openrouter.Client.Models` and `agent.RunWait`).

Queried `vuln.go.dev` directly for all four: each is fixed in **both**
`1.25.13` and `1.26.6`, and both releases exist on `go.dev/dl`. That matters
in two directions:

- ~~CI resolves `go-version-file: go.mod` → `go 1.25` → newest 1.25.x →
  **1.25.13**, which carries the fixes. CI was never going to go red.~~
  **Wrong, and disproved by the first push: CI went red on exactly this.**
  The runner installed **go1.25.12**. With `check-latest` unset (the
  default), setup-go uses a Go already in the runner image's tool cache
  whenever it satisfies the directive — and a bare `1.25` is satisfied by
  *any* 1.25.x, so it never consulted the version manifest, which did carry
  1.25.13. The minor-only directive expresses the intent but cannot deliver
  it alone. Fixed by adding `check-latest: true` to all six setup-go steps;
  see the comment on `.github/workflows/ci.yml`'s `quality` job for the
  rationale, and note this is the same assumption HANDOFF.md's `go 1.25`
  bullet stated ("`setup-go` resolves the newest 1.25.x rather than pinning
  the oldest") — true only with `check-latest`.
- Locally the toolchain was upgraded to go1.26.6 on 2026-08-15. `make security`
  now exits 0 ("No vulnerabilities found"), and **`make ci` exits 0 end to
  end**. Nothing in the repo needed to change.

One informational finding survives and is not reachable: GO-2026-5024 in
`golang.org/x/sys v0.36.0` (Windows-only, fixed in v0.44.0, not called).

**Correction, same day, after a later task tried the bump: the claim above
was backwards.** `x/sys@v0.44.0` declares `go 1.25.0` (three components),
and `go get golang.org/x/sys@v0.44.0 && go mod tidy` rewrites *this*
module's own directive from `go 1.25` to `go 1.25.0` to match — `go mod
tidy` re-derives the directive from the maximum precision anywhere in the
module graph, so hand-reverting to `go 1.25` and re-running `tidy` reproduces
the same rewrite every time. `go 1.25` and `go 1.25.0` name the same
numeric floor, but `actions/setup-go` resolves them differently. From
setup-go v7's `docs/advanced-usage.md`:

> The `go` directive in `go.mod` can specify a patch version or omit it
> altogether (e.g., `go 1.25.0` or `go 1.25`). If a patch version is
> specified, that specific patch version will be used. If no patch version is
> specified, it will search for the latest available patch version…

All six `setup-go` invocations in this repo (`.github/workflows/ci.yml`
lines 44, 83, 173, 216; `.github/workflows/release.yml` lines 21, 67) use
`go-version-file: go.mod` with no `check-latest`, so `go 1.25` floats to the
newest 1.25.x (today go1.25.13) while `go 1.25.0` would pin CI *and
releases* to exactly go1.25.0 — which is affected by the same four reachable
stdlib vulnerabilities this entry's first paragraph closed locally
(GO-2026-6218, GO-2026-6090, GO-2026-5972, GO-2026-5026, all fixed in
1.25.13). Trading those four back in, in CI and in every release archive, to
clear one uncalled Windows-only advisory is a net loss. **The bump was
therefore attempted and parked, not landed** — see Task 9 in
`docs/superpowers/plans/2026-08-15-external-review-fixes.md` for the record
of the attempt, and `docs/superpowers/plans/2026-08-09-ci-cd-makefile-readme.md:16`
for the original design rationale this corrected claim briefly
contradicted: "`go 1.25.0` would pin CI to the oldest 1.25 patch and
reproduce the very problem this raise was meant to fix."

Standing warning, unchanged: `go1.27rc3` is published. When 1.27 ships, 1.25
stops receiving patches and the `go 1.25` floor becomes a security defect
under Landmine 25 clause 3. The bump is `go 1.25` → `go 1.26`, minor-only.

### T2. `claude` accepts a conflicting model passthrough (qwen §2.2) — **Important**

`Claude.Command` (`internal/agent/claude.go:73-75`) builds
`["--model", model]` and appends `req.ExtraArgs` verbatim. Ten of the eleven
supported launchers call `rejectModelFlag` (droid, openclaw, codex, pi, cline,
kimi, qwen, omp, hermes, opencode); claude is the only one that does not.

So `orl claude -m anthropic/claude-opus-4.6 -- --model openai/gpt-4o-mini`
puts both flags on argv. Whichever one Claude Code's parser honours, this tool
has accepted an argument it explicitly refuses for every other agent, and its
own output, recorded last-selection, and profile all report the managed model.
The env vars (`ANTHROPIC_DEFAULT_*_MODEL`, `CLAUDE_CODE_SUBAGENT_MODEL`) still
carry the managed model, so the session can end up in a *mixed* state — the
top-level model from argv, subagents from env.

Nothing pins the permissive behaviour as deliberate: `claude_test.go` exercises
exactly one passthrough, `--resume`. Claude predates the shared helper
(Phase 1 vs Phase 4a); this reads as an omission, not a decision.

Checked for collateral damage before recommending the fix: `rejectModelFlag`'s
attached-form branch only fires on single-dash `-m…`, and Claude Code has no
single-dash flag beginning with `-m`, so no legitimate passthrough is caught.

### T3. README's Windows caveat is stale (qwen §2.3) — **Important**

`README.md:146` says the Windows leg is `continue-on-error` and reports "19
platform-fixture failures". `.github/workflows/ci.yml:163-168` sets
`experimental: false` for ubuntu, macos, **and** windows — all three legs are
blocking, and the last CI run (`31438274417`, 2026-08-10) succeeded. The README
was never updated after the 2026-08-09 fixture fixes, so **v0.3.0's release
archives ship a user-facing document claiming a green blocking gate is red and
advisory**.

The bullet's tail is still true and must survive the edit: nobody has run the
binary on real Windows, and exit-code propagation is unverified there. Only the
CI-status sentences are wrong. Every other caveat in that section (droid
routing, opencode exit code, omp/qwen shadowing, qwen `modelProviders`, the
non-anthropic Claude Code warning) was checked against the code and holds.

### T4. droid loses a wrong-shaped `customModels` (qwen §2.7) — **Important** (qwen rated it Minor)

`foreignDroidModels` (`internal/agent/droid.go:140`) does
`settings["customModels"].([]any)` and discards the `ok`. If a user's
`~/.factory/settings.local.json` holds valid JSON whose `customModels` is any
other type — a string, an object, a number — the assertion yields nil, `Apply`
overwrites the value with `[ourEntry]`, and `restore` then hits the same nil
path, takes the `len(kept) == 0` branch, and **deletes the key outright**. The
user's value is gone permanently, with no error at any point.

Rated above qwen's Minor because of what it contradicts. `readDroidSettingsFile`
hard-errors on unparseable JSON specifically so we "never clobber what we
cannot understand", and `Apply`'s contract is "returns the restore that undoes
exactly that". Both promises fail here, and droid is already the highest-risk
agent in the tree (unproven routing). The probability is genuinely low — real
droid files carry an array — but the guard is four lines and mirrors a test
that already exists (`TestDroidApplyRefusesUnparseableFile`).

### T5. Stale "four write sites" text (qwen §2.4) — **Minor**

`writesites_test.go:43-45` — the doc comment says a write primitive appears
"only in the four sanctioned files above, and that each of those four still
has one" while `writeSiteAllowlist` directly above it holds five. HANDOFF.md
says "four" in three places (~1540, ~1561, ~1567) and its "expected hits,
exhaustively" list omits `internal/agent/cline.go`.

The invariant itself is intact: the grep run today returns exactly the five
allowlisted files, and `TestWriteSitesAreExhaustivelyEnumerated` is green.
Only the prose lags the cline amendment.

### T6. `orl models --desc` is a silent no-op (qwen §2.6) — **Minor**

`MergeSort` applies `--desc` on top of the persisted column; with no persisted
column the key is `SortNone`, and `SortModels` returns an unchanged copy
regardless of `Desc`. Confirmed empirically against a fresh
`XDG_CONFIG_HOME`: `orl models` and `orl models --desc` print byte-identical
tables and both exit 0.

`internal/cli/models.go` already argues the fix in its own comment, about the
neighbouring case: *"A typo on the command line is fatal … the user is standing
right here, and printing catalog order would look like the sort was applied."*
A `--desc` that cannot do anything is the same failure with a different cause.

### T7. `FormatContext` / `FormatPrice` edge rendering (qwen §2.5, deepseek §9) — **Minor, latent**

`FormatContext` is `fmt.Sprintf("%dk", tokens/1000)`, so any context in 1..999
renders `0k` — "no context" rather than "small context". `FormatPrice` renders
any price under half a cent as `$0.00`, one rounding step from the `free`
reading Landmine 4 exists to prevent.

Reachability probed against the live catalog (413 models): **zero** models have
a context length between 1 and 999, and **zero** have a prompt price in
`(0, $0.005)`. Latent, not currently visible. Worth closing because the
rendering is wrong on its own terms, not because a user is hitting it.

### T8. No test pins omp/qwen *lacking* `CredentialShadowCheck` (glm M3) — **Minor**

Accurate. `ShadowedCredential` is implemented by kimi, openclaw, hermes, and pi.
omp and qwen deliberately do not implement it (omp's credentials live in SQLite
and the tool declines the dependency; qwen's gap is documented in the README),
but only cline's *absence* is pinned, by `TestClineDoesNotClaimCredentialShadowing`.
A contributor adding a best-effort check for omp or qwen would meet a comment,
not a test.

### T9. `perMillion`'s doc comment cites a non-example — **Minor** (not in any review)

Found while checking glm's M2/L5. `internal/openrouter/model.go:85` claims the
rounding turns `0.000015` into `15` "not `15.000000000000002`". Measured:
`0.000015 * 1e6 == 15` **exactly** in float64. The cited noise does not exist
at that value.

The rounding is nevertheless load-bearing — swept 20 000 realistic per-million
prices ($0.01–$200.00 in cent steps) and **530 of them** differ if `math.Round`
is deleted. The comment simply names the wrong example. A real one:
`0.00000097 → 0.97000000000000008438` without rounding.

### T10. No test distinguishes `math.Round` from its deletion — **Minor** (not in any review)

The corollary of T9. Every price asserted in `model_test.go` (15, 75, 1.1, 4.4)
round-trips identically with and without `math.Round`, so no current test can
fail if the rounding is removed. This is precisely the project's named recurring
failure mode.

---

## Verified real — deliberately not acted on

### kimi's `ShadowedCredential` is narrower than "legacy install detected" (qwen §2.7)

Accurate: `internal/agent/kimi.go:141` compares against `legacyKimiPaths(home)[0]`
only, so a legacy install resolving to `~/.local/bin/kimi` (index 1) never warns
— via `lookPath` *or* the fallback loop.

Not changed. The function's own comment scopes the heuristic to the uv tools
directory and states that a PATH hit is deliberately trusted, because the Kimi
Code installer renames legacy shims. kimi is doc-verified-only; widening a
credential heuristic on reasoning alone risks a false "you are on the wrong
account" warning, which is worse than the miss. Revisit with live evidence.

### CLI/TUI decline exit-code asymmetry (qwen §2.7)

Accurate: declining the CLI's `[y/N]` returns `errors.New("cancelled")` →
`Error: cancelled`, exit 1 (`internal/cli/launch.go:79`); backing out of the TUI
returns `tui.ErrCancelled`, which `runTUI` maps to nil, exit 0. Both are
defensible — a declined confirm is a refused instruction, a backed-out picker is
a completed session. Documentation-only, folded into the docs task.

### deepseek's structural concerns — all accurate, all deliberate

Verified and left alone: the catalog client has a 30s timeout and no retry or
backoff (`client.go:32`); `config.Config` has no `version` field;
`PlatformSupported` has no implementers; `picker.go` is 501 lines; catalog
operations are linear scans. Each is an accurate description of a standing
product decision or an open item already recorded in HANDOFF.md, not a defect.
deepseek's review contains no defect claim that verification turned up.

---

## Claims that did not survive verification

### glm H1 — "HANDOFF.md is tracked but deleted in the working tree" — **wrong diagnosis**

glm states `git ls-files HANDOFF.md` confirms the file is still tracked and
recommends `git restore HANDOFF.md`. Run today, `git ls-files HANDOFF.md`
returns **nothing**: the deletion is *staged* (`D ` in porcelain), and
`.gitignore` has been modified in the same working tree to add `/HANDOFF.md`.
That is a deliberate untrack-and-ignore in progress, not an accidental worktree
deletion, and the recommended remedy would not work against an empty index
entry.

Owner decision, 2026-08-15: **intentional — HANDOFF.md stays on disk, local and
untracked.** The file was restored from `HEAD` (1579 lines) and is correctly
ignored. Consequence to keep in view: README.md and CLAUDE.md both point
readers at HANDOFF.md, and 50 `Landmine` citations in `.go` files reference its
numbering, so a fresh clone now gets dangling pointers.

### glm M1 — "the cache is the lone exception to temp-file-then-rename" — **false**

glm argues `internal/openrouter/cache.go` is the only sanctioned write site not
using the atomic temp-then-rename shape, and recommends a comment explaining
why it is exempt. `internal/launch/handoff.go:77-80` (site 3) also uses plain
`os.MkdirAll` + `os.WriteFile`.

The real split is coherent and needs no apology: tool-owned, rebuildable files
are written directly (cache, staged launcher files); user-owned or secret files
go through temp-then-rename (config.json, droid's and cline's settings). Adding
glm's comment would document a distinction that does not exist. The rest of M1
— that a persistently failing cache write is silent — is accurate but is the
documented design ("a cache miss is recoverable and must never block a launch").

### glm L5 — "add a `0.000015 → 15` fixture to catch a rounding regression" — **would add the anti-pattern**

Two errors. First, the fixture already contains `0.000015` at `models[0]`
(`testdata/models.json:9`), asserted exactly by `TestDecodeModelsFields`'
struct comparison. Second and more important, `0.000015` converts to exactly
`15` **with or without** `math.Round` (see T9), so the proposed assertion
cannot fail under the mutation it is meant to catch — a test that passes for
the wrong reason, which CLAUDE.md names as this project's recurring finding.

glm's underlying worry is sound and is carried forward as T10; only its
proposed fixture is wrong. The value that actually discriminates is
`0.00000097 → 0.97`.

### glm M2 — "`v*1e6*1e6` could be `v*1e12`, no behavioral change" — **no value either way**

Swept the same 20 000 realistic prices through both forms: **zero** differences.
The claim is true in range, so the change is safe — and equally pointless. Not
scheduled. Note that "no behavioral change" is not true in general (two
roundings versus one), only across the price range this catalog can produce.

### glm L1–L4 — self-described non-issues, confirmed as such

The unreachable `return nil` after `syscall.Exec`, codex's prepended `-c`
overrides, `descriptionLines`' overflow marker at tiny widths, and `Rank`'s
full-catalog copy on an empty query. All read as described, all correct as
written, no action.

---

## Numbers re-run today

| Check | Result |
|---|---|
| `make ci` (go1.26.6) | **exit 0**, end to end |
| `make security` | exit 0, "No vulnerabilities found" |
| `go test ./... -count=1` | all packages green |
| `make cover-check` | 86.6% vs 80% floor |
| Write-site grep (Landmine 6) | exactly the five allowlisted files |
| `orl models` / `--desc` | byte-identical output (T6) |
| Live catalog probe | 413 models; 0 with context < 1k; 0 with price in (0, $0.005) |
