# Phase 2 Planner Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the launch sequence out of `internal/cli` into a new `internal/launch` package that performs IO but never touches the terminal, so a Phase 2 bubbletea TUI can drive it.

**Architecture:** A `launch.Service` carries its two seams as nilable fields (`Catalog`, `Run`) instead of package globals. `Service.Plan` runs the nine guards and returns a `Plan` holding a built `agent.Command` plus `[]Warning`; hard stops come back as typed errors carrying their data. `Service.Launch` records the selection and hands off in one function so neither call site can invert that order. `cli` shrinks to rendering warnings and confirming.

**Tech Stack:** Go 1.22, cobra, stdlib testing. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-08-07-phase-2-planner-refactor-design.md`

## Global Constraints

- **`go 1.22` in `go.mod` is a deliberate compatibility floor.** Do not bump it. Nothing in this plan needs past it.
- **No new module dependencies.** `go.mod` must be unchanged at the end.
- **Zero-touch is absolute.** The only two write sites in the whole tree are `$XDG_CACHE_HOME/openrouter-launch/models.json` and `$XDG_CONFIG_HOME/openrouter-launch/config.json`. Any new write site is a Critical defect.
- **`Launcher.Command` must stay pure** — no file writes, no network, no spawning.
- **Two base URLs must never be unified:** `openrouter.DefaultBaseURL` ends in `/v1`; `agent.AnthropicBaseURL` must NOT.
- **Every commit leaves the tree green:** `go test ./... -count=1`, `go vet ./...`, and `gofmt -l .` (empty output).
- **Test quality bar:** every test must answer *would this fail if the behavior it names were broken?* Nine of the ten Important findings in Phase 1 were tests that passed for the wrong reason — `strings.Contains(out, "installed")` that could never fail because `"installed"` is a substring of `"not installed"`; a required-flag test that passed with the guard deleted because it only checked `err != nil`.
- **Error strings are a contract.** Every typed error's `Error()` must reproduce today's CLI output byte-for-byte.

## File Structure

**Created:**

| File | Responsibility |
|---|---|
| `internal/launch/conditions.go` | Typed errors + `ErrNoModel` + `CheckSupported` |
| `internal/launch/conditions_test.go` | Error message contracts and payload recovery |
| `internal/launch/warning.go` | `Warning`, `WarningKind`, `StaleWarning` |
| `internal/launch/warning_test.go` | Warning construction |
| `internal/launch/service.go` | `Service`, its nilable seams, `Snapshot` |
| `internal/launch/service_test.go` | Catalog fixtures + `Snapshot` behavior |
| `internal/launch/filters.go` | `FilterFrom`, `MergeFilters`, flag-name constants |
| `internal/launch/filters_test.go` | Merge table, incl. explicit-false override |
| `internal/launch/plan.go` | `Request`, `Plan`, `Service.Plan` guard sequence |
| `internal/launch/plan_test.go` | Guard ordering, warnings, typed errors |
| `internal/launch/handoff.go` | `Service.Launch` — record then hand off |
| `internal/launch/handoff_test.go` | Ordering, save-failure warning |
| `internal/cli/harness_test.go` | Test harness replacing the package globals |

**Modified:**

| File | Change |
|---|---|
| `internal/cli/root.go` | `app` struct, `NewRootCmdWith` |
| `internal/cli/catalog.go` | Delegates to `svc.Snapshot`, then deleted in Task 7 |
| `internal/cli/launch.go` | `resolveAndRun` shrinks to rendering |
| `internal/cli/models.go` | Honors `cfg.Filters` via `MergeFilters` |
| `internal/cli/profile.go` | Takes `*app`; uses `launch.CheckSupported` |
| `internal/cli/*_test.go` | Converted to the harness |
| `HANDOFF.md` | Phase 2 section updated |

**Deleted:** `internal/cli/catalog.go`, `internal/cli/catalog_test.go`, the `runner` and `catalogSource` globals, `checkAgentSupported`.

---

### Task 1: Typed conditions

**Files:**
- Create: `internal/launch/conditions.go`
- Test: `internal/launch/conditions_test.go`

**Interfaces:**
- Consumes: `agent.Spec`, `agent.Status` from `internal/agent`.
- Produces: `ErrNoModel`, `UnsupportedAgentError{Agent, Reason}`, `UnsupportedPlatformError{Agent, Err}`, `NotInstalledError{Agent, DisplayName, Hint}`, `UnknownModelError{ModelID, Suggestions}`, `CheckSupported(*agent.Spec) error`. Tasks 4 and 7 use all of these.

- [ ] **Step 1: Write the failing test**

Create `internal/launch/conditions_test.go`:

```go
package launch

import (
	"errors"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
)

// The Error() strings below are the CLI's current output verbatim. They are
// asserted exactly, not by substring, because cobra prints them to the user
// and this refactor promises byte-identical output.

func TestUnsupportedAgentErrorMessage(t *testing.T) {
	err := &UnsupportedAgentError{Agent: "copilot", Reason: "talks to GitHub's own backend"}
	want := "copilot cannot be pointed at OpenRouter: talks to GitHub's own backend"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestNotInstalledErrorMessage(t *testing.T) {
	err := &NotInstalledError{
		Agent:       "claude",
		DisplayName: "Claude Code",
		Hint:        "Install Claude Code: https://code.claude.com/docs/en/quickstart",
	}
	want := "Claude Code is not installed.\nInstall Claude Code: https://code.claude.com/docs/en/quickstart"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// The TUI renders the install hint as its own element, so it has to be
// reachable as data and not only as a substring of Error().
func TestNotInstalledErrorCarriesPayload(t *testing.T) {
	var err error = &NotInstalledError{
		Agent: "claude", DisplayName: "Claude Code", Hint: "brew install claude",
	}
	var nie *NotInstalledError
	if !errors.As(err, &nie) {
		t.Fatalf("errors.As did not recover *NotInstalledError from %T", err)
	}
	if nie.Hint != "brew install claude" {
		t.Errorf("Hint = %q", nie.Hint)
	}
	if nie.Agent != "claude" {
		t.Errorf("Agent = %q", nie.Agent)
	}
}

func TestUnknownModelErrorWithoutSuggestions(t *testing.T) {
	err := &UnknownModelError{ModelID: "nope/nope"}
	want := `unknown model "nope/nope"`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestUnknownModelErrorWithSuggestions(t *testing.T) {
	err := &UnknownModelError{
		ModelID:     "anthropic/claude",
		Suggestions: []string{"anthropic/claude-opus-4.6", "anthropic/claude-sonnet-4.5"},
	}
	want := "unknown model \"anthropic/claude\". Did you mean:\n" +
		"  anthropic/claude-opus-4.6\n  anthropic/claude-sonnet-4.5"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// UnsupportedPlatformError must not restate the launcher's message: the CLI
// prints the launcher's error verbatim today, and errors.Is has to keep
// reaching it.
func TestUnsupportedPlatformErrorWrapsAndPreservesText(t *testing.T) {
	inner := errors.New("windows is not supported yet")
	err := &UnsupportedPlatformError{Agent: "droid", Err: inner}

	if got := err.Error(); got != "windows is not supported yet" {
		t.Errorf("Error() = %q, want the launcher's own text", got)
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is should reach the wrapped launcher error")
	}
}

func TestCheckSupportedRejectsUnsupportedAgent(t *testing.T) {
	unsupported := &agent.Spec{
		Name:   "copilot",
		Status: agent.Status{Supported: false, Reason: "talks to GitHub's own backend"},
	}
	err := CheckSupported(unsupported)

	var uae *UnsupportedAgentError
	if !errors.As(err, &uae) {
		t.Fatalf("CheckSupported returned %T (%v), want *UnsupportedAgentError", err, err)
	}
	if uae.Agent != "copilot" {
		t.Errorf("Agent = %q", uae.Agent)
	}
	if uae.Reason != "talks to GitHub's own backend" {
		t.Errorf("Reason = %q", uae.Reason)
	}
}

func TestCheckSupportedAcceptsSupportedAgent(t *testing.T) {
	supported := &agent.Spec{Name: "claude", Status: agent.Status{Supported: true}}
	if err := CheckSupported(supported); err != nil {
		t.Errorf("CheckSupported(supported) = %v, want nil", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/launch/ -count=1`
Expected: FAIL — `build failed`, undefined: `UnsupportedAgentError`, `NotInstalledError`, `UnknownModelError`, `UnsupportedPlatformError`, `CheckSupported`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/launch/conditions.go`:

```go
// Package launch resolves a launch request into a runnable command without
// touching the terminal, so both the CLI and a TUI can drive it.
//
// Every condition a user must see comes back as a value: an advisory
// Warning, or one of the typed errors below. Nothing here writes to stdout,
// stderr, or reads from stdin.
package launch

import (
	"errors"
	"fmt"
	"strings"

	"github.com/teggen/openrouter-launch/internal/agent"
)

// ErrNoModel reports that no model was selected. It is deliberately bare:
// the CLI's message names a CLI flag and the binary, which this package has
// no business knowing. Phase 2 turns this branch into "open the picker".
var ErrNoModel = errors.New("no model selected")

// UnsupportedAgentError reports an agent that cannot be pointed at
// OpenRouter at all.
type UnsupportedAgentError struct {
	Agent  string
	Reason string
}

func (e *UnsupportedAgentError) Error() string {
	return fmt.Sprintf("%s cannot be pointed at OpenRouter: %s", e.Agent, e.Reason)
}

// UnsupportedPlatformError reports an agent that cannot run on this
// platform. Error() is the launcher's own message unchanged, so CLI output
// is what it always was; Agent is carried for callers that want to name the
// agent themselves.
type UnsupportedPlatformError struct {
	Agent string
	Err   error
}

func (e *UnsupportedPlatformError) Error() string { return e.Err.Error() }
func (e *UnsupportedPlatformError) Unwrap() error { return e.Err }

// NotInstalledError reports a missing agent binary. The hint is a separate
// field rather than baked into the message so a caller can render it as
// something other than a line of error text.
type NotInstalledError struct {
	Agent       string
	DisplayName string
	Hint        string
}

func (e *NotInstalledError) Error() string {
	return fmt.Sprintf("%s is not installed.\n%s", e.DisplayName, e.Hint)
}

// UnknownModelError reports a slug that matched nothing, carrying the
// suggestions as data so a caller can offer them as choices rather than as a
// formatted list.
type UnknownModelError struct {
	ModelID     string
	Suggestions []string
}

func (e *UnknownModelError) Error() string {
	if len(e.Suggestions) == 0 {
		return fmt.Sprintf("unknown model %q", e.ModelID)
	}
	return fmt.Sprintf("unknown model %q. Did you mean:\n  %s",
		e.ModelID, strings.Join(e.Suggestions, "\n  "))
}

// CheckSupported reports why an agent cannot be pointed at OpenRouter. It is
// Plan's first guard, and `profile add` calls it directly to refuse saving a
// profile for an unsupported agent without planning a launch.
func CheckSupported(spec *agent.Spec) error {
	if !spec.Status.Supported {
		return &UnsupportedAgentError{Agent: spec.Name, Reason: spec.Status.Reason}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/launch/ -count=1 -v`
Expected: PASS — 7 tests.

- [ ] **Step 5: Verify formatting and vet**

Run: `gofmt -l internal/launch/ && go vet ./internal/launch/`
Expected: no output from either.

- [ ] **Step 6: Commit**

```bash
git add internal/launch/conditions.go internal/launch/conditions_test.go
git commit -m "feat(launch): add typed launch conditions

Hard stops carry their data - install hint, suggestions, agent name -
so a TUI can render them as panels and lists instead of parsing a
sentence. Error() reproduces today's CLI output byte-for-byte."
```

---

### Task 2: Service, Snapshot, and the Warning type

**Files:**
- Create: `internal/launch/service.go`, `internal/launch/warning.go`
- Test: `internal/launch/service_test.go`, `internal/launch/warning_test.go`

**Interfaces:**
- Consumes: `openrouter.Catalog`, `openrouter.Snapshot`, `openrouter.Cache`, `openrouter.CachePath`, `openrouter.DefaultTTL`, `openrouter.NewClient`, `agent.Command`, `agent.Run`.
- Produces: `Service{Catalog, Run}`, `(*Service).Snapshot(ctx, refresh) (openrouter.Snapshot, error)`, unexported `(*Service).run(agent.Command) error`, `Warning{Kind, Message, Question}`, `WarningKind` constants `WarnStaleCatalog`/`WarnIncompatibleModel`/`WarnSelectionNotSaved`, `StaleWarning(snap, now) (Warning, bool)`. Also produces the test fixtures `fakeCatalog`, `fakeModels()`, `erroringCatalog`, `writeCacheFileForTest`, `cachePathForTest` used by Tasks 4 and 5.

- [ ] **Step 1: Write the failing tests**

Create `internal/launch/warning_test.go`:

```go
package launch

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

func TestStaleWarningFreshSnapshotProducesNothing(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	snap := openrouter.Snapshot{FetchedAt: now.Add(-time.Hour)}

	if _, ok := StaleWarning(snap, now); ok {
		t.Error("a fresh snapshot should produce no warning")
	}
}

func TestStaleWarningReportsAgeAndCause(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	snap := openrouter.Snapshot{
		FetchedAt: now.Add(-90 * time.Minute),
		Stale:     true,
		StaleErr:  errors.New("network down"),
	}

	w, ok := StaleWarning(snap, now)
	if !ok {
		t.Fatal("a stale snapshot should produce a warning")
	}
	if w.Kind != WarnStaleCatalog {
		t.Errorf("Kind = %v, want WarnStaleCatalog", w.Kind)
	}
	// A stale catalog is informational. If it carried a Question, every
	// offline run would stop and wait for an answer.
	if w.Question != "" {
		t.Errorf("Question = %q, want empty for an informational warning", w.Question)
	}
	if !strings.Contains(w.Message, "network down") {
		t.Errorf("Message should name the refresh failure, got %q", w.Message)
	}
	if !strings.Contains(w.Message, "1h30m0s") {
		t.Errorf("Message should report the data's age, got %q", w.Message)
	}
}
```

Create `internal/launch/service_test.go`:

```go
package launch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// erroringCatalog always fails, forcing Snapshot down the stale-cache
// fallback path.
type erroringCatalog struct{}

func (erroringCatalog) Models(context.Context) ([]openrouter.Model, error) {
	return nil, errors.New("network down")
}

// fakeCatalog serves fixed models without touching the network.
type fakeCatalog struct{ models []openrouter.Model }

func (f *fakeCatalog) Models(context.Context) ([]openrouter.Model, error) {
	return f.models, nil
}

// fakeModels mirrors the CLI's fixture set. openai/o1-mini is the only entry
// without tool support, which is what several filter tests key on.
func fakeModels() []openrouter.Model {
	return []openrouter.Model{
		{ID: "anthropic/claude-opus-4.6", Name: "Anthropic: Claude Opus 4.6",
			ContextLength: 200000, PromptPricePerM: 15, CompletionPricePerM: 75,
			SupportsTools: true, Provider: "anthropic"},
		{ID: "qwen/qwen3-coder:free", Name: "Qwen: Qwen3 Coder (free)",
			ContextLength: 262144, SupportsTools: true, Provider: "qwen"},
		{ID: "openai/o1-mini", Name: "OpenAI: o1-mini",
			ContextLength: 128000, PromptPricePerM: 1.1, CompletionPricePerM: 4.4,
			Provider: "openai"},
	}
}

// writeCacheFileForTest writes a catalog cache file in the on-disk shape
// openrouter.Cache expects, without depending on its unexported cacheFile
// type: only the JSON shape needs to match.
func writeCacheFileForTest(t *testing.T, path string, fetchedAt time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(struct {
		FetchedAt time.Time          `json:"fetched_at"`
		Models    []openrouter.Model `json:"models"`
	}{FetchedAt: fetchedAt, Models: fakeModels()})
	if err != nil {
		t.Fatalf("marshal cache file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
}

// cachePathForTest isolates the catalog cache to a temp dir and returns its
// resolved path.
func cachePathForTest(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path, err := openrouter.CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	return path
}

func TestSnapshotServesStaleCacheWithoutFailing(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now().Add(-48*time.Hour)) // older than DefaultTTL

	svc := &Service{Catalog: erroringCatalog{}}
	snap, err := svc.Snapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !snap.Stale {
		t.Fatal("expected Stale when the refresh fails but a cache exists")
	}
	if len(snap.Models) == 0 {
		t.Error("a stale snapshot must still carry the cached models")
	}
}

func TestSnapshotDoesNotConsultSourceWhenFresh(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now()) // well within DefaultTTL

	// erroringCatalog fails if consulted at all, so a nil error here is the
	// evidence the fresh cache short-circuited the fetch.
	svc := &Service{Catalog: erroringCatalog{}}
	snap, err := svc.Snapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.Stale {
		t.Error("fresh cache reported as stale")
	}
}

func TestSnapshotWrapsHardFailure(t *testing.T) {
	cachePathForTest(t) // isolated, and deliberately no cache file written

	svc := &Service{Catalog: erroringCatalog{}}
	_, err := svc.Snapshot(context.Background(), false)
	if err == nil {
		t.Fatal("expected an error when the fetch fails with no cache to fall back on")
	}
	if !strings.Contains(err.Error(), "load model catalog") {
		t.Errorf("error should carry context, got: %v", err)
	}
}

func TestSnapshotForceRefreshBypassesFreshCache(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now()) // fresh, would normally short-circuit

	// A fresh cache plus a failing source: with refresh=true the source is
	// consulted, fails, and the cache is served as stale. Without the
	// bypass, Stale would be false.
	svc := &Service{Catalog: erroringCatalog{}}
	snap, err := svc.Snapshot(context.Background(), true)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !snap.Stale {
		t.Error("refresh=true should bypass the fresh cache and consult the source")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/launch/ -count=1`
Expected: FAIL — undefined: `Service`, `StaleWarning`, `WarnStaleCatalog`.

- [ ] **Step 3: Write the Warning type**

Create `internal/launch/warning.go`:

```go
package launch

import (
	"fmt"
	"time"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// WarningKind identifies an advisory condition so a caller can render it in
// its own idiom instead of parsing Message.
type WarningKind int

const (
	// WarnStaleCatalog reports that a catalog refresh failed and cached data
	// was served instead.
	WarnStaleCatalog WarningKind = iota
	// WarnIncompatibleModel reports a pairing the agent may not fully
	// support. Advisory by design: Claude Code works with many non-Anthropic
	// models, so hard-blocking would refuse valid setups.
	WarnIncompatibleModel
	// WarnSelectionNotSaved reports that the last selection could not be
	// persisted. The launch proceeds regardless.
	WarnSelectionNotSaved
)

// Warning is an advisory condition the caller renders.
type Warning struct {
	Kind WarningKind
	// Message is the diagnostic text, rendered after the caller's own
	// "warning: " prefix.
	Message string
	// Question is non-empty when the caller must get the user's approval
	// before launching, and is the prompt to put to them. Carrying the
	// wording here rather than a bare Confirm bool means a caller cannot
	// ask "Launch anyway?" about a warning that is not about launching.
	Question string
}

// StaleWarning returns the warning for a snapshot served from a failed
// refresh, and false when the snapshot is fresh. now is a parameter so this
// stays pure and testable.
func StaleWarning(snap openrouter.Snapshot, now time.Time) (Warning, bool) {
	if !snap.Stale {
		return Warning{}, false
	}
	return Warning{
		Kind: WarnStaleCatalog,
		Message: fmt.Sprintf(
			"could not refresh the model catalog (%v); using cached data from %s ago",
			snap.StaleErr, snap.Age(now).Round(time.Minute)),
	}, true
}
```

- [ ] **Step 4: Write the Service**

Create `internal/launch/service.go`:

```go
package launch

import (
	"context"
	"fmt"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Service resolves launch requests and hands off to agents. The zero value
// is usable and talks to the live OpenRouter API.
//
// Catalog and Run are fields rather than package globals so that a CLI run
// and a TUI session can hold their own, and so tests inject fakes without
// mutating process state.
type Service struct {
	// Catalog is the model source. nil means the live OpenRouter client.
	Catalog openrouter.Catalog
	// Run performs the process handoff. nil means agent.Run.
	Run func(agent.Command) error
}

func (s *Service) catalog() openrouter.Catalog {
	if s.Catalog != nil {
		return s.Catalog
	}
	return openrouter.NewClient()
}

func (s *Service) run(c agent.Command) error {
	if s.Run != nil {
		return s.Run(c)
	}
	return agent.Run(c)
}

// Snapshot returns the model catalog with its provenance. Staleness is
// reported on the Snapshot itself rather than written anywhere; callers turn
// it into a Warning with StaleWarning.
func (s *Service) Snapshot(ctx context.Context, refresh bool) (openrouter.Snapshot, error) {
	path, err := openrouter.CachePath()
	if err != nil {
		return openrouter.Snapshot{}, err
	}

	cache := &openrouter.Cache{
		Path:   path,
		TTL:    openrouter.DefaultTTL,
		Source: s.catalog(),
	}
	snap, err := cache.Load(ctx, refresh)
	if err != nil {
		return openrouter.Snapshot{}, fmt.Errorf("load model catalog: %w", err)
	}
	return snap, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/launch/ -count=1 -v`
Expected: PASS — 13 tests total.

- [ ] **Step 6: Verify formatting and vet**

Run: `gofmt -l internal/launch/ && go vet ./internal/launch/`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add internal/launch/service.go internal/launch/service_test.go \
        internal/launch/warning.go internal/launch/warning_test.go
git commit -m "feat(launch): add Service with injected seams and Warning values

Catalog and Run become nilable fields instead of the package globals
cli.catalogSource and cli.runner. Staleness stops being written to an
io.Writer and becomes a Warning the caller renders."
```

---

### Task 3: The filters bridge

**Files:**
- Create: `internal/launch/filters.go`
- Test: `internal/launch/filters_test.go`

**Interfaces:**
- Consumes: `config.Filters`, `openrouter.Filter`.
- Produces: `FlagTools`, `FlagFree`, `FlagMinContext`, `FlagMaxPrice` constants; `FilterFrom(config.Filters) openrouter.Filter`; `MergeFilters(config.Filters, openrouter.Filter, func(string) bool) openrouter.Filter`. Task 8 uses all of them.

- [ ] **Step 1: Write the failing test**

Create `internal/launch/filters_test.go`:

```go
package launch

import (
	"testing"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// changedSet builds the "was this flag explicitly set?" predicate that
// cmd.Flags().Changed provides in production.
func changedSet(names ...string) func(string) bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(name string) bool { return set[name] }
}

func TestFilterFromCopiesPersistedFields(t *testing.T) {
	got := FilterFrom(config.Filters{
		ToolsOnly: true, FreeOnly: true, MinContext: 1000, MaxPrice: 3,
	})

	want := openrouter.Filter{ToolsOnly: true, FreeOnly: true, MinContext: 1000, MaxPrice: 3}
	if got != want {
		t.Errorf("FilterFrom = %+v, want %+v", got, want)
	}
}

func TestFilterFromLeavesProviderAndSearchZero(t *testing.T) {
	// config.Filters has no Provider or Search field, so FilterFrom must not
	// invent one.
	got := FilterFrom(config.Filters{ToolsOnly: true})
	if got.Provider != "" {
		t.Errorf("Provider = %q, want empty", got.Provider)
	}
	if got.Search != "" {
		t.Errorf("Search = %q, want empty", got.Search)
	}
}

func TestMergeFiltersUsesPersistedWhenNoFlagSet(t *testing.T) {
	persisted := config.Filters{ToolsOnly: true, FreeOnly: true, MinContext: 100000, MaxPrice: 9}
	got := MergeFilters(persisted, openrouter.Filter{}, changedSet())

	want := openrouter.Filter{ToolsOnly: true, FreeOnly: true, MinContext: 100000, MaxPrice: 9}
	if got != want {
		t.Errorf("MergeFilters = %+v, want %+v", got, want)
	}
}

// This is the case the `changed` predicate exists for. An explicit
// --tools=false and an absent --tools are both `false` by value, so without
// the predicate this is unrepresentable and the persisted true silently
// wins. Deleting the changed(FlagTools) branch must fail this test.
func TestMergeFiltersExplicitFalseBeatsPersistedTrue(t *testing.T) {
	persisted := config.Filters{ToolsOnly: true}
	got := MergeFilters(persisted, openrouter.Filter{ToolsOnly: false}, changedSet(FlagTools))

	if got.ToolsOnly {
		t.Error("an explicit --tools=false must override a persisted ToolsOnly:true")
	}
}

func TestMergeFiltersFlagsOverridePersistedValues(t *testing.T) {
	persisted := config.Filters{FreeOnly: false, MinContext: 1000, MaxPrice: 100}
	flags := openrouter.Filter{FreeOnly: true, MinContext: 200000, MaxPrice: 5}
	got := MergeFilters(persisted, flags, changedSet(FlagFree, FlagMinContext, FlagMaxPrice))

	if !got.FreeOnly {
		t.Error("--free should override the persisted false")
	}
	if got.MinContext != 200000 {
		t.Errorf("MinContext = %d, want the flag value 200000", got.MinContext)
	}
	if got.MaxPrice != 5 {
		t.Errorf("MaxPrice = %v, want the flag value 5", got.MaxPrice)
	}
}

func TestMergeFiltersLeavesUnchangedFlagsAlone(t *testing.T) {
	// Only --free was set; MinContext must keep its persisted value rather
	// than being clobbered by the flag's zero.
	persisted := config.Filters{MinContext: 100000, MaxPrice: 7}
	flags := openrouter.Filter{FreeOnly: true}
	got := MergeFilters(persisted, flags, changedSet(FlagFree))

	if got.MinContext != 100000 {
		t.Errorf("MinContext = %d, want the persisted 100000", got.MinContext)
	}
	if got.MaxPrice != 7 {
		t.Errorf("MaxPrice = %v, want the persisted 7", got.MaxPrice)
	}
}

func TestMergeFiltersProviderAndSearchAlwaysComeFromFlags(t *testing.T) {
	// Neither has a persisted counterpart, so both pass through even though
	// `changed` reports nothing was set.
	flags := openrouter.Filter{Provider: "anthropic", Search: "opus"}
	got := MergeFilters(config.Filters{}, flags, changedSet())

	if got.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", got.Provider)
	}
	if got.Search != "opus" {
		t.Errorf("Search = %q, want opus", got.Search)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/launch/ -count=1 -run TestFilter -run TestMerge`
Expected: FAIL — undefined: `FilterFrom`, `MergeFilters`, `FlagTools`, `FlagFree`, `FlagMinContext`, `FlagMaxPrice`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/launch/filters.go`:

```go
package launch

import (
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Flag names for the filters that have a persisted counterpart. They are
// shared so that "was --tools set?" does not depend on a string literal
// duplicated between the flag registration and the merge.
const (
	FlagTools      = "tools"
	FlagFree       = "free"
	FlagMinContext = "min-context"
	FlagMaxPrice   = "max-price"
)

// FilterFrom converts persisted filter state into a catalog filter. Provider
// and Search have no persisted counterpart and stay zero.
func FilterFrom(f config.Filters) openrouter.Filter {
	return openrouter.Filter{
		ToolsOnly:  f.ToolsOnly,
		FreeOnly:   f.FreeOnly,
		MinContext: f.MinContext,
		MaxPrice:   f.MaxPrice,
	}
}

// MergeFilters returns the persisted filters overridden by each flag the
// user explicitly set. changed reports whether a flag was provided;
// cobra's cmd.Flags().Changed satisfies it directly.
//
// The predicate is load-bearing: an explicit --tools=false and an absent
// --tools are both false by value, so without it there is no way to turn a
// persisted ToolsOnly:true back off from the command line.
//
// Provider and Search always come from flags, having no persisted form.
func MergeFilters(persisted config.Filters, flags openrouter.Filter,
	changed func(string) bool) openrouter.Filter {

	out := FilterFrom(persisted)
	if changed(FlagTools) {
		out.ToolsOnly = flags.ToolsOnly
	}
	if changed(FlagFree) {
		out.FreeOnly = flags.FreeOnly
	}
	if changed(FlagMinContext) {
		out.MinContext = flags.MinContext
	}
	if changed(FlagMaxPrice) {
		out.MaxPrice = flags.MaxPrice
	}
	out.Provider = flags.Provider
	out.Search = flags.Search
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/launch/ -count=1 -v`
Expected: PASS — 20 tests total.

- [ ] **Step 5: Verify formatting and vet**

Run: `gofmt -l internal/launch/ && go vet ./internal/launch/`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add internal/launch/filters.go internal/launch/filters_test.go
git commit -m "feat(launch): bridge config.Filters and openrouter.Filter

config does not import openrouter and openrouter does not import config,
so launch is the only package that knows both. The changed() predicate
is what makes an explicit --tools=false distinguishable from an absent
--tools, both of which are false by value."
```

---

### Task 4: The planner

**Files:**
- Create: `internal/launch/plan.go`
- Test: `internal/launch/plan_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1 and 2; `agent.PlatformSupported`, `agent.Installable`, `agent.Compatible`, `agent.ErrIncompatibleModel`, `agent.Request`; `openrouter.FindByID`, `openrouter.Suggest`; `config.Load`, `config.ResolveAPIKey`.
- Produces: `Request{Spec, ModelID, ExtraArgs, Refresh}`, `Plan{Spec, Model, Command, Warnings}`, `(*Service).Plan(ctx, Request) (Plan, error)`. Also the test fixtures `fakeLauncher`, `spec()`, `newTestService()` used by Task 5.

- [ ] **Step 1: Write the failing test**

Create `internal/launch/plan_test.go`:

```go
package launch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// fakeLauncher implements only the required Launcher interface. The types
// below embed it to add one optional capability each, so every guard can be
// exercised in isolation - the real registry has one agent, which is always
// supported and never implements PlatformSupported, leaving those branches
// otherwise unreachable.
type fakeLauncher struct{}

func (*fakeLauncher) Name() string        { return "fake" }
func (*fakeLauncher) DisplayName() string { return "Fake Agent" }

func (*fakeLauncher) Command(req agent.Request) (agent.Command, error) {
	return agent.Command{
		Path: "/bin/fake",
		Args: append([]string{"--model", req.Model.ID}, req.ExtraArgs...),
		Env:  []string{"FAKE_API_KEY=" + req.APIKey},
	}, nil
}

// notInstalledLauncher reports its binary as absent.
type notInstalledLauncher struct{ fakeLauncher }

func (*notInstalledLauncher) CheckInstalled() bool { return false }
func (*notInstalledLauncher) InstallHint() string  { return "brew install fake" }

// unsupportedPlatformLauncher refuses to run here, whatever "here" is.
type unsupportedPlatformLauncher struct{ fakeLauncher }

func (*unsupportedPlatformLauncher) Supported() error {
	return errors.New("windows is not supported yet")
}

// blockedLauncher is both platform-unsupported and not installed, so a test
// can assert which of the two guards wins.
type blockedLauncher struct{ fakeLauncher }

func (*blockedLauncher) Supported() error      { return errors.New("windows is not supported yet") }
func (*blockedLauncher) CheckInstalled() bool  { return false }
func (*blockedLauncher) InstallHint() string   { return "brew install fake" }

// incompatibleLauncher returns an advisory ErrIncompatibleModel.
type incompatibleLauncher struct{ fakeLauncher }

func (*incompatibleLauncher) CheckModel(m openrouter.Model) error {
	return fmt.Errorf("%w: fake is optimized for anthropic/* models and may fail with %s",
		agent.ErrIncompatibleModel, m.ID)
}

// brokenCheckLauncher returns a genuine failure rather than an advisory one.
type brokenCheckLauncher struct{ fakeLauncher }

func (*brokenCheckLauncher) CheckModel(openrouter.Model) error {
	return errors.New("catalog service unreachable")
}

// buildErrorLauncher fails at command construction.
type buildErrorLauncher struct{ fakeLauncher }

func (*buildErrorLauncher) Command(agent.Request) (agent.Command, error) {
	return agent.Command{}, errors.New("binary vanished")
}

// spec wraps a launcher in a supported registry entry.
func spec(name string, l agent.Launcher) *agent.Spec {
	return &agent.Spec{Name: name, Launcher: l, Status: agent.Status{Supported: true}}
}

// newTestService isolates config and cache to a temp dir, serves fixed
// models, provides an API key, and stubs the handoff.
func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	return &Service{
		Catalog: &fakeCatalog{models: fakeModels()},
		Run:     func(agent.Command) error { return nil },
	}
}

// The guard order decides which of several simultaneous problems the user is
// told about first. Each ordering test below violates its own guard AND at
// least one later guard, so moving a guard down the sequence fails the test
// rather than quietly changing the message.

func TestPlanUnsupportedAgentBeatsEveryLaterGuard(t *testing.T) {
	svc := newTestService(t)
	s := &agent.Spec{
		Name:     "copilot",
		Launcher: &blockedLauncher{}, // also platform-unsupported and not installed
		Status:   agent.Status{Supported: false, Reason: "talks to GitHub's own backend"},
	}

	// ModelID empty, so ErrNoModel is live too.
	_, err := svc.Plan(context.Background(), Request{Spec: s, ModelID: ""})

	var uae *UnsupportedAgentError
	if !errors.As(err, &uae) {
		t.Fatalf("Plan returned %T (%v), want *UnsupportedAgentError", err, err)
	}
}

func TestPlanPlatformBeatsNoModelAndNotInstalled(t *testing.T) {
	svc := newTestService(t)
	s := spec("droid", &blockedLauncher{})

	_, err := svc.Plan(context.Background(), Request{Spec: s, ModelID: ""})

	var upe *UnsupportedPlatformError
	if !errors.As(err, &upe) {
		t.Fatalf("Plan returned %T (%v), want *UnsupportedPlatformError", err, err)
	}
	if upe.Agent != "droid" {
		t.Errorf("Agent = %q, want droid", upe.Agent)
	}
}

// The handoff document listed the sequence as support -> platform ->
// install -> catalog, omitting this check entirely. The code has always run
// the empty-model check BEFORE the install check, and Phase 2 turns this
// exact branch into "open the picker" - so a user with no agent installed
// must still reach the picker rather than a dead end.
func TestPlanNoModelBeatsNotInstalled(t *testing.T) {
	svc := newTestService(t)
	s := spec("fake", &notInstalledLauncher{})

	_, err := svc.Plan(context.Background(), Request{Spec: s, ModelID: ""})

	if !errors.Is(err, ErrNoModel) {
		t.Fatalf("Plan returned %v, want ErrNoModel", err)
	}
}

func TestPlanNotInstalledBeatsUnknownModel(t *testing.T) {
	svc := newTestService(t)
	s := spec("fake", &notInstalledLauncher{})

	_, err := svc.Plan(context.Background(), Request{Spec: s, ModelID: "no/such-model"})

	var nie *NotInstalledError
	if !errors.As(err, &nie) {
		t.Fatalf("Plan returned %T (%v), want *NotInstalledError", err, err)
	}
	if nie.Hint != "brew install fake" {
		t.Errorf("Hint = %q", nie.Hint)
	}
	if nie.DisplayName != "Fake Agent" {
		t.Errorf("DisplayName = %q", nie.DisplayName)
	}
}

func TestPlanUnknownModelCarriesSuggestions(t *testing.T) {
	svc := newTestService(t)

	// "anthropic/claude-opus" is not an exact slug, but it is a substring of
	// anthropic/claude-opus-4.6, which is what Suggest's matching needs.
	_, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &fakeLauncher{}), ModelID: "anthropic/claude-opus",
	})

	var ume *UnknownModelError
	if !errors.As(err, &ume) {
		t.Fatalf("Plan returned %T (%v), want *UnknownModelError", err, err)
	}
	if len(ume.Suggestions) == 0 {
		t.Fatal("expected suggestions for a near-miss slug")
	}
	if ume.Suggestions[0] != "anthropic/claude-opus-4.6" {
		t.Errorf("Suggestions = %v", ume.Suggestions)
	}
}

func TestPlanMissingAPIKeyFails(t *testing.T) {
	svc := newTestService(t)
	t.Setenv("OPENROUTER_API_KEY", "")

	_, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &fakeLauncher{}), ModelID: "anthropic/claude-opus-4.6",
	})

	if !errors.Is(err, config.ErrNoAPIKey) {
		t.Fatalf("Plan returned %v, want config.ErrNoAPIKey", err)
	}
}

func TestPlanIncompatibleModelYieldsConfirmableWarning(t *testing.T) {
	svc := newTestService(t)

	p, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &incompatibleLauncher{}), ModelID: "qwen/qwen3-coder:free",
	})
	if err != nil {
		t.Fatalf("an advisory incompatibility must not fail the plan: %v", err)
	}

	if len(p.Warnings) != 1 {
		t.Fatalf("Warnings = %+v, want exactly one", p.Warnings)
	}
	w := p.Warnings[0]
	if w.Kind != WarnIncompatibleModel {
		t.Errorf("Kind = %v, want WarnIncompatibleModel", w.Kind)
	}
	// Without a Question the CLI would launch an incompatible pairing
	// silently, which is the behavior this warning exists to prevent.
	if w.Question == "" {
		t.Error("an incompatibility warning must carry a confirmation prompt")
	}
	if !strings.Contains(w.Message, "qwen/qwen3-coder:free") {
		t.Errorf("Message should name the model, got %q", w.Message)
	}
	// The plan is still runnable: confirming is the caller's job, not a
	// reason to withhold the command.
	if p.Command.Path == "" {
		t.Error("the plan should still carry a built command")
	}
}

// A genuine (non-ErrIncompatibleModel) CheckModel error is a hard failure,
// not something to soften into a warning and continue past. The returned
// error IS the assertion: if the error were downgraded to a warning, Plan
// would succeed and err would be nil.
//
// Do not add an `if len(p.Warnings) != 0` check here. Plan returns Plan{}
// on every error path, so such an assertion can never fail regardless of
// what the implementation does.
func TestPlanGenuineCheckModelErrorIsFatal(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &brokenCheckLauncher{}), ModelID: "anthropic/claude-opus-4.6",
	})
	if err == nil {
		t.Fatal("a non-advisory CheckModel error must fail the plan")
	}
	if !strings.Contains(err.Error(), "catalog service unreachable") {
		t.Errorf("error should propagate unchanged, got %v", err)
	}
}

func TestPlanStaleCatalogWarningPrecedesCompatibilityWarning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	path, err := openrouter.CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	writeCacheFileForTest(t, path, time.Now().Add(-48*time.Hour))

	svc := &Service{Catalog: erroringCatalog{}, Run: func(agent.Command) error { return nil }}
	p, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &incompatibleLauncher{}), ModelID: "qwen/qwen3-coder:free",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(p.Warnings) != 2 {
		t.Fatalf("Warnings = %+v, want two", p.Warnings)
	}
	// Slice order is the contract: the CLI prints these in sequence, and a
	// stale-catalog notice emitted after "Launch anyway? [y/N] " would
	// arrive once the user had already answered.
	if p.Warnings[0].Kind != WarnStaleCatalog {
		t.Errorf("Warnings[0].Kind = %v, want WarnStaleCatalog", p.Warnings[0].Kind)
	}
	if p.Warnings[1].Kind != WarnIncompatibleModel {
		t.Errorf("Warnings[1].Kind = %v, want WarnIncompatibleModel", p.Warnings[1].Kind)
	}
}

func TestPlanHappyPathBuildsCommandWithoutWarnings(t *testing.T) {
	svc := newTestService(t)

	p, err := svc.Plan(context.Background(), Request{
		Spec:      spec("fake", &fakeLauncher{}),
		ModelID:   "anthropic/claude-opus-4.6",
		ExtraArgs: []string{"--resume"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(p.Warnings) != 0 {
		t.Errorf("Warnings = %+v, want none", p.Warnings)
	}
	if p.Command.Path != "/bin/fake" {
		t.Errorf("Command.Path = %q", p.Command.Path)
	}
	// ExtraArgs and the resolved API key must reach the launcher.
	if len(p.Command.Args) != 3 || p.Command.Args[2] != "--resume" {
		t.Errorf("Command.Args = %v, want the trailing --resume", p.Command.Args)
	}
	if len(p.Command.Env) != 1 || p.Command.Env[0] != "FAKE_API_KEY=sk-or-test" {
		t.Errorf("Command.Env = %v, want the resolved API key", p.Command.Env)
	}
	if p.Model.ID != "anthropic/claude-opus-4.6" {
		t.Errorf("Model.ID = %q", p.Model.ID)
	}
	if p.Spec.Name != "fake" {
		t.Errorf("Spec.Name = %q", p.Spec.Name)
	}
}

func TestPlanPropagatesCommandBuildError(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &buildErrorLauncher{}), ModelID: "anthropic/claude-opus-4.6",
	})

	if err == nil || !strings.Contains(err.Error(), "binary vanished") {
		t.Fatalf("Plan returned %v, want the launcher's build error", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/launch/ -count=1 -run TestPlan`
Expected: FAIL — undefined: `Request`, `Plan`, `svc.Plan`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/launch/plan.go`:

```go
package launch

import (
	"context"
	"errors"
	"time"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Request is a launch request.
type Request struct {
	Spec      *agent.Spec
	ModelID   string
	ExtraArgs []string
	// Refresh bypasses the cached catalog.
	Refresh bool
}

// Plan is a resolved launch: a runnable command plus the conditions the
// caller must render and, where Warning.Question is set, get approved.
type Plan struct {
	Spec     *agent.Spec
	Model    openrouter.Model
	Command  agent.Command
	Warnings []Warning
}

// Plan resolves req into a runnable command. It performs IO - catalog fetch,
// config read - but never touches the terminal: every condition a user must
// see comes back as a Warning or a typed error.
//
// The guard order is load-bearing. It decides which of several simultaneous
// problems the user is told about first, and the empty-model check sits
// deliberately ahead of the install check so that a user with no agent
// installed still reaches the model picker in Phase 2.
//
// Confirmation is NOT performed here. The caller renders the warnings,
// obtains approval, and only then calls Launch.
func (s *Service) Plan(ctx context.Context, req Request) (Plan, error) {
	spec := req.Spec

	if err := CheckSupported(spec); err != nil {
		return Plan{}, err
	}

	if platform, ok := spec.Launcher.(agent.PlatformSupported); ok {
		if err := platform.Supported(); err != nil {
			return Plan{}, &UnsupportedPlatformError{Agent: spec.Name, Err: err}
		}
	}

	if req.ModelID == "" {
		return Plan{}, ErrNoModel
	}

	if installable, ok := spec.Launcher.(agent.Installable); ok && !installable.CheckInstalled() {
		return Plan{}, &NotInstalledError{
			Agent:       spec.Name,
			DisplayName: spec.Launcher.DisplayName(),
			Hint:        installable.InstallHint(),
		}
	}

	snap, err := s.Snapshot(ctx, req.Refresh)
	if err != nil {
		return Plan{}, err
	}

	var warnings []Warning
	if w, ok := StaleWarning(snap, time.Now()); ok {
		warnings = append(warnings, w)
	}

	model, ok := openrouter.FindByID(snap.Models, req.ModelID)
	if !ok {
		return Plan{}, &UnknownModelError{
			ModelID:     req.ModelID,
			Suggestions: openrouter.Suggest(snap.Models, req.ModelID, 5),
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return Plan{}, err
	}
	apiKey, err := config.ResolveAPIKey(cfg)
	if err != nil {
		return Plan{}, err
	}

	if compatible, ok := spec.Launcher.(agent.Compatible); ok {
		if err := compatible.CheckModel(model); err != nil {
			// Incompatibility is advisory: Claude Code works with many
			// non-Anthropic models, so this warns rather than aborts.
			// Anything else is a genuine failure.
			if !errors.Is(err, agent.ErrIncompatibleModel) {
				return Plan{}, err
			}
			warnings = append(warnings, Warning{
				Kind:     WarnIncompatibleModel,
				Message:  err.Error(),
				Question: "Launch anyway?",
			})
		}
	}

	command, err := spec.Launcher.Command(agent.Request{
		Model:     model,
		APIKey:    apiKey,
		ExtraArgs: req.ExtraArgs,
	})
	if err != nil {
		return Plan{}, err
	}

	return Plan{Spec: spec, Model: model, Command: command, Warnings: warnings}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/launch/ -count=1 -v`
Expected: PASS — 31 tests total (TestPlanGenuineCheckModelErrorIsFatal among them).

- [ ] **Step 5: Verify formatting and vet**

Run: `gofmt -l internal/launch/ && go vet ./internal/launch/`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add internal/launch/plan.go internal/launch/plan_test.go
git commit -m "feat(launch): add the pure planner

Plan runs the nine guards and returns a built command plus warnings as
values. Confirmation moves out to the caller so a bubbletea program can
render it without a mid-frame write to stderr.

Tests pin the guard ordering by violating each guard alongside a later
one, so reordering fails rather than quietly changing the message."
```

---

### Task 5: The handoff

**Files:**
- Create: `internal/launch/handoff.go`
- Test: `internal/launch/handoff_test.go`

**Interfaces:**
- Consumes: `Plan`, `Warning`, `WarnSelectionNotSaved`, `(*Service).run`; `config.Load`, `config.Save`.
- Produces: `(*Service).Launch(Plan, func(Warning)) error`. Task 7 uses it.

- [ ] **Step 1: Write the failing test**

Create `internal/launch/handoff_test.go`:

```go
package launch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// testPlan is a minimal already-resolved Plan.
func testPlan() Plan {
	return Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Model:   openrouter.Model{ID: "anthropic/claude-opus-4.6"},
		Command: agent.Command{Path: "/bin/fake", Args: []string{"--model", "x"}, Env: []string{"K=V"}},
	}
}

// blockConfigWrites points XDG_CONFIG_HOME at a regular file, so the config
// directory cannot be created and both Load and Save fail.
func blockConfigWrites(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocker)
}

// The ordering here is the whole reason save and handoff live in one
// function. On Unix the handoff is syscall.Exec, which replaces the process:
// a save placed after it would never run, and the stub used here returns
// normally, so the bug would be invisible to any end-state assertion. This
// inspects the config from inside the handoff itself.
func TestLaunchSavesSelectionBeforeHandoff(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var savedBeforeHandoff bool
	svc := &Service{Run: func(agent.Command) error {
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("config.Load inside the handoff: %v", err)
		}
		savedBeforeHandoff = cfg.LastAgent == "fake" &&
			cfg.LastModel == "anthropic/claude-opus-4.6"
		return nil
	}}

	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !savedBeforeHandoff {
		t.Error("the selection must be persisted before control reaches the handoff")
	}
}

func TestLaunchRecordsAgentAndModel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	svc := &Service{Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.LastAgent != "fake" {
		t.Errorf("LastAgent = %q", cfg.LastAgent)
	}
	if cfg.LastModel != "anthropic/claude-opus-4.6" {
		t.Errorf("LastModel = %q", cfg.LastModel)
	}
}

func TestLaunchWarnsButProceedsWhenSelectionCannotBeSaved(t *testing.T) {
	blockConfigWrites(t)

	var handedOff bool
	var got []Warning
	svc := &Service{Run: func(agent.Command) error { handedOff = true; return nil }}

	err := svc.Launch(testPlan(), func(w Warning) { got = append(got, w) })
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	// Failing to remember the last selection is a convenience loss, not a
	// reason to refuse to start the agent.
	if !handedOff {
		t.Error("a failed save must not block the launch")
	}
	if len(got) != 1 {
		t.Fatalf("warnings = %+v, want exactly one", got)
	}
	if got[0].Kind != WarnSelectionNotSaved {
		t.Errorf("Kind = %v, want WarnSelectionNotSaved", got[0].Kind)
	}
	if got[0].Question != "" {
		t.Error("an unsaved selection is informational; it must not gate the launch on an answer")
	}
}

// warn is documented as optional. The crash case is a save failure with no
// callback to report it to, so that is what this exercises.
func TestLaunchNilWarnIsSafeOnSaveFailure(t *testing.T) {
	blockConfigWrites(t)

	svc := &Service{Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
}

func TestLaunchDoesNotWarnOnSuccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var got []Warning
	svc := &Service{Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), func(w Warning) { got = append(got, w) }); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("warnings = %+v, want none on a successful save", got)
	}
}

func TestLaunchHandsOffTheBuiltCommandUnchanged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var got agent.Command
	svc := &Service{Run: func(c agent.Command) error { got = c; return nil }}
	p := testPlan()

	if err := svc.Launch(p, nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if got.Path != p.Command.Path {
		t.Errorf("Path = %q, want %q", got.Path, p.Command.Path)
	}
	if len(got.Args) != len(p.Command.Args) {
		t.Errorf("Args = %v, want %v", got.Args, p.Command.Args)
	}
	if len(got.Env) != len(p.Command.Env) {
		t.Errorf("Env = %v, want %v", got.Env, p.Command.Env)
	}
}

func TestLaunchPropagatesHandoffError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := errors.New("exec failed")
	svc := &Service{Run: func(agent.Command) error { return want }}

	if err := svc.Launch(testPlan(), nil); !errors.Is(err, want) {
		t.Fatalf("Launch returned %v, want %v", err, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/launch/ -count=1 -run TestLaunch`
Expected: FAIL — `svc.Launch undefined`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/launch/handoff.go`:

```go
package launch

import (
	"github.com/teggen/openrouter-launch/internal/config"
)

// Launch records the selection and then hands off to the agent.
//
// The order is load-bearing: on Unix the handoff is syscall.Exec, which
// replaces the process, so nothing after it runs. Recording afterwards would
// mean never recording at all. Save and handoff live in one function so that
// no call site can get the order wrong.
//
// warn is called synchronously for any non-fatal problem encountered before
// the handoff. It must not block, and may be nil. It cannot be a return
// value for the same reason the ordering matters: on Unix, Launch does not
// return on success, so a returned warning would never be seen.
//
// A config that cannot be read or written costs the user their remembered
// last selection. That is a convenience, not a precondition, so it warns
// rather than refusing to start the agent.
func (s *Service) Launch(p Plan, warn func(Warning)) error {
	if err := recordSelection(p); err != nil && warn != nil {
		warn(Warning{
			Kind:    WarnSelectionNotSaved,
			Message: "could not save last selection: " + err.Error(),
		})
	}
	return s.run(p.Command)
}

// recordSelection persists the agent and model for the next run. The config
// is re-read rather than threaded through from Plan: in a TUI a profile may
// have been added between planning and launching, and that edit must not be
// clobbered.
func recordSelection(p Plan) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.LastAgent = p.Spec.Name
	cfg.LastModel = p.Model.ID
	return config.Save(cfg)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/launch/ -count=1 -v`
Expected: PASS — 38 tests total.

- [ ] **Step 5: Verify the whole tree is still green**

Run: `go test ./... -count=1 && go vet ./... && gofmt -l .`
Expected: all packages `ok`, no vet or gofmt output. `internal/cli` is untouched so far and must still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/launch/handoff.go internal/launch/handoff_test.go
git commit -m "feat(launch): add Launch, recording the selection before handoff

The save-failure warning goes through a callback rather than a return
value: on Unix the handoff never returns, so a returned warning would
never reach the user."
```

---

### Task 6: Wire the service into the CLI

Introduces `app` and `NewRootCmdWith`, points `loadCatalog` at `svc.Snapshot`, and deletes the `catalogSource` global. `resolveAndRun` keeps its own guard sequence for now, and the `runner` global survives until Task 7 — this task is only about the plumbing.

**Files:**
- Modify: `internal/cli/root.go`, `internal/cli/catalog.go`, `internal/cli/models.go`, `internal/cli/profile.go`, `internal/cli/launch.go`
- Create: `internal/cli/harness_test.go`
- Modify: `internal/cli/catalog_test.go`, `internal/cli/models_test.go`, `internal/cli/agents_test.go`, `internal/cli/launch_test.go`, `internal/cli/profile_test.go`

**Interfaces:**
- Consumes: `launch.Service`, `(*launch.Service).Snapshot`, `launch.StaleWarning` from Task 2.
- Produces: `app{svc, flags}`, `NewRootCmdWith(*launch.Service) *cobra.Command`, `loadCatalog(ctx, *launch.Service, bool, io.Writer)`, and the test helpers `harness`, `newHarness(t)`, `(*harness).root(*bytes.Buffer)`, `(*harness).run(t, ...string) string`, `(*harness).exec(...string) (string, error)`. Tasks 7 and 8 use the harness.

- [ ] **Step 1: Rewrite root.go**

Replace `internal/cli/root.go` in full:

```go
// Package cli wires the openrouter-launch commands together.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/launch"
)

// globalFlags holds values shared by every subcommand.
type globalFlags struct {
	refresh bool
	yes     bool
}

// app is what every subcommand needs: the shared launch service and the
// global flag values.
type app struct {
	svc   *launch.Service
	flags *globalFlags
}

// NewRootCmd builds the command tree against the live OpenRouter API.
func NewRootCmd() *cobra.Command {
	return NewRootCmdWith(&launch.Service{})
}

// NewRootCmdWith builds the command tree against the given service. It is a
// constructor rather than a package-level variable so tests get an isolated
// tree per run, and it takes the service as an argument rather than reading
// a package global so that a Phase 2 TUI can share the same instance.
func NewRootCmdWith(svc *launch.Service) *cobra.Command {
	a := &app{svc: svc, flags: &globalFlags{}}

	root := &cobra.Command{
		Use:   "openrouter-launch",
		Short: "Launch coding agents against OpenRouter models",
		Long: "openrouter-launch picks an OpenRouter model and starts a coding " +
			"agent configured to use it, without modifying the agent's own configuration.",
		SilenceUsage: true,
	}

	root.PersistentFlags().BoolVar(&a.flags.refresh, "refresh", false,
		"bypass the cached model catalog and fetch a fresh copy")
	root.PersistentFlags().BoolVarP(&a.flags.yes, "yes", "y", false,
		"skip confirmation prompts")

	root.AddCommand(newAgentsCmd())
	root.AddCommand(newModelsCmd(a))
	root.AddCommand(newProfileCmd(a))
	for _, cmd := range newLaunchCmds(a) {
		root.AddCommand(cmd)
	}

	return root
}

// Execute runs the CLI, returning a non-nil error on failure. Cobra has
// already printed the error, so main only needs the exit code.
func Execute() error {
	return NewRootCmd().Execute()
}
```

- [ ] **Step 2: Point loadCatalog at the service**

Replace `internal/cli/catalog.go` in full:

```go
package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// loadCatalog returns the model catalog, writing to warnings when stale data
// is served because a refresh failed. Callers pass cmd.ErrOrStderr() so the
// warning honors cobra's IO redirection like every other CLI diagnostic.
//
// This is a shim: Task 7 removes it once resolveAndRun calls the planner,
// which returns the same warning as a value.
func loadCatalog(ctx context.Context, svc *launch.Service, refresh bool, warnings io.Writer) (openrouter.Snapshot, error) {
	snap, err := svc.Snapshot(ctx, refresh)
	if err != nil {
		return openrouter.Snapshot{}, err
	}
	if w, ok := launch.StaleWarning(snap, time.Now()); ok {
		fmt.Fprintf(warnings, "warning: %s\n", w.Message)
	}
	return snap, nil
}
```

- [ ] **Step 3: Thread `*app` through the command constructors**

In `internal/cli/models.go`, change the signature and the `loadCatalog` call:

```go
func newModelsCmd(a *app) *cobra.Command {
```

```go
			snap, err := loadCatalog(cmd.Context(), a.svc, a.flags.refresh, cmd.ErrOrStderr())
```

In `internal/cli/profile.go`, change three signatures:

```go
func newProfileCmd(a *app) *cobra.Command {
```

```go
	cmd.AddCommand(
		newProfileListCmd(),
		newProfileAddCmd(),
		newProfileLaunchCmd(a),
		newProfileRemoveCmd(),
		newProfileRenameCmd(),
	)
```

```go
func newProfileLaunchCmd(a *app) *cobra.Command {
```

and its `resolveAndRun` call:

```go
			return resolveAndRun(cmd, a, spec, profile.Model, profile.Args)
```

In `internal/cli/launch.go`, change `newLaunchCmds` and `resolveAndRun` to take `*app` instead of `*globalFlags`, replacing every `global.refresh` with `a.flags.refresh`, every `global.yes` via `confirm(cmd, a.flags, ...)`, and the `loadCatalog` call with the four-argument form:

```go
func newLaunchCmds(a *app) []*cobra.Command {
```

```go
			RunE: func(cmd *cobra.Command, args []string) error {
				return resolveAndRun(cmd, a, spec, modelID, args)
			},
```

```go
func resolveAndRun(cmd *cobra.Command, a *app, spec *agent.Spec, modelID string, extraArgs []string) error {
```

```go
	snap, err := loadCatalog(cmd.Context(), a.svc, a.flags.refresh, cmd.ErrOrStderr())
```

```go
			ok, cerr := confirm(cmd, a.flags, "Launch anyway?")
```

`confirm` keeps its `*globalFlags` parameter — it only needs the `yes` flag.

- [ ] **Step 4: Create the test harness**

Create `internal/cli/harness_test.go`:

```go
package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/launch"
)

// harness builds a CLI wired to an in-memory catalog, with config and cache
// isolated to a temp dir. It replaces the mutable package globals the CLI
// used to carry for this purpose.
type harness struct {
	svc *launch.Service
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Run stays nil until Task 7: resolveAndRun still calls the package
	// runner, which launch tests stub with captureRun.
	return &harness{svc: &launch.Service{Catalog: &fakeCatalog{models: fakeModels()}}}
}

// root returns a fresh command tree with both streams writing into out.
func (h *harness) root(out *bytes.Buffer) *cobra.Command {
	root := NewRootCmdWith(h.svc)
	root.SetOut(out)
	root.SetErr(out)
	return root
}

// run executes args, failing the test on error, and returns the output.
func (h *harness) run(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := h.root(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return out.String()
}

// exec executes args and returns the output and the error, for tests that
// assert on failure. Note the output is read AFTER Execute returns.
func (h *harness) exec(args ...string) (string, error) {
	var out bytes.Buffer
	root := h.root(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}
```

- [ ] **Step 5: Convert the existing tests to the harness**

In `internal/cli/agents_test.go`, delete `runCmd` (the harness replaces it) and update its two call sites in that file to use a harness. `newAgentsCmd` takes no `*app`, so a harness is still the cheapest way to get an isolated tree:

```go
func TestAgentsCommandListsClaude(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "agents")
	// ... existing assertions unchanged
}
```

In `internal/cli/models_test.go`, delete `useFakeCatalog` and replace every `useFakeCatalog(t)` + `runCmd(t, ...)` pair:

```go
func TestModelsCommandListsAll(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models")
	// ... existing assertions unchanged
}
```

Keep `fakeCatalog`, `fakeModels`, and `mustLoadConfig` where they are — the harness uses the first two.

In `internal/cli/catalog_test.go`, replace both tests' global-patching with a service, and drop `erroringCatalog`'s duplicate if it now conflicts:

```go
func TestLoadCatalogWarnsOnProvidedWriterWhenStale(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now().Add(-48*time.Hour))

	svc := &launch.Service{Catalog: erroringCatalog{}}

	var warnings bytes.Buffer
	snap, err := loadCatalog(context.Background(), svc, false, &warnings)
	if err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	if !snap.Stale {
		t.Fatal("expected a stale snapshot")
	}
	if !strings.Contains(warnings.String(), "warning:") {
		t.Errorf("warning text = %q, want it to mention the stale refresh", warnings.String())
	}
}

func TestLoadCatalogWritesNothingWhenFresh(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now())

	// erroringCatalog must not even be consulted for a fresh cache.
	svc := &launch.Service{Catalog: erroringCatalog{}}

	var warnings bytes.Buffer
	snap, err := loadCatalog(context.Background(), svc, false, &warnings)
	if err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	if snap.Stale {
		t.Fatal("fresh cache reported as stale")
	}
	if warnings.Len() != 0 {
		t.Errorf("expected no warning written for fresh data, got %q", warnings.String())
	}
}
```

In `internal/cli/launch_test.go` and `internal/cli/profile_test.go`, replace `setupLaunch`/`useFakeCatalog` + `NewRootCmd()` with the harness. `setupLaunch` becomes:

```go
func setupLaunch(t *testing.T) (*harness, *agent.Command) {
	t.Helper()
	h := newHarness(t)
	stubClaudePath(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	return h, captureRun(t)
}
```

and every inline `root := NewRootCmd()` becomes `root := h.root(&out)`. Tests calling `resolveAndRun` directly now pass `&app{svc: h.svc, flags: &globalFlags{}}` instead of `&globalFlags{}`:

```go
	err := resolveAndRun(root, &app{svc: h.svc, flags: &globalFlags{}}, spec, "anthropic/claude-opus-4.6", nil)
```

- [ ] **Step 6: Delete the catalogSource global**

Confirm it is gone:

Run: `grep -rn "catalogSource" internal/`
Expected: no output.

- [ ] **Step 7: Run the full suite**

Run: `go test ./... -count=1`
Expected: all packages `ok`. Behavior is unchanged in this task, so every existing assertion should still hold.

- [ ] **Step 8: Verify formatting and vet**

Run: `go vet ./... && gofmt -l .`
Expected: no output.

- [ ] **Step 9: Commit**

```bash
git add internal/cli/
git commit -m "refactor(cli): thread a launch.Service through the command tree

Adds app and NewRootCmdWith, points loadCatalog at svc.Snapshot, and
deletes the catalogSource global in favor of a test harness that injects
a fake catalog. No behavior change."
```

---

### Task 7: Replace resolveAndRun's body with the planner

**Files:**
- Modify: `internal/cli/launch.go`, `internal/cli/profile.go`, `internal/cli/harness_test.go`, `internal/cli/launch_test.go`
- Delete: `internal/cli/catalog.go`, `internal/cli/catalog_test.go`

**Interfaces:**
- Consumes: `(*launch.Service).Plan`, `(*launch.Service).Launch`, `launch.Request`, `launch.ErrNoModel`, `launch.CheckSupported`, `launch.Warning`.
- Produces: nothing new. `checkAgentSupported`, `runner`, and `loadCatalog` cease to exist.

- [ ] **Step 1: Update the harness to capture the handoff**

In `internal/cli/harness_test.go`, add the `ran` field and set `Run`:

```go
type harness struct {
	svc *launch.Service
	// ran is the command the handoff would have executed.
	ran agent.Command
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	h := &harness{}
	h.svc = &launch.Service{
		Catalog: &fakeCatalog{models: fakeModels()},
		Run: func(c agent.Command) error {
			h.ran = c
			return nil
		},
	}
	return h
}
```

Add `"github.com/teggen/openrouter-launch/internal/agent"` to the imports.

- [ ] **Step 2: Rewrite resolveAndRun**

Replace `resolveAndRun` and delete `checkAgentSupported` and `runner` in `internal/cli/launch.go`. The file becomes:

```go
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/launch"
)

// newLaunchCmds builds one subcommand per registered agent.
func newLaunchCmds(a *app) []*cobra.Command {
	specs := agent.List()
	cmds := make([]*cobra.Command, 0, len(specs))

	for _, spec := range specs {
		spec := spec
		var modelID string

		cmd := &cobra.Command{
			Use:     spec.Name,
			Short:   "Launch " + spec.Launcher.DisplayName(),
			Aliases: spec.Aliases,
			Args:    cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return resolveAndRun(cmd, a, spec, modelID, args)
			},
		}
		cmd.Flags().StringVarP(&modelID, "model", "m", "", "OpenRouter model slug (required)")
		cmds = append(cmds, cmd)
	}
	return cmds
}

// resolveAndRun plans the launch, renders whatever the plan reports, and
// hands off. All of the decision-making lives in launch.Service; this
// function is the cobra-flavored rendering of it.
func resolveAndRun(cmd *cobra.Command, a *app, spec *agent.Spec, modelID string, extraArgs []string) error {
	plan, err := a.svc.Plan(cmd.Context(), launch.Request{
		Spec:      spec,
		ModelID:   modelID,
		ExtraArgs: extraArgs,
		Refresh:   a.flags.refresh,
	})
	if errors.Is(err, launch.ErrNoModel) {
		// Phase 2 replaces this branch with the interactive picker. The
		// planner reports the bare condition; naming a CLI flag is this
		// layer's job.
		return fmt.Errorf("a model is required: pass --model <slug> (run %q to browse; "+
			"the interactive picker arrives in Phase 2)", "openrouter-launch models")
	}
	if err != nil {
		return err
	}

	for _, w := range plan.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
		if w.Question == "" {
			continue
		}
		ok, cerr := confirm(cmd, a.flags, w.Question)
		if cerr != nil {
			return cerr
		}
		if !ok {
			return errors.New("cancelled")
		}
	}

	if err := a.svc.Launch(plan, func(w launch.Warning) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
	}); err != nil {
		if isAgentExitError(err) {
			// On Windows, agent.Run waits for the child instead of replacing
			// the process (exec_windows.go), so a nonzero exit reaches here
			// as an error wrapping *exec.ExitError. The agent already
			// inherited stderr and reported its own failure, so cobra's
			// default "Error: ..." line would just be redundant noise on
			// top; main still receives the real error to extract the exit
			// code from (see exitCode in main.go).
			cmd.SilenceErrors = true
		}
		return err
	}
	return nil
}

// isAgentExitError reports whether err carries the launched agent's own
// exit code, i.e. it wraps a value with an ExitCode() int method (the
// structural shape of *exec.ExitError). On Unix, agent.Run only returns an
// error when syscall.Exec itself fails to start the process, so this is
// always false there.
func isAgentExitError(err error) bool {
	var ec interface{ ExitCode() int }
	return errors.As(err, &ec)
}

// confirm asks a yes/no question, defaulting to no. --yes answers yes.
func confirm(cmd *cobra.Command, global *globalFlags, question string) (bool, error) {
	if global.yes {
		return true, nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", question)
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}

	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
```

- [ ] **Step 3: Point profile add at launch.CheckSupported**

In `internal/cli/profile.go`, replace the `checkAgentSupported(spec)` call inside `newProfileAddCmd`:

```go
			if err := launch.CheckSupported(spec); err != nil {
				return err
			}
```

and add `"github.com/teggen/openrouter-launch/internal/launch"` to that file's imports.

- [ ] **Step 4: Delete the catalog shim**

```bash
rm internal/cli/catalog.go internal/cli/catalog_test.go
```

`loadCatalog`'s only remaining caller is `newModelsCmd`, which Task 8 converts. Until then, `models.go` must call `a.svc.Snapshot` directly. Replace the `loadCatalog` call in `internal/cli/models.go`:

```go
			snap, err := a.svc.Snapshot(cmd.Context(), a.flags.refresh)
			if err != nil {
				return err
			}
			if w, ok := launch.StaleWarning(snap, time.Now()); ok {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
			}
```

adding `"time"` and the `launch` import to `models.go`.

- [ ] **Step 5: Move the cli tests that no longer belong**

In `internal/cli/launch_test.go`:

- Delete `TestCheckAgentSupported` — `TestCheckSupportedRejectsUnsupportedAgent` in `internal/launch` covers it.
- Delete `TestLaunchSavesSelectionBeforeHandoff` — `internal/launch/handoff_test.go` covers it, and it can no longer observe the ordering from here.
- Delete `TestResolveAndRunUnsupportedAgent`, `TestResolveAndRunUnsupportedPlatform`, and `TestResolveAndRunPropagatesGenuineCheckModelError`, along with `fakeLauncher`, `fakePlatformLauncher`, and `fakeIncompatibleLauncher` — all three guards are now tested in `internal/launch/plan_test.go` against the same synthetic launchers, and `resolveAndRun` no longer contains the logic.
- Delete `captureRun`; `setupLaunch` returns the harness, whose `ran` field replaces it:

```go
func setupLaunch(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	stubClaudePath(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	return h
}
```

- Update the surviving tests to read `h.ran` instead of the captured pointer. For example:

```go
func TestLaunchBuildsCommand(t *testing.T) {
	h := setupLaunch(t)
	h.run(t, "claude", "-m", "anthropic/claude-opus-4.6")

	if h.ran.Path != "/usr/local/bin/claude" {
		t.Errorf("Path = %q", h.ran.Path)
	}
	if len(h.ran.Args) < 2 || h.ran.Args[0] != "--model" ||
		h.ran.Args[1] != "anthropic/claude-opus-4.6" {
		t.Errorf("Args = %v", h.ran.Args)
	}

	var foundKey bool
	for _, e := range h.ran.Env {
		if e == "ANTHROPIC_API_KEY=sk-or-test" {
			foundKey = true
		}
	}
	if !foundKey {
		t.Errorf("API key not passed through: %v", h.ran.Env)
	}
}
```

- `TestLaunchAgentExitErrorSuppressesCobraErrorLine` overrides the handoff to return an exit error, so it sets `h.svc.Run` directly instead of patching the deleted global:

```go
func TestLaunchAgentExitErrorSuppressesCobraErrorLine(t *testing.T) {
	h := newHarness(t)
	stubClaudePath(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	h.svc.Run = func(agent.Command) error {
		return fmt.Errorf("run claude: %w", fakeExitCoder{code: 3})
	}

	out, err := h.exec("claude", "-m", "anthropic/claude-opus-4.6")
	if err == nil {
		t.Fatal("expected the agent's exit error to propagate to main")
	}
	if strings.Contains(out, "Error:") {
		t.Errorf("cobra's own error line should be suppressed for an agent exit code, got: %q", out)
	}
}
```

- Keep `TestLaunchRequiresModelFlag`, `TestLaunchUnknownModelSuggests`, `TestLaunchIncompatibleModel*`, `TestConfirm*`, `TestLaunchOtherErrorsStillPrintCobraErrorLine`, `TestLaunchMissingAPIKeyFails`, and `TestLaunchRecordsLastSelection` — these assert CLI-level rendering, which is exactly what stays here. Convert each to `h.run`/`h.exec`.

- [ ] **Step 6: Confirm the globals are gone**

Run: `grep -rn "runner\|catalogSource\|checkAgentSupported\|loadCatalog" internal/cli/`
Expected: no output.

- [ ] **Step 7: Run the full suite**

Run: `go test ./... -count=1`
Expected: all packages `ok`.

- [ ] **Step 8: Verify formatting and vet**

Run: `go vet ./... && gofmt -l .`
Expected: no output.

- [ ] **Step 9: Commit**

```bash
git add -A internal/cli/
git commit -m "refactor(cli): drive launches through launch.Service

resolveAndRun becomes rendering only: it plans, prints warnings,
confirms the ones carrying a Question, and hands off. The runner global,
checkAgentSupported, and the loadCatalog shim are gone.

Guard-sequence tests move to internal/launch, where the logic now is."
```

---

### Task 8: models honors persisted filters

**Files:**
- Modify: `internal/cli/models.go`, `internal/cli/models_test.go`

**Interfaces:**
- Consumes: `launch.MergeFilters`, `launch.FlagTools`, `launch.FlagFree`, `launch.FlagMinContext`, `launch.FlagMaxPrice` from Task 3; `config.Load`.
- Produces: nothing new.

- [ ] **Step 1: Write the failing tests**

Add to `internal/cli/models_test.go`:

```go
func TestModelsCommandDefaultsToToolCapableModels(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models")

	// config.defaults() sets ToolsOnly:true because a coding agent without
	// tool calling is unusable, and openai/o1-mini is the only fixture model
	// without tool support.
	if strings.Contains(got, "openai/o1-mini") {
		t.Errorf("bare `models` should honor the saved tools-only default:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("tool-capable models should still be listed:\n%s", got)
	}
}

func TestModelsCommandExplicitToolsFalseOverridesSavedDefault(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--tools=false")

	// --tools=false and an absent --tools are both false by value; only the
	// Changed() check distinguishes them. If that check is dropped, the
	// persisted true wins and o1-mini stays hidden.
	if !strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--tools=false should override the saved ToolsOnly:true:\n%s", got)
	}
}
```

Update the four tests that assume no persisted filter. Each gains `--tools=false` so it keeps testing its own dimension:

```go
func TestModelsCommandListsAll(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--tools=false")

	for _, id := range []string{"anthropic/claude-opus-4.6", "qwen/qwen3-coder:free", "openai/o1-mini"} {
		if !strings.Contains(got, id) {
			t.Errorf("output missing %s:\n%s", id, got)
		}
	}
}

func TestModelsCommandProviderFilter(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--provider", "openai", "--tools=false")

	if !strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--provider dropped the match:\n%s", got)
	}
	if strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--provider should exclude other vendors:\n%s", got)
	}
	if strings.Contains(got, "qwen/qwen3-coder:free") {
		t.Errorf("--provider should exclude qwen models:\n%s", got)
	}
}

// --tools=false matters here even though the assertion is about absence:
// without it, o1-mini would be filtered out by the tools default and this
// test would pass whether or not the min-context guard works.
func TestModelsCommandMinContextFilter(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--min-context", "200000", "--tools=false")

	if strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--min-context should exclude the 128k model:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--min-context should include exactly 200k model:\n%s", got)
	}
	if !strings.Contains(got, "qwen/qwen3-coder:free") {
		t.Errorf("--min-context should include 262k model:\n%s", got)
	}
}

func TestModelsCommandMaxPriceFilter(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--max-price", "5", "--tools=false")

	if strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--max-price should exclude the $75 model:\n%s", got)
	}
	if !strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--max-price dropped a cheap model:\n%s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify the new ones fail**

Run: `go test ./internal/cli/ -count=1 -run TestModelsCommandDefaultsToToolCapableModels`
Expected: FAIL — `bare "models" should honor the saved tools-only default`, because `newModelsCmd` still uses a zero-valued `Filter`.

- [ ] **Step 3: Wire MergeFilters into newModelsCmd**

In `internal/cli/models.go`, rename the local `filter` to `flagFilter`, read the config, and merge. The `RunE` body becomes:

```go
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// Persisted filters are the baseline; a flag the user actually
			// typed wins. Changed() is what makes an explicit --tools=false
			// distinguishable from an absent --tools.
			filter := launch.MergeFilters(cfg.Filters, flagFilter, cmd.Flags().Changed)
			if len(args) == 1 {
				filter.Search = args[0]
			}

			snap, err := a.svc.Snapshot(cmd.Context(), a.flags.refresh)
			if err != nil {
				return err
			}
			if w, ok := launch.StaleWarning(snap, time.Now()); ok {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
			}

			models := openrouter.Apply(snap.Models, filter)
			if len(models) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No models match those filters.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "MODEL\tCONTEXT\tPROMPT/M\tCOMPLETION/M\tTOOLS")
			for _, m := range models {
				tools := ""
				if m.SupportsTools {
					tools = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					m.ID, formatContext(m.ContextLength),
					formatPrice(m.PromptPricePerM, m.PricingUnknown),
					formatPrice(m.CompletionPricePerM, m.PricingUnknown), tools)
			}
			return w.Flush()
		},
```

and the flag registrations use the shared names:

```go
	cmd.Flags().BoolVar(&flagFilter.ToolsOnly, launch.FlagTools, false,
		"only models supporting tool calling (defaults to the saved filter)")
	cmd.Flags().BoolVar(&flagFilter.FreeOnly, launch.FlagFree, false, "only free models")
	cmd.Flags().StringVar(&flagFilter.Provider, "provider", "",
		"only models from this provider (e.g. anthropic)")
	cmd.Flags().IntVar(&flagFilter.MinContext, launch.FlagMinContext, 0,
		"minimum context window in tokens")
	cmd.Flags().Float64Var(&flagFilter.MaxPrice, launch.FlagMaxPrice, 0,
		"maximum USD per million completion tokens")
```

Add `"github.com/teggen/openrouter-launch/internal/config"` to the imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -count=1 -v -run TestModels`
Expected: PASS — 8 models tests.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... -count=1`
Expected: all packages `ok`.

- [ ] **Step 6: Verify formatting and vet**

Run: `go vet ./... && gofmt -l .`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/models.go internal/cli/models_test.go
git commit -m "feat(cli): models honors persisted filters, flags override

Bare \`models\` is now tools-only, matching config.defaults(). Four
existing tests gain --tools=false: three would otherwise fail, and
min-context would have kept passing on the tools filter rather than on
the behavior it names.

The CLI reads filters and never writes them; the Phase 2 TUI is the only
writer."
```

---

### Task 9: Whole-tree verification and handoff update

**Files:**
- Modify: `HANDOFF.md`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing.

- [ ] **Step 1: Verify the zero-touch guarantee**

Landmine 6 says the only two write sites in the tree are the catalog cache and the config file. The new package must not have added a third.

Run:
```bash
grep -rn "os.WriteFile\|os.Create\|os.OpenFile\|os.MkdirAll\|os.Rename" \
  --include='*.go' . | grep -v '_test.go'
```
Expected: hits only in `internal/openrouter/cache.go` (catalog cache) and `internal/config/config.go` (config save). Any hit in `internal/launch/` or `internal/cli/` is a Critical defect — stop and report it.

- [ ] **Step 2: Verify no agent config is touched**

Run:
```bash
grep -rni "\.claude\|\.codex\|settings.json\|config.toml" --include='*.go' internal/launch/
```
Expected: no output. (`internal/agent/claude.go` legitimately references `~/.claude/local` for *binary discovery* — that is a read, and it is not in this package.)

- [ ] **Step 3: Run the full verification suite**

Run:
```bash
go test ./... -count=1
go vet ./... && gofmt -l .
GOOS=windows go build ./... && GOOS=darwin go build ./...
```
Expected: all packages `ok`; no vet, gofmt, or build output.

- [ ] **Step 4: Smoke-test the built binary**

Run:
```bash
go build -o /tmp/orl . && /tmp/orl agents
/tmp/orl models --tools=false | head -5
/tmp/orl models | head -5
```
Expected: `agents` lists claude. Both `models` forms list models against the live catalog; the `--tools=false` listing is a superset of the bare one.

- [ ] **Step 5: Confirm go.mod is unchanged**

Run: `git diff --stat main -- go.mod go.sum`
Expected: no output. No dependency was added.

- [ ] **Step 6: Update HANDOFF.md**

Replace the "Phase 2 — the prerequisite refactor" section with a record of what was done, and update the "Where things are" tree to include `internal/launch/`. The section becomes:

```markdown
## Phase 2 — the TUI

The prerequisite refactor is **done**. `internal/launch` now owns the launch
sequence and never touches the terminal:

- `launch.Service{Catalog, Run}` carries both seams as nilable fields. The
  `cli.runner` and `cli.catalogSource` globals are gone; `NewRootCmdWith`
  takes the service, and the TUI will take the same instance.
- `Service.Plan` runs the nine guards and returns a built `agent.Command`
  plus `[]Warning`. A `Warning` with a non-empty `Question` is one the
  caller must confirm.
- Hard stops are typed errors carrying their data: `NotInstalledError.Hint`,
  `UnknownModelError.Suggestions`, `UnsupportedAgentError.Reason`.
- `Service.Launch` records the selection and hands off in one function, so
  Landmine 5's ordering cannot be inverted by a call site. Its `warn`
  callback exists because on Unix nothing after the handoff runs.
- `launch.MergeFilters` bridges `config.Filters` and `openrouter.Filter`.
  `models` reads persisted filters; only the TUI will write them.

Still to do for the TUI itself:

1. Root gains a `RunE` for bare invocation.
2. The `launch.ErrNoModel` branch in `resolveAndRun` becomes "open the
   picker" instead of an error.
3. `internal/tui` imports `internal/launch`. It must never import
   `internal/cli` — `cli` imports `tui`, so that would be a cycle. This is
   why the planner is its own package.
4. Reconsider the shared mutable `&Claude{}` in the registry if a background
   refresh goroutine ever races the `LookPath` field tests patch.
```

Also update the "Current state" table row for Phase 2, and add `internal/launch/` to the directory listing with the note "the terminal-free planner: guards, warnings, typed conditions".

- [ ] **Step 7: Commit**

```bash
git add HANDOFF.md
git commit -m "docs: record the completed Phase 2 planner refactor

Documents the new internal/launch boundary and what the TUI still needs,
including why tui must never import cli."
```

---

## Self-Review

**Spec coverage.** Every spec section maps to a task:

| Spec section | Task |
|---|---|
| Why a new package | 1 (package created) |
| Service | 2 |
| Warnings | 2 |
| Typed errors + `CheckSupported` | 1, and 7 for the `profile add` call site |
| Methods (`Snapshot`, `Plan`, `Launch`, `StaleWarning`) | 2, 4, 5 |
| Guard sequence + documented deviation | 4 |
| Filters bridge | 3, wired in 8 |
| Call sites (`app`, `NewRootCmdWith`, `resolveAndRun`) | 6, 7 |
| Behavior change 1 (tools-only default) | 8 |
| Behavior change 2 (build error without prompt) | 4, inherent in `Plan` |
| Testing (guard ordering, payloads, merge table, save-before-handoff) | 1–5, 8 |
| Landmine 6 re-verification | 9 |
| Deferred items | 9 (recorded in HANDOFF) |

**Type consistency.** `Service.Catalog`/`Service.Run` (Task 2) are the fields set in Tasks 6 and 7. `Warning{Kind, Message, Question}` (Task 2) is destructured identically in Tasks 4, 5, and 7. `Request{Spec, ModelID, ExtraArgs, Refresh}` (Task 4) matches the literal in Task 7. `Plan{Spec, Model, Command, Warnings}` (Task 4) is what Task 5's `recordSelection` reads and Task 7 iterates. `MergeFilters(config.Filters, openrouter.Filter, func(string) bool)` (Task 3) matches the Task 8 call. The `harness` gains its `ran` field in Task 7, after Task 6 introduces the type without it — Task 7 Step 1 restates the whole struct so it cannot drift.

**Known ordering hazard.** Task 6 leaves `harness.svc.Run` nil while `resolveAndRun` still uses the `runner` global; launch tests keep `captureRun` for one task. Task 7 Step 1 sets `Run` and Step 5 deletes `captureRun`. A `Service` with a nil `Run` falls back to the real `agent.Run`, so if Task 7 is skipped or half-applied, a CLI test could spawn a real process. Do not stop between Tasks 6 and 7.
