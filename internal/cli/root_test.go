package cli

import "testing"

// TestNewRootCmdWithPanicsOnNilService pins that a nil *launch.Service is
// caught at construction rather than building a command tree that panics
// later, on first use, when a subcommand's RunE dereferences the nil
// receiver (e.g. Snapshot). Failing at construction makes the bug
// immediately diagnosable, mirroring agent.buildIndex's nil-Launcher guard.
func TestNewRootCmdWithPanicsOnNilService(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for a nil *launch.Service")
		}
	}()
	NewRootCmdWith(nil)
}
