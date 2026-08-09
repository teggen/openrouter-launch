# Phase 4a — Six Zero-Touch Tier 2 Launchers: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `pi`, `hermes`, `qwen`, `cline`, `kimi`, and `omp` as zero-touch
launchers, plus the shared plumbing they need: extracted passthrough-conflict
helpers and the `CredentialShadowCheck` advisory capability.

**Architecture:** Six new per-agent files in `internal/agent`, each a small
struct with a pure `Command()` following the `Claude`/`Codex` pattern. One new
opt-in capability (`CredentialShadowCheck`) detected by type assertion in the
planner, surfacing as an advisory `Warning` exactly like the Landmine 7
incompatible-model confirm. No TUI changes; the CLI/TUI already render
`Question`-bearing warnings generically.

**Tech Stack:** Go 1.24, stdlib only (no new dependencies).

**Spec:** `docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md` —
read it first; per-agent research with doc citations is in
`.superpowers/sdd/2026-08-09-tier-2-research/`. The "Landmines" section of
`HANDOFF.md` is binding.

## Global Constraints

- `Command()` MUST be pure: no file writes, no network, no process spawn, no
  `exec.LookPath` except through the injectable `LookPath` field.
- Zero-touch (Landmine 6): this plan adds NO write sites — the tree keeps
  exactly two (cache + config). `ShadowedCredential()` detectors READ agent
  files; they never write. (Plan 4b adds the two sanctioned new sites.)
- All six agents use `openrouter.DefaultBaseURL` (`https://openrouter.ai/api/v1`,
  **with** `/v1`) where a base URL appears. Only Claude Code uses the no-`/v1`
  form (Landmine 1).
- Slug dialects: `omp` prefixes the OpenRouter slug with `openrouter/`; the
  other five pass it verbatim. A bare slug is a *valid-looking wrong value*
  for omp — its mutation check is mandatory.
- Landmine 8, widened: `hermes` and `pi` are REALLY installed on this machine
  (`~/.local/bin/hermes`, `~/.local/bin/pi`). Every test that needs a binary or
  a credential store to look absent sets `t.Setenv("HOME", t.TempDir())`.
- Key delivery is env-only (owner decision): never place the API key on argv.
- Every non-obvious behavior gets a mutation check: break it, watch the named
  test FAIL, revert.
- Doc-verified values are version-stamped (pi 0.84.1, hermes 0.20.0, qwen
  0.21.8, cline 3.0.51, kimi-code 0.34.0, omp 17.2.11, all 2026-08-09). Task 9
  re-verifies live; if a value is falsified, fix the launcher + its test in
  that task and record it in the spec (the Landmine 18 protocol).
- Commit directly to `main`. `gofmt -l .` empty and `go vet ./...` clean
  before every commit. Commit messages end with:
  `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`

---

### Task 1: Shared passthrough-conflict helpers

HANDOFF's deferred-minors list says the model-flag matcher duplicated across
codex/opencode gets extracted "on third use" — this phase is uses three
through eight. Extract it, plus a generic managed-flag rejector, and refactor
the two existing call sites. Behavior-preserving: the existing codex/opencode
tests must stay green untouched.

**Files:**
- Create: `internal/agent/args.go`
- Create: `internal/agent/args_test.go`
- Modify: `internal/agent/codex.go` (replace the model-flag case inside
  `codexValidateExtraArgs`)
- Modify: `internal/agent/opencode.go` (replace `opencodeValidateExtraArgs`)

**Interfaces:**
- Consumes: nothing new.
- Produces (package-private, used by every launcher task):
  `rejectModelFlag(agentName string, args []string) error` and
  `rejectFlags(agentName string, args []string, flags ...string) error`.

- [ ] **Step 1: Write the failing tests**

`internal/agent/args_test.go`:

```go
package agent

import (
	"strings"
	"testing"
)

func TestRejectModelFlag(t *testing.T) {
	for _, arg := range []string{"-m", "-mfoo", "--model", "--model=x/y"} {
		err := rejectModelFlag("pi", []string{"--verbose", arg})
		if err == nil {
			t.Errorf("%q accepted, want error", arg)
			continue
		}
		if !strings.Contains(err.Error(), arg) || !strings.Contains(err.Error(), "pi") {
			t.Errorf("%q: error %q does not name the argument and agent", arg, err)
		}
	}
	if err := rejectModelFlag("pi", []string{"--verbose", "-p", "hello", "--mode", "fast"}); err != nil {
		t.Errorf("benign args rejected: %v", err)
	}
}

func TestRejectFlags(t *testing.T) {
	// Long flag: separate and equals forms. Short flag: separate, attached.
	for _, arg := range []string{"--provider", "--provider=x", "-P", "-Px"} {
		err := rejectFlags("cline", []string{arg}, "--provider", "-P")
		if err == nil {
			t.Errorf("%q accepted, want error", arg)
			continue
		}
		if !strings.Contains(err.Error(), arg) {
			t.Errorf("%q: error %q does not name the argument", arg, err)
		}
	}
	// --providerfoo is a DIFFERENT flag, not an attached form of --provider.
	for _, arg := range []string{"--providerfoo", "-Q", "--prov"} {
		if err := rejectFlags("cline", []string{arg}, "--provider", "-P"); err != nil {
			t.Errorf("%q rejected, want accepted: %v", arg, err)
		}
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run 'TestRejectModelFlag|TestRejectFlags' -v`
Expected: FAIL — `undefined: rejectModelFlag`.

- [ ] **Step 3: Implement `internal/agent/args.go`**

```go
package agent

import (
	"fmt"
	"strings"
)

// rejectModelFlag returns an error naming the first passthrough argument
// that would override the managed model selection: -m / --model in
// separate, attached (-mfoo), or equals (--model=foo) form. Launchers
// reject these because the agent-side flag outranks the managed
// configuration, so accepting one would silently launch a different model
// while the tool reports success — the Landmine 3 failure class, on argv.
func rejectModelFlag(agentName string, args []string) error {
	for _, arg := range args {
		if arg == "-m" || arg == "--model" ||
			strings.HasPrefix(arg, "--model=") ||
			(strings.HasPrefix(arg, "-m") && len(arg) > len("-m")) {
			return fmt.Errorf("%s: conflicting argument %q: openrouter-launch manages the model; pick it with openrouter-launch %s -m", agentName, arg, agentName)
		}
	}
	return nil
}

// rejectFlags returns an error naming the first passthrough argument that
// matches one of flags, in separate ("--flag value"), equals
// ("--flag=value"), or — for single-dash short flags — attached ("-Pval")
// form. Launchers list the flags whose values the managed launch owns.
func rejectFlags(agentName string, args []string, flags ...string) error {
	for _, arg := range args {
		for _, f := range flags {
			short := len(f) == 2 && f[0] == '-' && f[1] != '-'
			if arg == f || strings.HasPrefix(arg, f+"=") ||
				(short && strings.HasPrefix(arg, f) && len(arg) > len(f)) {
				return fmt.Errorf("%s: conflicting argument %q: openrouter-launch manages this setting for the launch", agentName, arg)
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Refactor the two existing call sites**

In `internal/agent/opencode.go`: delete `opencodeValidateExtraArgs` entirely
and replace its call in `Command` with:

```go
	if err := rejectModelFlag("opencode", req.ExtraArgs); err != nil {
		return Command{}, err
	}
```

In `internal/agent/codex.go`: inside `codexValidateExtraArgs`, delete the
first `case` (the `-m`/`--model` arm) and instead call the helper before the
loop:

```go
func codexValidateExtraArgs(args []string) error {
	if err := rejectModelFlag("codex", args); err != nil {
		return err
	}
	for i, arg := range args {
		switch {
		case arg == "-c" || arg == "--config":
			// … existing -c/--config cases unchanged …
```

Do not touch `codexOverrideConflicts`.

- [ ] **Step 5: Run the agent package tests, verify green with no test edits**

Run: `go test ./internal/agent/ -count=1 -v | grep -E 'FAIL|ok '`
Expected: PASS. `TestCodexCommandRejectsConflictingExtras` and
`TestOpenCodeCommandRejectsModelExtras` prove the refactor preserved both
messages (they assert the argument is named). If either fails, the helper's
message drifted — fix the helper, never the tests.

- [ ] **Step 6: Mutation checks**

1. Make `rejectModelFlag` return nil always → `TestRejectModelFlag`,
   `TestCodexCommandRejectsConflictingExtras`, and
   `TestOpenCodeCommandRejectsModelExtras` all FAIL.
2. Drop the attached-form arm (`-mfoo`) → `TestRejectModelFlag` FAILS.
3. In `rejectFlags`, drop the `short` attached arm → `TestRejectFlags` FAILS
   on `-Px`.

- [ ] **Step 7: Full package check and commit**

Run: `go test ./internal/agent/ -count=1 && go vet ./internal/agent/ && gofmt -l internal/agent/` (must print nothing)

```bash
git add internal/agent/args.go internal/agent/args_test.go internal/agent/codex.go internal/agent/opencode.go
git commit -m "refactor(agent): extract shared passthrough-conflict helpers

Third use arrived (six Tier 2 launchers); the codex/opencode model-flag
matchers fold into rejectModelFlag/rejectFlags.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: CredentialShadowCheck capability and planner warning

Five Tier 2 agents keep credential stores that outrank the process
environment. Owner decision: env-only key delivery + a Landmine 7-style
advisory. This task adds the capability, the warning kind, and the planner
guard — with a fake launcher; real detectors arrive with each agent.

**Files:**
- Modify: `internal/agent/agent.go` (append the interface)
- Modify: `internal/launch/warning.go` (append `WarnShadowedCredential`)
- Modify: `internal/launch/plan.go` (insert guard after the `Compatible`
  block, currently plan.go:109-123, before the `Command` build)
- Test: `internal/launch/plan_test.go` (extend)

**Interfaces:**
- Consumes: `fakeLauncher`, `spec(name, launcher)`, `newTestService(t)` from
  `internal/launch/plan_test.go`; mirror
  `TestPlanIncompatibleModelYieldsConfirmableWarning` (plan_test.go:218) for
  harness usage — reuse ITS `launch.Request` literal (same `ModelID` the
  harness catalog serves).
- Produces: `agent.CredentialShadowCheck` interface with
  `ShadowedCredential() string`; `launch.WarnShadowedCredential`
  `WarningKind`. Tasks 3, 4, 6, 7 implement the interface; Plan 4b's openclaw
  does too.

- [ ] **Step 1: Write the failing test**

Append to `internal/launch/plan_test.go`:

```go
type shadowingLauncher struct {
	fakeLauncher
	msg string
}

func (s *shadowingLauncher) ShadowedCredential() string { return s.msg }

func TestPlanShadowedCredentialYieldsConfirmableWarning(t *testing.T) {
	svc := newTestService(t)
	// Copy the Request literal from
	// TestPlanIncompatibleModelYieldsConfirmableWarning so the fake catalog
	// resolves the model; only the Spec differs.
	sh := &shadowingLauncher{msg: "fake has a stored OpenRouter credential that outranks the launched key"}
	p, err := svc.Plan(context.Background(), Request{
		Spec:    spec("fake", sh),
		ModelID: "anthropic/claude-opus-4.6",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var found *Warning
	for i := range p.Warnings {
		if p.Warnings[i].Kind == WarnShadowedCredential {
			found = &p.Warnings[i]
		}
	}
	if found == nil {
		t.Fatalf("no WarnShadowedCredential in %+v", p.Warnings)
	}
	if found.Message != sh.msg {
		t.Errorf("Message = %q, want %q", found.Message, sh.msg)
	}
	if found.Question == "" {
		t.Error("Question empty: warning is not confirmable, launch would proceed unasked")
	}
}

func TestPlanNoShadowWarningWhenDetectorSilent(t *testing.T) {
	svc := newTestService(t)
	p, err := svc.Plan(context.Background(), Request{
		Spec:    spec("fake", &shadowingLauncher{msg: ""}),
		ModelID: "anthropic/claude-opus-4.6",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, w := range p.Warnings {
		if w.Kind == WarnShadowedCredential {
			t.Fatalf("empty detector produced warning %+v", w)
		}
	}
}
```

(If the harness catalog serves a different model ID, take it verbatim from
`TestPlanIncompatibleModelYieldsConfirmableWarning` — both new tests use the
same one.)

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/launch/ -run TestPlanShadow -v`
Expected: FAIL — `undefined: WarnShadowedCredential`.

- [ ] **Step 3: Implement**

Append to the interface list in `internal/agent/agent.go`:

```go
// CredentialShadowCheck reports stored agent-side state that would make a
// launch ignore the environment this tool provides — a saved credential
// that outranks env vars (pi, cline, hermes document exactly that), or a
// binary generation that does not read them (legacy kimi-cli). Read-only
// and best-effort: implementations must never write, and must return ""
// (no warning) when the state is absent, unreadable, or unparseable —
// a detector failure must never block a launch.
type CredentialShadowCheck interface {
	ShadowedCredential() string
}
```

Append to the `const` block in `internal/launch/warning.go` (AFTER
`WarnSelectionNotSaved` — appending keeps existing iota values stable):

```go
	// WarnShadowedCredential reports agent-side stored credentials or state
	// that outrank the environment this launch provides. Advisory: the
	// wrong-account risk is made visible, the user decides.
	WarnShadowedCredential
```

Insert in `internal/launch/plan.go`, immediately after the `Compatible`
block and before the `spec.Launcher.Command(...)` call:

```go
	if shadow, ok := spec.Launcher.(agent.CredentialShadowCheck); ok {
		if msg := shadow.ShadowedCredential(); msg != "" {
			warnings = append(warnings, Warning{
				Kind:     WarnShadowedCredential,
				Message:  msg,
				Question: "Launch anyway?",
			})
		}
	}
```

- [ ] **Step 4: Run tests, verify they pass**

Run: `go test ./internal/launch/ -count=1 -v | grep -E 'FAIL|ok '`
Expected: PASS.

- [ ] **Step 5: Confirm the CLI and TUI render the new kind generically**

Run: `grep -rn "WarnIncompatibleModel\|\.Kind" internal/cli/ internal/tui/ --include="*.go" | grep -v _test`

Expected: no exhaustive switch on `WarningKind` — both layers key off
`Warning.Question`/`Warning.Message`. If a switch on Kind exists anywhere,
add a `WarnShadowedCredential` case mirroring the `WarnIncompatibleModel`
arm and extend that layer's test the same way its incompatible-model case is
tested. Then run `go test ./... -count=1`.

- [ ] **Step 6: Mutation checks**

1. Delete the new guard block in `plan.go` →
   `TestPlanShadowedCredentialYieldsConfirmableWarning` FAILS.
2. Drop `Question:` from the new Warning →
   the same test FAILS on the empty-Question assertion.
3. Make the guard append the warning even for `msg == ""` →
   `TestPlanNoShadowWarningWhenDetectorSilent` FAILS.

- [ ] **Step 7: Full check and commit**

Run: `go test ./... -count=1 && go vet ./... && gofmt -l .` (must print nothing)

```bash
git add internal/agent/agent.go internal/launch/warning.go internal/launch/plan.go internal/launch/plan_test.go
git commit -m "feat(launch): advisory warning for agent-side credential shadowing

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: pi launcher

Doc-verified on pi 0.84.1: built-in `openrouter` provider, bare slugs
(confirmed against the shipped catalog), `OPENROUTER_API_KEY`, and a
documented precedence trap — `~/.pi/agent/auth.json` outranks the env var.

**Files:**
- Create: `internal/agent/pi.go`
- Test: `internal/agent/pi_test.go`
- Modify: `internal/agent/registry.go` (insert entry before `chatgpt`)
- Modify: `internal/agent/registry_test.go` (add `TestRegistryTier2Agents`)

**Interfaces:**
- Consumes: `rejectModelFlag`, `rejectFlags` (Task 1); test helpers
  `stubLookPath`, `testModel()`, `envValue` from `claude_test.go`.
- Produces: `type Pi struct { LookPath func(string) (string, error) }` with
  `Name() "pi"`, `DisplayName() "Pi"`, `Command`, `CheckInstalled`,
  `InstallHint`, `ShadowedCredential`. Registry entry `pi`.

- [ ] **Step 1: Write the failing tests**

`internal/agent/pi_test.go`:

```go
package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPiCommandPathArgsEnv(t *testing.T) {
	p := &Pi{LookPath: stubLookPath("/usr/local/bin/pi")}
	cmd, err := p.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--thinking", "high"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != "/usr/local/bin/pi" {
		t.Errorf("Path = %q", cmd.Path)
	}
	// Bare OpenRouter slug: pi selects the provider with --provider, never
	// an "openrouter/" prefix on the model (that is omp's dialect, not pi's).
	want := []string{"--provider", "openrouter", "--model", "anthropic/claude-opus-4.6", "--thinking", "high"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
	for _, e := range cmd.Env {
		if strings.Contains(e, "sk-or-test") && !strings.HasPrefix(e, "OPENROUTER_API_KEY=") {
			t.Errorf("key leaked into unexpected env entry %q", e)
		}
	}
}

func TestPiCommandRequiresAPIKey(t *testing.T) {
	p := &Pi{LookPath: stubLookPath("/usr/local/bin/pi")}
	if _, err := p.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestPiCommandRejectsConflictingExtras(t *testing.T) {
	p := &Pi{LookPath: stubLookPath("/usr/local/bin/pi")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"-mx/y"}, {"--model", "x/y"}, {"--model=x/y"},
		{"--provider", "anthropic"}, {"--provider=anthropic"},
	} {
		if _, err := p.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
}

// Landmine 8: pi is really installed at ~/.local/bin/pi on this machine.
func TestPiFindPathFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }

	missing := &Pi{LookPath: notOnPath}
	if missing.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}

	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !missing.CheckInstalled() {
		t.Error("CheckInstalled = false with binary at ~/.local/bin/pi")
	}
}

func TestPiInstallHint(t *testing.T) {
	p := &Pi{}
	if hint := p.InstallHint(); !strings.Contains(hint, "@earendil-works/pi-coding-agent") {
		t.Errorf("InstallHint = %q; the legacy @mariozechner package is deprecated", hint)
	}
}

func TestPiShadowedCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p := &Pi{}

	if msg := p.ShadowedCredential(); msg != "" {
		t.Errorf("no auth.json: msg = %q, want empty", msg)
	}

	dir := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(dir, "auth.json")

	if err := os.WriteFile(authPath, []byte(`{"anthropic":{"type":"oauth"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := p.ShadowedCredential(); msg != "" {
		t.Errorf("auth.json without openrouter: msg = %q, want empty", msg)
	}

	if err := os.WriteFile(authPath, []byte(`{"openrouter":{"type":"api_key"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := p.ShadowedCredential(); !strings.Contains(msg, "auth.json") {
		t.Errorf("stored openrouter credential: msg = %q, want it to name auth.json", msg)
	}

	if err := os.WriteFile(authPath, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := p.ShadowedCredential(); msg != "" {
		t.Errorf("malformed auth.json: msg = %q, want empty (detector failure must not warn)", msg)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestPi -v`
Expected: FAIL — `undefined: Pi`.

- [ ] **Step 3: Implement `internal/agent/pi.go`**

```go
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Pi launches the pi coding agent (earendil-works/pi) against an OpenRouter
// model. OpenRouter is a built-in pi provider with the base URL baked in
// upstream, so the launch is two flags plus one env var; nothing is written.
// Doc-verified on pi 0.84.1 (2026-08-09); see
// .superpowers/sdd/2026-08-09-tier-2-research/pi.md.
type Pi struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (p *Pi) Name() string        { return "pi" }
func (p *Pi) DisplayName() string { return "Pi" }

func (p *Pi) lookPath(file string) (string, error) {
	if p.LookPath != nil {
		return p.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the pi binary, falling back to the install script's
// ~/.local/bin location, which is not reliably on PATH.
func (p *Pi) findPath() (string, error) {
	if path, err := p.lookPath("pi"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("pi binary not found: %w", err)
	}
	candidate := filepath.Join(home, ".local", "bin", "pi")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("pi binary not found")
}

// Command builds the pi invocation. Pure: nothing written, nothing spawned.
// The slug passes through verbatim — pi's catalog keys models by bare
// OpenRouter slugs; the provider is selected by --provider, never by an
// "openrouter/" prefix on the model.
func (p *Pi) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("pi: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("pi", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags("pi", req.ExtraArgs, "--provider"); err != nil {
		return Command{}, err
	}
	path, err := p.findPath()
	if err != nil {
		return Command{}, err
	}
	args := []string{"--provider", "openrouter", "--model", req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{"OPENROUTER_API_KEY=" + req.APIKey}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckInstalled reports whether the pi binary can be found.
func (p *Pi) CheckInstalled() bool {
	_, err := p.findPath()
	return err == nil
}

// InstallHint tells the user how to install pi. The legacy
// @mariozechner/pi-coding-agent npm package is deprecated; install only the
// earendil-works one.
func (p *Pi) InstallHint() string {
	return "Install pi: npm install -g --ignore-scripts @earendil-works/pi-coding-agent"
}

// ShadowedCredential reports pi's documented precedence trap: a credential
// in ~/.pi/agent/auth.json (e.g. from "/login openrouter") outranks the
// OPENROUTER_API_KEY env var, so the session would bill that stored account
// instead of the key this launch provides.
func (p *Pi) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "auth.json"))
	if err != nil {
		return ""
	}
	var store map[string]json.RawMessage
	if err := json.Unmarshal(data, &store); err != nil {
		return ""
	}
	if _, ok := store["openrouter"]; !ok {
		return ""
	}
	return "pi has a stored OpenRouter credential (~/.pi/agent/auth.json) that outranks the key this launch provides"
}
```

- [ ] **Step 4: Register and pin the registry entry**

In `internal/agent/registry.go`, insert into the `specs` literal immediately
before the `chatgpt` entry:

```go
	{
		Name:        "pi",
		Launcher:    &Pi{},
		Description: "Minimal extensible terminal coding agent",
		Status:      Status{Supported: true},
	},
```

Append to `internal/agent/registry_test.go` (later agent tasks extend the
slice — that is this test's designed growth path):

```go
func TestRegistryTier2Agents(t *testing.T) {
	// Grows by one name per Phase 4a agent task.
	for _, name := range []string{"pi"} {
		spec, err := Lookup(name)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", name, err)
		}
		if !spec.Status.Supported {
			t.Errorf("%q registered unsupported", name)
		}
		if len(spec.Aliases) != 0 {
			t.Errorf("%q has aliases %q, spec says none", name, spec.Aliases)
		}
	}
}
```

- [ ] **Step 5: Run the full suite**

Run: `go test ./... -count=1`
Expected: PASS. A `cli` or `tui` failure that is just "one more row in a
listing" means a test hardcoded the registry — update the TEST expectation,
never the registry. Anything semantic: STOP and report.

- [ ] **Step 6: Mutation checks**

1. Swap `req.Model.ID` for `"openrouter/" + req.Model.ID` in args →
   `TestPiCommandPathArgsEnv` FAILS (this pins pi's bare-slug dialect
   against omp's prefixed one).
2. Drop the `--provider openrouter` pair → `TestPiCommandPathArgsEnv` FAILS.
3. In `ShadowedCredential`, return the message unconditionally →
   `TestPiShadowedCredential` FAILS on the no-file and no-openrouter cases.
4. Remove the `~/.local/bin` fallback → `TestPiFindPathFallback` FAILS.

- [ ] **Step 7: Commit**

Run: `go test ./internal/agent/ -count=1 && go vet ./... && gofmt -l .` (must print nothing)

```bash
git add internal/agent/pi.go internal/agent/pi_test.go internal/agent/registry.go internal/agent/registry_test.go
git commit -m "feat(agent): pi launcher via built-in openrouter provider

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

(Include any cli/tui test files updated in Step 5.)

---

### Task 4: hermes launcher

Doc-verified on Hermes Agent v0.20.0: `hermes chat --provider openrouter
--model <slug>` are documented per-run overrides ("no mutation to
config.yaml"); `OPENROUTER_API_KEY` + documented `OPENROUTER_BASE_URL` pin;
hermes hard-rejects models with <64K context at startup.

**Files:**
- Create: `internal/agent/hermes.go`
- Test: `internal/agent/hermes_test.go`
- Modify: `internal/agent/registry.go` (insert entry before `chatgpt`)
- Modify: `internal/agent/registry_test.go` (add `"hermes"` to
  `TestRegistryTier2Agents`)

**Interfaces:**
- Consumes: Task 1 helpers, Task 2 capability, `ErrIncompatibleModel`,
  `openrouter.DefaultBaseURL`, test helpers as in Task 3.
- Produces: `type Hermes struct { LookPath func(string) (string, error) }`
  with `Name() "hermes"`, `DisplayName() "Hermes Agent"`, `Command`,
  `CheckModel`, `CheckInstalled`, `InstallHint`, `ShadowedCredential`.

- [ ] **Step 1: Write the failing tests**

`internal/agent/hermes_test.go`:

```go
package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

func TestHermesCommandPathArgsEnv(t *testing.T) {
	h := &Hermes{LookPath: stubLookPath("/usr/local/bin/hermes")}
	cmd, err := h.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--source", "tool"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	want := []string{"chat", "--provider", "openrouter", "--model", "anthropic/claude-opus-4.6", "--source", "tool"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
	// The base-URL pin is deliberate hardening; losing it silently is the
	// class of drift Landmine 18 exists for.
	if got, ok := envValue(cmd.Env, "OPENROUTER_BASE_URL"); !ok || got != "https://openrouter.ai/api/v1" {
		t.Errorf("OPENROUTER_BASE_URL = %q, %v", got, ok)
	}
}

func TestHermesCommandRequiresAPIKey(t *testing.T) {
	h := &Hermes{LookPath: stubLookPath("/usr/local/bin/hermes")}
	if _, err := h.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestHermesCommandRejectsConflictingExtras(t *testing.T) {
	h := &Hermes{LookPath: stubLookPath("/usr/local/bin/hermes")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"--model", "x/y"}, {"--model=x/y"},
		{"--provider", "nous"}, {"--provider=nous"},
		{"gateway"},          // our flags are chat-scoped; another subcommand misapplies them
		{"chat", "--voice"},  // duplicate subcommand
	} {
		if _, err := h.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
	if _, err := h.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: []string{"-q", "hello"}}); err != nil {
		t.Errorf("chat flags rejected: %v", err)
	}
}

func TestHermesCheckModelContextFloor(t *testing.T) {
	h := &Hermes{}
	small := openrouter.Model{ID: "small/model", ContextLength: 32768}
	err := h.CheckModel(small)
	if !errors.Is(err, ErrIncompatibleModel) {
		t.Fatalf("32K context: err = %v, want ErrIncompatibleModel (advisory)", err)
	}
	if !strings.Contains(err.Error(), "small/model") {
		t.Errorf("error %q does not name the model", err)
	}
	if err := h.CheckModel(openrouter.Model{ID: "big/model", ContextLength: 65536}); err != nil {
		t.Errorf("64K context rejected: %v", err)
	}
	// Unknown context (0) stays silent: missing catalog data is not evidence
	// of incompatibility.
	if err := h.CheckModel(openrouter.Model{ID: "unknown/model"}); err != nil {
		t.Errorf("unknown context rejected: %v", err)
	}
}

// Landmine 8: hermes is really installed at ~/.local/bin/hermes here.
func TestHermesFindPathFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }

	h := &Hermes{LookPath: notOnPath}
	if h.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hermes"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !h.CheckInstalled() {
		t.Error("CheckInstalled = false with binary at ~/.local/bin/hermes")
	}
}

func TestHermesShadowedCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := &Hermes{}

	if msg := h.ShadowedCredential(); msg != "" {
		t.Errorf("fresh HOME: msg = %q, want empty", msg)
	}

	dir := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// .env with an unrelated key: silent.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("BRAVE_API_KEY=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := h.ShadowedCredential(); msg != "" {
		t.Errorf(".env without our key: msg = %q, want empty", msg)
	}

	// .env with a stored OpenRouter key (export form included): warns.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("export OPENROUTER_API_KEY=sk-or-old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := h.ShadowedCredential(); !strings.Contains(msg, ".env") {
		t.Errorf("stored .env key: msg = %q, want it to name ~/.hermes/.env", msg)
	}

	// Remove .env; auth.json pool entry alone also warns.
	if err := os.Remove(filepath.Join(dir, ".env")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"openrouter":[{"key":"sk-or-old"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := h.ShadowedCredential(); !strings.Contains(msg, "auth.json") {
		t.Errorf("auth pool: msg = %q, want it to name auth.json", msg)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestHermes -v`
Expected: FAIL — `undefined: Hermes`.

- [ ] **Step 3: Implement `internal/agent/hermes.go`**

```go
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// hermesMinContext is the context floor hermes enforces at startup: models
// under 64K tokens are refused by hermes itself, so warn before launching.
const hermesMinContext = 65536

// Hermes launches Nous Research's Hermes Agent CLI against an OpenRouter
// model. OpenRouter is a first-class hermes provider; --provider/--model on
// the chat subcommand are vendor-documented per-run overrides with "no
// mutation to ~/.hermes/config.yaml", and CLI args outrank all config files.
// Doc-verified on v0.20.0 (2026-08-09); see
// .superpowers/sdd/2026-08-09-tier-2-research/hermes.md.
type Hermes struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (h *Hermes) Name() string        { return "hermes" }
func (h *Hermes) DisplayName() string { return "Hermes Agent" }

func (h *Hermes) lookPath(file string) (string, error) {
	if h.LookPath != nil {
		return h.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the hermes binary: PATH, then the installer's
// ~/.local/bin shim, then the native-Windows install location.
func (h *Hermes) findPath() (string, error) {
	if path, err := h.lookPath("hermes"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("hermes binary not found: %w", err)
	}
	candidates := []string{filepath.Join(home, ".local", "bin", "hermes")}
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates,
				filepath.Join(localAppData, "hermes", "hermes-agent", "venv", "Scripts", "hermes.exe"))
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("hermes binary not found")
}

// Command builds the hermes invocation. Pure: nothing written, nothing
// spawned. Managed flags ride the chat subcommand, so a passthrough that
// starts with a different subcommand is refused rather than silently
// misconfigured.
func (h *Hermes) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("hermes: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("hermes", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags("hermes", req.ExtraArgs, "--provider"); err != nil {
		return Command{}, err
	}
	if len(req.ExtraArgs) > 0 && !strings.HasPrefix(req.ExtraArgs[0], "-") {
		return Command{}, fmt.Errorf("hermes: passthrough %q looks like a hermes subcommand: openrouter-launch always runs \"hermes chat\"; pass chat flags only", req.ExtraArgs[0])
	}
	path, err := h.findPath()
	if err != nil {
		return Command{}, err
	}
	args := []string{"chat", "--provider", "openrouter", "--model", req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{
		"OPENROUTER_API_KEY=" + req.APIKey,
		// Documented hardening pin; hermes's default already matches.
		"OPENROUTER_BASE_URL=" + openrouter.DefaultBaseURL,
	}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckModel warns (advisory, Landmine 7) for models under hermes's 64K
// context floor. Unknown context stays silent: missing catalog data is not
// evidence of incompatibility.
func (h *Hermes) CheckModel(m openrouter.Model) error {
	if m.ContextLength > 0 && m.ContextLength < hermesMinContext {
		return fmt.Errorf("hermes refuses models with less than a 64K context window at startup (%s has %d tokens): %w",
			m.ID, m.ContextLength, ErrIncompatibleModel)
	}
	return nil
}

// CheckInstalled reports whether the hermes binary can be found.
func (h *Hermes) CheckInstalled() bool {
	_, err := h.findPath()
	return err == nil
}

// InstallHint tells the user how to install Hermes. Printed, never run.
func (h *Hermes) InstallHint() string {
	return "Install Hermes: curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash"
}

// ShadowedCredential reports stored hermes credentials that can outrank or
// rotate past the key this launch provides: an OPENROUTER_API_KEY line in
// ~/.hermes/.env, or an OpenRouter credential pool in ~/.hermes/auth.json.
func (h *Hermes) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if hermesEnvHasOpenRouterKey(filepath.Join(home, ".hermes", ".env")) {
		return "hermes has an OPENROUTER_API_KEY saved in ~/.hermes/.env that may override the key this launch provides"
	}
	if hermesAuthHasOpenRouter(filepath.Join(home, ".hermes", "auth.json")) {
		return "hermes has stored OpenRouter credentials (~/.hermes/auth.json) that may rotate past the key this launch provides"
	}
	return ""
}

func hermesEnvHasOpenRouterKey(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if v, ok := strings.CutPrefix(line, "OPENROUTER_API_KEY="); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func hermesAuthHasOpenRouter(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var store map[string]json.RawMessage
	if err := json.Unmarshal(data, &store); err != nil {
		return false
	}
	_, ok := store["openrouter"]
	return ok
}
```

- [ ] **Step 4: Register**

Registry entry (before `chatgpt`):

```go
	{
		Name:        "hermes",
		Launcher:    &Hermes{},
		Description: "Nous Research's terminal agent",
		Status:      Status{Supported: true},
	},
```

Add `"hermes"` to the `TestRegistryTier2Agents` slice.

- [ ] **Step 5: Run the full suite** — same expectations as Task 3 Step 5.

- [ ] **Step 6: Mutation checks**

1. Drop the `chat` subcommand from args → `TestHermesCommandPathArgsEnv` FAILS.
2. Drop `OPENROUTER_BASE_URL` from env → `TestHermesCommandPathArgsEnv` FAILS.
3. Change `hermesMinContext` to 0 → `TestHermesCheckModelContextFloor` FAILS.
4. Make `CheckModel` return a plain error (not wrapping
   `ErrIncompatibleModel`) → `TestHermesCheckModelContextFloor` FAILS on
   `errors.Is` (this pins advisory-not-fatal, Landmine 7).
5. Remove the leading-subcommand rejection →
   `TestHermesCommandRejectsConflictingExtras` FAILS on `{"gateway"}`.

- [ ] **Step 7: Commit**

```bash
git add internal/agent/hermes.go internal/agent/hermes_test.go internal/agent/registry.go internal/agent/registry_test.go
git commit -m "feat(agent): hermes launcher via chat --provider openrouter

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: qwen launcher

Doc-verified on Qwen Code 0.21.8. The spec's non-obvious requirement:
`--auth-type openai` on argv is MANDATORY — without it, persisted or default
auth (`qwen-oauth`) silently ignores every `OPENAI_*` env var (upstream
issue #891).

**Files:**
- Create: `internal/agent/qwen.go`
- Test: `internal/agent/qwen_test.go`
- Modify: `internal/agent/registry.go`, `internal/agent/registry_test.go`
  (add `"qwen"`)

**Interfaces:**
- Consumes: Task 1 helpers, `openrouter.DefaultBaseURL`, test helpers.
- Produces: `type Qwen struct { LookPath func(string) (string, error) }`
  with `Name() "qwen"`, `DisplayName() "Qwen Code"`, `Command`,
  `CheckInstalled`, `InstallHint`. (No shadow detector: qwen's stores do not
  outrank process env — its hazard is the flag, which we always pass.)

- [ ] **Step 1: Write the failing tests**

`internal/agent/qwen_test.go`:

```go
package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestQwenCommandPathArgsEnv(t *testing.T) {
	q := &Qwen{LookPath: stubLookPath("/usr/local/bin/qwen")}
	cmd, err := q.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--approval-mode", "plan"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// --auth-type openai is MANDATORY: without it qwen-code resolves auth
	// from persisted settings or its qwen-oauth default, both of which
	// silently ignore every OPENAI_* env var (upstream issue #891).
	want := []string{"--auth-type", "openai", "--model", "anthropic/claude-opus-4.6", "--approval-mode", "plan"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	for key, val := range map[string]string{
		"OPENAI_BASE_URL":    "https://openrouter.ai/api/v1",
		"OPENAI_API_KEY":     "sk-or-test",
		"OPENROUTER_API_KEY": "sk-or-test",
		"OPENAI_MODEL":       "anthropic/claude-opus-4.6",
	} {
		if got, ok := envValue(cmd.Env, key); !ok || got != val {
			t.Errorf("%s = %q, %v; want %q", key, got, ok, val)
		}
	}
}

func TestQwenCommandRequiresAPIKey(t *testing.T) {
	q := &Qwen{LookPath: stubLookPath("/usr/local/bin/qwen")}
	if _, err := q.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestQwenCommandRejectsConflictingExtras(t *testing.T) {
	q := &Qwen{LookPath: stubLookPath("/usr/local/bin/qwen")}
	for _, extras := range [][]string{
		{"-m", "x"}, {"--model", "x"}, {"--model=x"},
		{"--auth-type", "qwen-oauth"}, {"--auth-type=qwen-oauth"},
		{"--openai-api-key", "sk-mine"}, {"--openai-api-key=sk-mine"},
		{"--openai-base-url", "http://mine"}, {"--openai-base-url=http://mine"},
	} {
		if _, err := q.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
	if _, err := q.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: []string{"--yolo", "-p", "hi"}}); err != nil {
		t.Errorf("benign extras rejected: %v", err)
	}
}

func TestQwenFindPathFallbacks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	q := &Qwen{LookPath: notOnPath}

	if q.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}
	for _, rel := range []string{
		filepath.Join(".npm-global", "bin"),
		filepath.Join(".local", "bin"),
		filepath.Join(".nvm", "versions", "node", "v22.19.0", "bin"),
	} {
		dir := filepath.Join(home, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(dir, "qwen")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !q.CheckInstalled() {
			t.Errorf("CheckInstalled = false with binary at %s", bin)
		}
		if err := os.Remove(bin); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQwenInstallHint(t *testing.T) {
	q := &Qwen{}
	if hint := q.InstallHint(); !strings.Contains(hint, "@qwen-code/qwen-code") {
		t.Errorf("InstallHint = %q", hint)
	}
}
```

(Add `"strings"` to the imports for the hint test.)

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestQwen -v`
Expected: FAIL — `undefined: Qwen`.

- [ ] **Step 3: Implement `internal/agent/qwen.go`**

```go
package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Qwen launches Qwen Code against an OpenRouter model through its generic
// OpenAI-protocol auth: OPENAI_BASE_URL/OPENAI_API_KEY/OPENAI_MODEL env vars
// plus the MANDATORY --auth-type openai flag. Without the flag, qwen-code
// resolves auth from the user's persisted settings or its qwen-oauth
// default, both of which silently ignore every OPENAI_* env var (upstream
// issue #891) — the launch would look configured and run against the wrong
// backend. Doc-verified on 0.21.8 (2026-08-09); see
// .superpowers/sdd/2026-08-09-tier-2-research/qwen.md.
type Qwen struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (q *Qwen) Name() string        { return "qwen" }
func (q *Qwen) DisplayName() string { return "Qwen Code" }

func (q *Qwen) lookPath(file string) (string, error) {
	if q.LookPath != nil {
		return q.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the qwen binary: PATH, then npm-global and installer
// locations, then nvm's per-version bins (highest version wins).
func (q *Qwen) findPath() (string, error) {
	if path, err := q.lookPath("qwen"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("qwen binary not found: %w", err)
	}
	var candidates []string
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA")} {
			if base != "" {
				candidates = append(candidates,
					filepath.Join(base, "npm", "qwen.cmd"),
					filepath.Join(base, "npm", "qwen.exe"))
			}
		}
	} else {
		candidates = append(candidates,
			filepath.Join(home, ".npm-global", "bin", "qwen"),
			filepath.Join(home, ".local", "bin", "qwen"))
		if nvm, err := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "qwen")); err == nil {
			sort.Strings(nvm) // ascending: the last entry is the highest version
			for i := len(nvm) - 1; i >= 0; i-- {
				candidates = append(candidates, nvm[i])
			}
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("qwen binary not found")
}

// Command builds the qwen invocation. Pure: nothing written, nothing
// spawned. Both OPENAI_API_KEY and OPENROUTER_API_KEY carry the key: the
// generic openai auth path reads the former, qwen-code's dedicated
// OpenRouter recipe documents the latter.
func (q *Qwen) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("qwen: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("qwen", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags("qwen", req.ExtraArgs, "--auth-type", "--openai-api-key", "--openai-base-url"); err != nil {
		return Command{}, err
	}
	path, err := q.findPath()
	if err != nil {
		return Command{}, err
	}
	args := []string{"--auth-type", "openai", "--model", req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{
		"OPENAI_BASE_URL=" + openrouter.DefaultBaseURL,
		"OPENAI_API_KEY=" + req.APIKey,
		"OPENROUTER_API_KEY=" + req.APIKey,
		"OPENAI_MODEL=" + req.Model.ID,
	}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckInstalled reports whether the qwen binary can be found.
func (q *Qwen) CheckInstalled() bool {
	_, err := q.findPath()
	return err == nil
}

// InstallHint tells the user how to install Qwen Code. Printed, never run.
func (q *Qwen) InstallHint() string {
	return "Install Qwen Code: npm install -g @qwen-code/qwen-code@latest"
}
```

- [ ] **Step 4: Register**

Insert into the `specs` literal in `internal/agent/registry.go`, immediately
before the `chatgpt` entry:

```go
	{
		Name:        "qwen",
		Launcher:    &Qwen{},
		Description: "Qwen's terminal coding agent",
		Status:      Status{Supported: true},
	},
```

Add `"qwen"` to the `TestRegistryTier2Agents` slice in
`internal/agent/registry_test.go`.

- [ ] **Step 5: Run the full suite** — same expectations as Task 3 Step 5:
  fix row-count listing tests only; STOP on anything semantic.

- [ ] **Step 6: Mutation checks**

1. Drop `--auth-type openai` from args → `TestQwenCommandPathArgsEnv` FAILS
   (this is THE qwen landmine — issue #891).
2. Drop `OPENAI_MODEL` from env → `TestQwenCommandPathArgsEnv` FAILS.
3. Remove `--openai-base-url` from the rejected-flags list →
   `TestQwenCommandRejectsConflictingExtras` FAILS.
4. Remove the nvm glob → `TestQwenFindPathFallbacks` FAILS on the nvm case.

- [ ] **Step 7: Commit**

```bash
git add internal/agent/qwen.go internal/agent/qwen_test.go internal/agent/registry.go internal/agent/registry_test.go
git commit -m "feat(agent): qwen launcher via OPENAI_* env and mandatory --auth-type

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: cline launcher

Doc-verified on Cline CLI 3.0.51: native builtin `openrouter` provider
(`-P openrouter`), bare slugs via `-m`, `OPENROUTER_API_KEY` documented in
the apps/cli README. Saved provider state outranks env — detector required.

**Files:**
- Create: `internal/agent/cline.go`
- Test: `internal/agent/cline_test.go`
- Modify: `internal/agent/registry.go`, `internal/agent/registry_test.go`
  (add `"cline"`)

**Interfaces:**
- Consumes: Task 1 helpers, Task 2 capability, test helpers.
- Produces: `type Cline struct { LookPath func(string) (string, error) }`
  with `Name() "cline"`, `DisplayName() "Cline CLI"`, `Command`,
  `CheckInstalled`, `InstallHint`, `ShadowedCredential`.

- [ ] **Step 1: Write the failing tests**

`internal/agent/cline_test.go`:

```go
package agent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestClineCommandPathArgsEnv(t *testing.T) {
	c := &Cline{LookPath: stubLookPath("/usr/local/bin/cline")}
	cmd, err := c.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--auto-approve", "false"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	want := []string{"-P", "openrouter", "-m", "anthropic/claude-opus-4.6", "--auto-approve", "false"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
	// Owner decision: key travels in env only, never on argv (-k would put
	// it in /proc/<pid>/cmdline).
	for _, a := range cmd.Args {
		if strings.Contains(a, "sk-or-test") {
			t.Errorf("key leaked onto argv: %q", a)
		}
	}
}

func TestClineCommandRequiresAPIKey(t *testing.T) {
	c := &Cline{LookPath: stubLookPath("/usr/local/bin/cline")}
	if _, err := c.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestClineCommandRejectsConflictingExtras(t *testing.T) {
	c := &Cline{LookPath: stubLookPath("/usr/local/bin/cline")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"-mx/y"}, {"--model", "x/y"}, {"--model=x/y"},
		{"-P", "cline"}, {"-Pcline"}, {"--provider", "cline"}, {"--provider=cline"},
	} {
		if _, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
	// -k is the user's explicit per-run key override — allowed, their call.
	if _, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: []string{"-k", "sk-or-theirs"}}); err != nil {
		t.Errorf("-k rejected: %v", err)
	}
}

func TestClineShadowedCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	c := &Cline{}

	if msg := c.ShadowedCredential(); msg != "" {
		t.Errorf("fresh HOME: msg = %q, want empty", msg)
	}

	dir := filepath.Join(home, ".cline", "data", "settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "providers.json")

	// openrouter entry without a key: silent.
	if err := os.WriteFile(path, []byte(`{"providers":{"openrouter":{"settings":{"model":"x"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := c.ShadowedCredential(); msg != "" {
		t.Errorf("keyless entry: msg = %q, want empty", msg)
	}

	// apiKey at the entry level: warns.
	if err := os.WriteFile(path, []byte(`{"providers":{"openrouter":{"apiKey":"sk-or-old"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := c.ShadowedCredential(); !strings.Contains(msg, "providers.json") {
		t.Errorf("entry-level key: msg = %q, want it to name providers.json", msg)
	}

	// apiKey nested under settings: warns.
	if err := os.WriteFile(path, []byte(`{"providers":{"openrouter":{"settings":{"apiKey":"sk-or-old"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := c.ShadowedCredential(); msg == "" {
		t.Error("settings-level key: msg empty, want warning")
	}

	// Malformed file: silent.
	if err := os.WriteFile(path, []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := c.ShadowedCredential(); msg != "" {
		t.Errorf("malformed file: msg = %q, want empty", msg)
	}
}

func TestClineInstallHint(t *testing.T) {
	c := &Cline{}
	if hint := c.InstallHint(); !strings.Contains(hint, "npm install -g cline") {
		t.Errorf("InstallHint = %q", hint)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestCline -v`
Expected: FAIL — `undefined: Cline`.

- [ ] **Step 3: Implement `internal/agent/cline.go`**

```go
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Cline launches the Cline CLI against an OpenRouter model via its native
// builtin openrouter provider (base URL baked in upstream). Nothing is
// written — ollama's integration writes providers.json + globalState.json,
// which the CLI's own -P/-m flags make unnecessary. Note cline's
// --auto-approve defaults to TRUE upstream; per the owner decision recorded
// in the Phase 4 spec, the launcher does not override agent behavior
// defaults. Doc-verified on 3.0.51 (2026-08-09); see
// .superpowers/sdd/2026-08-09-tier-2-research/cline.md.
type Cline struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (c *Cline) Name() string        { return "cline" }
func (c *Cline) DisplayName() string { return "Cline CLI" }

func (c *Cline) lookPath(file string) (string, error) {
	if c.LookPath != nil {
		return c.LookPath(file)
	}
	return exec.LookPath(file)
}

// Command builds the cline invocation. Pure: nothing written, nothing
// spawned. The key travels in env only; passthrough -k stays allowed as the
// user's explicit choice, but the launcher itself never puts a key on argv.
func (c *Cline) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("cline: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("cline", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags("cline", req.ExtraArgs, "-P", "--provider"); err != nil {
		return Command{}, err
	}
	path, err := c.lookPath("cline")
	if err != nil {
		return Command{}, fmt.Errorf("cline binary not found: %w", err)
	}
	args := []string{"-P", "openrouter", "-m", req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{"OPENROUTER_API_KEY=" + req.APIKey}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckInstalled reports whether the cline binary can be found. npm global
// installs land on PATH; there is no home-dir fallback.
func (c *Cline) CheckInstalled() bool {
	_, err := c.lookPath("cline")
	return err == nil
}

// InstallHint tells the user how to install the Cline CLI.
func (c *Cline) InstallHint() string {
	return "Install Cline CLI: npm install -g cline"
}

// ShadowedCredential reports cline's documented precedence trap: a saved
// OpenRouter key in ~/.cline/data/settings/providers.json outranks the
// OPENROUTER_API_KEY env var (source: resolveApiKey — saved key → OAuth →
// env), so the session would bill the stored account.
func (c *Cline) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".cline", "data", "settings", "providers.json"))
	if err != nil {
		return ""
	}
	var doc struct {
		Providers map[string]struct {
			APIKey   string `json:"apiKey"`
			Settings struct {
				APIKey string `json:"apiKey"`
			} `json:"settings"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return ""
	}
	entry, ok := doc.Providers["openrouter"]
	if !ok {
		return ""
	}
	if entry.APIKey == "" && entry.Settings.APIKey == "" {
		return ""
	}
	return "cline has a saved OpenRouter key (~/.cline/data/settings/providers.json) that outranks the key this launch provides"
}
```

- [ ] **Step 4: Register**

Insert into the `specs` literal in `internal/agent/registry.go`, immediately
before the `chatgpt` entry:

```go
	{
		Name:        "cline",
		Launcher:    &Cline{},
		Description: "Cline's terminal coding agent",
		Status:      Status{Supported: true},
	},
```

Add `"cline"` to the `TestRegistryTier2Agents` slice in
`internal/agent/registry_test.go`.

- [ ] **Step 5: Run the full suite** — same expectations as Task 3 Step 5:
  fix row-count listing tests only; STOP on anything semantic.

- [ ] **Step 6: Mutation checks**

1. Drop `-P openrouter` → `TestClineCommandPathArgsEnv` FAILS (without it
   cline defaults to its own hosted account — the wrong-account class).
2. Put the key on argv as `-k` instead of env →
   `TestClineCommandPathArgsEnv` FAILS on the argv-leak scan.
3. In `ShadowedCredential`, warn whenever the openrouter entry exists
   (ignore the key fields) → `TestClineShadowedCredential` FAILS on the
   keyless-entry case.

- [ ] **Step 7: Commit**

```bash
git add internal/agent/cline.go internal/agent/cline_test.go internal/agent/registry.go internal/agent/registry_test.go
git commit -m "feat(agent): cline launcher via -P openrouter

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: kimi launcher

The Landmine 18 analog, pre-empted: ollama's `kimi --config '<json>'`
mechanism targets the DEPRECATED legacy Python kimi-cli. The current Kimi
Code CLI (0.34.0) uses the `KIMI_MODEL_*` env family — synthesized in
memory, documented "nothing written back", outranked only by a `-m` flag we
never pass and reject in passthrough.

**Files:**
- Create: `internal/agent/kimi.go`
- Test: `internal/agent/kimi_test.go`
- Modify: `internal/agent/registry.go`, `internal/agent/registry_test.go`
  (add `"kimi"`)

**Interfaces:**
- Consumes: Task 1 helpers, Task 2 capability, `openrouter.DefaultBaseURL`,
  test helpers.
- Produces: `type Kimi struct { LookPath func(string) (string, error) }`
  with `Name() "kimi"`, `DisplayName() "Kimi Code CLI"`, `Command`,
  `CheckInstalled`, `InstallHint`, `ShadowedCredential` (legacy-binary
  heuristic).

- [ ] **Step 1: Write the failing tests**

`internal/agent/kimi_test.go`:

```go
package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

func kimiModel() openrouter.Model {
	return openrouter.Model{ID: "moonshotai/kimi-k2.5", ContextLength: 262144}
}

func TestKimiCommandPathArgsEnv(t *testing.T) {
	k := &Kimi{LookPath: stubLookPath("/usr/local/bin/kimi")}
	cmd, err := k.Command(Request{
		Model:     kimiModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--verbose"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// The whole configuration is the KIMI_MODEL_* env family; argv carries
	// only passthrough. No --config: that flag belongs to the deprecated
	// legacy Python kimi-cli and does not exist on Kimi Code CLI.
	if want := []string{"--verbose"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	for key, val := range map[string]string{
		"KIMI_MODEL_NAME":             "moonshotai/kimi-k2.5",
		"KIMI_MODEL_API_KEY":          "sk-or-test",
		"KIMI_MODEL_PROVIDER_TYPE":    "openai",
		"KIMI_MODEL_BASE_URL":         "https://openrouter.ai/api/v1",
		"KIMI_MODEL_MAX_CONTEXT_SIZE": "262144",
	} {
		if got, ok := envValue(cmd.Env, key); !ok || got != val {
			t.Errorf("%s = %q, %v; want %q", key, got, ok, val)
		}
	}
}

func TestKimiCommandOmitsContextSizeWhenUnknown(t *testing.T) {
	k := &Kimi{LookPath: stubLookPath("/usr/local/bin/kimi")}
	cmd, err := k.Command(Request{
		Model:  openrouter.Model{ID: "moonshotai/kimi-k2.5"}, // ContextLength 0
		APIKey: "sk-or-test",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// Better kimi's documented default (262144) than our fabricated zero:
	// KIMI_MODEL_MAX_CONTEXT_SIZE=0 would break the session.
	if _, ok := envValue(cmd.Env, "KIMI_MODEL_MAX_CONTEXT_SIZE"); ok {
		t.Error("KIMI_MODEL_MAX_CONTEXT_SIZE set for unknown context, want omitted")
	}
}

func TestKimiCommandRequiresAPIKey(t *testing.T) {
	k := &Kimi{LookPath: stubLookPath("/usr/local/bin/kimi")}
	if _, err := k.Command(Request{Model: kimiModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestKimiCommandRejectsConflictingExtras(t *testing.T) {
	k := &Kimi{LookPath: stubLookPath("/usr/local/bin/kimi")}
	for _, extras := range [][]string{
		{"-m", "other"}, {"-mother"}, {"--model", "other"}, {"--model=other"},
		{"--config", "{}"}, {"--config={}"},
		{"--config-file", "x.toml"}, {"--config-file=x.toml"},
	} {
		if _, err := k.Command(Request{Model: kimiModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
}

// Landmine 8 discipline for every location probe below.
func TestKimiFindPathPrefersKimiCodeOverLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	k := &Kimi{LookPath: notOnPath}

	if k.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}

	legacy := filepath.Join(home, ".local", "share", "uv", "tools", "kimi-cli", "bin")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "kimi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !k.CheckInstalled() {
		t.Fatal("CheckInstalled = false with legacy binary present")
	}

	kimiCode := filepath.Join(home, ".kimi-code", "bin")
	if err := os.MkdirAll(kimiCode, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kimiCode, "kimi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd, err := k.Command(Request{Model: kimiModel(), APIKey: "sk"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != filepath.Join(kimiCode, "kimi") {
		t.Errorf("Path = %q: Kimi Code's own install dir must beat the legacy uv path", cmd.Path)
	}
}

func TestKimiShadowedCredentialFlagsLegacyOnlyInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	k := &Kimi{LookPath: notOnPath}

	if msg := k.ShadowedCredential(); msg != "" {
		t.Errorf("no binary anywhere: msg = %q, want empty", msg)
	}

	legacy := filepath.Join(home, ".local", "share", "uv", "tools", "kimi-cli", "bin")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "kimi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if msg := k.ShadowedCredential(); !strings.Contains(msg, "legacy") {
		t.Errorf("legacy-only install: msg = %q, want a legacy kimi-cli warning", msg)
	}

	kimiCode := filepath.Join(home, ".kimi-code", "bin")
	if err := os.MkdirAll(kimiCode, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kimiCode, "kimi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if msg := k.ShadowedCredential(); msg != "" {
		t.Errorf("Kimi Code present: msg = %q, want empty", msg)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestKimi -v`
Expected: FAIL — `undefined: Kimi`.

- [ ] **Step 3: Implement `internal/agent/kimi.go`**

```go
package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Kimi launches Moonshot AI's Kimi Code CLI against an OpenRouter model via
// the KIMI_MODEL_* env family, which synthesizes a provider+model in memory
// ("nothing is written back to the config file" — vendor docs) and outranks
// config.toml; only a -m flag beats it, which we never pass and reject in
// passthrough.
//
// Deliberately NOT ported from ollama: its `kimi --config '<json>'` with
// provider type "openai_legacy" targets the deprecated legacy Python
// kimi-cli. Kimi Code CLI has neither the flag nor the type — porting it
// would repeat Landmine 18 (see the Phase 4 spec and
// .superpowers/sdd/2026-08-09-tier-2-research/kimi.md). Doc-verified on
// kimi-code 0.34.0, KIMI_MODEL_* channel present since 0.6.0 (2026-08-09).
type Kimi struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (k *Kimi) Name() string        { return "kimi" }
func (k *Kimi) DisplayName() string { return "Kimi Code CLI" }

func (k *Kimi) lookPath(file string) (string, error) {
	if k.LookPath != nil {
		return k.LookPath(file)
	}
	return exec.LookPath(file)
}

func kimiBinary() string {
	if runtime.GOOS == "windows" {
		return "kimi.exe"
	}
	return "kimi"
}

// kimiCodePath is the current CLI's own install location; legacyKimiPaths
// are where the deprecated Python kimi-cli lands. Both generations install
// a binary named "kimi", so the order here IS the disambiguation: Kimi
// Code's dir always wins over uv tool paths.
func kimiCodePath(home string) string {
	return filepath.Join(home, ".kimi-code", "bin", kimiBinary())
}

func legacyKimiPaths(home string) []string {
	return []string{
		filepath.Join(home, ".local", "share", "uv", "tools", "kimi-cli", "bin", "kimi"),
		filepath.Join(home, ".local", "bin", "kimi"),
	}
}

func (k *Kimi) findPath() (string, error) {
	if path, err := k.lookPath("kimi"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("kimi binary not found: %w", err)
	}
	candidates := append([]string{kimiCodePath(home)}, legacyKimiPaths(home)...)
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("kimi binary not found")
}

// Command builds the kimi invocation. Pure: nothing written, nothing
// spawned. KIMI_MODEL_MAX_CONTEXT_SIZE comes from the catalog; when the
// catalog does not know the context length, the variable is omitted so
// kimi's documented default applies instead of a fabricated zero.
func (k *Kimi) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("kimi: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("kimi", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags("kimi", req.ExtraArgs, "--config", "--config-file"); err != nil {
		return Command{}, err
	}
	path, err := k.findPath()
	if err != nil {
		return Command{}, err
	}
	env := []string{
		"KIMI_MODEL_NAME=" + req.Model.ID,
		"KIMI_MODEL_API_KEY=" + req.APIKey,
		"KIMI_MODEL_PROVIDER_TYPE=openai",
		"KIMI_MODEL_BASE_URL=" + openrouter.DefaultBaseURL,
	}
	if req.Model.ContextLength > 0 {
		env = append(env, fmt.Sprintf("KIMI_MODEL_MAX_CONTEXT_SIZE=%d", req.Model.ContextLength))
	}
	return Command{Path: path, Args: append([]string(nil), req.ExtraArgs...), Env: env}, nil
}

// CheckInstalled reports whether a kimi binary can be found.
func (k *Kimi) CheckInstalled() bool {
	_, err := k.findPath()
	return err == nil
}

// InstallHint tells the user how to install Kimi Code CLI. Printed, never
// run. Windows additionally needs Git for Windows (kimi uses Git Bash as
// its shell backend).
func (k *Kimi) InstallHint() string {
	return "Install Kimi Code CLI: curl -fsSL https://code.kimi.com/kimi-code/install.sh | bash"
}

// ShadowedCredential flags a legacy-only install: the deprecated Python
// kimi-cli ignores KIMI_MODEL_* entirely, so a launch would silently run on
// the user's Moonshot account instead of OpenRouter. Pure path heuristic —
// executing the binary to ask its version would violate launch purity. A
// PATH hit is trusted (the Kimi Code installer renames legacy shims to
// kimi-legacy); only a uv-tools-dir resolution with no Kimi Code install
// alongside is confidently legacy.
func (k *Kimi) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if _, err := os.Stat(kimiCodePath(home)); err == nil {
		return ""
	}
	path, err := k.findPath()
	if err != nil {
		return ""
	}
	if path == legacyKimiPaths(home)[0] {
		return "the kimi binary at " + path + " looks like the legacy Python kimi-cli, which ignores KIMI_MODEL_* configuration and would run on your Moonshot account; install Kimi Code CLI (https://code.kimi.com)"
	}
	return ""
}
```

- [ ] **Step 4: Register**

Insert into the `specs` literal in `internal/agent/registry.go`, immediately
before the `chatgpt` entry:

```go
	{
		Name:        "kimi",
		Launcher:    &Kimi{},
		Description: "Moonshot AI's Kimi Code CLI",
		Status:      Status{Supported: true},
	},
```

Add `"kimi"` to the `TestRegistryTier2Agents` slice in
`internal/agent/registry_test.go`.

- [ ] **Step 5: Run the full suite** — same expectations as Task 3 Step 5:
  fix row-count listing tests only; STOP on anything semantic.

- [ ] **Step 6: Mutation checks**

1. Add `"--config"` + inline JSON to args (the ollama port) →
   `TestKimiCommandPathArgsEnv` FAILS on the args assertion. THE mutation of
   this task: it proves the test refuses the legacy mechanism.
2. Drop `KIMI_MODEL_PROVIDER_TYPE` → `TestKimiCommandPathArgsEnv` FAILS
   (default is `kimi`, which routes to Moonshot's own API, not OpenRouter).
3. Emit `KIMI_MODEL_MAX_CONTEXT_SIZE=0` for unknown context →
   `TestKimiCommandOmitsContextSizeWhenUnknown` FAILS.
4. Reorder `findPath` candidates legacy-first →
   `TestKimiFindPathPrefersKimiCodeOverLegacy` FAILS.

- [ ] **Step 7: Commit**

```bash
git add internal/agent/kimi.go internal/agent/kimi_test.go internal/agent/registry.go internal/agent/registry_test.go
git commit -m "feat(agent): kimi launcher via KIMI_MODEL_* env family

Targets Kimi Code CLI, not the deprecated legacy kimi-cli whose --config
mechanism ollama still ports (the Landmine 18 analog, pre-empted).

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: omp launcher

Doc-verified on Oh My Pi 17.2.11: built-in openrouter provider,
`OPENROUTER_API_KEY`, and THE slug transform of this phase — `--model
openrouter/<slug>`. omp's credential store is sqlite (`agent.db`), so no
runtime detector: the shadowing caveat is documented, not warned.

**Files:**
- Create: `internal/agent/omp.go`
- Test: `internal/agent/omp_test.go`
- Modify: `internal/agent/registry.go`, `internal/agent/registry_test.go`
  (add `"omp"`)

**Interfaces:**
- Consumes: Task 1 helpers, test helpers.
- Produces: `type OMP struct { LookPath func(string) (string, error) }`
  with `Name() "omp"`, `DisplayName() "Oh My Pi"`, `Command`,
  `CheckInstalled`, `InstallHint`.

- [ ] **Step 1: Write the failing tests**

`internal/agent/omp_test.go`:

```go
package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOMPCommandPathArgsEnv(t *testing.T) {
	o := &OMP{LookPath: stubLookPath("/usr/local/bin/omp")}
	cmd, err := o.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"-p", "hi"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// The openrouter/ prefix IS the provider selection in omp's dialect. A
	// bare slug is a valid-looking wrong value (pi's dialect, not omp's).
	want := []string{"--model", "openrouter/anthropic/claude-opus-4.6", "-p", "hi"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
}

func TestOMPCommandRequiresAPIKey(t *testing.T) {
	o := &OMP{LookPath: stubLookPath("/usr/local/bin/omp")}
	if _, err := o.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestOMPCommandRejectsConflictingExtras(t *testing.T) {
	o := &OMP{LookPath: stubLookPath("/usr/local/bin/omp")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"-mx/y"}, {"--model", "x/y"}, {"--model=x/y"},
		{"--provider", "openai"}, {"--provider=openai"},
	} {
		if _, err := o.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
	// --api-key is the user's explicit override of omp's stored-credential
	// precedence — allowed, their call (documented in the spec's key policy).
	if _, err := o.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: []string{"--api-key", "sk-or-theirs"}}); err != nil {
		t.Errorf("--api-key rejected: %v", err)
	}
}

func TestOMPFindPathFallbacks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	o := &OMP{LookPath: notOnPath}

	if o.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}
	for _, rel := range []string{filepath.Join(".local", "bin"), filepath.Join(".bun", "bin")} {
		dir := filepath.Join(home, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(dir, "omp")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !o.CheckInstalled() {
			t.Errorf("CheckInstalled = false with binary at %s", bin)
		}
		if err := os.Remove(bin); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOMPInstallHint(t *testing.T) {
	o := &OMP{}
	if hint := o.InstallHint(); !strings.Contains(hint, "https://omp.sh/install") {
		t.Errorf("InstallHint = %q", hint)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestOMP -v`
Expected: FAIL — `undefined: OMP`.

- [ ] **Step 3: Implement `internal/agent/omp.go`**

```go
package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// OMP launches Oh My Pi against an OpenRouter model. OpenRouter is a
// built-in omp provider with the base URL baked in upstream; the model
// selector is "openrouter/<slug>" — the prefix IS the provider selection in
// omp's dialect (unlike its ancestor pi, which takes --provider plus a bare
// slug). Nothing is written; ollama's models.yml write existed only because
// Ollama is a custom provider there.
//
// Known, documented, NOT runtime-detected: omp's stored credentials
// (~/.omp/agent/agent.db, sqlite) outrank the env key — "env vars are a
// fallback, not an override". No sqlite dependency for one advisory; the
// caveat lives in the spec and README. Doc-verified on 17.2.11
// (2026-08-09); see .superpowers/sdd/2026-08-09-tier-2-research/omp.md.
type OMP struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (o *OMP) Name() string        { return "omp" }
func (o *OMP) DisplayName() string { return "Oh My Pi" }

func (o *OMP) lookPath(file string) (string, error) {
	if o.LookPath != nil {
		return o.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the omp binary: PATH, then the install script's
// ~/.local/bin, then bun's global bin.
func (o *OMP) findPath() (string, error) {
	if path, err := o.lookPath("omp"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("omp binary not found: %w", err)
	}
	for _, c := range []string{
		filepath.Join(home, ".local", "bin", "omp"),
		filepath.Join(home, ".bun", "bin", "omp"),
	} {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("omp binary not found")
}

// Command builds the omp invocation. Pure: nothing written, nothing
// spawned. Passthrough --api-key stays allowed: it is the user's explicit,
// documented override of omp's stored-credential precedence.
func (o *OMP) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("omp: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("omp", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags("omp", req.ExtraArgs, "--provider"); err != nil {
		return Command{}, err
	}
	path, err := o.findPath()
	if err != nil {
		return Command{}, err
	}
	args := []string{"--model", "openrouter/" + req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{"OPENROUTER_API_KEY=" + req.APIKey}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckInstalled reports whether the omp binary can be found.
func (o *OMP) CheckInstalled() bool {
	_, err := o.findPath()
	return err == nil
}

// InstallHint tells the user how to install Oh My Pi. Printed, never run.
func (o *OMP) InstallHint() string {
	return "Install Oh My Pi: curl -fsSL https://omp.sh/install | sh"
}
```

- [ ] **Step 4: Register**

Insert into the `specs` literal in `internal/agent/registry.go`, immediately
before the `chatgpt` entry:

```go
	{
		Name:        "omp",
		Launcher:    &OMP{},
		Description: "Oh My Pi terminal coding agent",
		Status:      Status{Supported: true},
	},
```

Add `"omp"` to the `TestRegistryTier2Agents` slice in
`internal/agent/registry_test.go`.

- [ ] **Step 5: Run the full suite** — same expectations as Task 3 Step 5:
  fix row-count listing tests only; STOP on anything semantic.

- [ ] **Step 6: Mutation checks**

1. Drop the `openrouter/` prefix (bare slug) → `TestOMPCommandPathArgsEnv`
   FAILS. THE mutation of this task — it distinguishes omp's dialect from
   pi's; run it together with Task 3's mutation 1 to confirm the two tests
   pin opposite behaviors.
2. Remove the `~/.bun/bin` fallback → `TestOMPFindPathFallbacks` FAILS.

- [ ] **Step 7: Commit**

```bash
git add internal/agent/omp.go internal/agent/omp_test.go internal/agent/registry.go internal/agent/registry_test.go
git commit -m "feat(agent): omp launcher via openrouter/-prefixed model selector

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: Live verification gates

Owner-approved cents-level spend on `openai/gpt-4o-mini`. `pi` and `hermes`
are installed on this machine — run their gates first. The other four need
installs: **ask the owner before installing anything** (`npm install -g
cline`, `npm install -g @qwen-code/qwen-code@latest`, the kimi and omp curl
installers). A gate that cannot run (owner declines the install) moves that
agent's items to HANDOFF open items — the launcher still ships, marked
doc-verified-only.

Log every run to `.superpowers/sdd/2026-08-09-phase-4a/live-<agent>.log`
(create the directory; grep each log for the key afterwards — it must never
appear). If any gate falsifies a doc-verified value, fix launcher + test in
this task (the Landmine 18 protocol: update the literal, watch the named
test fail first, commit as `fix(agent): <agent> <value> live-verified as …`)
and record it in the spec's per-agent section.

**Files:**
- Modify (only on falsified values): the affected launcher + test
- Modify: `docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md`
  (append a "Live verification results (2026-08-XX)" subsection, Phase 3
  spec style)

- [ ] **Step 1: Resolve the key** (never print it)

```bash
KEY=$(jq -r '.api_key // empty' "${XDG_CONFIG_HOME:-$HOME/.config}/openrouter-launch/config.json")
[ -n "$KEY" ] || KEY="$OPENROUTER_API_KEY"
[ -n "$KEY" ] || echo "NO KEY — stop and ask the user"
LOG="$PWD/.superpowers/sdd/2026-08-09-phase-4a"   # absolute: later steps cd into temp dirs
mkdir -p "$LOG"
go build -o /tmp/orl4 .
```

- [ ] **Step 2: pi gate** (installed)

```bash
cd "$(mktemp -d)" && OPENROUTER_API_KEY=$KEY PI_OFFLINE=1 \
  pi --provider openrouter --model openai/gpt-4o-mini -p "Reply with exactly the word OK" \
  2>&1 | tee "$LOG/live-pi-raw.log"
cd "$(mktemp -d)" && /tmp/orl4 pi -m openai/gpt-4o-mini -- -p "Reply with exactly the word OK" \
  2>&1 | tee "$LOG/live-pi-orl.log"
```

Must answer: bare slug resolves exactly (no fuzzy mismatch); a `:free` slug
(`pi --provider openrouter --model <some>:free -p …`) is not mis-parsed as a
thinking suffix; error shape when the model is absent from pi's curated
catalog (try a real OpenRouter slug pi lacks; note the message for the spec).
Zero-touch audit: snapshot `ls -laR ~/.pi/agent` before/after — pi's own
session/state writes are fine; nothing from us.

- [ ] **Step 3: hermes gate** (installed)

```bash
cd "$(mktemp -d)" && OPENROUTER_API_KEY=$KEY \
  hermes chat --provider openrouter --model openai/gpt-4o-mini -q "Reply with exactly the word OK" \
  2>&1 | tee "$LOG/live-hermes-raw.log"
cd "$(mktemp -d)" && /tmp/orl4 hermes -m openai/gpt-4o-mini -- -q "Reply with exactly the word OK" \
  2>&1 | tee "$LOG/live-hermes-orl.log"
```

Must answer: flags work under `chat` as documented; `~/.hermes/config.yaml`
mtime/content unchanged (the "no mutation" claim); if a `~/.hermes/.env`
holds an old key, which wins (set a bogus `OPENROUTER_API_KEY` in `.env` on
a COPY of the home via `HOME=<tempcopy>`, never edit the real one); the <64K
context rejection's actual message (try a small-context model).

- [ ] **Step 4: qwen, cline, kimi, omp gates** (each needs an install — ask first)

Same shape per agent; the specific must-answer items, from the spec:

- qwen: env+flag launch works with a virgin `HOME`; the `modelProviders`
  collision (craft `~/.qwen/settings.json` in a temp HOME with a
  `modelProviders.openai[]` entry whose id equals the slug; confirm our
  `--auth-type openai --model` + env still wins); headless form is `-p "…"`.
- cline: `cline -P openrouter -m openai/gpt-4o-mini "Reply with exactly the
  word OK"` with env key only, virgin `~/.cline` — no `cline auth` first;
  does a first-run wizard interpose?
- kimi: `KIMI_MODEL_*` one-shot on a virgin HOME; `kimi --help | grep -c
  config` — expected: the new CLI has no `--config`; version output format
  for legacy-vs-new disambiguation.
- omp: `OPENROUTER_API_KEY=$KEY omp --model openrouter/openai/gpt-4o-mini -p
  "Reply with exactly the word OK"`; selector round-trips unmangled;
  first-run onboarding behavior.

All four also get the through-our-binary form (`/tmp/orl4 <agent> -m
openai/gpt-4o-mini -- …`) and a before/after config-tree audit.

- [ ] **Step 5: Record results**

Append the dated results subsection to the Phase 4 spec (what ran, versions,
what was falsified/confirmed, log paths), Phase 3 style. Commit:

```bash
git add docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md
git commit -m "docs: record Phase 4a live verification results

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 10: Full verification suite and handoff update

**Files:**
- Modify: `HANDOFF.md`

- [ ] **Step 1: Run the complete verification suite**

```bash
go test ./... -count=1
go test ./internal/tui/ -race -count=1
go vet ./... && gofmt -l .
GOOS=windows go build ./... && GOOS=darwin go build ./...
HOME=$(mktemp -d) PATH="/usr/local/go/bin:/usr/bin:/bin" go test ./... -count=1
```

All green, `gofmt -l` empty. The `HOME` line now also proves the suite
ignores this machine's real hermes and pi installs.

- [ ] **Step 2: Zero-touch grep** (Landmine 6 — still exactly two write sites in 4a)

```bash
grep -rn "os.WriteFile\|os.Create\|os.MkdirAll\|os.Rename\|OpenFile" --include="*.go" . | grep -v _test
```

Expected: hits only in `internal/openrouter` (cache) and `internal/config`
(config). The new `ShadowedCredential` detectors use `os.ReadFile` only —
if this grep shows a write call in any launcher, that is a Critical defect.

- [ ] **Step 3: Update `HANDOFF.md`**

- Current-state table: agents shipped → `claude, codex, opencode, pi,
  hermes, qwen, cline, kimi, omp; 3 desktop apps unsupported`; add a Phase 4a
  row; refresh the test count.
- Working commands: add one line per new agent (the `-m <slug> -- …` forms
  that Task 9 smoke-tested; mark doc-verified-only any agent whose gate was
  skipped).
- Landmine 8's text: add hermes and pi to the really-installed list.
- Add a Landmine for the kimi legacy-CLI trap (ollama's `--config` mechanism
  targets the deprecated CLI; `KIMI_MODEL_*` is the verified channel), and
  one for the omp/openclaw `openrouter/` slug-prefix dialect vs everyone
  else's bare slugs. Do not renumber existing landmines.
- Open items: add the stored-credential advisory's coarse edges (omp's
  sqlite store is documented-only), any skipped live gates, and "six new
  agents' interactive TUIs not yet driven by a human".
- "Phase 3+ — more agents": rewrite to state Tier 2 zero-touch is done;
  droid + openclaw follow in Plan 4b.

- [ ] **Step 4: Commit and push**

```bash
git add HANDOFF.md
git commit -m "docs: Phase 4a complete — six zero-touch Tier 2 launchers

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
git push && git status -sb
```

Expected: `## main...origin/main` with no ahead/behind marker.

---

## Execution notes

- One fresh subagent per task; two-stage review per
  superpowers:subagent-driven-development; whole-branch review at the end of
  4b (one review covers both plans).
- Reviewer prompts must name the known failure pattern: **tests that pass
  for the wrong reason** (substring assertions satisfiable by unrelated
  output, guards whose deletion still errors differently, fixtures that
  cannot distinguish a property from its negation). Special attention this
  phase: the pi/omp slug-dialect pair — each test must FAIL under the other
  agent's dialect (Task 3 mutation 1 and Task 8 mutation 1 are the proof).
- Task 9 spends real money (cents) and wants four installs; both were
  discussed at spec review, but ASK before each install anyway.
- Plan 4b (`2026-08-09-phase-4b-configwriter-openclaw-droid.md`) depends on
  Tasks 1–2 of this plan and should be executed after 4a completes.
