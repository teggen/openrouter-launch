package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
)

func TestProfileAddAndList(t *testing.T) {
	h, _ := setupLaunch(t)

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
	h, _ := setupLaunch(t)

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
	h, _ := setupLaunch(t)

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
	h, got := setupLaunch(t)

	h.run(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")
	h.run(t, "profile", "launch", "opus-cc")

	if got.Path != "/usr/local/bin/claude" {
		t.Errorf("Path = %q, want the claude binary", got.Path)
	}
	if len(got.Args) < 2 || got.Args[1] != "anthropic/claude-opus-4.6" {
		t.Errorf("Args = %v, want the profile's model", got.Args)
	}
}

func TestProfileLaunchPassesStoredArgs(t *testing.T) {
	h, got := setupLaunch(t)

	h.run(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6", "--", "--resume")
	h.run(t, "profile", "launch", "opus-cc")

	if len(got.Args) != 3 || got.Args[2] != "--resume" {
		t.Errorf("Args = %v, want the stored --resume", got.Args)
	}
}

func TestProfileLaunchUnknown(t *testing.T) {
	h, _ := setupLaunch(t)

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
	h, _ := setupLaunch(t)

	h.run(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")
	h.run(t, "profile", "rm", "opus-cc")

	got := h.run(t, "profile", "list")
	if strings.Contains(got, "opus-cc") {
		t.Errorf("profile still listed after removal:\n%s", got)
	}
}

func TestProfileRename(t *testing.T) {
	h, _ := setupLaunch(t)

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
	h, _ := setupLaunch(t)

	got := h.run(t, "profile", "list")
	if !strings.Contains(strings.ToLower(got), "no profiles") {
		t.Errorf("expected an empty-state message, got:\n%s", got)
	}
}
