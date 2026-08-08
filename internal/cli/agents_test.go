package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
)

func claudeStatusField(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "claude") {
			continue
		}
		fields := regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(line), -1)
		if len(fields) < 3 {
			t.Fatalf("unexpected row shape: %q", line)
		}
		return fields[2]
	}
	t.Fatalf("no claude row in output:\n%s", out)
	return ""
}

func TestAgentsCommandListsClaude(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "agents")
	if !strings.Contains(got, "claude") {
		t.Errorf("output missing claude:\n%s", got)
	}
	if !strings.Contains(got, "Claude Code") {
		t.Errorf("output missing display name:\n%s", got)
	}
}

func TestAgentsCommandShowsInstalledWhenBinaryFound(t *testing.T) {
	spec, err := agent.Lookup("claude")
	if err != nil {
		t.Fatalf("lookup claude: %v", err)
	}
	claude := spec.Launcher.(*agent.Claude)
	prev := claude.LookPath
	claude.LookPath = func(string) (string, error) { return "/usr/local/bin/claude", nil }
	t.Cleanup(func() { claude.LookPath = prev })

	h := newHarness(t)
	got := h.run(t, "agents")
	status := claudeStatusField(t, got)
	if status != "installed" {
		t.Errorf("expected status %q, got %q", "installed", status)
	}
}

func TestAgentsCommandShowsNotInstalledWhenBinaryNotFound(t *testing.T) {
	spec, err := agent.Lookup("claude")
	if err != nil {
		t.Fatalf("lookup claude: %v", err)
	}
	claude := spec.Launcher.(*agent.Claude)
	prev := claude.LookPath
	claude.LookPath = func(string) (string, error) { return "", agent.ErrUnknownAgent }
	t.Cleanup(func() { claude.LookPath = prev })
	t.Setenv("HOME", t.TempDir())

	h := newHarness(t)
	got := h.run(t, "agents")
	status := claudeStatusField(t, got)
	if status != "not installed" {
		t.Errorf("expected status %q, got %q", "not installed", status)
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

func TestYesFlagHasShorthand(t *testing.T) {
	root := NewRootCmd()
	if root.PersistentFlags().ShorthandLookup("y") == nil {
		t.Errorf("missing shorthand -y for --yes flag")
	}
}
