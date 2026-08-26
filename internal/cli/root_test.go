package cli

import (
	"fmt"
	"reflect"
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

// TestNewServiceWiresEverySeam guards the composition root against the one
// way it can fail silently. launch.Service is now nothing but function
// fields, and a nil one is not a compile error — LoadCatalog and APIKey
// report themselves missing at the first launch, but RecordSelection nil is a
// SUPPORTED configuration meaning "this tool does not remember selections",
// so forgetting it here would quietly delete a feature.
//
// The field count is asserted for the same reason: a seam added to
// launch.Service and not wired here would otherwise be nil in production
// while every test in this package still passed.
func TestNewServiceWiresEverySeam(t *testing.T) {
	svc := newService(nil)

	for _, f := range []struct {
		name string
		set  bool
	}{
		{"LoadCatalog", svc.LoadCatalog != nil},
		{"APIKey", svc.APIKey != nil},
		{"RecordSelection", svc.RecordSelection != nil},
		{"StageDir", svc.StageDir != nil},
	} {
		if !f.set {
			t.Errorf("newService left %s nil", f.name)
		}
	}

	// Run and RunWait are deliberately nil: agent.Run and agent.RunWait are
	// the right defaults, because a process handoff is a syscall rather than
	// a policy this tool gets to have an opinion about.
	const wantFields = 6
	if got := reflect.TypeOf(launch.Service{}).NumField(); got != wantFields {
		t.Errorf("launch.Service has %d fields, this test knows about %d; "+
			"a new seam must be wired in newService or deliberately left to its default",
			got, wantFields)
	}
}
