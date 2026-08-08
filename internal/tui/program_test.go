package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsTerminalRejectsARegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-tty")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	if isTerminal(f) {
		t.Error("isTerminal reported a regular file as a terminal")
	}
}

// Without a terminal the picker cannot run at all, and bubbletea's own
// failure would be obscure. The message must name the way forward.
func TestRunWithoutATerminalNamesTheFlagToUseInstead(t *testing.T) {
	original := isTTY
	t.Cleanup(func() { isTTY = original })
	isTTY = func() bool { return false }

	_, err := Run(context.Background(), Options{Service: nil})
	if err == nil {
		t.Fatal("Run with no terminal returned no error")
	}
	if !strings.Contains(err.Error(), "--model") {
		t.Errorf("err = %q, does not point at --model", err)
	}
	// The terminal check must come before the Service check, or a CLI that
	// forgot the service would report the wrong problem.
	if strings.Contains(err.Error(), "Service") {
		t.Errorf("err = %q, reports the service instead of the missing terminal", err)
	}
}

func TestLiveScreensRequiresATerminal(t *testing.T) {
	original := isTTY
	t.Cleanup(func() { isTTY = original })
	isTTY = func() bool { return false }

	if _, err := liveScreens(); err == nil {
		t.Error("liveScreens succeeded with no terminal")
	}
}

func TestLiveScreensProvidesEveryScreen(t *testing.T) {
	original := isTTY
	t.Cleanup(func() { isTTY = original })
	isTTY = func() bool { return true }

	sc, err := liveScreens()
	if err != nil {
		t.Fatalf("liveScreens: %v", err)
	}
	// A nil screen would panic on first use, deep inside a session, rather
	// than failing here.
	if sc.root == nil || sc.pick == nil || sc.prompt == nil ||
		sc.confirm == nil || sc.notice == nil {
		t.Errorf("liveScreens left a screen nil: %+v", sc)
	}
}

func TestRunPropagatesTheTerminalErrorRatherThanPanicking(t *testing.T) {
	original := isTTY
	t.Cleanup(func() { isTTY = original })
	isTTY = func() bool { return false }

	_, err := Run(context.Background(), Options{})
	if err == nil || errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want a terminal error", err)
	}
}
