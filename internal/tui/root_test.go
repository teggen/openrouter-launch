package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
)

func allInstalled(*agent.Spec) bool { return true }

// threeAgents is deliberately ordered supported / unsupported / supported so
// that cursor movement has an unselectable row to step over in the middle.
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

func TestRootListsProfilesBeforeAgents(t *testing.T) {
	m := rootFixture([]config.Profile{{Name: "opus-cc", Agent: "claude", Model: "anthropic/x"}}, "")
	got := m.View()

	pi, ai := strings.Index(got, "Profiles"), strings.Index(got, "Agents")
	if pi < 0 || ai < 0 {
		t.Fatalf("View = %q, missing a section header", got)
	}
	if pi > ai {
		t.Error("the Agents section renders before Profiles")
	}
}

func TestRootOmitsTheProfilesHeaderWhenThereAreNone(t *testing.T) {
	if got := rootFixture(nil, "").View(); strings.Contains(got, "Profiles") {
		t.Errorf("View = %q, shows an empty Profiles section", got)
	}
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

// An agent that cannot be pointed at OpenRouter is shown with its reason but
// must not be selectable, so the cursor steps over it.
func TestRootCursorSkipsUnsupportedAgents(t *testing.T) {
	m := rootFixture(nil, "")
	m = press(t, m, typeKey(tea.KeyDown), typeKey(tea.KeyEnter))

	if m.choice.Agent == nil {
		t.Fatal("no agent selected")
	}
	if m.choice.Agent.Name != "codex" {
		t.Errorf("selected %q, want codex — the cursor stopped on the unsupported agent",
			m.choice.Agent.Name)
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

func TestRootCursorStopsAtBothEnds(t *testing.T) {
	m := rootFixture(nil, "")
	// Up from the first selectable row must not wrap or go negative.
	m = press(t, m, typeKey(tea.KeyUp), typeKey(tea.KeyUp), typeKey(tea.KeyEnter))
	if m.choice.Agent == nil || m.choice.Agent.Name != "claude" {
		t.Errorf("up past the top selected %v, want the first agent", m.choice.Agent)
	}

	m2 := rootFixture(nil, "")
	m2 = press(t, m2, typeKey(tea.KeyDown), typeKey(tea.KeyDown), typeKey(tea.KeyDown), typeKey(tea.KeyEnter))
	if m2.choice.Agent == nil || m2.choice.Agent.Name != "codex" {
		t.Errorf("down past the bottom selected %v, want the last agent", m2.choice.Agent)
	}
}

func TestRootViewShowsUnsupportedAgentsWithTheirReason(t *testing.T) {
	got := rootFixture(nil, "").View()
	if !strings.Contains(got, "cannot be pointed at a custom endpoint") {
		t.Errorf("View = %q, does not explain why copilot is unavailable", got)
	}
}

func TestRootHandlesVimKeys(t *testing.T) {
	m := press(t, rootFixture(nil, ""), runeKey('j'), typeKey(tea.KeyEnter))
	if m.choice.Agent == nil || m.choice.Agent.Name != "codex" {
		t.Errorf("j selected %v, want the same as down", m.choice.Agent)
	}
}
