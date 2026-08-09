# Phase 4b — Staged Files, openclaw, ConfigWriter, droid: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `openclaw` (via a new `Staged` launcher-owned-file capability)
and `droid` (the first `ConfigWriter`, on a new fork-and-wait launch path),
amending the write-site invariant (Landmine 6) as each site lands.

**Architecture:** Two new capabilities, both computed purely and materialized
in the one side-effect function (`launch.Service.Launch`, Landmine 5's home):
`Staged` declares launcher-owned files as data; `ConfigWriter` (interface
already exists, unimplemented) gets its promised fork-and-wait path so
`restore` can run after the agent exits. Two new launchers consume them.

**Tech Stack:** Go 1.24, stdlib only (no new dependencies).

**Spec:** `docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md`;
research in `.superpowers/sdd/2026-08-09-tier-2-research/{openclaw,droid}.md`.
The "Landmines" section of `HANDOFF.md` is binding — this plan AMENDS
Landmine 6 (owner-approved at spec review); each new write site and its grep
update land in the same commit.

**Prerequisite:** Plan 4a Tasks 1–2 are merged (`rejectModelFlag`,
`rejectFlags`, `CredentialShadowCheck`, `WarnShadowedCredential`). Both
launchers here use them.

## Global Constraints

- `Command()` and `StagedFiles()` MUST be pure: no writes, no network, no
  spawning. All writes happen in `launch.Service.Launch` (staging) or via
  `ConfigWriter.Apply`/`restore` on the fork-and-wait path — nowhere else.
- Staged files must never contain secrets (mode 0644, model ref only); the
  API key stays in env. droid's written entry carries the literal string
  `${OPENROUTER_API_KEY}`, never the key itself — a test greps for the key.
- Landmine 5 ordering extends to: recordSelection → stage → Apply → run →
  restore, all in one function.
- ConfigWriter agents NEVER take `syscall.Exec` (restore must run);
  everything else still does. openclaw is NOT a ConfigWriter — its file is
  launcher-owned, needs no undo, and keeps the exec handoff.
- Slug transform for openclaw: `openrouter/` prefix, lowercased.
- Every non-obvious behavior gets a mutation check.
- Commit directly to `main`; `gofmt -l .` empty, `go vet ./...` clean before
  every commit; messages end with:
  `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`

---

### Task 1: `Staged` capability — declared purely, materialized in Launch

**Files:**
- Modify: `internal/agent/agent.go` (add `StagedFile`, `Staged`; add `os` import)
- Modify: `internal/launch/plan.go` (build the `agent.Request` once, expose
  it on `Plan`, compute staged files at plan time)
- Modify: `internal/launch/handoff.go` (materialize before run)
- Test: `internal/launch/plan_test.go`, `internal/launch/handoff_test.go` (extend)

**Interfaces:**
- Consumes: plan_test harness (`fakeLauncher`, `spec`, `newTestService`),
  `config.Dir() (string, error)`.
- Produces: `agent.StagedFile{Path string; Contents []byte; Mode os.FileMode}`;
  `agent.Staged` with `StagedFiles(Request) ([]StagedFile, error)`;
  `launch.Plan` gains `AgentRequest agent.Request` and
  `Staged []agent.StagedFile`. Task 2's openclaw implements `Staged`;
  Task 3 consumes `Plan.AgentRequest`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/launch/plan_test.go`:

```go
type stagedLauncher struct {
	fakeLauncher
	files []agent.StagedFile
}

func (s *stagedLauncher) StagedFiles(agent.Request) ([]agent.StagedFile, error) {
	return s.files, nil
}

func TestPlanCarriesStagedFilesAndAgentRequest(t *testing.T) {
	svc := newTestService(t)
	want := []agent.StagedFile{{Path: "/tmp/x/openclaw.json", Contents: []byte("{}"), Mode: 0o644}}
	p, err := svc.Plan(context.Background(), Request{
		Spec:      spec("fake", &stagedLauncher{files: want}),
		ModelID:   "anthropic/claude-opus-4.6",
		ExtraArgs: []string{"--flag"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Staged) != 1 || p.Staged[0].Path != want[0].Path {
		t.Errorf("Staged = %+v, want %+v", p.Staged, want)
	}
	if p.AgentRequest.Model.ID != "anthropic/claude-opus-4.6" {
		t.Errorf("AgentRequest.Model.ID = %q", p.AgentRequest.Model.ID)
	}
	if !slices.Equal(p.AgentRequest.ExtraArgs, []string{"--flag"}) {
		t.Errorf("AgentRequest.ExtraArgs = %q", p.AgentRequest.ExtraArgs)
	}
}
```

(Use the same `ModelID` as `TestPlanIncompatibleModelYieldsConfirmableWarning`
if the harness catalog differs; add `"slices"` to imports if missing.)

Append to `internal/launch/handoff_test.go`:

```go
func TestLaunchStagesFilesBeforeRun(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	staged := agent.StagedFile{
		Path:     filepath.Join(dir, "openclaw.json"),
		Contents: []byte(`{"agents":{}}`),
		Mode:     0o644,
	}
	var contentAtRun []byte
	svc := &Service{Run: func(agent.Command) error {
		contentAtRun, _ = os.ReadFile(staged.Path)
		return nil
	}}
	p := Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Command: agent.Command{Path: "/bin/true"},
		Staged:  []agent.StagedFile{staged},
	}
	if err := svc.Launch(p, nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if string(contentAtRun) != string(staged.Contents) {
		t.Errorf("at run time file held %q, want %q — staging must precede the handoff", contentAtRun, staged.Contents)
	}
	info, err := os.Stat(staged.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestLaunchRefusesStagedFileOutsideConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	outside := filepath.Join(t.TempDir(), "evil.json")
	ran := false
	svc := &Service{Run: func(agent.Command) error { ran = true; return nil }}
	p := Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Command: agent.Command{Path: "/bin/true"},
		Staged:  []agent.StagedFile{{Path: outside, Contents: []byte("x"), Mode: 0o644}},
	}
	err := svc.Launch(p, nil)
	if err == nil {
		t.Fatal("Launch staged a file outside the launcher config dir")
	}
	if ran {
		t.Error("run happened despite staging failure")
	}
	if _, statErr := os.Stat(outside); statErr == nil {
		t.Error("the outside file was written")
	}
}
```

(Imports for handoff_test.go: ensure `os`, `path/filepath`,
`github.com/teggen/openrouter-launch/internal/agent`,
`github.com/teggen/openrouter-launch/internal/config` are present.)

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/launch/ -run 'TestPlanCarriesStaged|TestLaunchStages|TestLaunchRefuses' -v`
Expected: FAIL — `undefined: agent.StagedFile`.

- [ ] **Step 3: Implement**

Append to `internal/agent/agent.go` (add `"os"` to its imports):

```go
// StagedFile is a launcher-owned file a launch needs on disk — openclaw's
// model config is the canonical case. Declared as data so Command stays
// pure; launch.Service.Launch materializes it. Staged files live under this
// tool's own config dir and must never contain secrets.
type StagedFile struct {
	Path     string
	Contents []byte
	Mode     os.FileMode
}

// Staged is implemented by launchers that need launcher-owned files at
// launch time. StagedFiles MUST be pure, like Command. Distinct from
// ConfigWriter on purpose: Staged writes OUR files (idempotent overwrite,
// no undo, syscall.Exec handoff unaffected); ConfigWriter writes an
// AGENT'S file (backup and restore required, forces fork-and-wait). Do not
// merge them — the distinction is the amended Landmine 6 in type form.
type Staged interface {
	StagedFiles(Request) ([]StagedFile, error)
}
```

In `internal/launch/plan.go`: add fields to `Plan`:

```go
type Plan struct {
	Spec     *agent.Spec
	Model    openrouter.Model
	Command  agent.Command
	// AgentRequest is the request Command was built from. The fork-and-wait
	// path re-uses it for ConfigWriter.Apply, which cannot run at plan time
	// (it writes).
	AgentRequest agent.Request
	// Staged are launcher-owned files Launch materializes before the
	// handoff. Computed here (purely) so Launch stays a straight line.
	Staged   []agent.StagedFile
	Warnings []Warning
}
```

Replace the tail of `(*Service).Plan` (from the `spec.Launcher.Command(...)`
call down) with:

```go
	areq := agent.Request{
		Model:     model,
		APIKey:    apiKey,
		ExtraArgs: req.ExtraArgs,
	}
	command, err := spec.Launcher.Command(areq)
	if err != nil {
		return Plan{Warnings: warnings}, err
	}

	var staged []agent.StagedFile
	if st, ok := spec.Launcher.(agent.Staged); ok {
		staged, err = st.StagedFiles(areq)
		if err != nil {
			return Plan{Warnings: warnings}, err
		}
	}

	return Plan{
		Spec:         spec,
		Model:        model,
		Command:      command,
		AgentRequest: areq,
		Staged:       staged,
		Warnings:     warnings,
	}, nil
```

In `internal/launch/handoff.go`: insert between `recordSelection` and
`s.run` (imports gain `fmt`, `os`, `path/filepath`, `strings`,
`github.com/teggen/openrouter-launch/internal/agent`):

```go
	if err := stageFiles(p.Staged); err != nil {
		return fmt.Errorf("stage launcher-owned config: %w", err)
	}
```

And append:

```go
// stageFiles writes the plan's launcher-owned files. It refuses any path
// outside this tool's own config dir: staged files are write site #3 of the
// amended Landmine 6, and the boundary is enforced here, not trusted.
func stageFiles(files []agent.StagedFile) error {
	if len(files) == 0 {
		return nil
	}
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	for _, f := range files {
		rel, err := filepath.Rel(dir, f.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("staged file %q is outside the launcher config dir %q", f.Path, dir)
		}
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(f.Path, f.Contents, f.Mode); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests, verify green**

Run: `go test ./internal/launch/ -count=1 && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Mutation checks**

1. Move the `stageFiles` call AFTER `s.run(p.Command)` →
   `TestLaunchStagesFilesBeforeRun` FAILS (empty content at run time).
2. Delete the outside-dir guard in `stageFiles` →
   `TestLaunchRefusesStagedFileOutsideConfigDir` FAILS.
3. Drop `AgentRequest` population → `TestPlanCarriesStagedFilesAndAgentRequest`
   FAILS.

- [ ] **Step 6: Update the write-site record IN THE SAME COMMIT** (Landmine 6)

In `HANDOFF.md`: Landmine 6's text becomes the amended form — three
launcher-owned write sites (cache, config, staged files under the config
dir via `stageFiles` in `internal/launch/handoff.go`), agent-owned writes
still forbidden outside ConfigWriter (arriving in Task 3/4). Update the
"Verify the tree is sound" grep expectation to allow
`internal/launch/handoff.go` hits.

- [ ] **Step 7: Commit**

```bash
git add internal/agent/agent.go internal/launch/plan.go internal/launch/handoff.go internal/launch/plan_test.go internal/launch/handoff_test.go HANDOFF.md
git commit -m "feat(launch): Staged capability — launcher-owned files, one write site

Landmine 6 amended in the same commit: write site #3 (staged files under
our config dir), boundary enforced in stageFiles.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: openclaw launcher

Doc-verified on OpenClaw 2026.7.1-2. Interactive `tui --local` (embedded
runtime, no gateway/daemon) has no model flag — the model rides a
launcher-owned config passed via `OPENCLAW_CONFIG_PATH` (write site #3,
Task 1's machinery). One-shot `agent exec` passthrough needs no file at all.
Owner-approved consequence: `OPENCLAW_CONFIG_PATH` replaces the user's whole
openclaw config for the session.

**Files:**
- Create: `internal/agent/openclaw.go`
- Test: `internal/agent/openclaw_test.go`
- Modify: `internal/agent/registry.go` (entry before `chatgpt`),
  `internal/agent/registry_test.go` (add `"openclaw"` to
  `TestRegistryTier2Agents`)

**Interfaces:**
- Consumes: `rejectModelFlag`, `rejectFlags` (4a Task 1), `Staged`/`StagedFile`
  (Task 1), `config.Dir()`, test helpers.
- Produces: `type OpenClaw struct { LookPath func(string) (string, error) }`
  with `Name() "openclaw"`, `DisplayName() "OpenClaw"`, `Command`,
  `StagedFiles`, `CheckInstalled`, `InstallHint`, `ShadowedCredential`.

- [ ] **Step 1: Write the failing tests**

`internal/agent/openclaw_test.go`:

```go
package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

func TestOpenClawInteractiveCommandAndStagedFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := &OpenClaw{LookPath: stubLookPath("/usr/local/bin/openclaw")}
	req := Request{Model: testModel(), APIKey: "sk-or-test"}

	cmd, err := o.Command(req)
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if want := []string{"tui", "--local"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}

	files, err := o.StagedFiles(req)
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("StagedFiles returned %d files, want 1", len(files))
	}
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "openclaw.json"); files[0].Path != want {
		t.Errorf("staged path = %q, want %q", files[0].Path, want)
	}
	// Refs are lowercased and openrouter/-prefixed; map marshaling sorts
	// keys, so the content is deterministic.
	wantCfg := `{"agents":{"defaults":{"model":{"primary":"openrouter/anthropic/claude-opus-4.6"},"models":{"openrouter/anthropic/claude-opus-4.6":{}}}}}`
	if string(files[0].Contents) != wantCfg {
		t.Errorf("staged contents =\n%s\nwant\n%s", files[0].Contents, wantCfg)
	}
	if files[0].Mode != 0o644 {
		t.Errorf("mode = %v, want 0644 (no secret inside)", files[0].Mode)
	}
	if strings.Contains(string(files[0].Contents), "sk-or-test") {
		t.Error("API key leaked into the staged file")
	}

	if got, ok := envValue(cmd.Env, "OPENCLAW_CONFIG_PATH"); !ok || got != files[0].Path {
		t.Errorf("OPENCLAW_CONFIG_PATH = %q, %v; want %q", got, ok, files[0].Path)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
}

func TestOpenClawLowercasesModelRef(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := &OpenClaw{LookPath: stubLookPath("/usr/local/bin/openclaw")}
	files, err := o.StagedFiles(Request{
		Model:  openrouter.Model{ID: "MoonshotAI/Kimi-K2.5"},
		APIKey: "k",
	})
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if !strings.Contains(string(files[0].Contents), `"openrouter/moonshotai/kimi-k2.5"`) {
		t.Errorf("ref not lowercased: %s", files[0].Contents)
	}
}

func TestOpenClawOneShotSkipsStagingAndInjectsModel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := &OpenClaw{LookPath: stubLookPath("/usr/local/bin/openclaw")}
	req := Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"agent", "exec", "say OK"},
	}
	cmd, err := o.Command(req)
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// --auth-env-only composes config in memory and shuts off stored auth
	// profiles: nothing read from disk, nothing written, env key only.
	want := []string{"agent", "exec", "say OK", "--model", "openrouter/anthropic/claude-opus-4.6", "--auth-env-only"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if _, ok := envValue(cmd.Env, "OPENCLAW_CONFIG_PATH"); ok {
		t.Error("OPENCLAW_CONFIG_PATH set on the one-shot path; --auth-env-only loads no config")
	}
	files, err := o.StagedFiles(req)
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("one-shot staged %d files, want 0", len(files))
	}
}

func TestOpenClawCommandRejectsConflictsAndOtherSubcommands(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := &OpenClaw{LookPath: stubLookPath("/usr/local/bin/openclaw")}
	for _, extras := range [][]string{
		{"--model", "x"}, {"--model=x"}, {"-m", "x"},
		{"gateway", "run"}, {"daemon", "start"}, {"onboard"},
	} {
		if _, err := o.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want error", extras)
		}
	}
}

func TestOpenClawRequiresAPIKeyAndBinary(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := &OpenClaw{LookPath: stubLookPath("/usr/local/bin/openclaw")}
	if _, err := o.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
	missing := &OpenClaw{LookPath: func(string) (string, error) { return "", errors.New("no") }}
	if missing.CheckInstalled() {
		t.Error("CheckInstalled = true with neither openclaw nor clawdbot")
	}
}

func TestOpenClawFallsBackToClawdbotBinary(t *testing.T) {
	o := &OpenClaw{LookPath: func(name string) (string, error) {
		if name == "clawdbot" {
			return "/usr/local/bin/clawdbot", nil
		}
		return "", errors.New("not found")
	}}
	if !o.CheckInstalled() {
		t.Error("CheckInstalled = false with legacy clawdbot binary present")
	}
}

func TestOpenClawShadowedCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	o := &OpenClaw{}

	if msg := o.ShadowedCredential(); msg != "" {
		t.Errorf("fresh HOME: msg = %q, want empty", msg)
	}
	dir := filepath.Join(home, ".openclaw", "agents", "main", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "auth-profiles.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{"anthropic:default":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := o.ShadowedCredential(); msg != "" {
		t.Errorf("non-openrouter profile: msg = %q, want empty", msg)
	}
	if err := os.WriteFile(path, []byte(`{"profiles":{"openrouter:default":{"key":"sk-or-old"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := o.ShadowedCredential(); !strings.Contains(msg, "auth profile") {
		t.Errorf("openrouter profile: msg = %q, want an auth-profile warning", msg)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestOpenClaw -v`
Expected: FAIL — `undefined: OpenClaw`.

- [ ] **Step 3: Implement `internal/agent/openclaw.go`**

```go
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/teggen/openrouter-launch/internal/config"
)

// OpenClaw launches OpenClaw against an OpenRouter model. Interactive
// sessions run `openclaw tui --local` — the embedded runtime, no gateway or
// daemon — with the model in a LAUNCHER-OWNED config file passed via
// OPENCLAW_CONFIG_PATH (openclaw's tui has no model flag or env var). That
// file is write site #3 of the amended Landmine 6; it lives under OUR
// config dir, holds only the model ref, and replaces the user's own
// openclaw config for the session (owner-approved at spec review — a
// launched session deliberately does not load their channels/plugins).
// One-shot `agent exec` passthrough needs no file: --model plus
// --auth-env-only compose config in memory. Doc-verified on 2026.7.1-2
// (2026-08-09); see .superpowers/sdd/2026-08-09-tier-2-research/openclaw.md.
type OpenClaw struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (o *OpenClaw) Name() string        { return "openclaw" }
func (o *OpenClaw) DisplayName() string { return "OpenClaw" }

func (o *OpenClaw) lookPath(file string) (string, error) {
	if o.LookPath != nil {
		return o.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the openclaw binary, falling back to the pre-rename
// clawdbot name.
func (o *OpenClaw) findPath() (string, error) {
	if path, err := o.lookPath("openclaw"); err == nil {
		return path, nil
	}
	if path, err := o.lookPath("clawdbot"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("openclaw binary not found")
}

// openclawModelRef converts an OpenRouter slug to openclaw's model ref:
// provider-prefixed and lowercased (openclaw normalizes refs to lowercase).
func openclawModelRef(slug string) string {
	return "openrouter/" + strings.ToLower(slug)
}

// openclawConfigPath is the launcher-owned staged config location.
func openclawConfigPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "openclaw.json"), nil
}

// openclawOneShot reports whether the passthrough invokes openclaw's
// embedded one-shot mode (`agent exec …`), which takes --model directly and
// needs no staged config.
func openclawOneShot(extras []string) bool {
	return len(extras) > 0 && extras[0] == "agent"
}

// Command builds the openclaw invocation. Pure: the staged file is declared
// by StagedFiles and written by the launch service, never here.
func (o *OpenClaw) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("openclaw: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("openclaw", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if len(req.ExtraArgs) > 0 && !strings.HasPrefix(req.ExtraArgs[0], "-") && req.ExtraArgs[0] != "agent" {
		return Command{}, fmt.Errorf("openclaw: passthrough %q is platform administration, not a launch: openrouter-launch runs \"openclaw tui --local\" (or \"agent exec …\" passthrough)", req.ExtraArgs[0])
	}
	path, err := o.findPath()
	if err != nil {
		return Command{}, err
	}
	ref := openclawModelRef(req.Model.ID)

	if openclawOneShot(req.ExtraArgs) {
		args := append(append([]string(nil), req.ExtraArgs...), "--model", ref, "--auth-env-only")
		return Command{
			Path: path,
			Args: args,
			Env:  []string{"OPENROUTER_API_KEY=" + req.APIKey},
		}, nil
	}

	cfgPath, err := openclawConfigPath()
	if err != nil {
		return Command{}, err
	}
	args := append([]string{"tui", "--local"}, req.ExtraArgs...)
	env := []string{
		"OPENCLAW_CONFIG_PATH=" + cfgPath,
		"OPENROUTER_API_KEY=" + req.APIKey,
	}
	return Command{Path: path, Args: args, Env: env}, nil
}

// StagedFiles declares the launcher-owned model config for interactive
// launches. Pure: returns data; launch.Service.Launch writes it. No secret
// goes in — the key travels in env only — so the mode is 0644.
func (o *OpenClaw) StagedFiles(req Request) ([]StagedFile, error) {
	if openclawOneShot(req.ExtraArgs) {
		return nil, nil
	}
	ref := openclawModelRef(req.Model.ID)
	cfg := map[string]any{
		"agents": map[string]any{
			"defaults": map[string]any{
				"model":  map[string]any{"primary": ref},
				"models": map[string]any{ref: map[string]any{}},
			},
		},
	}
	contents, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("openclaw: building staged config: %w", err)
	}
	path, err := openclawConfigPath()
	if err != nil {
		return nil, err
	}
	return []StagedFile{{Path: path, Contents: contents, Mode: 0o644}}, nil
}

// CheckInstalled reports whether an openclaw (or legacy clawdbot) binary
// can be found. npm global installs land on PATH.
func (o *OpenClaw) CheckInstalled() bool {
	_, err := o.findPath()
	return err == nil
}

// InstallHint tells the user how to install OpenClaw. Printed, never run.
func (o *OpenClaw) InstallHint() string {
	return "Install OpenClaw: npm install -g openclaw@latest"
}

// ShadowedCredential reports stored OpenClaw auth profiles for OpenRouter:
// a prior onboard/OAuth stores a key that participates in auth rotation,
// and its precedence against the env key is undocumented — surface it.
func (o *OpenClaw) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	matches, err := filepath.Glob(filepath.Join(home, ".openclaw", "agents", "*", "agent", "auth-profiles.json"))
	if err != nil {
		return ""
	}
	for _, m := range matches {
		if openclawProfilesHaveOpenRouter(m) {
			return "openclaw has a stored OpenRouter auth profile (" + m + ") that may take precedence over the key this launch provides"
		}
	}
	return ""
}

func openclawProfilesHaveOpenRouter(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc struct {
		Profiles map[string]json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	for key := range doc.Profiles {
		if strings.Contains(key, "openrouter") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Register and run the full suite**

Insert into the `specs` literal in `internal/agent/registry.go`, immediately
before the `chatgpt` entry:

```go
	{
		Name:        "openclaw",
		Launcher:    &OpenClaw{},
		Description: "Personal AI assistant with a terminal session",
		Status:      Status{Supported: true},
	},
```

Add `"openclaw"` to the `TestRegistryTier2Agents` slice in
`internal/agent/registry_test.go`. Run `go test ./... -count=1` — fix
row-count listing tests only, as in 4a; STOP on anything semantic.

- [ ] **Step 5: Mutation checks**

1. Drop the lowercasing in `openclawModelRef` →
   `TestOpenClawLowercasesModelRef` FAILS.
2. Stage the file on the one-shot path too →
   `TestOpenClawOneShotSkipsStagingAndInjectsModel` FAILS.
3. Drop `--auth-env-only` from the one-shot args → same test FAILS.
4. Put the API key into the staged config →
   `TestOpenClawInteractiveCommandAndStagedFile` FAILS on the key-leak scan.
5. Accept `{"gateway", "run"}` passthrough →
   `TestOpenClawCommandRejectsConflictsAndOtherSubcommands` FAILS.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/openclaw.go internal/agent/openclaw_test.go internal/agent/registry.go internal/agent/registry_test.go
git commit -m "feat(agent): openclaw launcher — staged config for tui, env-only one-shot

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Fork-and-wait launch path for ConfigWriter agents

The main spec always promised this: an agent implementing `ConfigWriter`
cannot take `syscall.Exec`, because its `restore` must run after the session
ends. This task builds the path with a fake; droid (Task 4) is its first
real user.

**Files:**
- Create: `internal/agent/exec_wait.go`
- Test: `internal/agent/exec_wait_test.go`
- Modify: `internal/launch/service.go` (add `RunWait` field + accessor)
- Modify: `internal/launch/handoff.go` (branch on `ConfigWriter`)
- Test: `internal/launch/handoff_test.go` (extend)

**Interfaces:**
- Consumes: `ExecArgs` (exists), `ConfigWriter` (exists, unimplemented),
  `Plan.AgentRequest` (Task 1).
- Produces: `agent.RunWait(Command) error`; `launch.Service.RunWait
  func(agent.Command) error` (nil → `agent.RunWait`). Task 4's droid rides
  this path with zero further launch changes.

- [ ] **Step 1: Write the failing tests**

`internal/agent/exec_wait_test.go`:

```go
package agent

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"
)

func TestRunWaitPropagatesExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	err := RunWait(Command{Path: "/bin/sh", Args: []string{"-c", "exit 3"}})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want *exec.ExitError", err)
	}
	if code := exitErr.ExitCode(); code != 3 {
		t.Errorf("ExitCode = %d, want 3", code)
	}
	if err := RunWait(Command{Path: "/bin/sh", Args: []string{"-c", "exit 0"}}); err != nil {
		t.Errorf("clean exit returned %v", err)
	}
}
```

Append to `internal/launch/handoff_test.go`:

```go
type recordingConfigWriter struct {
	fakeLauncher
	log      *[]string
	applyErr error
}

func (r *recordingConfigWriter) Apply(agent.Request) (func() error, error) {
	if r.applyErr != nil {
		return nil, r.applyErr
	}
	*r.log = append(*r.log, "apply")
	return func() error {
		*r.log = append(*r.log, "restore")
		return nil
	}, nil
}

func TestLaunchConfigWriterOrderApplyRunRestore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var log []string
	svc := &Service{
		RunWait: func(agent.Command) error { log = append(log, "run"); return nil },
		Run: func(agent.Command) error {
			t.Error("exec-style Run used for a ConfigWriter agent")
			return nil
		},
	}
	p := Plan{
		Spec:    spec("fake", &recordingConfigWriter{log: &log}),
		Command: agent.Command{Path: "/bin/true"},
	}
	if err := svc.Launch(p, nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if want := []string{"apply", "run", "restore"}; !slices.Equal(log, want) {
		t.Errorf("order = %v, want %v", log, want)
	}
}

func TestLaunchConfigWriterRestoreRunsOnRunFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var log []string
	runErr := errors.New("agent exited 1")
	svc := &Service{RunWait: func(agent.Command) error { return runErr }}
	p := Plan{
		Spec:    spec("fake", &recordingConfigWriter{log: &log}),
		Command: agent.Command{Path: "/bin/true"},
	}
	err := svc.Launch(p, nil)
	if !errors.Is(err, runErr) {
		t.Errorf("err = %v, want the run error preserved (main extracts the exit code from it)", err)
	}
	if !slices.Contains(log, "restore") {
		t.Error("restore did not run after a failed session")
	}
}

func TestLaunchConfigWriterApplyFailureSkipsRun(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var log []string
	ran := false
	svc := &Service{RunWait: func(agent.Command) error { ran = true; return nil }}
	p := Plan{
		Spec:    spec("fake", &recordingConfigWriter{log: &log, applyErr: errors.New("settings file unparseable")}),
		Command: agent.Command{Path: "/bin/true"},
	}
	if err := svc.Launch(p, nil); err == nil {
		t.Fatal("Launch succeeded despite Apply failure")
	}
	if ran {
		t.Error("agent ran despite Apply failure")
	}
}
```

(Imports: `slices`, `errors` as needed.)

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./internal/agent/ -run TestRunWait -v && go test ./internal/launch/ -run TestLaunchConfigWriter -v`
Expected: agent FAIL — `undefined: RunWait`; launch FAIL — `unknown field RunWait`.

- [ ] **Step 3: Implement**

`internal/agent/exec_wait.go`:

```go
package agent

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// RunWait runs the command as a child process and waits for it — the launch
// path for ConfigWriter agents, whose restore must run after the session
// ends (syscall.Exec would replace this process and nothing after it could
// run). Same env merge as Run via ExecArgs; stdio is inherited. SIGINT and
// SIGTERM are forwarded to the child so the interactive session dies on its
// own terms while our restore still runs; on Windows Signal is best-effort
// and a failed forward is ignored. The returned error is cmd.Wait()'s —
// including *exec.ExitError, which main's exit-code extraction understands.
func RunWait(c Command) error {
	argv, env := ExecArgs(c)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Args = argv
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case s := <-sig:
			_ = cmd.Process.Signal(s)
		case err := <-done:
			return err
		}
	}
}
```

In `internal/launch/service.go`, add the field and accessor:

```go
	// RunWait performs the fork-and-wait handoff for ConfigWriter agents.
	// nil means agent.RunWait.
	RunWait func(agent.Command) error
```

```go
func (s *Service) runWait(c agent.Command) error {
	if s.RunWait != nil {
		return s.RunWait(c)
	}
	return agent.RunWait(c)
}
```

In `internal/launch/handoff.go`, replace the final `return s.run(p.Command)`
with (imports gain `errors`):

```go
	if cw, ok := p.Spec.Launcher.(agent.ConfigWriter); ok {
		return s.launchConfigWriter(p, cw)
	}
	return s.run(p.Command)
```

and append:

```go
// launchConfigWriter is the fork-and-wait path: Apply writes the agent's
// config (the one sanctioned agent-owned write, Landmine 6 as amended), the
// agent runs as a waited-on child, and restore undoes the write afterwards —
// including after a failed session. The run error is preserved through
// errors.Join so main's exit-code extraction still sees the *exec.ExitError.
func (s *Service) launchConfigWriter(p Plan, cw agent.ConfigWriter) error {
	restore, err := cw.Apply(p.AgentRequest)
	if err != nil {
		return fmt.Errorf("configure %s: %w", p.Spec.Name, err)
	}
	runErr := s.runWait(p.Command)
	if rerr := restore(); rerr != nil {
		return errors.Join(runErr, fmt.Errorf("restore %s config: %w", p.Spec.Name, rerr))
	}
	return runErr
}
```

- [ ] **Step 4: Run tests, verify green**

Run: `go test ./internal/agent/ ./internal/launch/ -count=1`
Expected: PASS.

- [ ] **Step 5: Mutation checks**

1. Swap `s.runWait` for `s.run` in `launchConfigWriter` →
   `TestLaunchConfigWriterOrderApplyRunRestore` FAILS (the exec-style Run
   trips the guard `t.Error`).
2. Return early on `runErr != nil` before calling `restore()` →
   `TestLaunchConfigWriterRestoreRunsOnRunFailure` FAILS.
3. Call `s.runWait` even when Apply errors →
   `TestLaunchConfigWriterApplyFailureSkipsRun` FAILS.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/exec_wait.go internal/agent/exec_wait_test.go internal/launch/service.go internal/launch/handoff.go internal/launch/handoff_test.go
git commit -m "feat(launch): fork-and-wait path for ConfigWriter agents

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: droid — the first ConfigWriter

Doc-verified on droid 0.190.0. No zero-touch surface exists (owner decision:
ConfigWriter, not unsupported). `Apply` upserts ONE marker-owned entry into
`~/.factory/settings.local.json` and points the default-model key at it;
`restore` puts everything back. Model selection lives in the file, not on
argv — forced by purity (the entry's index is only knowable at Apply time)
and it sidesteps the reported `--model custom:` upstream bug.

**Files:**
- Create: `internal/agent/droid.go`
- Test: `internal/agent/droid_test.go`
- Modify: `internal/agent/registry.go` (entry before `chatgpt`),
  `internal/agent/registry_test.go` (add `"droid"` to
  `TestRegistryTier2Agents`)

**Interfaces:**
- Consumes: `rejectModelFlag` (4a Task 1), `ConfigWriter` + fork-and-wait
  path (Task 3), `openrouter.DefaultBaseURL`, test helpers.
- Produces: `type Droid struct { LookPath func(string) (string, error) }`
  with `Name() "droid"`, `DisplayName() "Factory Droid"`, `Command`,
  `Apply`, `CheckInstalled`, `InstallHint`.

- [ ] **Step 1: Write the failing tests**

`internal/agent/droid_test.go`:

```go
package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func droidSettingsPath(t *testing.T, home string) string {
	t.Helper()
	return filepath.Join(home, ".factory", "settings.local.json")
}

func readDroidSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return m
}

func TestDroidCommandArgsAndEnv(t *testing.T) {
	d := &Droid{LookPath: stubLookPath("/usr/local/bin/droid")}
	cmd, err := d.Command(Request{Model: testModel(), APIKey: "sk-or-test", ExtraArgs: []string{"exec", "hi"}})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// No -m: model selection lives in the settings file Apply writes. The
	// index-derived custom: ID is only knowable at Apply time, and Command
	// is pure.
	if want := []string{"exec", "hi"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
	for _, extras := range [][]string{{"-m", "x"}, {"--model", "x"}, {"--model=x"}} {
		if _, err := d.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
}

func TestDroidApplyFreshFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	d := &Droid{}

	restore, err := d.Apply(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	path := droidSettingsPath(t, home)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-or-test") {
		t.Fatal("the real key was written to disk; only ${OPENROUTER_API_KEY} may appear")
	}
	m := readDroidSettings(t, path)
	models := m["customModels"].([]any)
	if len(models) != 1 {
		t.Fatalf("customModels has %d entries, want 1", len(models))
	}
	entry := models[0].(map[string]any)
	for key, want := range map[string]string{
		"displayName": "openrouter-launch",
		"provider":    "generic-chat-completion-api",
		"baseUrl":     "https://openrouter.ai/api/v1",
		"model":       "anthropic/claude-opus-4.6",
		"apiKey":      "${OPENROUTER_API_KEY}",
	} {
		if entry[key] != want {
			t.Errorf("entry[%q] = %v, want %q", key, entry[key], want)
		}
	}
	if m["model"] != "custom:openrouter-launch-0" {
		t.Errorf("model = %v, want custom:openrouter-launch-0", m["model"])
	}

	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("restore left behind a file we created into an empty state")
	}
}

func TestDroidApplyPreservesForeignEntriesAndPriorDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	prior := `{"model":"gpt-5.5-codex","customModels":[{"displayName":"Mine","provider":"generic-chat-completion-api","baseUrl":"http://mine","model":"m","apiKey":"k"}],"theme":"dark"}`
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(prior), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &Droid{}
	restore, err := d.Apply(Request{Model: testModel(), APIKey: "sk"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	m := readDroidSettings(t, path)
	models := m["customModels"].([]any)
	if len(models) != 2 {
		t.Fatalf("customModels has %d entries, want 2 (theirs + ours)", len(models))
	}
	if models[0].(map[string]any)["displayName"] != "Mine" {
		t.Error("foreign entry displaced from index 0")
	}
	// Ours is at index 1, so the selection ID must say 1.
	if m["model"] != "custom:openrouter-launch-1" {
		t.Errorf("model = %v, want custom:openrouter-launch-1", m["model"])
	}
	if m["theme"] != "dark" {
		t.Error("unrelated setting clobbered")
	}

	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	m = readDroidSettings(t, path)
	if m["model"] != "gpt-5.5-codex" {
		t.Errorf("restore: model = %v, want prior gpt-5.5-codex", m["model"])
	}
	models = m["customModels"].([]any)
	if len(models) != 1 || models[0].(map[string]any)["displayName"] != "Mine" {
		t.Errorf("restore: customModels = %v, want only the foreign entry", models)
	}
}

func TestDroidApplyReplacesStaleMarkerEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A crashed prior run left our marker entry with an old model.
	stale := `{"customModels":[{"displayName":"openrouter-launch","provider":"generic-chat-completion-api","baseUrl":"https://openrouter.ai/api/v1","model":"old/model","apiKey":"${OPENROUTER_API_KEY}"}]}`
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &Droid{}
	if _, err := d.Apply(Request{Model: testModel(), APIKey: "sk"}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	m := readDroidSettings(t, path)
	models := m["customModels"].([]any)
	if len(models) != 1 {
		t.Fatalf("stale marker not replaced: %d entries", len(models))
	}
	if got := models[0].(map[string]any)["model"]; got != "anthropic/claude-opus-4.6" {
		t.Errorf("model = %v, want the fresh slug", got)
	}
}

func TestDroidApplyRefusesUnparseableFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(`{definitely not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	d := &Droid{}
	if _, err := d.Apply(Request{Model: testModel(), APIKey: "sk"}); err == nil {
		t.Fatal("Apply clobbered a file it could not parse")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{definitely not json` {
		t.Error("unparseable file was modified")
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile**

Run: `go test ./internal/agent/ -run TestDroid -v`
Expected: FAIL — `undefined: Droid`.

- [ ] **Step 3: Implement `internal/agent/droid.go`**

```go
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// droidMarker owns our entry in droid's settings: displayName is what droid
// derives selection IDs from, and it is deliberately dash-safe. Apply
// replaces marker-owned entries and never touches others.
const droidMarker = "openrouter-launch"

// Droid launches Factory's droid via the ConfigWriter escape hatch — the
// ONE sanctioned agent-owned write (Landmine 6 as amended). Factory
// documents OpenRouter BYOK, but the only declaration surface is a
// .factory settings file: no env var, no flag, no inline config (owner
// decision at spec review: ConfigWriter, not unsupported). Apply writes a
// single marker-owned customModels entry into ~/.factory/settings.local.json
// (the merge-friendly local layer, never settings.json) with
// apiKey "${OPENROUTER_API_KEY}" — env interpolation, so the key never
// touches disk — and points the default-model key at it; restore puts both
// back. Model selection lives in the file, NOT on argv: the entry's
// index-derived custom: ID is only knowable at Apply time, and Command is
// pure. Requires a Factory account even for BYOK. Doc-verified on 0.190.0
// (2026-08-09); see .superpowers/sdd/2026-08-09-tier-2-research/droid.md.
type Droid struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (d *Droid) Name() string        { return "droid" }
func (d *Droid) DisplayName() string { return "Factory Droid" }

func (d *Droid) lookPath(file string) (string, error) {
	if d.LookPath != nil {
		return d.LookPath(file)
	}
	return exec.LookPath(file)
}

func droidSettingsFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".factory", "settings.local.json"), nil
}

// Command builds the droid invocation: passthrough only, no -m (see the
// type comment), key in env for the ${OPENROUTER_API_KEY} interpolation.
func (d *Droid) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("droid: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("droid", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	path, err := d.lookPath("droid")
	if err != nil {
		return Command{}, fmt.Errorf("droid binary not found: %w", err)
	}
	return Command{
		Path: path,
		Args: append([]string(nil), req.ExtraArgs...),
		Env:  []string{"OPENROUTER_API_KEY=" + req.APIKey},
	}, nil
}

// Apply upserts the marker-owned model entry and default-model key, and
// returns the restore that undoes exactly that. An unparseable settings
// file is a hard error — never clobber what we cannot understand.
func (d *Droid) Apply(req Request) (func() error, error) {
	path, err := droidSettingsFile()
	if err != nil {
		return nil, err
	}
	settings, existed, err := readDroidSettingsFile(path)
	if err != nil {
		return nil, err
	}

	priorModel, hadModel := settings["model"]

	kept := foreignDroidModels(settings)
	entry := map[string]any{
		"displayName":     droidMarker,
		"provider":        "generic-chat-completion-api",
		"baseUrl":         openrouter.DefaultBaseURL,
		"model":           req.Model.ID,
		"apiKey":          "${OPENROUTER_API_KEY}",
		"maxOutputTokens": 64000,
	}
	settings["customModels"] = append(kept, entry)
	settings["model"] = fmt.Sprintf("custom:%s-%d", droidMarker, len(kept))

	if err := writeDroidSettingsFile(path, settings); err != nil {
		return nil, err
	}

	restore := func() error {
		settings, _, err := readDroidSettingsFile(path)
		if err != nil {
			return err
		}
		kept := foreignDroidModels(settings)
		if len(kept) == 0 {
			delete(settings, "customModels")
		} else {
			settings["customModels"] = kept
		}
		if hadModel {
			settings["model"] = priorModel
		} else {
			delete(settings, "model")
		}
		if !existed && len(settings) == 0 {
			return os.Remove(path)
		}
		return writeDroidSettingsFile(path, settings)
	}
	return restore, nil
}

// readDroidSettingsFile loads the settings map; a missing file is an empty
// map with existed=false.
func readDroidSettingsFile(path string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false, fmt.Errorf("droid: %s is not valid JSON (%w); refusing to modify it", path, err)
	}
	return m, true, nil
}

// foreignDroidModels returns customModels entries we do not own, in their
// original order. A user editing the file mid-session keeps their entries.
func foreignDroidModels(settings map[string]any) []any {
	models, _ := settings["customModels"].([]any)
	var kept []any
	for _, item := range models {
		if entry, ok := item.(map[string]any); ok && entry["displayName"] == droidMarker {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// writeDroidSettingsFile writes atomically: temp file in the same dir, then
// rename (the Landmine 9 shape; 0644 because no secret is inside — the
// apiKey field holds the literal interpolation string).
func writeDroidSettingsFile(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// CheckInstalled reports whether the droid binary can be found. The
// standalone installer puts it in ~/.local/bin, which the installer adds to
// PATH; there is no reliable secondary location.
func (d *Droid) CheckInstalled() bool {
	_, err := d.lookPath("droid")
	return err == nil
}

// InstallHint tells the user how to install droid. Printed, never run.
// Droid requires a Factory account even on the BYOK-only tier.
func (d *Droid) InstallHint() string {
	return "Install droid: curl -fsSL https://app.factory.ai/cli | sh (requires a Factory account, even for BYOK)"
}
```

- [ ] **Step 4: Register and run the full suite**

Insert into the `specs` literal in `internal/agent/registry.go`, immediately
before the `chatgpt` entry:

```go
	{
		Name:        "droid",
		Launcher:    &Droid{},
		Description: "Factory's terminal coding agent (session-scoped managed config; Factory account required)",
		Status:      Status{Supported: true},
	},
```

Add `"droid"` to the `TestRegistryTier2Agents` slice in
`internal/agent/registry_test.go`. Run `go test ./... -count=1` — the launch
path picks fork-and-wait automatically via the `ConfigWriter` type assertion
(Task 3); fix row-count listing tests only; STOP on anything semantic.

- [ ] **Step 5: Mutation checks**

1. Write `req.APIKey` instead of the interpolation literal →
   `TestDroidApplyFreshFile` FAILS on the key-on-disk grep.
2. In `foreignDroidModels`, match on `provider` instead of `displayName` →
   `TestDroidApplyPreservesForeignEntriesAndPriorDefault` FAILS (the foreign
   entry uses the same provider).
3. Hardcode index 0 in the selection ID → the same test FAILS
   (`custom:openrouter-launch-1`).
4. Make `restore` delete the `model` key unconditionally → the same test
   FAILS on the prior-default assertion.
5. Make `readDroidSettingsFile` return an empty map on parse errors →
   `TestDroidApplyRefusesUnparseableFile` FAILS.

- [ ] **Step 6: Update the write-site record IN THE SAME COMMIT** (Landmine 6)

`HANDOFF.md`: Landmine 6 gains write site #4 — the one agent-owned
exception, `~/.factory/settings.local.json`, capability-gated behind
`ConfigWriter`, marker-owned entries only, restore on exit; the grep
expectation now allows `internal/agent/droid.go`. Also update the
architecture note "`ConfigWriter` … No agent implements it" — droid does now.

- [ ] **Step 7: Commit**

```bash
git add internal/agent/droid.go internal/agent/droid_test.go internal/agent/registry.go internal/agent/registry_test.go HANDOFF.md
git commit -m "feat(agent): droid launcher — first ConfigWriter, marker-owned settings entry

Landmine 6 amended in the same commit: write site #4, the one
capability-gated agent-owned exception.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Live verification gates — openclaw and droid

Neither is installed here; both gates need installs and droid needs a
**Factory account** — STOP and ask the owner before each. A declined gate
moves its items to HANDOFF open items; the launcher ships doc-verified-only.
Log to `.superpowers/sdd/2026-08-09-phase-4b/live-<agent>.log`; grep logs
for the key afterwards. Falsified values follow the Landmine 18 protocol
(fix launcher + test, watch it fail first, commit, record in the spec).

- [ ] **Step 1: openclaw gate** (after owner-approved `npm install -g openclaw@latest`)

```bash
KEY=$(jq -r '.api_key // empty' "${XDG_CONFIG_HOME:-$HOME/.config}/openrouter-launch/config.json")
LOG="$PWD/.superpowers/sdd/2026-08-09-phase-4b"   # absolute: later steps cd into temp dirs
mkdir -p "$LOG" && go build -o /tmp/orl4b .
cd "$(mktemp -d)" && OPENROUTER_API_KEY=$KEY \
  openclaw agent exec "Reply with exactly the word OK" --model openrouter/openai/gpt-4o-mini --auth-env-only \
  2>&1 | tee "$LOG/live-openclaw-raw.log"
cd "$(mktemp -d)" && /tmp/orl4b openclaw -m openai/gpt-4o-mini -- agent exec "Reply with exactly the word OK" \
  2>&1 | tee "$LOG/live-openclaw-orl.log"
```

Must answer: `agent exec` + `--auth-env-only` exist and behave as documented
on the installed version; the ref format is accepted; THE interactive
question — does `OPENCLAW_CONFIG_PATH=<our file> openclaw tui --local` on a
fresh `OPENCLAW_STATE_DIR` reach a usable session without onboarding? (This
one needs a TTY: hand it to the owner as a scripted check, exactly like the
Phase 2/3 human smoke tests.) Zero-touch audit: `~/.openclaw/openclaw.json`
untouched byte-for-byte.

- [ ] **Step 2: droid gate** (after owner-approved install AND Factory login)

```bash
cd "$(mktemp -d)" && OPENROUTER_API_KEY=$KEY /tmp/orl4b droid -m openai/gpt-4o-mini -- exec "Reply with exactly the word OK" \
  2>&1 | tee "$LOG/live-droid-orl.log"
```

Must answer, in order:
1. Which default-model key the installed droid honors — top-level `model`
   (docs-current, what Task 4 writes) or `sessionDefaultSettings.model`
   (what ollama writes). If the latter: that is a falsified value — change
   `Apply`/`restore` + tests, commit as
   `fix(agent): droid default-model key live-verified`.
2. **The routing proof:** rerun with `OPENROUTER_API_KEY=sk-or-invalid` —
   the run MUST fail with an OpenRouter auth error. A completion despite a
   bogus key means droid silently fell back to a Factory-billed model — the
   failure mode the spec refuses to ship; if it happens, STOP: droid moves
   to unsupported-with-reason per the spec's contingency, evidence recorded.
3. `${OPENROUTER_API_KEY}` interpolation works in settings.local.json.
4. After the session: `settings.local.json` is byte-identical to its
   pre-launch state (or absent again, on a fresh machine) — restore proof.

- [ ] **Step 3: Record results in the spec and commit** (Phase 3 style, dated)

```bash
git add docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md
git commit -m "docs: record Phase 4b live verification results

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Full verification suite and handoff update

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

All green, `gofmt -l` empty.

- [ ] **Step 2: Write-site grep — the AMENDED Landmine 6 verification**

```bash
grep -rn "os.WriteFile\|os.Create\|os.MkdirAll\|os.Rename\|OpenFile\|CreateTemp" --include="*.go" . | grep -v _test
```

Expected hits, exhaustively: `internal/openrouter` (cache),
`internal/config` (config), `internal/launch/handoff.go` (`stageFiles`),
`internal/agent/droid.go` (`Apply`/`restore`/`writeDroidSettingsFile`).
Anything else — any other file in `internal/agent` included — is a Critical
defect. Record this exact expectation in HANDOFF's "Verify the tree is
sound" section.

- [ ] **Step 3: Update `HANDOFF.md`**

- Current-state table: all eight Tier 2 agents shipped; Phase 4 complete
  row; refreshed test count.
- Landmine 6: confirm the final amended text matches the spec's four-site
  table verbatim (Tasks 1 and 4 already edited it; make it coherent as one
  reading now that both sites exist).
- New landmines from this plan's work: openclaw's `OPENCLAW_CONFIG_PATH`
  replaces the user's whole config for the session (deliberate, owner-
  approved — do not "fix" it by merging configs); droid's model selection
  must stay in the settings file, never `-m custom:` (purity + the upstream
  bug); plus anything Task 5 surfaced. Do not renumber existing landmines.
- Open items: the openclaw interactive TTY check if still pending; skipped
  gates; the fork-and-wait SIGINT path has no automated test (same honest-
  gap class as the TUI's signal handling — record it, don't hide it).
- "Phase 3+ — more agents": rewrite — Tier 2 is COMPLETE (all eight);
  what remains is Tier 3 territory and the standing open items.

- [ ] **Step 4: Commit and push**

```bash
git add HANDOFF.md
git commit -m "docs: Phase 4 complete — all eight Tier 2 agents shipped

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
git push && git status -sb
```

Expected: `## main...origin/main` with no ahead/behind marker.

---

## Execution notes

- Execute AFTER Plan 4a. One fresh subagent per task; two-stage review per
  superpowers:subagent-driven-development; then ONE whole-branch review
  covering 4a + 4b together.
- Reviewer prompts must name the known failure pattern (tests passing for
  the wrong reason) AND this plan's specific risks: droid's restore
  semantics (foreign-entry preservation is where a subtle bug bites a real
  user's config) and the stageFiles boundary check (a path-prefix bug here
  is a write-anywhere primitive).
- Task 5 needs installs, a Factory account, and cents of spend — ASK before
  each. Droid's routing proof (bogus-key MUST fail) is the one gate that can
  demote an agent to unsupported; treat a demotion as spec-contingency
  execution, not a plan failure.
- After Task 6, the owner drives the interactive smoke tests (openclaw
  `tui --local`, droid interactive, plus the six 4a agents) — outside
  subagent scope, recorded as open items until done.
