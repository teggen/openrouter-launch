# openrouter-launch

[![CI](https://github.com/teggen/openrouter-launch/actions/workflows/ci.yml/badge.svg)](https://github.com/teggen/openrouter-launch/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/teggen/openrouter-launch?include_prereleases)](https://github.com/teggen/openrouter-launch/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Pick a model from OpenRouter's catalog and launch a coding agent already
configured to use it.

```
openrouter-launch                              # interactive: pick an agent, pick a model
openrouter-launch claude -m anthropic/claude-opus-4.6
openrouter-launch models --tools --free --provider anthropic
openrouter-launch profile add --name opus-cc --agent claude --model anthropic/claude-opus-4.6
```

## Zero-touch configuration

**This tool does not reconfigure your agents.** Agents are pointed at
OpenRouter through environment variables, inline-config env content, or CLI
overrides — not by editing the config files you maintain. Two agents cannot be
driven that way at all (`droid` and `cline`, below); for those, the launcher
restores the file it touched when the session ends, so there is no `--restore`
flag to run yourself.

It writes exactly five files, and only two of them ever exist for a typical
launch:

| Path | Owner | Holds a secret? |
|---|---|---|
| `$XDG_CACHE_HOME/openrouter-launch/models.json` | this tool | no |
| `$XDG_CONFIG_HOME/openrouter-launch/config.json` | this tool | **yes — mode 0600** |
| `$XDG_CONFIG_HOME/openrouter-launch/openclaw.json` | this tool (openclaw only) | no |
| `~/.factory/settings.local.json` | **Factory Droid** (droid only) | no — the key is an env interpolation |
| `~/.cline/data/settings/providers.json` | **Cline CLI** (cline only) | **yes, during the session — written by cline itself, removed again on exit** |

The last two are the sanctioned exceptions, both capability-gated and both
restored when the session ends. Factory exposes no environment variable or
flag that selects a custom model, so `droid` gets a writer that touches only
its own marker-owned entry. Cline is the inverse: this tool writes nothing
there, but cline persists the key it is given, so the launcher snapshots that
file first and puts it back afterwards — otherwise your key would remain
cline's saved credential long after the session.

Your OpenRouter API key is read from `OPENROUTER_API_KEY` or from
`config.json`, and is never written anywhere else by this tool.

**One exception to keep in mind:** for `cline`, the key is passed on the
command line (`-k`). Cline's interactive session cannot be configured any
other way — it reads credentials from saved settings and from a background
daemon it may have started long before, so an environment variable does not
reach it. While a cline session runs, the key is therefore visible to other
processes on the machine via `/proc/<pid>/cmdline` (and to `ps`). Every other
agent gets the key through the environment only.

## Install

Download an archive from [Releases](https://github.com/teggen/openrouter-launch/releases),
extract it, and put `openrouter-launch` on your `PATH`.

| OS | x86-64 | arm64 |
|---|---|---|
| Linux | `openrouter-launch_<v>_linux_amd64.tar.gz` | `openrouter-launch_<v>_linux_arm64.tar.gz` |
| macOS (Intel / Apple Silicon) | `openrouter-launch_<v>_darwin_amd64.tar.gz` | `openrouter-launch_<v>_darwin_arm64.tar.gz` |
| Windows | `openrouter-launch_<v>_windows_amd64.zip` | `openrouter-launch_<v>_windows_arm64.zip` |

Verify your download against `checksums.txt`, then confirm the binary:

```bash
openrouter-launch --version
```

With a Go toolchain (**Go 1.25 or newer** — 1.24 is end-of-life and carries
unpatched standard-library vulnerabilities):

```bash
go install github.com/teggen/openrouter-launch@latest
```

From source (this is what produces the short `orl` name used throughout the
development docs):

```bash
git clone https://github.com/teggen/openrouter-launch.git
cd openrouter-launch
make build          # produces ./orl
```

If you prefer the short name for a downloaded binary:
`alias orl=openrouter-launch`.

## Supported agents

**Live-verified** means a real completion was produced through OpenRouter using
the installed agent, with a before/after audit confirming nothing was written
into the agent's own config. **Doc-verified** means the mechanism was
established from the agent's documentation and pinned by tests, but has not
been run against a real install.

| Agent | Name | How it is pointed at OpenRouter | Verified |
|---|---|---|---|
| Claude Code | `claude` | environment variables | live |
| Codex CLI | `codex` | managed `-c` overrides plus `-m` | live |
| OpenCode | `opencode` | `OPENCODE_CONFIG_CONTENT` inline JSON | live |
| Pi | `pi` | environment variables | live |
| Hermes Agent | `hermes` | environment variables | live |
| Cline CLI | `cline` | `-P openrouter -m <slug>` plus the key on argv (`-k`), with its provider store snapshotted and restored | live |
| Qwen Code | `qwen` | `--auth-type openai` plus `OPENAI_*` | doc |
| Kimi Code CLI | `kimi` | `KIMI_MODEL_*` environment family | doc |
| Oh My Pi | `omp` | `--model openrouter/<slug>` | doc |
| OpenClaw | `openclaw` | staged config via `OPENCLAW_CONFIG_PATH` | doc |
| Factory Droid | `droid` | marker-owned entry in `settings.local.json`, restored on exit | doc |

`chatgpt`, `claude-desktop`, and `hermes-desktop` are registered as
**unsupported with a stated reason**: a desktop app authenticates through its
own account, so a launcher cannot inject a provider. Running them reports that
reason rather than "unknown agent". They are **hidden from the default
listing** — their reason is long enough to widen every column of the table —
so use `openrouter-launch agents --all` to see them and why.

Run `openrouter-launch agents` for the live list, including what is installed.

## Known caveats

- **`droid`'s routing has never been proven.** The check that distinguishes
  "routed through OpenRouter" from "silently billed to your Factory account" —
  launching with a deliberately invalid OpenRouter key and confirming it fails
  with an auth error — has not been run. Do this before using `droid` for
  anything you care about the bill for.
- **`opencode run` can exit 1 after succeeding.** Once opencode's own
  `models.json` cache is populated, it prints the completion and then exits 1
  with `Error: [DecimalError] Invalid argument: [object Object]`. This
  reproduces with a raw `opencode run`, with none of this tool's code involved.
  Do not gate scripts on that exit code.
- **`omp` and `qwen` get no stored-credential warning.** Both can hold
  credentials that outrank the environment (omp's live in a SQLite database,
  which this tool will not take a dependency on to read). Other agents warn
  when a stored credential may shadow the key you passed; these two cannot.
- **`qwen`'s routing can be silently overridden by your own settings.** If
  `~/.qwen/settings.json` has a `modelProviders.openai[]` entry whose `id`
  matches the launched model slug, it may take precedence over this tool's
  `--auth-type openai` plus `OPENAI_*` configuration — the session would then
  not route through OpenRouter, with no warning from this tool. This has never
  been confirmed against a real qwen install, and it is the most consequential
  open question left after qwen's launcher shipped.
- **Nobody has run the binary on real Windows.** All three CI legs (Linux,
  macOS, Windows) are blocking and green — the Windows platform-fixture
  failures were fixed on 2026-08-09, not skipped — but that covers the test
  suite, not the shipped binary. No end-to-end run (catalog fetch, interactive
  TUI, agent launch) has happened on Windows, and exit-code propagation in
  particular is unverified there.
- **A model that is not `anthropic/*` under Claude Code is advisory, not
  blocked.** You get a warning and a confirm; it works for many models.
- **Do not run two launcher sessions of the same agent at once for `cline` or
  `droid`.** These two are the agents whose own config files this tool writes
  and then restores when the session ends. Nothing locks those files, so
  overlapping sessions interleave their snapshot-and-restore and the second one
  to finish puts back what the first one's session had left in place. For
  `cline` that means your OpenRouter key stays saved in
  `~/.cline/data/settings/providers.json` after everything exits — the exact
  residue the restore exists to prevent. For `droid` it means
  `~/.factory/settings.local.json` is left with its `model` pointing at a
  `custom:openrouter-launch-*` entry that no longer exists, which you have to
  clear by hand. Sequential sessions are fine, and so are concurrent sessions
  of any other agent, or of one `cline` alongside one `droid`.

## Commands

| Command | What it does |
|---|---|
| `openrouter-launch` | interactive: profiles, agents, model picker |
| `openrouter-launch <agent>` | pick a model for that agent, then launch |
| `openrouter-launch <agent> -m <slug> -- <args>` | launch directly, passing `<args>` through |
| `openrouter-launch agents` | list launchable agents and installation status; `--all` adds the unsupported ones with their reason |
| `openrouter-launch models` | list models; `--tools --free --provider --min-context --max-price --sort --desc` |
| `openrouter-launch profile add\|list\|launch\|rm\|rename` | named agent+model favorites |
| `openrouter-launch --refresh …` | bypass the cached catalog |
| `openrouter-launch --version` | build identity |

`--sort` takes `model`, `context`, `input`, `output`, or `tools`, and `--desc`
reverses it. The interactive picker sorts by the same columns from its `ctrl+f`
**Filter & Sort** screen, and remembers the choice between runs. Models whose
pricing OpenRouter does not report show `?` and sort last either way, so a
cheapest-first list never opens with a price nobody knows.

## Development

```bash
make help           # every target
make pre-commit     # clean, fmt-check, vet, lint, security, test
make ci             # everything CI runs
make tools          # install the pinned lint/security tools
make build          # ./orl with version info linked in
make snapshot       # build all six release artifacts locally, publish nothing
```

`make tools` installs analysis tools with your local Go toolchain on purpose —
prebuilt binaries break whenever your Go version moves ahead of theirs. It
sets `GOTOOLCHAIN=auto` so Go may fetch a newer toolchain solely to *build*
those tools; what builds and tests this project stays on the `go.mod` floor.

### The launch layer is a separate module

The agent registry, the eleven launchers and the planner live in
[`github.com/teggen/agentlaunch`](https://github.com/teggen/agentlaunch), a
dependency-free library this tool is the first consumer of. `orl` supplies the
OpenRouter descriptor (`internal/provider`); the module supplies everything
that knows how to configure and hand off to an agent.

Working on both at once wants a Go workspace, which must stay **uncommitted**:

```bash
git clone https://github.com/teggen/agentlaunch ../agentlaunch
go work init . ../agentlaunch      # run from THIS repo's root — see below
```

Create the workspace **inside this repository**, not in the parent directory.
Go finds a `go.work` by walking up from wherever you run it, so one placed in
a folder that holds several projects governs all of them, and any module not
in its `use` list stops building entirely. `go.work` is gitignored here.

The workspace makes your local checkout shadow the published module, so
`go test ./...` here exercises your edits immediately. `make ci` deliberately
sets `GOWORK=off` so it resolves the *tagged* version instead — that is what
makes a green run locally the same claim CI makes.

`make build` and `make snapshot` set it too. A binary built inside the
workspace records `dep github.com/teggen/agentlaunch (devel)` — no version and
no hash, so nothing can tell later which source it contains, which is a poor
property for an artifact you might keep or install. Use `make build-workspace`
when you specifically want a local module change in the binary; it says so on
stdout.

Never commit a `replace` directive: it passes `make tidy-check` on your
machine and fails in CI.

A change spanning both repositories is two commits and a tag, in order: land
and tag the module, then `go get github.com/teggen/agentlaunch@vX.Y.Z` here.

Branches: `develop` is the working branch, `main` holds released code. Stable
tags (`vX.Y.Z`) are cut on `main`, prerelease tags (`vX.Y.Z-beta.N`) on
`develop`; CI refuses a tag cut on the wrong branch.

Versioning is [semantic](https://semver.org/). For this tool that means:
**major** — a command or flag removed or renamed, a breaking `config.json`
schema change, an agent dropped, or a change to the environment contract handed
to a launched agent; **minor** — a new agent, flag, or screen; **patch** —
fixes.

Design docs live in `docs/superpowers/specs/` and implementation plans in
`docs/superpowers/plans/`; read the relevant spec for *why* before changing
*what*. The `.go` comments cite invariants as `Landmine N` — each cost real
debugging, and each sits beside a comment explaining it. Treat them as
binding before changing anything that looks like it could be simplified.
`CLAUDE.md` has the fuller orientation.

## License

MIT — see [LICENSE](LICENSE).
