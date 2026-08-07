package cli

import (
	"bytes"
	"strings"
	"testing"
)

func runCmd(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return out.String()
}

func TestAgentsCommandListsClaude(t *testing.T) {
	got := runCmd(t, "agents")
	if !strings.Contains(got, "claude") {
		t.Errorf("output missing claude:\n%s", got)
	}
	if !strings.Contains(got, "Claude Code") {
		t.Errorf("output missing display name:\n%s", got)
	}
}

func TestAgentsCommandShowsInstallState(t *testing.T) {
	got := runCmd(t, "agents")
	if !strings.Contains(got, "installed") && !strings.Contains(got, "not installed") {
		t.Errorf("output missing install state:\n%s", got)
	}
}

func TestRootHasPersistentFlags(t *testing.T) {
	root := NewRootCmd()
	for _, name := range []string{"refresh", "yes"} {
		if root.PersistentFlags().Lookup(name) == nil {
			t.Errorf("missing persistent flag --%s", name)
		}
	}
}
