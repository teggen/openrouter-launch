package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// stubClaudePath makes the registry's Claude launcher resolve without a real
// binary on this machine.
func stubClaudePath(t *testing.T) {
	t.Helper()
	spec, err := agent.Lookup("claude")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	claude, ok := spec.Launcher.(*agent.Claude)
	if !ok {
		t.Fatalf("claude launcher has unexpected type %T", spec.Launcher)
	}
	prev := claude.LookPath
	claude.LookPath = func(string) (string, error) { return "/usr/local/bin/claude", nil }
	t.Cleanup(func() { claude.LookPath = prev })
}

func setupLaunch(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	stubClaudePath(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	return h
}

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

func TestLaunchPassesExtraArgsAfterDoubleDash(t *testing.T) {
	h := setupLaunch(t)
	h.run(t, "claude", "-m", "anthropic/claude-opus-4.6", "--", "--resume")

	if len(h.ran.Args) != 3 || h.ran.Args[2] != "--resume" {
		t.Errorf("Args = %v, want the trailing --resume", h.ran.Args)
	}
}

func TestLaunchRecordsLastSelection(t *testing.T) {
	h := setupLaunch(t)
	h.run(t, "claude", "-m", "anthropic/claude-opus-4.6")

	cfg := mustLoadConfig(t)
	if cfg.LastAgent != "claude" {
		t.Errorf("LastAgent = %q", cfg.LastAgent)
	}
	if cfg.LastModel != "anthropic/claude-opus-4.6" {
		t.Errorf("LastModel = %q", cfg.LastModel)
	}
}

func TestLaunchUnknownModelSuggests(t *testing.T) {
	h := setupLaunch(t)

	// "anthropic/claude-opus" (missing the version suffix) is not an exact
	// slug match, but it is a substring of anthropic/claude-opus-4.6, which
	// is what openrouter.Suggest's substring matching requires to surface it.
	_, err := h.exec("claude", "-m", "anthropic/claude-opus")
	if err == nil {
		t.Fatal("expected an error for an unknown model")
	}
	if !strings.Contains(err.Error(), "anthropic/claude-opus-4.6") {
		t.Errorf("error should suggest a close match, got: %v", err)
	}
}

func TestLaunchRequiresModelFlag(t *testing.T) {
	h := setupLaunch(t)

	_, err := h.exec("claude")
	if err == nil {
		t.Fatal("expected an error when --model is omitted in Phase 1")
	}
	// Asserting only that *some* error occurred would also pass if the
	// modelID=="" guard were deleted: with no model given, an empty query
	// reaches openrouter.Suggest, whose empty-query branch matches every
	// model, so resolveAndRun still returns a non-nil (but wrong) "unknown
	// model" error. Pin down the right error.
	if !strings.Contains(err.Error(), "a model is required") {
		t.Errorf("error should name the missing --model flag, got: %v", err)
	}
	if strings.Contains(err.Error(), "unknown model") {
		t.Errorf("missing --model should be reported as missing, not as an unknown model: %v", err)
	}
}

func TestLaunchIncompatibleModelRequiresConfirmation(t *testing.T) {
	h := setupLaunch(t)

	// --yes accepts the compatibility warning without prompting.
	var stderr bytes.Buffer
	root := h.root(&stderr)
	root.SetArgs([]string{"claude", "-m", "qwen/qwen3-coder:free", "--yes"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if h.ran.Path == "" {
		t.Error("launch should proceed once confirmed")
	}
	// Proceeding alone doesn't prove CheckModel's warning path ran at all
	// (a no-op compatibility check would look identical); the warning
	// reaching stderr is the actual evidence.
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("expected a compatibility warning on stderr, got: %q", stderr.String())
	}
}

func TestLaunchIncompatibleModelConfirmedViaPrompt(t *testing.T) {
	h := setupLaunch(t)

	var stderr bytes.Buffer
	root := h.root(&stderr)
	root.SetIn(strings.NewReader("y\n"))
	root.SetArgs([]string{"claude", "-m", "qwen/qwen3-coder:free"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if h.ran.Path == "" {
		t.Error("launch should proceed once the user types y")
	}
	// As with the --yes sibling test: proceeding alone doesn't prove
	// CheckModel's warning path actually ran (a no-op compatibility check
	// would look identical). The warning reaching stderr is the evidence.
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("expected a compatibility warning on stderr, got: %q", stderr.String())
	}
}

func TestLaunchIncompatibleModelDeclinedCancels(t *testing.T) {
	h := setupLaunch(t)

	root := h.root(&bytes.Buffer{})
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"claude", "-m", "qwen/qwen3-coder:free"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error when the user declines the compatibility warning")
	}
	if h.ran.Path != "" {
		t.Errorf("launch should not proceed when declined, got Path = %q", h.ran.Path)
	}
}

func TestConfirmYesFlagSkipsPrompt(t *testing.T) {
	h := newHarness(t)
	root := h.root(&bytes.Buffer{})
	root.SetIn(strings.NewReader("")) // must not even be consulted
	global := &globalFlags{yes: true}

	ok, err := confirm(root, global, "Proceed?")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !ok {
		t.Error("expected --yes to answer affirmatively without reading stdin")
	}
}

func TestConfirmReadsYesFromStdin(t *testing.T) {
	h := newHarness(t)
	root := h.root(&bytes.Buffer{})
	root.SetIn(strings.NewReader("y\n"))
	global := &globalFlags{}

	ok, err := confirm(root, global, "Proceed?")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !ok {
		t.Error("expected confirm to return true for typed y")
	}
}

func TestConfirmReadsNoFromStdin(t *testing.T) {
	h := newHarness(t)
	root := h.root(&bytes.Buffer{})
	root.SetIn(strings.NewReader("n\n"))
	global := &globalFlags{}

	ok, err := confirm(root, global, "Proceed?")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if ok {
		t.Error("expected confirm to return false for typed n")
	}
}

func TestConfirmDefaultsToNoOnEmptyInput(t *testing.T) {
	h := newHarness(t)
	root := h.root(&bytes.Buffer{})
	root.SetIn(strings.NewReader("\n")) // Enter with no answer
	global := &globalFlags{}

	ok, err := confirm(root, global, "Proceed?")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if ok {
		t.Error("expected confirm to default to false on an empty answer")
	}
}

func TestConfirmDefaultsToNoOnEOF(t *testing.T) {
	h := newHarness(t)
	root := h.root(&bytes.Buffer{})
	root.SetIn(strings.NewReader("")) // immediate EOF, nothing to read
	global := &globalFlags{}

	ok, err := confirm(root, global, "Proceed?")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if ok {
		t.Error("expected confirm to default to false on EOF")
	}
}

// fakeExitCoder mimics *exec.ExitError's ExitCode() method (promoted from
// its embedded *os.ProcessState) without spawning a real process, which
// tests must not do. This is the shape of the error agent.Run
// (exec_windows.go) returns when the launched agent exits nonzero on
// Windows: resolveAndRun must recognize it and suppress cobra's own
// "Error: ..." line, since the agent already inherited stderr and reported
// its own failure - printing cobra's wrapped "run claude: exit status N" on
// top would be redundant noise, and main still needs the real error value
// to extract the exit code from (see exitCode in main.go).
type fakeExitCoder struct{ code int }

func (e fakeExitCoder) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e fakeExitCoder) ExitCode() int { return e.code }

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

// TestLaunchOtherErrorsStillPrintCobraErrorLine guards against
// over-broadly silencing: only an agent exit-code error should suppress
// cobra's default line, not every failure.
func TestLaunchOtherErrorsStillPrintCobraErrorLine(t *testing.T) {
	h := setupLaunch(t)

	out, err := h.exec("claude") // missing --model
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(out, "Error:") {
		t.Errorf("expected cobra's default error line for a non-exit-code error, got: %q", out)
	}
}

func TestLaunchMissingAPIKeyFails(t *testing.T) {
	h := newHarness(t)
	stubClaudePath(t)
	t.Setenv("OPENROUTER_API_KEY", "")

	_, err := h.exec("claude", "-m", "anthropic/claude-opus-4.6")
	if err == nil {
		t.Fatal("expected an error when no API key is available")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Errorf("error should name the environment variable, got: %v", err)
	}
}

// erroringCatalog always fails to fetch, forcing Service.Plan down the
// stale-cache fallback path so a pre-seeded cache file is what gets served.
type erroringCatalog struct{}

func (erroringCatalog) Models(context.Context) ([]openrouter.Model, error) {
	return nil, errors.New("network down")
}

// This is the regression test for the bug this change fixes: loadCatalog
// used to print the stale-catalog warning unconditionally, the moment a
// stale snapshot came back, so it survived whatever failed afterward. Once
// that print moved into Service.Plan's accumulated Warnings slice, every
// fatal guard past the catalog load returned Plan{} and silently dropped
// it - so "offline, cache is stale, mistyped model slug" surfaced only the
// unknown-model error, losing the one clue that explains it. Both lines
// must reach stderr even though the command ultimately fails.
func TestLaunchStaleCatalogWarningSurvivesUnknownModelError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	stubClaudePath(t)

	path, err := openrouter.CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(struct {
		FetchedAt time.Time          `json:"fetched_at"`
		Models    []openrouter.Model `json:"models"`
	}{FetchedAt: time.Now().Add(-48 * time.Hour), Models: fakeModels()}) // older than DefaultTTL
	if err != nil {
		t.Fatalf("marshal cache file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	h := &harness{}
	h.svc = &launch.Service{
		Catalog: erroringCatalog{},
		Run:     func(c agent.Command) error { h.ran = c; return nil },
	}

	var stderr bytes.Buffer
	root := h.root(&stderr)
	root.SetArgs([]string{"claude", "-m", "no/such-model"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for an unknown model")
	}

	out := stderr.String()
	if !strings.Contains(out, "could not refresh the model catalog") {
		t.Errorf("stderr should contain the stale-catalog warning, got: %q", out)
	}
	if !strings.Contains(out, "unknown model") {
		t.Errorf("stderr should contain the unknown-model error, got: %q", out)
	}
}
