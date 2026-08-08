package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/tui"
)

func TestBareInvocationOpensTheTUI(t *testing.T) {
	h := newHarness(t)
	if _, err := h.exec(); err != nil {
		t.Fatalf("bare invocation: %v", err)
	}
	if h.tuiCalls != 1 {
		t.Fatalf("TUI opened %d times, want 1", h.tuiCalls)
	}
	if h.tuiOpts[0].Agent != nil {
		t.Error("bare invocation preselected an agent; it should start at the root screen")
	}
}

// A typo must report itself rather than opening the picker: `openrouter-launch
// bogus` is an unknown-command error, not a silent route into the TUI.
func TestUnknownSubcommandStillErrors(t *testing.T) {
	h := newHarness(t)
	out, err := h.exec("bogus")
	if err == nil {
		t.Fatalf("`bogus` was accepted; output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("err = %q, want an unknown-command error", err)
	}
	if h.tuiCalls != 0 {
		t.Errorf("an unknown subcommand opened the TUI %d times", h.tuiCalls)
	}
}

func TestLaunchWithoutAModelOpensThePickerForThatAgent(t *testing.T) {
	h := newHarness(t)
	if _, err := h.exec("claude"); err != nil {
		t.Fatalf("claude with no --model: %v", err)
	}
	if h.tuiCalls != 1 {
		t.Fatalf("TUI opened %d times, want 1", h.tuiCalls)
	}
	if h.tuiOpts[0].Agent == nil || h.tuiOpts[0].Agent.Name != "claude" {
		t.Errorf("TUI opened for %v, want claude", h.tuiOpts[0].Agent)
	}
}

func TestLaunchPassesExtraArgsThroughToTheTUI(t *testing.T) {
	h := newHarness(t)
	if _, err := h.exec("claude", "--", "--resume"); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if h.tuiCalls != 1 {
		t.Fatalf("TUI opened %d times, want 1", h.tuiCalls)
	}
	got := h.tuiOpts[0].ExtraArgs
	if len(got) != 1 || got[0] != "--resume" {
		t.Errorf("ExtraArgs = %v, want [--resume]", got)
	}
}

func TestGlobalFlagsReachTheTUI(t *testing.T) {
	h := newHarness(t)
	if _, err := h.exec("--refresh", "--yes"); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if h.tuiCalls != 1 {
		t.Fatalf("TUI opened %d times, want 1", h.tuiCalls)
	}
	if !h.tuiOpts[0].Refresh {
		t.Error("--refresh did not reach the TUI")
	}
	if !h.tuiOpts[0].AssumeYes {
		t.Error("--yes did not reach the TUI")
	}
}

// Cancelling is not a failure: it must exit 0 with nothing printed, and it
// must still have gone through the TUI — this would also pass if RunE
// returned nil without ever invoking it.
func TestCancellingTheTUIExitsCleanly(t *testing.T) {
	h := newHarness(t)
	h.tuiErr = tui.ErrCancelled

	out, err := h.exec()
	if err != nil {
		t.Errorf("cancelling returned %v, want nil", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("cancelling printed %q, want nothing", out)
	}
	if h.tuiCalls != 1 {
		t.Errorf("TUI opened %d times, want 1", h.tuiCalls)
	}
}

func TestTUIErrorsOtherThanCancellationPropagate(t *testing.T) {
	h := newHarness(t)
	h.tuiErr = errors.New("catalog unreachable")

	if _, err := h.exec(); err == nil || !strings.Contains(err.Error(), "catalog unreachable") {
		t.Errorf("err = %v, want the TUI's error", err)
	}
}

func TestAnApprovedPlanIsHandedOff(t *testing.T) {
	h := newHarness(t)
	h.tuiErr = nil
	h.tuiPlan = launch.Plan{
		Spec:    mustLookup(t, "claude"),
		Model:   openrouter.Model{ID: "anthropic/claude-opus-4.6"},
		Command: agent.Command{Path: "/usr/bin/claude", Args: []string{"claude"}},
	}

	if _, err := h.exec(); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if h.ran.Path != "/usr/bin/claude" {
		t.Errorf("handed off %+v, want the approved plan's command", h.ran)
	}
}

// The picker runs in the alt screen, so everything it drew is gone by the
// time the agent starts. The stderr line is the only lasting trace.
func TestPlanWarningsReachStderrAfterTheTUI(t *testing.T) {
	h := newHarness(t)
	h.tuiErr = nil
	h.tuiPlan = launch.Plan{
		Spec:    mustLookup(t, "claude"),
		Model:   openrouter.Model{ID: "anthropic/claude-opus-4.6"},
		Command: agent.Command{Path: "/usr/bin/claude"},
		Warnings: []launch.Warning{
			{Kind: launch.WarnStaleCatalog, Message: "using cached data from 3h ago"},
		},
	}

	out, err := h.exec()
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(out, "using cached data") {
		t.Errorf("output = %q, does not carry the plan's warning", out)
	}
}

func mustLookup(t *testing.T, name string) *agent.Spec {
	t.Helper()
	spec, err := agent.Lookup(name)
	if err != nil {
		t.Fatalf("agent.Lookup(%q): %v", name, err)
	}
	return spec
}
