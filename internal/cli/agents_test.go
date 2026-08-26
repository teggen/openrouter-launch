package cli

import (
	"strings"
	"testing"

	"github.com/teggen/agentlaunch/agent"
	"github.com/teggen/openrouter-launch/internal/ui"
)

// wideLauncher is a stub whose description is far longer than any real
// agent's. Landmine 10: Spec.Launcher must never be nil.
type wideLauncher struct{}

func (wideLauncher) Name() string        { return "wide" }
func (wideLauncher) DisplayName() string { return "Wide Agent" }
func (wideLauncher) Command(agent.Request) (agent.Command, error) {
	return agent.Command{}, nil
}

func claudeStatusField(t *testing.T, out string) string {
	t.Helper()
	return tableRow(t, out, "claude")[2]
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
	h := newHarness(t)
	h.stubClaudePath(t)

	if got := claudeStatusField(t, h.run(t, "agents")); got != "✓ installed" {
		t.Errorf("status = %q, want %q", got, "✓ installed")
	}
}

func TestAgentsCommandShowsNotInstalledWhenBinaryNotFound(t *testing.T) {
	testHome(t) // Landmine 8

	h := newHarness(t)
	claude := h.mustLookup(t, "claude").Launcher.(*agent.Claude)
	claude.LookPath = func(string) (string, error) { return "", agent.ErrUnknownAgent }

	if got := claudeStatusField(t, h.run(t, "agents")); got != "✗ not installed" {
		t.Errorf("status = %q, want %q", got, "✗ not installed")
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

// The reason wraps across several lines inside its cell, so this asserts on
// the REASON column reconstructed by tableRows, not on a raw substring that
// only survives at one particular column width.
func TestAgentsAllShowsUnsupportedWithReason(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "agents", "--all")

	for _, name := range desktopApps {
		row := tableRow(t, out, name)
		if row[2] != "⚠ unsupported" {
			t.Errorf("%s status = %q, want %q", name, row[2], "⚠ unsupported")
		}
		if !strings.Contains(row[3], "desktop app authenticates through its own account") {
			t.Errorf("%s reason = %q, want the full reason", name, row[3])
		}
	}

	// A supported agent keeps its description in that same column, so the
	// fold cannot be satisfied by putting the reason everywhere.
	if got := tableRow(t, out, "claude")[3]; got != "Anthropic's coding tool with subagents" {
		t.Errorf("claude's description column = %q, want its description", got)
	}
}

// --all is where the 227-column line came from. It must wrap now instead.
func TestAgentsAllWrapsRatherThanWidening(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "agents", "--all")

	for _, line := range strings.Split(out, "\n") {
		if n := len([]rune(line)); n > ui.MaxTableWidth {
			t.Errorf("line is %d columns, want <= %d:\n%s", n, ui.MaxTableWidth, line)
		}
	}
	// A wrapped table must carry row rules, or the multi-line rows read as
	// one block. One "├" is the header rule; more means rules between rows.
	if got := strings.Count(out, "├"); got < 2 {
		t.Errorf("wrapped table has %d rules, want rules between rows:\n%s", got, out)
	}
}

// TestAgentsOutputStaysNarrow pins the width cap, and it must be fed a
// SYNTHETIC spec to do so.
//
// The previous version rendered the live registry, whose longest
// description leaves the table at 94 columns. Once ui.Table carries a
// MaxWidth, deleting that cap would not have widened the real listing at
// all, so the test would have kept passing while testing nothing. A
// 200-character description is what makes the cap the only reason it
// passes — and it is why agentsTable takes its specs as an argument.
func TestAgentsOutputStaysNarrow(t *testing.T) {
	specs := []*agent.Spec{{
		Name:        "wide",
		Launcher:    wideLauncher{},
		Description: strings.Repeat("extremely verbose description ", 7),
		Status:      agent.Status{Supported: true},
	}}

	out := ui.NewTheme(new(strings.Builder)).Render(
		agentsTable(specs, func(*agent.Spec) bool { return true }, false))

	for _, line := range strings.Split(out, "\n") {
		if n := len([]rune(line)); n > ui.MaxTableWidth {
			t.Errorf("line is %d columns, want <= %d:\n%s", n, ui.MaxTableWidth, line)
		}
	}
	// The cap must not have swallowed the row entirely.
	if !strings.Contains(out, "wide") {
		t.Errorf("capped table lost its row:\n%s", out)
	}
}

// Every CLI test writes to a buffer, and so does every pipe. Escapes there
// would land in files and greps.
func TestListingsEmitNoEscapesWhenNotATerminal(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{"agents"}, {"agents", "--all"}} {
		if got := h.run(t, args...); strings.Contains(got, "\x1b") {
			t.Errorf("%v emitted ANSI escapes to a buffer:\n%q", args, got)
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
