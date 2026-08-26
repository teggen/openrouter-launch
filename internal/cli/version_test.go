package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/teggen/agentlaunch/launch"
	"github.com/teggen/openrouter-launch/internal/version"
)

// TestRootPrintsVersion drives the real flag, not the struct field: cobra
// only synthesises --version when Command.Version is non-empty, so asserting
// on the field alone would pass even if the flag never reached the user.
// --version short-circuits before RunE, so this makes no API call.
func TestRootPrintsVersion(t *testing.T) {
	root := NewRootCmdWith(&launch.Service{})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("--version returned an error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, version.String()) {
		t.Errorf("--version printed %q, want it to contain %q", got, version.String())
	}
}
