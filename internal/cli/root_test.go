package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/launch"
)

// TestNewRootCmdWithPanicsOnNilService pins that a nil *launch.Service is
// caught at construction rather than building a command tree that panics
// later, on first use, when a subcommand's RunE dereferences the nil
// receiver (e.g. Snapshot). Failing at construction makes the bug
// immediately diagnosable, mirroring agent.MustRegistry's guard on a
// malformed registry.
func TestNewRootCmdWithPanicsOnNilService(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for a nil *launch.Service")
		}
	}()
	NewRootCmdWith(nil)
}

// TestNewRootCmdPanicsOnNilRegistry is the registry's half of the guard
// above, and it asserts the MESSAGE rather than merely that something
// panicked: newLaunchCmds calls List while the tree is still being built, so
// a nil registry brings the construction down either way. Checking only for a
// panic would pass just as happily against an unguarded nil dereference,
// which is the obscure failure the guard exists to replace.
func TestNewRootCmdPanicsOnNilRegistry(t *testing.T) {
	var msg any
	func() {
		defer func() { msg = recover() }()
		newRootCmd(&launch.Service{}, nil, nil)
	}()
	if msg == nil {
		t.Fatal("newRootCmd with a nil registry did not panic")
	}
	if !strings.Contains(fmt.Sprint(msg), "Registry") {
		t.Errorf("panic should name the missing argument, got: %v", msg)
	}
}
