package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// fakeLauncher is a minimal agent.Launcher for exercising resolveAndRun's
// guards directly. The real registry only has one agent (claude), which is
// always Status.Supported and never implements PlatformSupported, so those
// guard branches are otherwise unreachable from CLI-level tests.
type fakeLauncher struct {
	name        string
	displayName string
}

func (f *fakeLauncher) Name() string        { return f.name }
func (f *fakeLauncher) DisplayName() string { return f.displayName }
func (f *fakeLauncher) Command(agent.Request) (agent.Command, error) {
	return agent.Command{Path: "/bin/fake"}, nil
}

// fakePlatformLauncher additionally reports itself as unsupported on this
// platform, regardless of what it actually runs on.
type fakePlatformLauncher struct {
	fakeLauncher
}

func (f *fakePlatformLauncher) Supported() error {
	return errors.New("windows is not supported yet")
}

// fakeIncompatibleLauncher's CheckModel returns a plain error rather than
// one wrapping agent.ErrIncompatibleModel, exercising the genuine-failure
// branch of resolveAndRun's compatibility check. agent.Claude never takes
// this branch (it returns either nil or an ErrIncompatibleModel-wrapped
// error), so it is otherwise unreachable from CLI-level tests.
type fakeIncompatibleLauncher struct {
	fakeLauncher
	err error
}

func (f *fakeIncompatibleLauncher) CheckModel(openrouter.Model) error {
	return f.err
}

// captureRun replaces the process handoff so tests observe the command
// instead of executing it.
func captureRun(t *testing.T) *agent.Command {
	t.Helper()
	var got agent.Command
	prev := runner
	runner = func(c agent.Command) error {
		got = c
		return nil
	}
	t.Cleanup(func() { runner = prev })
	return &got
}

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

func setupLaunch(t *testing.T) *agent.Command {
	t.Helper()
	useFakeCatalog(t)
	stubClaudePath(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	return captureRun(t)
}

func TestLaunchBuildsCommand(t *testing.T) {
	got := setupLaunch(t)
	runCmd(t, "claude", "-m", "anthropic/claude-opus-4.6")

	if got.Path != "/usr/local/bin/claude" {
		t.Errorf("Path = %q", got.Path)
	}
	if len(got.Args) < 2 || got.Args[0] != "--model" || got.Args[1] != "anthropic/claude-opus-4.6" {
		t.Errorf("Args = %v", got.Args)
	}

	var foundKey bool
	for _, e := range got.Env {
		if e == "ANTHROPIC_API_KEY=sk-or-test" {
			foundKey = true
		}
	}
	if !foundKey {
		t.Errorf("API key not passed through: %v", got.Env)
	}
}

func TestLaunchPassesExtraArgsAfterDoubleDash(t *testing.T) {
	got := setupLaunch(t)
	runCmd(t, "claude", "-m", "anthropic/claude-opus-4.6", "--", "--resume")

	if len(got.Args) != 3 || got.Args[2] != "--resume" {
		t.Errorf("Args = %v, want the trailing --resume", got.Args)
	}
}

func TestLaunchRecordsLastSelection(t *testing.T) {
	setupLaunch(t)
	runCmd(t, "claude", "-m", "anthropic/claude-opus-4.6")

	cfg := mustLoadConfig(t)
	if cfg.LastAgent != "claude" {
		t.Errorf("LastAgent = %q", cfg.LastAgent)
	}
	if cfg.LastModel != "anthropic/claude-opus-4.6" {
		t.Errorf("LastModel = %q", cfg.LastModel)
	}
}

// TestLaunchSavesSelectionBeforeHandoff proves the ordering, not just the
// end state: on Unix, runner (syscall.Exec) never returns on success, so if
// the save happened after the call instead of before, it would silently
// never happen. captureRun's stub does return, so a same-process reordering
// bug wouldn't otherwise be observable; this test inspects the config from
// inside the runner callback itself, before resolveAndRun continues.
func TestLaunchSavesSelectionBeforeHandoff(t *testing.T) {
	useFakeCatalog(t)
	stubClaudePath(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	var savedBeforeHandoff bool
	prev := runner
	runner = func(c agent.Command) error {
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("config.Load inside runner: %v", err)
		}
		savedBeforeHandoff = cfg.LastAgent == "claude" && cfg.LastModel == "anthropic/claude-opus-4.6"
		return nil
	}
	t.Cleanup(func() { runner = prev })

	runCmd(t, "claude", "-m", "anthropic/claude-opus-4.6")

	if !savedBeforeHandoff {
		t.Error("expected the last selection to be persisted before control reaches runner")
	}
}

func TestLaunchUnknownModelSuggests(t *testing.T) {
	setupLaunch(t)

	// "anthropic/claude-opus" (missing the version suffix) is not an exact
	// slug match, but it is a substring of anthropic/claude-opus-4.6, which
	// is what openrouter.Suggest's substring matching requires to surface it.
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"claude", "-m", "anthropic/claude-opus"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown model")
	}
	if !strings.Contains(err.Error(), "anthropic/claude-opus-4.6") {
		t.Errorf("error should suggest a close match, got: %v", err)
	}
}

func TestLaunchRequiresModelFlag(t *testing.T) {
	setupLaunch(t)

	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"claude"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error when --model is omitted in Phase 1")
	}
	// Asserting only that *some* error occurred would also pass if the
	// modelID=="" guard were deleted: with no model given, an empty query
	// reaches openrouter.Suggest, whose empty-query branch matches every
	// model, so FindByID still fails and resolveAndRun still returns a
	// non-nil (but wrong) "unknown model" error. Pin down the right error.
	if !strings.Contains(err.Error(), "a model is required") {
		t.Errorf("error should name the missing --model flag, got: %v", err)
	}
	if strings.Contains(err.Error(), "unknown model") {
		t.Errorf("missing --model should be reported as missing, not as an unknown model: %v", err)
	}
}

// TestResolveAndRunUnsupportedAgent and TestResolveAndRunUnsupportedPlatform
// call resolveAndRun directly with a synthetic spec, since the real registry
// only has claude, which is always Status.Supported and never implements
// PlatformSupported.
func TestResolveAndRunUnsupportedAgent(t *testing.T) {
	setupLaunch(t)
	spec := &agent.Spec{
		Name:     "nope",
		Launcher: &fakeLauncher{name: "nope", displayName: "Nope"},
		Status:   agent.Status{Supported: false, Reason: "talks to its own backend"},
	}

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := resolveAndRun(root, spec, "anthropic/claude-opus-4.6", nil, &globalFlags{})
	if err == nil {
		t.Fatal("expected an error for an unsupported agent")
	}
	if !strings.Contains(err.Error(), "talks to its own backend") {
		t.Errorf("error should include the unsupported reason, got: %v", err)
	}
}

func TestResolveAndRunUnsupportedPlatform(t *testing.T) {
	setupLaunch(t)
	spec := &agent.Spec{
		Name: "platform-agent",
		Launcher: &fakePlatformLauncher{
			fakeLauncher{name: "platform-agent", displayName: "Platform Agent"},
		},
		Status: agent.Status{Supported: true},
	}

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := resolveAndRun(root, spec, "anthropic/claude-opus-4.6", nil, &globalFlags{})
	if err == nil {
		t.Fatal("expected an error for an unsupported platform")
	}
	if !strings.Contains(err.Error(), "windows is not supported yet") {
		t.Errorf("error should surface the platform error, got: %v", err)
	}
}

func TestResolveAndRunPropagatesGenuineCheckModelError(t *testing.T) {
	setupLaunch(t)
	wantErr := errors.New("catalog service unreachable")
	spec := &agent.Spec{
		Name: "picky",
		Launcher: &fakeIncompatibleLauncher{
			fakeLauncher: fakeLauncher{name: "picky", displayName: "Picky"},
			err:          wantErr,
		},
		Status: agent.Status{Supported: true},
	}

	var stderr bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)

	err := resolveAndRun(root, spec, "anthropic/claude-opus-4.6", nil, &globalFlags{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the genuine CheckModel error to propagate unchanged, got: %v", err)
	}
	// A genuine (non-ErrIncompatibleModel) error must be a hard failure: no
	// warning, no confirmation prompt.
	if strings.Contains(stderr.String(), "warning:") {
		t.Errorf("a genuine CheckModel error should not print a warning, stderr = %q", stderr.String())
	}
}

func TestLaunchIncompatibleModelRequiresConfirmation(t *testing.T) {
	got := setupLaunch(t)

	// --yes accepts the compatibility warning without prompting.
	var stderr bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	root.SetArgs([]string{"claude", "-m", "qwen/qwen3-coder:free", "--yes"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if got.Path == "" {
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
	got := setupLaunch(t)

	var stderr bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader("y\n"))
	root.SetArgs([]string{"claude", "-m", "qwen/qwen3-coder:free"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got.Path == "" {
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
	got := setupLaunch(t)

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"claude", "-m", "qwen/qwen3-coder:free"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error when the user declines the compatibility warning")
	}
	if got.Path != "" {
		t.Errorf("launch should not proceed when declined, got Path = %q", got.Path)
	}
}

func TestConfirmYesFlagSkipsPrompt(t *testing.T) {
	root := NewRootCmd()
	root.SetIn(strings.NewReader("")) // must not even be consulted
	root.SetErr(&bytes.Buffer{})
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
	root := NewRootCmd()
	root.SetIn(strings.NewReader("y\n"))
	root.SetErr(&bytes.Buffer{})
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
	root := NewRootCmd()
	root.SetIn(strings.NewReader("n\n"))
	root.SetErr(&bytes.Buffer{})
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
	root := NewRootCmd()
	root.SetIn(strings.NewReader("\n")) // Enter with no answer
	root.SetErr(&bytes.Buffer{})
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
	root := NewRootCmd()
	root.SetIn(strings.NewReader("")) // immediate EOF, nothing to read
	root.SetErr(&bytes.Buffer{})
	global := &globalFlags{}

	ok, err := confirm(root, global, "Proceed?")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if ok {
		t.Error("expected confirm to default to false on EOF")
	}
}

func TestLaunchMissingAPIKeyFails(t *testing.T) {
	useFakeCatalog(t)
	stubClaudePath(t)
	captureRun(t)
	t.Setenv("OPENROUTER_API_KEY", "")

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"claude", "-m", "anthropic/claude-opus-4.6"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error when no API key is available")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Errorf("error should name the environment variable, got: %v", err)
	}
}
