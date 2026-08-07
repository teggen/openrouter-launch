package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/config"
)

func TestProfileAddAndList(t *testing.T) {
	setupLaunch(t)

	runCmd(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")

	got := runCmd(t, "profile", "list")
	if !strings.Contains(got, "opus-cc") {
		t.Errorf("list output missing the profile:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("list output missing the model:\n%s", got)
	}
}

func TestProfileAddRejectsUnknownAgent(t *testing.T) {
	setupLaunch(t)

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"profile", "add", "--name", "x", "--agent", "nope", "--model", "a/b"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for an unknown agent")
	}
}

func TestProfileAddRejectsDuplicate(t *testing.T) {
	setupLaunch(t)

	runCmd(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for a duplicate profile name")
	}
}

func TestProfileLaunch(t *testing.T) {
	got := setupLaunch(t)

	runCmd(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")
	runCmd(t, "profile", "launch", "opus-cc")

	if got.Path != "/usr/local/bin/claude" {
		t.Errorf("Path = %q, want the claude binary", got.Path)
	}
	if len(got.Args) < 2 || got.Args[1] != "anthropic/claude-opus-4.6" {
		t.Errorf("Args = %v, want the profile's model", got.Args)
	}
}

func TestProfileLaunchPassesStoredArgs(t *testing.T) {
	got := setupLaunch(t)

	runCmd(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6", "--", "--resume")
	runCmd(t, "profile", "launch", "opus-cc")

	if len(got.Args) != 3 || got.Args[2] != "--resume" {
		t.Errorf("Args = %v, want the stored --resume", got.Args)
	}
}

func TestProfileLaunchUnknown(t *testing.T) {
	setupLaunch(t)

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
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
	setupLaunch(t)

	runCmd(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")
	runCmd(t, "profile", "rm", "opus-cc")

	got := runCmd(t, "profile", "list")
	if strings.Contains(got, "opus-cc") {
		t.Errorf("profile still listed after removal:\n%s", got)
	}
}

func TestProfileRename(t *testing.T) {
	setupLaunch(t)

	runCmd(t, "profile", "add", "--name", "opus-cc",
		"--agent", "claude", "--model", "anthropic/claude-opus-4.6")
	runCmd(t, "profile", "rename", "opus-cc", "flagship")

	got := runCmd(t, "profile", "list")
	if !strings.Contains(got, "flagship") {
		t.Errorf("renamed profile missing:\n%s", got)
	}
	if strings.Contains(got, "opus-cc") {
		t.Errorf("old name still listed:\n%s", got)
	}
}

func TestProfileListEmpty(t *testing.T) {
	setupLaunch(t)

	got := runCmd(t, "profile", "list")
	if !strings.Contains(strings.ToLower(got), "no profiles") {
		t.Errorf("expected an empty-state message, got:\n%s", got)
	}
}
