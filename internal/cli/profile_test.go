package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/teggen/agentlaunch/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/ui"
)

func TestProfileAddAndList(t *testing.T) {
	h := setupLaunch(t)

	h.run(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")

	got := h.run(t, "profile", "list")
	if !strings.Contains(got, "opus-cc") {
		t.Errorf("list output missing the profile:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("list output missing the model:\n%s", got)
	}
}

func TestProfileAddRejectsUnknownAgent(t *testing.T) {
	h := setupLaunch(t)

	root := h.root(&bytes.Buffer{})
	root.SetArgs([]string{"profile", "add", "--name", "x", "--agent", "nope", "--model", "a/b"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown agent")
	}
	if !errors.Is(err, agent.ErrUnknownAgent) {
		t.Errorf("expected %v, got: %v", agent.ErrUnknownAgent, err)
	}
}

func TestProfileAddRejectsDuplicate(t *testing.T) {
	h := setupLaunch(t)

	h.run(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")

	root := h.root(&bytes.Buffer{})
	root.SetArgs([]string{"profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for a duplicate profile name")
	}
	if !errors.Is(err, config.ErrProfileExists) {
		t.Errorf("expected %v, got: %v", config.ErrProfileExists, err)
	}
}

func TestProfileLaunch(t *testing.T) {
	h := setupLaunch(t)

	h.run(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")
	h.run(t, "profile", "launch", "opus-cc")

	if h.ran.Path != "/usr/local/bin/claude" {
		t.Errorf("Path = %q, want the claude binary", h.ran.Path)
	}
	if len(h.ran.Args) < 2 || h.ran.Args[1] != "anthropic/claude-opus-4.6" {
		t.Errorf("Args = %v, want the profile's model", h.ran.Args)
	}
}

func TestProfileLaunchPassesStoredArgs(t *testing.T) {
	h := setupLaunch(t)

	h.run(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6", "--", "--resume")
	h.run(t, "profile", "launch", "opus-cc")

	if len(h.ran.Args) != 3 || h.ran.Args[2] != "--resume" {
		t.Errorf("Args = %v, want the stored --resume", h.ran.Args)
	}
}

func TestProfileLaunchUnknown(t *testing.T) {
	h := setupLaunch(t)

	root := h.root(&bytes.Buffer{})
	root.SetArgs([]string{"profile", "launch", "nope"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown profile")
	}
	// Asserting only that *some* error occurred would also pass if the
	// cfg.Profile "not found" guard were deleted: a zero-value Profile has
	// an empty Agent field, so agent.Lookup("") still returns a (different,
	// wrong) error. Pin down the right error so that mutation is caught.
	if !errors.Is(err, config.ErrProfileNotFound) {
		t.Errorf("expected %v, got: %v", config.ErrProfileNotFound, err)
	}
}

func TestProfileRemove(t *testing.T) {
	h := setupLaunch(t)

	h.run(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")
	h.run(t, "profile", "rm", "opus-cc")

	got := h.run(t, "profile", "list")
	if strings.Contains(got, "opus-cc") {
		t.Errorf("profile still listed after removal:\n%s", got)
	}
}

func TestProfileRename(t *testing.T) {
	h := setupLaunch(t)

	h.run(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")
	h.run(t, "profile", "rename", "opus-cc", "flagship")

	got := h.run(t, "profile", "list")
	if !strings.Contains(got, "flagship") {
		t.Errorf("renamed profile missing:\n%s", got)
	}
	if strings.Contains(got, "opus-cc") {
		t.Errorf("old name still listed:\n%s", got)
	}
}

func TestProfileListEmpty(t *testing.T) {
	h := setupLaunch(t)

	got := h.run(t, "profile", "list")
	if !strings.Contains(strings.ToLower(got), "no profiles") {
		t.Errorf("expected an empty-state message, got:\n%s", got)
	}
}

func TestProfileListShowsAgentInstallState(t *testing.T) {
	profiles := []config.Profile{{Name: "p1", Agent: "claude", Model: "anthropic/x"}}
	lookup := func(string) (*agent.Spec, error) {
		return &agent.Spec{
			Name: "claude", Launcher: wideLauncher{},
			Status: agent.Status{Supported: true},
		}, nil
	}

	out := ui.NewTheme(new(strings.Builder)).Render(
		profilesTable(profiles, lookup, func(*agent.Spec) bool { return false }))

	if got := tableRow(t, out, "p1")[2]; got != "✗ not installed" {
		t.Errorf("status = %q, want %q", got, "✗ not installed")
	}
}

// A profile naming an agent that is no longer registered. `profile add`
// validates the name, so this arrives only from a hand-edited config or an
// agent dropped between releases — and without this column the failure is
// invisible until you try to launch it.
func TestProfileListFlagsAnUnknownAgent(t *testing.T) {
	profiles := []config.Profile{{Name: "old", Agent: "vscode", Model: "openai/x"}}
	lookup := func(string) (*agent.Spec, error) { return nil, agent.ErrUnknownAgent }

	out := ui.NewTheme(new(strings.Builder)).Render(
		profilesTable(profiles, lookup, func(*agent.Spec) bool { return true }))

	if got := tableRow(t, out, "old")[2]; got != "⚠ unknown agent" {
		t.Errorf("status = %q, want %q", got, "⚠ unknown agent")
	}
}

func TestProfileListRendersNameAgentModelAndArgs(t *testing.T) {
	h := newHarness(t)
	h.run(t, "profile", "add", "--name", "opus-cc", "--agent", "claude",
		"--model", "anthropic/claude-opus-4.6", "--", "--resume")

	out := h.run(t, "profile", "list")
	wantColumns(t, out, "NAME", "AGENT", "STATUS", "MODEL", "ARGS")

	row := tableRow(t, out, "opus-cc")
	for i, want := range map[int]string{1: "claude", 3: "anthropic/claude-opus-4.6", 4: "--resume"} {
		if row[i] != want {
			t.Errorf("column %d = %q, want %q", i, row[i], want)
		}
	}
}

func TestProfileListEmptyStateIsUnchanged(t *testing.T) {
	h := newHarness(t)
	if got := h.run(t, "profile", "list"); !strings.Contains(got, "No profiles saved.") {
		t.Errorf("empty state = %q, want the add hint", got)
	}
}

func TestProfileListEmitsNoEscapesWhenNotATerminal(t *testing.T) {
	h := newHarness(t)
	h.run(t, "profile", "add", "--name", "p", "--agent", "claude", "--model", "anthropic/x")
	if got := h.run(t, "profile", "list"); strings.Contains(got, "\x1b") {
		t.Errorf("profile list emitted ANSI escapes to a buffer:\n%q", got)
	}
}
