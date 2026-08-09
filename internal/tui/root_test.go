package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
)

func allInstalled(*agent.Spec) bool { return true }

// threeAgents holds an unsupported agent in the MIDDLE on purpose: it is
// what TestRootOmitsUnsupportedAgents has to omit, and putting it between
// two supported agents means a filter that dropped the wrong element (an
// off-by-one on the index, say) changes which agents survive rather than
// only how many. It used to be positioned so cursor movement had an
// unselectable row to step over; that row no longer renders.
func threeAgents() []*agent.Spec {
	return []*agent.Spec{
		stubSpec("claude"),
		unsupportedSpec("copilot", "cannot be pointed at a custom endpoint"),
		stubSpec("codex"),
	}
}

func rootFixture(profiles []config.Profile, lastAgent string) rootModel {
	return newRootModel(rootInput{
		Profiles:  profiles,
		Agents:    threeAgents(),
		Installed: allInstalled,
		LastAgent: lastAgent,
	})
}

// The section labels are uppercase, matching the table headers.
//
// TestRootOmitsTheProfilesHeaderWhenThereAreNone asserts an ABSENCE, so it
// had to be updated in lockstep with this: left looking for "Profiles" it
// would pass against a screen that says "PROFILES" while testing nothing.
func TestRootListsProfilesBeforeAgents(t *testing.T) {
	m := rootFixture([]config.Profile{{Name: "opus-cc", Agent: "claude", Model: "anthropic/x"}}, "")
	got := m.View()

	pi, ai := strings.Index(got, "PROFILES"), strings.Index(got, "AGENTS")
	if pi < 0 || ai < 0 {
		t.Fatalf("View = %q, missing a section label", got)
	}
	if pi > ai {
		t.Error("the Agents section renders before Profiles")
	}
}

func TestRootOmitsTheProfilesHeaderWhenThereAreNone(t *testing.T) {
	if got := rootFixture(nil, "").View(); strings.Contains(got, "PROFILES") {
		t.Errorf("View = %q, shows an empty Profiles section", got)
	}
}

func TestRootRendersRowsInsideABorderedTable(t *testing.T) {
	got := rootFixture(nil, "").View()
	for _, want := range []string{"╭", "│", "├", "╰", "NAME", "AGENT", "STATUS"} {
		if !strings.Contains(got, want) {
			t.Errorf("View is missing %q:\n%s", want, got)
		}
	}
}

// The marker must be in the first column of the SELECTED row and nowhere
// else. Asserting only that "›" appears somewhere would pass for a marker
// stapled to a fixed row.
func TestRootCursorMarkerTracksTheSelection(t *testing.T) {
	m := rootFixture(nil, "")
	if got := markedRow(t, m.View()); got != "claude" {
		t.Errorf("marker on %q, want claude", got)
	}

	m.move(1)
	if got := markedRow(t, m.View()); got != "codex" {
		t.Errorf("after moving down the marker is on %q, want codex", got)
	}
}

// markedRow returns the NAME cell of the row carrying the cursor marker,
// failing unless exactly one row carries it.
func markedRow(t *testing.T, view string) string {
	t.Helper()

	var found []string
	for _, line := range strings.Split(view, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "│") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "│"), "│")
		if len(cells) > 1 && strings.TrimSpace(cells[0]) == "›" {
			found = append(found, strings.TrimSpace(cells[1]))
		}
	}
	if len(found) != 1 {
		t.Fatalf("found %d marked rows (%v), want exactly 1:\n%s", len(found), found, view)
	}
	return found[0]
}

// Section headers are rendered rows but must never be landed on.
func TestRootCursorSkipsSectionHeaders(t *testing.T) {
	m := rootFixture([]config.Profile{{Name: "p1", Agent: "claude", Model: "m"}}, "")
	// Starts on the profile; one press down must clear the "Agents" header
	// and land on the first agent.
	m = press(t, m, typeKey(tea.KeyDown), typeKey(tea.KeyEnter))

	if m.choice.Kind != choiceAgent {
		t.Fatalf("choice kind = %v, want an agent (cursor landed on a header)", m.choice.Kind)
	}
	if m.choice.Agent.Name != "claude" {
		t.Errorf("selected %q, want claude", m.choice.Agent.Name)
	}
}

// An agent that cannot be pointed at OpenRouter is not listed at all: the
// root screen exists to pick something to launch, and its reason is still
// reported by `openrouter-launch <agent>`, whose subcommand is unaffected.
//
// This replaces TestRootCursorSkipsUnsupportedAgents, which asserted the
// cursor stepped OVER the unsupported row. That assertion survives the
// filter unchanged — with copilot gone, Down still lands on codex — so
// keeping it would have left a test that passes for the wrong reason. The
// assertion below cannot: it fails on the old code, where the row renders.
func TestRootOmitsUnsupportedAgents(t *testing.T) {
	m := rootFixture(nil, "")
	got := m.View()

	if strings.Contains(got, "copilot") {
		t.Errorf("View lists the unsupported agent:\n%s", got)
	}
	if strings.Contains(got, "cannot be pointed at a custom endpoint") {
		t.Errorf("View still renders the unsupported reason:\n%s", got)
	}
	// The filter must drop only the unsupported one.
	for _, want := range []string{"claude", "codex"} {
		if !strings.Contains(got, want) {
			t.Errorf("View dropped the supported agent %q:\n%s", want, got)
		}
	}
}

func TestRootPreselectsLastAgent(t *testing.T) {
	// codex is the third agent, so this differs from the default of first
	// selectable — a preselect that did nothing would fail.
	m := rootFixture(nil, "codex")
	m = press(t, m, typeKey(tea.KeyEnter))

	if m.choice.Agent == nil || m.choice.Agent.Name != "codex" {
		t.Errorf("enter selected %v, want the preselected codex", m.choice.Agent)
	}
}

func TestRootPreselectsFirstSelectableWhenLastAgentIsUnknown(t *testing.T) {
	m := rootFixture(nil, "no-such-agent")
	m = press(t, m, typeKey(tea.KeyEnter))

	if m.choice.Agent == nil || m.choice.Agent.Name != "claude" {
		t.Errorf("enter selected %v, want the first selectable agent", m.choice.Agent)
	}
}

func TestRootPreselectsAProfileNeverOverridingLastAgent(t *testing.T) {
	m := rootFixture([]config.Profile{{Name: "p1", Agent: "claude", Model: "m"}}, "codex")
	m = press(t, m, typeKey(tea.KeyEnter))

	if m.choice.Kind != choiceAgent {
		t.Errorf("last_agent names an agent but a profile was preselected")
	}
}

// Plan checks the empty model BEFORE the install guard, deliberately, so a
// user with nothing installed still reaches the picker. Making an uninstalled
// agent unselectable here would undo that.
func TestRootUninstalledAgentIsStillSelectable(t *testing.T) {
	m := newRootModel(rootInput{
		Agents:    []*agent.Spec{stubSpec("claude")},
		Installed: func(*agent.Spec) bool { return false },
	})
	m = press(t, m, typeKey(tea.KeyEnter))

	if m.choice.Kind != choiceAgent {
		t.Error("an uninstalled agent was not selectable")
	}
	if got := m.View(); !strings.Contains(got, "not installed") {
		t.Errorf("View = %q, does not show the install state", got)
	}
}

func TestRootEnterOnAProfileReturnsThatProfile(t *testing.T) {
	want := config.Profile{Name: "opus-cc", Agent: "claude", Model: "anthropic/x", Args: []string{"--resume"}}
	m := rootFixture([]config.Profile{want}, "")
	m = press(t, m, typeKey(tea.KeyEnter))

	if m.choice.Kind != choiceProfile {
		t.Fatalf("choice kind = %v, want a profile", m.choice.Kind)
	}
	if m.choice.Profile.Name != want.Name || len(m.choice.Profile.Args) != 1 {
		t.Errorf("choice.Profile = %+v, want %+v", m.choice.Profile, want)
	}
}

func TestRootEscCancels(t *testing.T) {
	m := press(t, rootFixture(nil, ""), typeKey(tea.KeyEsc))
	if !m.done || m.choice.Kind != choiceCancel {
		t.Errorf("done=%v kind=%v, want done and cancel", m.done, m.choice.Kind)
	}
}

// threeAgents has only two selectable rows (claude, codex), so under
// wraparound semantics they'd form a 2-cycle: pressing Up or Down any number
// of times would land on one of the same two rows a stopping implementation
// would reach, and no press count could tell the two semantics apart. A
// single boundary press is the only case where they diverge — stopping is a
// no-op, wrapping jumps to the opposite end — so that is what this test
// asserts.
func TestRootCursorStopsAtBothEnds(t *testing.T) {
	m := rootFixture(nil, "")
	// claude is the first selectable row; Up must be a no-op, not a wrap to codex.
	m = press(t, m, typeKey(tea.KeyUp), typeKey(tea.KeyEnter))
	if m.choice.Agent == nil || m.choice.Agent.Name != "claude" {
		t.Errorf("up past the top selected %v, want the first agent", m.choice.Agent)
	}

	m2 := rootFixture(nil, "codex")
	// codex is the last selectable row; Down must be a no-op, not a wrap to claude.
	m2 = press(t, m2, typeKey(tea.KeyDown), typeKey(tea.KeyEnter))
	if m2.choice.Agent == nil || m2.choice.Agent.Name != "codex" {
		t.Errorf("down past the bottom selected %v, want the last agent", m2.choice.Agent)
	}
}

func TestRootHandlesVimKeys(t *testing.T) {
	m := press(t, rootFixture(nil, ""), runeKey('j'), typeKey(tea.KeyEnter))
	if m.choice.Agent == nil || m.choice.Agent.Name != "codex" {
		t.Errorf("j selected %v, want the same as down", m.choice.Agent)
	}
}
