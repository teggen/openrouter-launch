package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/catalog"
)

func setupLaunch(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.stubClaudePath(t)
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

// TestLaunchUnsupportedAgentRefusesWithRegistryReason pins the seam between
// the registry and newLaunchCmds: every registered spec, supported or not,
// gets a subcommand (see newLaunchCmds's comment), and an unsupported one
// refuses through the planner's UnsupportedAgentError rather than cobra
// failing to recognize the subcommand at all. If a refactor ever taught
// newLaunchCmds to skip unsupported specs, "chatgpt" would silently degrade
// to cobra's "unknown command" error - same exit code, misleading message,
// and no test would catch it without this one.
func TestLaunchUnsupportedAgentRefusesWithRegistryReason(t *testing.T) {
	h := newHarness(t)

	spec := h.mustLookup(t, "chatgpt")

	_, execErr := h.exec("chatgpt", "-m", "some/model")
	if execErr == nil {
		t.Fatal("expected an error for an unsupported agent")
	}
	if !strings.Contains(execErr.Error(), spec.Status.Reason) {
		t.Errorf("error should contain the registry's reason %q, got: %v", spec.Status.Reason, execErr)
	}
	if strings.Contains(execErr.Error(), "unknown command") {
		t.Errorf("chatgpt must be a real subcommand, not cobra's unknown-command fallback, got: %v", execErr)
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
	// is what catalog.Suggest's substring matching requires to surface it.
	_, err := h.exec("claude", "-m", "anthropic/claude-opus")
	if err == nil {
		t.Fatal("expected an error for an unknown model")
	}
	if !strings.Contains(err.Error(), "anthropic/claude-opus-4.6") {
		t.Errorf("error should suggest a close match, got: %v", err)
	}
}

// Omitting --model used to be an error ("a model is required" in Phase 1);
// Task 10 replaces that with opening the picker for the named agent instead.
// See TestLaunchWithoutAModelOpensThePickerForThatAgent in tui_test.go for
// that behavior's coverage.

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
	h.stubClaudePath(t)
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
//
// A missing --model no longer serves as the "other error" here: Task 10
// makes resolveAndRun open the picker for that case instead of failing (see
// TestLaunchWithoutAModelOpensThePickerForThatAgent in tui_test.go). An
// unknown model slug is still a plain, non-exit-code error.
func TestLaunchOtherErrorsStillPrintCobraErrorLine(t *testing.T) {
	h := setupLaunch(t)

	out, err := h.exec("claude", "-m", "totally-bogus-model")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(out, "Error:") {
		t.Errorf("expected cobra's default error line for a non-exit-code error, got: %q", out)
	}
}

func TestLaunchMissingAPIKeyFails(t *testing.T) {
	h := newHarness(t)
	h.stubClaudePath(t)
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

func (erroringCatalog) Models(context.Context) ([]catalog.Model, error) {
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
	h := newHarnessWith(t, erroringCatalog{})
	seedStaleCache(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	h.stubClaudePath(t)

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

// TestLaunchRendersStaleThenCompatibilityThenPrompt pins the full rendered
// order on the success path: the stale-catalog notice, then the
// compatibility warning, then the "Launch anyway?" confirmation prompt. The
// ordering is the contract - a stale-catalog notice printed after the user
// had already answered the prompt would arrive too late to inform their
// answer, and so would be useless to them.
func TestLaunchRendersStaleThenCompatibilityThenPrompt(t *testing.T) {
	h := newHarnessWith(t, erroringCatalog{}) // forces the stale-cache fallback
	seedStaleCache(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	h.stubClaudePath(t)

	var stderr bytes.Buffer
	root := h.root(&stderr)
	root.SetIn(strings.NewReader("y\n"))
	// qwen/qwen3-coder:free is in the cached fixture set but its provider
	// isn't anthropic, so Claude's CheckModel raises the advisory
	// ErrIncompatibleModel warning alongside the stale-catalog one.
	root.SetArgs([]string{"claude", "-m", "qwen/qwen3-coder:free"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := stderr.String()
	staleIdx := strings.Index(out, "could not refresh the model catalog")
	compatIdx := strings.Index(out, "optimized for anthropic/* models")
	promptIdx := strings.Index(out, "Launch anyway?")

	if staleIdx == -1 {
		t.Fatalf("stderr should contain the stale-catalog warning, got: %q", out)
	}
	if compatIdx == -1 {
		t.Fatalf("stderr should contain the compatibility warning, got: %q", out)
	}
	if promptIdx == -1 {
		t.Fatalf("stderr should contain the confirmation prompt, got: %q", out)
	}
	if !(staleIdx < compatIdx && compatIdx < promptIdx) {
		t.Errorf("expected stale (%d) < compatibility (%d) < prompt (%d), got: %q",
			staleIdx, compatIdx, promptIdx, out)
	}

	// The launch must actually have proceeded, or the ordering above would
	// hold vacuously on a path that failed before reaching the prompt.
	if h.ran.Path == "" {
		t.Error("launch should have proceeded once the user typed y")
	}
}
