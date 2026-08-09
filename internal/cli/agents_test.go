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

// desktopApps are the Tier 3 specs registered unsupported-with-a-reason.
// They stay in the registry (and keep their launch subcommand, which is what
// reports the reason) but no longer appear in the default listing.
var desktopApps = []string{"chatgpt", "claude-desktop", "hermes-desktop"}

func TestAgentsHidesUnsupportedByDefault(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "agents")

	for _, name := range desktopApps {
		if strings.Contains(got, name) {
			t.Errorf("default listing still shows the unsupported agent %q:\n%s", name, got)
		}
	}
	if strings.Contains(got, "unsupported") {
		t.Errorf("default listing still carries an unsupported status:\n%s", got)
	}
	// The filter must remove only the unsupported ones — a filter that
	// dropped everything would satisfy the assertions above.
	if !strings.Contains(got, "claude") || !strings.Contains(got, "droid") {
		t.Errorf("filter removed supported agents too:\n%s", got)
	}
}

func TestAgentsAllShowsUnsupportedWithReason(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "agents", "--all")

	for _, name := range desktopApps {
		if !strings.Contains(got, name) {
			t.Errorf("--all is missing the unsupported agent %q:\n%s", name, got)
		}
	}
	if !strings.Contains(got, "desktop app authenticates through its own account") {
		t.Errorf("--all dropped the reason, which is the only thing it exists to show:\n%s", got)
	}
}

// TestAgentsOutputStaysNarrow pins the property the change was actually for.
// tabwriter pads every column to its widest cell, so ONE long status blew
// all 14 rows out to 150-227 columns. Asserting on the rendered width, not
// on which rows are present, is what makes this fail if a future long
// description or status reintroduces the problem by another route.
// --all is deliberately not bounded: printing the full reason is the whole
// point of that flag.
func TestAgentsOutputStaysNarrow(t *testing.T) {
	const maxWidth = 100
	h := newHarness(t)

	for _, line := range strings.Split(strings.TrimRight(h.run(t, "agents"), "\n"), "\n") {
		if len(line) > maxWidth {
			t.Errorf("line is %d columns, want <= %d:\n%s", len(line), maxWidth, line)
		}
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
