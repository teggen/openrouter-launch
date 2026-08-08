# Phase 3 — Codex + OpenCode Launchers, Tier 3 Registry: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `codex` and `opencode` as zero-touch launchers and register three
desktop apps as unsupported-with-reason.

**Architecture:** Two new per-agent files in `internal/agent`, each a small
struct with a pure `Command()` following the `Claude` pattern, plus one shared
`stub` launcher for unsupported registry entries. No planner, TUI, or CLI
changes — those layers are already agent-generic.

**Tech Stack:** Go 1.24, stdlib only (no new dependencies).

**Spec:** `docs/superpowers/specs/2026-08-08-phase-3-agents-design.md` — read it
first. The "Landmines" section of `HANDOFF.md` is binding.

## Global Constraints

- `Command()` MUST be pure: no file writes, no network, no process spawn, no
  `exec.LookPath` except through the injectable seam (`LookPath` field).
- Zero-touch (Landmine 6): the tree has exactly two write sites (cache +
  config). This phase adds none. Never write into `~/.codex`, opencode state,
  or any agent-owned path.
- Codex/opencode use `openrouter.DefaultBaseURL` (`https://openrouter.ai/api/v1`,
  **with** `/v1`). Only Claude Code uses `agent.AnthropicBaseURL` (no `/v1`).
  Landmine 1.
- Tests that need a binary to look ABSENT must `t.Setenv("HOME", t.TempDir())`
  when a home-dir fallback path is involved (Landmine 8). `codex` is really
  installed at `~/.local/bin/codex` on this machine.
- `Spec.Launcher` must never be nil (Landmine 10) — unsupported entries get the
  stub.
- Every non-obvious behavior gets a mutation check: break it, watch the named
  test FAIL, revert. A test never seen failing is not evidence.
- Commit directly to `main` (owner's choice). `gofmt -l .` empty and
  `go vet ./...` clean before every commit.
- Commit messages end with:
  `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`

---

### Task 1: Codex launcher

**Files:**
- Create: `internal/agent/codex.go`
- Test: `internal/agent/codex_test.go`

**Interfaces:**
- Consumes (already exist in package `agent`): `Request{Model openrouter.Model;
  APIKey string; ExtraArgs []string}`, `Command{Path string; Args []string;
  Env []string}`, `Launcher`, `Installable`; test helpers in `claude_test.go`
  (same package): `stubLookPath(path string) func(string) (string, error)`,
  `testModel() openrouter.Model`, `envValue(env []string, key string) (string, bool)`.
- Produces: `type Codex struct { LookPath func(string) (string, error) }` with
  methods `Name() string` → `"codex"`, `DisplayName() string` → `"Codex CLI"`,
  `Command(Request) (Command, error)`, `CheckInstalled() bool`,
  `InstallHint() string`. Task 3 registers `&Codex{}`.

- [ ] **Step 1: Write the failing tests**

`internal/agent/codex_test.go`:

```go
package agent

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCodexCommandPathArgsEnv(t *testing.T) {
	c := &Codex{LookPath: stubLookPath("/usr/local/bin/codex")}
	cmd, err := c.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"resume"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != "/usr/local/bin/codex" {
		t.Errorf("Path = %q", cmd.Path)
	}
	// Managed overrides first, then -m, then passthrough. Order matters:
	// codex options are global, so managed flags must precede a passthrough
	// subcommand like "resume".
	want := []string{
		"-c", `model_provider="openrouter"`,
		"-c", `model_providers.openrouter.name="OpenRouter"`,
		"-c", `model_providers.openrouter.base_url="https://openrouter.ai/api/v1"`,
		"-c", `model_providers.openrouter.env_key="OPENROUTER_API_KEY"`,
		"-c", `model_providers.openrouter.wire_api="chat"`,
		"-m", "anthropic/claude-opus-4.6",
		"resume",
	}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args =\n%q\nwant\n%q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
}

func TestCodexCommandRequiresAPIKey(t *testing.T) {
	c := &Codex{LookPath: stubLookPath("/usr/local/bin/codex")}
	if _, err := c.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestCodexCommandRejectsConflictingExtras(t *testing.T) {
	c := &Codex{LookPath: stubLookPath("/usr/local/bin/codex")}
	for _, extras := range [][]string{
		{"-m", "gpt-5"},
		{"-mgpt-5"},
		{"--model", "gpt-5"},
		{"--model=gpt-5"},
		{"-c", "model=gpt-5"},
		{"-c", `model_provider="mine"`},
		{"-c", "model_providers.mine.base_url=http://x"},
		{`-cmodel_provider="mine"`},
		{"--config", "model_provider=mine"},
		{"--config=model_providers.mine.name=X"},
	} {
		_, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras})
		if err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
			continue
		}
		named := strings.Contains(err.Error(), extras[0]) ||
			(len(extras) > 1 && strings.Contains(err.Error(), extras[1]))
		if !named {
			t.Errorf("extras %q: error %q does not name the argument", extras, err)
		}
	}
}

func TestCodexCommandAllowsBenignExtras(t *testing.T) {
	c := &Codex{LookPath: stubLookPath("/usr/local/bin/codex")}
	extras := []string{"exec", "--full-auto", "-c", "foo=bar", "--profile", "mine", "--config", "sandbox_mode=read-only"}
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras})
	if err != nil {
		t.Fatalf("benign extras rejected: %v", err)
	}
	if !slices.Equal(cmd.Args[len(cmd.Args)-len(extras):], extras) {
		t.Errorf("extras not appended verbatim: %q", cmd.Args)
	}
}

func TestCodexCommandBinaryMissing(t *testing.T) {
	c := &Codex{LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	if _, err := c.Command(Request{Model: testModel(), APIKey: "k"}); err == nil {
		t.Fatal("Command with missing binary succeeded, want error")
	}
}

func TestCodexInstallable(t *testing.T) {
	installed := &Codex{LookPath: stubLookPath("/usr/local/bin/codex")}
	if !installed.CheckInstalled() {
		t.Error("CheckInstalled = false with binary present")
	}
	missing := &Codex{LookPath: func(string) (string, error) { return "", errors.New("no") }}
	if missing.CheckInstalled() {
		t.Error("CheckInstalled = true with binary absent")
	}
	if hint := missing.InstallHint(); !strings.Contains(hint, "npm install -g @openai/codex") {
		t.Errorf("InstallHint = %q", hint)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestCodex -v`
Expected: FAIL — `undefined: Codex`.

- [ ] **Step 3: Implement `internal/agent/codex.go`**

```go
package agent

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Codex launches the OpenAI Codex CLI against an OpenRouter model. All
// configuration travels as -c overrides on the command line; nothing is
// written into ~/.codex.
type Codex struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (c *Codex) Name() string        { return "codex" }
func (c *Codex) DisplayName() string { return "Codex CLI" }

func (c *Codex) lookPath(file string) (string, error) {
	if c.LookPath != nil {
		return c.LookPath(file)
	}
	return exec.LookPath(file)
}

// Command builds the codex invocation. It is pure: nothing is written and no
// process is started. Managed overrides come before passthrough args so they
// apply even when the passthrough starts with a subcommand; conflicting
// passthrough is rejected because a later -c with the same key would win and
// silently point codex somewhere else.
func (c *Codex) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("codex: an OpenRouter API key is required")
	}
	if err := codexValidateExtraArgs(req.ExtraArgs); err != nil {
		return Command{}, err
	}
	path, err := c.lookPath("codex")
	if err != nil {
		return Command{}, fmt.Errorf("codex binary not found: %w", err)
	}

	args := []string{
		"-c", `model_provider="openrouter"`,
		"-c", `model_providers.openrouter.name="OpenRouter"`,
		"-c", `model_providers.openrouter.base_url="` + openrouter.DefaultBaseURL + `"`,
		"-c", `model_providers.openrouter.env_key="OPENROUTER_API_KEY"`,
		"-c", `model_providers.openrouter.wire_api="chat"`,
		"-m", req.Model.ID,
	}
	args = append(args, req.ExtraArgs...)

	// env_key makes codex read the key from this variable; setting it here
	// (rather than relying on the user's shell) means ExecArgs' dedupe
	// guarantees our value wins over any stray export.
	env := []string{"OPENROUTER_API_KEY=" + req.APIKey}

	return Command{Path: path, Args: args, Env: env}, nil
}

// codexValidateExtraArgs rejects passthrough that would defeat the managed
// provider config. Later -c overrides win in codex, so silently accepting
// these would let a user flag beat ours while the tool reports success.
func codexValidateExtraArgs(args []string) error {
	for i, arg := range args {
		switch {
		case arg == "-m" || arg == "--model" ||
			strings.HasPrefix(arg, "--model=") ||
			(strings.HasPrefix(arg, "-m") && len(arg) > len("-m")):
			return fmt.Errorf("codex: conflicting argument %q: openrouter-launch manages the model; pick it with openrouter-launch codex -m", arg)
		case arg == "-c" || arg == "--config":
			if i+1 < len(args) && codexOverrideConflicts(args[i+1]) {
				return fmt.Errorf("codex: conflicting override %q: openrouter-launch manages the model provider", args[i+1])
			}
		case strings.HasPrefix(arg, "-c") && len(arg) > len("-c"):
			if codexOverrideConflicts(strings.TrimPrefix(arg, "-c")) {
				return fmt.Errorf("codex: conflicting override %q: openrouter-launch manages the model provider", arg)
			}
		case strings.HasPrefix(arg, "--config="):
			if codexOverrideConflicts(strings.TrimPrefix(arg, "--config=")) {
				return fmt.Errorf("codex: conflicting override %q: openrouter-launch manages the model provider", arg)
			}
		}
	}
	return nil
}

func codexOverrideConflicts(override string) bool {
	key, _, ok := strings.Cut(strings.TrimSpace(override), "=")
	if !ok {
		return false
	}
	key = strings.Trim(strings.TrimSpace(key), `"'`)
	return key == "model" || key == "model_provider" ||
		strings.HasPrefix(key, "model_providers.")
}

// CheckInstalled reports whether the codex binary can be found. npm global
// installs land on PATH, so there is no home-dir fallback.
func (c *Codex) CheckInstalled() bool {
	_, err := c.lookPath("codex")
	return err == nil
}

// InstallHint tells the user how to install Codex.
func (c *Codex) InstallHint() string {
	return "Install Codex: npm install -g @openai/codex"
}
```

- [ ] **Step 4: Run tests, verify they pass**

Run: `go test ./internal/agent/ -run TestCodex -v`
Expected: PASS, all six.

- [ ] **Step 5: Mutation checks** (each: break, run the named test, see FAIL, revert)

1. Delete the `wire_api` override line → `TestCodexCommandPathArgsEnv` FAILS.
2. Move `args = append(args, req.ExtraArgs...)` before the managed overrides
   (extras first) → `TestCodexCommandPathArgsEnv` FAILS (order).
3. In `codexOverrideConflicts`, drop the `model_providers.` prefix case →
   `TestCodexCommandRejectsConflictingExtras` FAILS.
4. Return `nil` error from `codexValidateExtraArgs` unconditionally →
   `TestCodexCommandRejectsConflictingExtras` FAILS.

- [ ] **Step 6: Full package check and commit**

Run: `go test ./internal/agent/ -count=1 && go vet ./internal/agent/ && gofmt -l internal/agent/` (must print nothing)

```bash
git add internal/agent/codex.go internal/agent/codex_test.go
git commit -m "feat(agent): codex launcher via managed -c overrides

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: OpenCode launcher

**Files:**
- Create: `internal/agent/opencode.go`
- Test: `internal/agent/opencode_test.go`

**Interfaces:**
- Consumes: same package pieces and test helpers as Task 1.
- Produces: `type OpenCode struct { LookPath func(string) (string, error) }`
  with methods `Name() string` → `"opencode"`, `DisplayName() string` →
  `"OpenCode"`, `Command(Request) (Command, error)`, `CheckInstalled() bool`,
  `InstallHint() string`. Task 3 registers `&OpenCode{}`.

- [ ] **Step 1: Write the failing tests**

`internal/agent/opencode_test.go`:

```go
package agent

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestOpenCodeCommandPathArgsEnv(t *testing.T) {
	o := &OpenCode{LookPath: stubLookPath("/usr/local/bin/opencode")}
	cmd, err := o.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"run", "hello"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != "/usr/local/bin/opencode" {
		t.Errorf("Path = %q", cmd.Path)
	}
	if want := []string{"run", "hello"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	// The model reference is provider/model where the model id itself
	// contains a slash; opencode splits on the FIRST slash.
	wantCfg := `{"$schema":"https://opencode.ai/config.json","model":"openrouter/anthropic/claude-opus-4.6"}`
	if got, ok := envValue(cmd.Env, "OPENCODE_CONFIG_CONTENT"); !ok || got != wantCfg {
		t.Errorf("OPENCODE_CONFIG_CONTENT = %q, want %q", got, wantCfg)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
}

func TestOpenCodeCommandRequiresAPIKey(t *testing.T) {
	o := &OpenCode{LookPath: stubLookPath("/usr/local/bin/opencode")}
	if _, err := o.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestOpenCodeCommandRejectsModelExtras(t *testing.T) {
	o := &OpenCode{LookPath: stubLookPath("/usr/local/bin/opencode")}
	for _, extras := range [][]string{
		{"-m", "x/y"},
		{"--model", "x/y"},
		{"--model=x/y"},
		{"-mx/y"},
	} {
		if _, err := o.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
}

// Landmine 8: the fallback path is under HOME, and a real opencode install
// must not make the "absent" cases pass or fail by accident.
func TestOpenCodeFindPathFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }

	missing := &OpenCode{LookPath: notOnPath}
	if _, err := missing.Command(Request{Model: testModel(), APIKey: "k"}); err == nil {
		t.Fatal("Command found a binary in an empty HOME, want error")
	}
	if missing.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}
}

func TestOpenCodeInstallable(t *testing.T) {
	installed := &OpenCode{LookPath: stubLookPath("/usr/local/bin/opencode")}
	if !installed.CheckInstalled() {
		t.Error("CheckInstalled = false with binary present")
	}
	o := &OpenCode{}
	if hint := o.InstallHint(); !strings.Contains(hint, "https://opencode.ai/install") {
		t.Errorf("InstallHint = %q", hint)
	}
}

// The curl installer drops the binary at ~/.opencode/bin without reliably
// editing PATH; findPath must look there. Landmine 8 discipline: build the
// fixture inside a temp HOME so the machine's real install state is invisible.
func TestOpenCodeFindPathUsesInstallerLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".opencode", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	o := &OpenCode{LookPath: func(string) (string, error) { return "", errors.New("not on PATH") }}
	if !o.CheckInstalled() {
		t.Fatal("CheckInstalled = false with binary at ~/.opencode/bin")
	}
}
```

(Imports for this file: `errors`, `os`, `path/filepath`, `slices`,
`strings`, `testing`.)

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestOpenCode -v`
Expected: FAIL — `undefined: OpenCode`.

- [ ] **Step 3: Implement `internal/agent/opencode.go`**

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
)

// OpenCode launches opencode against an OpenRouter model. The entire config
// travels inline in OPENCODE_CONFIG_CONTENT; opencode's native openrouter
// provider reads OPENROUTER_API_KEY. Nothing is written to disk — in
// particular not opencode's model-state file, which ollama's integration
// edits and we deliberately do not.
type OpenCode struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (o *OpenCode) Name() string        { return "opencode" }
func (o *OpenCode) DisplayName() string { return "OpenCode" }

func (o *OpenCode) lookPath(file string) (string, error) {
	if o.LookPath != nil {
		return o.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the opencode binary, falling back to the curl
// installer's location, which is not reliably added to PATH.
func (o *OpenCode) findPath() (string, error) {
	if p, err := o.lookPath("opencode"); err == nil {
		return p, nil
	}
	name := "opencode"
	if runtime.GOOS == "windows" {
		name = "opencode.exe"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("opencode binary not found: %w", err)
	}
	candidate := filepath.Join(home, ".opencode", "bin", name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("opencode binary not found")
}

// opencodeConfig is the inline JSON for OPENCODE_CONFIG_CONTENT. The model
// reference is "openrouter/<slug>"; opencode splits provider from model on
// the first slash, so the slug's own slash survives.
type opencodeConfig struct {
	Schema string `json:"$schema"`
	Model  string `json:"model"`
}

// Command builds the opencode invocation. It is pure: nothing is written and
// no process is started.
func (o *OpenCode) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("opencode: an OpenRouter API key is required")
	}
	if err := opencodeValidateExtraArgs(req.ExtraArgs); err != nil {
		return Command{}, err
	}
	path, err := o.findPath()
	if err != nil {
		return Command{}, err
	}

	cfg, err := json.Marshal(opencodeConfig{
		Schema: "https://opencode.ai/config.json",
		Model:  "openrouter/" + req.Model.ID,
	})
	if err != nil {
		return Command{}, fmt.Errorf("opencode: building inline config: %w", err)
	}

	env := []string{
		"OPENCODE_CONFIG_CONTENT=" + string(cfg),
		"OPENROUTER_API_KEY=" + req.APIKey,
	}
	return Command{Path: path, Args: append([]string(nil), req.ExtraArgs...), Env: env}, nil
}

// opencodeValidateExtraArgs rejects a passthrough model flag: the CLI flag
// outranks the inline config, so it would silently beat the selected model.
func opencodeValidateExtraArgs(args []string) error {
	for _, arg := range args {
		if arg == "-m" || arg == "--model" ||
			strings.HasPrefix(arg, "--model=") ||
			(strings.HasPrefix(arg, "-m") && len(arg) > len("-m")) {
			return fmt.Errorf("opencode: conflicting argument %q: openrouter-launch manages the model; pick it with openrouter-launch opencode -m", arg)
		}
	}
	return nil
}

// CheckInstalled reports whether the opencode binary can be found.
func (o *OpenCode) CheckInstalled() bool {
	_, err := o.findPath()
	return err == nil
}

// InstallHint tells the user how to install OpenCode. Printed, never run.
func (o *OpenCode) InstallHint() string {
	return "Install OpenCode: curl -fsSL https://opencode.ai/install | bash (or: npm install -g opencode-ai)"
}
```

- [ ] **Step 4: Run tests, verify they pass**

Run: `go test ./internal/agent/ -run TestOpenCode -v`
Expected: PASS, all six.

- [ ] **Step 5: Mutation checks**

1. Change the config's model to `"openrouter"` alone (drop the slug) →
   `TestOpenCodeCommandPathArgsEnv` FAILS.
2. Drop `OPENROUTER_API_KEY` from env → `TestOpenCodeCommandPathArgsEnv` FAILS.
3. Remove the `~/.opencode/bin` fallback from `findPath` →
   `TestOpenCodeFindPathUsesInstallerLocation` FAILS. (Note
   `TestOpenCodeFindPathFallback` alone cannot catch this — it pins the
   absent case, which passes with or without the fallback. That is why the
   installer-location test exists.)
4. Make `opencodeValidateExtraArgs` return nil always →
   `TestOpenCodeCommandRejectsModelExtras` FAILS.

- [ ] **Step 6: Full package check and commit**

Run: `go test ./internal/agent/ -count=1 && go vet ./internal/agent/ && gofmt -l internal/agent/` (must print nothing)

```bash
git add internal/agent/opencode.go internal/agent/opencode_test.go
git commit -m "feat(agent): opencode launcher via OPENCODE_CONFIG_CONTENT

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Stub launcher and registry entries

**Files:**
- Create: `internal/agent/stub.go`
- Modify: `internal/agent/registry.go:32-40` (the `specs` literal only)
- Test: `internal/agent/registry_test.go` (extend), `internal/launch/conditions_test.go` (extend)

**Interfaces:**
- Consumes: `Codex` (Task 1), `OpenCode` (Task 2), `Spec`, `Status`,
  `Launcher`, `launch.CheckSupported(*agent.Spec) error`,
  `*launch.UnsupportedAgentError`.
- Produces: registry entries `codex`, `opencode`, `chatgpt`,
  `claude-desktop`, `hermes-desktop`; unexported `stub` type. Nothing outside
  the registry consumes the stub directly.

- [ ] **Step 1: Write the failing tests**

Append to `internal/agent/registry_test.go`:

```go
func TestRegistryPhase3Agents(t *testing.T) {
	for _, name := range []string{"codex", "opencode"} {
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

func TestRegistryUnsupportedDesktopApps(t *testing.T) {
	for _, name := range []string{"chatgpt", "claude-desktop", "hermes-desktop"} {
		spec, err := Lookup(name)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", name, err)
		}
		if spec.Status.Supported {
			t.Errorf("%q registered as supported", name)
		}
		if spec.Status.Reason == "" {
			t.Errorf("%q has no reason", name)
		}
		if spec.Launcher == nil {
			t.Errorf("%q has nil Launcher", name) // Landmine 10
		}
		if spec.Launcher.Name() != name {
			t.Errorf("%q launcher Name() = %q", name, spec.Launcher.Name())
		}
	}
}

func TestStubCommandErrors(t *testing.T) {
	s := &stub{name: "chatgpt", display: "ChatGPT"}
	if _, err := s.Command(Request{Model: testModel(), APIKey: "k"}); err == nil {
		t.Fatal("stub.Command succeeded; it must always error")
	}
}
```

Append to `internal/launch/conditions_test.go` (this pins that the stub's
error is unreachable through the launch path — the guard fires first, for
every real unsupported registry entry, present and future):

```go
func TestCheckSupportedCoversEveryUnsupportedRegistryEntry(t *testing.T) {
	sawUnsupported := false
	for _, spec := range agent.List() {
		if spec.Status.Supported {
			if err := CheckSupported(spec); err != nil {
				t.Errorf("%q: CheckSupported = %v, want nil", spec.Name, err)
			}
			continue
		}
		sawUnsupported = true
		err := CheckSupported(spec)
		var uae *UnsupportedAgentError
		if !errors.As(err, &uae) {
			t.Errorf("%q: CheckSupported returned %T (%v), want *UnsupportedAgentError", spec.Name, err, err)
			continue
		}
		if uae.Reason != spec.Status.Reason {
			t.Errorf("%q: Reason = %q, want %q", spec.Name, uae.Reason, spec.Status.Reason)
		}
	}
	if !sawUnsupported {
		t.Fatal("registry contains no unsupported agents; this test no longer tests anything")
	}
}
```

(Check the file's imports include `errors` and
`github.com/teggen/openrouter-launch/internal/agent`; add if missing.)

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./internal/agent/ -run 'TestRegistryPhase3Agents|TestRegistryUnsupportedDesktopApps|TestStubCommandErrors' -v && go test ./internal/launch/ -run TestCheckSupportedCoversEveryUnsupportedRegistryEntry -v`
Expected: agent tests FAIL (`undefined: stub`, `Lookup("codex")` unknown
agent); launch test FAILS on `sawUnsupported`.

- [ ] **Step 3: Implement the stub and registry entries**

`internal/agent/stub.go`:

```go
package agent

import "fmt"

// stub satisfies Launcher for agents that cannot be pointed at OpenRouter.
// Spec.Launcher must never be nil — buildIndex panics on construction, which
// would take down the binary and every CLI test (see registry.go). The
// planner's CheckSupported guard fires on Status before any Command call, so
// this error is unreachable through the launch path; the launch package pins
// that with TestCheckSupportedCoversEveryUnsupportedRegistryEntry.
type stub struct {
	name    string
	display string
}

func (s *stub) Name() string        { return s.name }
func (s *stub) DisplayName() string { return s.display }

func (s *stub) Command(Request) (Command, error) {
	return Command{}, fmt.Errorf("%s cannot be pointed at OpenRouter", s.display)
}
```

In `internal/agent/registry.go`, extend the `specs` literal (claude stays
first; supported agents before unsupported, in this order):

```go
var specs = []*Spec{
	{
		Name:        "claude",
		Aliases:     []string{"claude-code", "cc"},
		Launcher:    &Claude{},
		Description: "Anthropic's coding tool with subagents",
		Status:      Status{Supported: true},
	},
	{
		Name:        "codex",
		Launcher:    &Codex{},
		Description: "OpenAI's Codex CLI",
		Status:      Status{Supported: true},
	},
	{
		Name:        "opencode",
		Launcher:    &OpenCode{},
		Description: "Open-source terminal coding agent",
		Status:      Status{Supported: true},
	},
	{
		Name:        "chatgpt",
		Launcher:    &stub{name: "chatgpt", display: "ChatGPT / Codex app"},
		Description: "OpenAI's desktop app",
		Status:      Status{Supported: false, Reason: "desktop app authenticates through its own account; a launcher cannot inject a provider"},
	},
	{
		Name:        "claude-desktop",
		Launcher:    &stub{name: "claude-desktop", display: "Claude Desktop"},
		Description: "Anthropic's desktop app",
		Status:      Status{Supported: false, Reason: "desktop app authenticates through its own account; a launcher cannot inject a provider"},
	},
	{
		Name:        "hermes-desktop",
		Launcher:    &stub{name: "hermes-desktop", display: "Hermes Desktop"},
		Description: "Nous Research's desktop app",
		Status:      Status{Supported: false, Reason: "desktop app authenticates through its own account; a launcher cannot inject a provider"},
	},
}
```

- [ ] **Step 4: Run the full suite, verify everything passes**

Run: `go test ./... -count=1`
Expected: PASS everywhere. Failures in `internal/cli` or `internal/tui` mean
an existing test hardcoded the one-agent registry — fix the TEST expectation
(e.g. an `agents` golden listing) to include the new rows, never the
registry. If a cli/tui test failure looks semantic rather than
one-more-row, STOP and report it.

The spec's TUI requirement (unsupported rows render dimmed with reasons) is
already covered by fixture tests — `internal/tui/root_test.go` builds
`unsupportedSpec(...)` rows and pins both rendering and cursor skip. Confirm
those tests still pass with the real registry grown; no new TUI test is
needed.

- [ ] **Step 5: Mutation checks**

1. Flip `chatgpt` to `Supported: true` →
   `TestRegistryUnsupportedDesktopApps` FAILS.
2. Empty the `chatgpt` Reason →
   `TestRegistryUnsupportedDesktopApps` FAILS (and the launch test's Reason
   equality FAILS).
3. Remove all three unsupported entries →
   `TestCheckSupportedCoversEveryUnsupportedRegistryEntry` FAILS on its
   `sawUnsupported` guard.

- [ ] **Step 6: Smoke the listing and unsupported refusal, then commit**

```bash
go build -o /tmp/orl-task3 .
/tmp/orl-task3 agents                       # six rows; three "unsupported: …"
/tmp/orl-task3 chatgpt -m openai/gpt-5; echo "exit=$?"   # refusal naming the reason, exit=1
```

Expected: `chatgpt` exits 1 with the reason (Landmine 15 pinned this
exit-code contract for fatal plan errors).

```bash
git add internal/agent/stub.go internal/agent/registry.go internal/agent/registry_test.go internal/launch/conditions_test.go
git commit -m "feat(agent): register codex, opencode, and unsupported desktop apps

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

(Include any cli/tui test files updated in Step 4 in the same commit.)

---

### Task 4: Live verification against OpenRouter

**Files:**
- Modify: `docs/superpowers/specs/2026-08-08-phase-3-agents-design.md`
  (record what was actually verified — only if a value had to change)

Owner-approved credit spend: a few cheap one-shot runs. Use
`openai/gpt-4o-mini`. This task writes no production code unless
verification falsifies a config value; each such change goes back through
the Task 1/2 tests (update the expected literal, run, commit).

- [ ] **Step 1: Resolve the API key**

```bash
KEY=$(jq -r '.api_key // empty' "${XDG_CONFIG_HOME:-$HOME/.config}/openrouter-launch/config.json")
[ -n "$KEY" ] || KEY="$OPENROUTER_API_KEY"
[ -n "$KEY" ] || echo "NO KEY — stop and ask the user"
```

Never print the key itself.

- [ ] **Step 2: Verify the codex mechanism raw** (managed values, subcommand form)

```bash
cd "$(mktemp -d)" && OPENROUTER_API_KEY=$KEY codex exec --skip-git-repo-check \
  -c 'model_provider="openrouter"' \
  -c 'model_providers.openrouter.name="OpenRouter"' \
  -c 'model_providers.openrouter.base_url="https://openrouter.ai/api/v1"' \
  -c 'model_providers.openrouter.env_key="OPENROUTER_API_KEY"' \
  -c 'model_providers.openrouter.wire_api="chat"' \
  -m openai/gpt-4o-mini \
  "Reply with exactly the word OK and nothing else"
```

Expected: a reply through OpenRouter (the key is only valid there, so any
successful completion proves base_url + env_key + wire_api). If codex
rejects `wire_api="chat"` or errors on the wire protocol: retry with
`wire_api="responses"`; whichever works becomes the value in `codex.go`
(update the Task 1 test literal + implementation, rerun
`go test ./internal/agent/`, note it in the spec, commit as
`fix(agent): codex wire_api verified as <value>`).

- [ ] **Step 3: Verify codex through our binary** (arg placement end to end)

```bash
go build -o /tmp/orl3 . 
cd "$(mktemp -d)" && /tmp/orl3 codex -m openai/gpt-4o-mini -- exec --skip-git-repo-check "Reply with exactly the word OK and nothing else"
```

Expected: same successful reply — proves managed global flags before the
`exec` subcommand parse correctly. If codex rejects root-level `-m` before a
subcommand, record the finding: interactive launch (no subcommand) is the
primary path and unaffected; note the passthrough-subcommand limitation in
the spec's codex section and in `HANDOFF.md` open items. Do not reorder
managed args after passthrough — that reopens the conflict-ordering hole.

- [ ] **Step 4: Verify the opencode mechanism raw** (first-slash split + env auth)

```bash
cd "$(mktemp -d)" && \
OPENCODE_CONFIG_CONTENT='{"$schema":"https://opencode.ai/config.json","model":"openrouter/openai/gpt-4o-mini"}' \
OPENROUTER_API_KEY=$KEY \
opencode run "Reply with exactly the word OK and nothing else"
```

Expected: a reply. If auth fails with env alone, switch the inline config to
the explicit provider block (still zero-touch) and encode it in
`opencode.go` + the Task 2 test literal:

```json
{"$schema":"https://opencode.ai/config.json","provider":{"openrouter":{"options":{"apiKey":"{env:OPENROUTER_API_KEY}"}}},"model":"openrouter/openai/gpt-4o-mini"}
```

Commit any such change as `fix(agent): opencode inline config verified`,
and record the verified variant in the spec.

- [ ] **Step 5: Verify opencode through our binary**

```bash
cd "$(mktemp -d)" && /tmp/orl3 opencode -m openai/gpt-4o-mini -- run "Reply with exactly the word OK and nothing else"
```

Expected: same successful reply.

- [ ] **Step 6: Zero-touch audit of the live runs**

```bash
ls -la ~/.codex/ | head; stat -c '%y %n' ~/.codex/config.toml
ls ~/.local/state/opencode/ 2>/dev/null; ls ~/.opencode 2>/dev/null
```

Expected: no file under `~/.codex` or opencode's state modified by our runs
(codex itself may write its own session/history files — that is the agent
acting on its own behalf, not us writing agent config; what must NOT change
is `config.toml` content). Compare `config.toml` mtime/content against a
pre-run copy taken in Step 2 if in doubt.

- [ ] **Step 7: Record and commit**

If any value changed in Steps 2–5 it was already committed there. Update the
spec's Verification section with a dated note of what was verified
(codex version, opencode version, wire_api value, config variant), then:

```bash
git add docs/superpowers/specs/2026-08-08-phase-3-agents-design.md
git commit -m "docs: record live-verified codex/opencode values

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Full verification suite and handoff update

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

All green, `gofmt -l` empty. The `HOME` line is Landmine 8's machine-
independence check — codex/opencode are really installed here, and the new
tests must pass with both invisible.

Also confirm the `ExecArgs` env-dedupe pin (Landmine 3) is still green —
`go test ./internal/agent/ -run TestExecArgs -v`. This phase must not touch
`ExecArgs`; both new launchers rely on its dedupe to make their
`OPENROUTER_API_KEY` beat a stray export, so its existing mutation check
stands in for the spec's "remove the env dedupe interaction" mutation.

- [ ] **Step 2: Zero-touch grep** (Landmine 6 — verify, don't assert)

```bash
grep -rn "os.WriteFile\|os.Create\|os.MkdirAll\|os.Rename\|OpenFile" --include="*.go" . | grep -v _test | grep -v "/scratch"
```

Expected: hits only in `internal/openrouter` (cache) and `internal/config`
(config) — none in `internal/agent`, `internal/launch`, `internal/tui`,
`internal/cli`, `main.go`.

- [ ] **Step 3: Update `HANDOFF.md`**

- "Current state" table: Agents shipped row → `claude, codex, opencode; 3
  desktop apps registered unsupported`; add a Phase 3 row: `Complete: codex +
  opencode launchers, Tier 3 registry, live-verified against OpenRouter`;
  refresh the test count (from `go test ./... -count=1` output).
- "Working commands": add `openrouter-launch codex -m <slug> -- …` and
  `openrouter-launch opencode -m <slug> -- …` as smoke-tested.
- "Phase 3+ — more agents" section: rewrite — Tier 1 is done; next is Tier 2
  (the eight agents, mechanism verification first); note the owner decision
  that `copilot`, `pool`, `vscode` are unregistered.
- "Open items": replace the first item's claude-only wording; add "codex and
  opencode interactive TUIs not yet driven by a human — headless one-shot
  runs verified live" and keep the standing items.
- Add a Landmine if Task 4 discovered one (e.g. a verified-value surprise or
  the root-`-m`-before-subcommand finding). Do not renumber existing ones.

- [ ] **Step 4: Commit and push**

```bash
git add HANDOFF.md
git commit -m "docs: Phase 3 complete — codex, opencode, Tier 3 registry

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
git push && git status -sb
```

Expected: `## main...origin/main` with no ahead/behind marker.

---

## Execution notes

- One fresh subagent per task; two-stage review per
  superpowers:subagent-driven-development; whole-branch review at the end.
- Reviewer prompts must name the known failure pattern: **tests that pass
  for the wrong reason** (substring assertions satisfiable by unrelated
  output, guards whose deletion still errors differently, fixtures that
  cannot distinguish the property from its negation). Nine of ten Phase 1
  Important findings and eight in the TUI phase were this class.
- Task 4 needs the live API key and spends cents; it was owner-approved
  during brainstorming. If the key is missing, stop and ask.
- After Task 5, the user does the interactive smoke test
  (`openrouter-launch codex`, `openrouter-launch opencode`, root screen,
  picker) — that is outside subagent scope and stays an open item until done.
