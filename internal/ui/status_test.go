package ui

import (
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
)

// stubLauncher satisfies agent.Launcher so a test can build a Spec.
// Spec.Launcher must never be nil (Landmine 10).
type stubLauncher struct{ name string }

func (s stubLauncher) Name() string        { return s.name }
func (s stubLauncher) DisplayName() string { return s.name }
func (s stubLauncher) Command(agent.Request) (agent.Command, error) {
	return agent.Command{}, nil
}

func spec(supported bool) *agent.Spec {
	return &agent.Spec{
		Name:     "x",
		Launcher: stubLauncher{name: "X"},
		Status:   agent.Status{Supported: supported, Reason: "because"},
	}
}

func TestAgentStatusVocabulary(t *testing.T) {
	cases := []struct {
		name      string
		spec      *agent.Spec
		installed bool
		wantText  string
		wantRole  Role
	}{
		{"installed", spec(true), true, "✓ installed", RoleOK},
		{"not installed", spec(true), false, "✗ not installed", RoleBad},
		// Unsupported wins over installed-ness: the binary may well be on
		// the machine, but it still cannot be pointed at OpenRouter.
		{"unsupported beats installed", spec(false), true, "⚠ unsupported", RoleWarn},
		{"unsupported", spec(false), false, "⚠ unsupported", RoleWarn},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, role := AgentStatus(c.spec, c.installed)
			if text != c.wantText {
				t.Errorf("text = %q, want %q", text, c.wantText)
			}
			if role != c.wantRole {
				t.Errorf("role = %v, want %v", role, c.wantRole)
			}
		})
	}
}

func TestUnknownAgentStatus(t *testing.T) {
	text, role := UnknownAgentStatus()
	if text != "⚠ unknown agent" {
		t.Errorf("text = %q, want %q", text, "⚠ unknown agent")
	}
	if role != RoleWarn {
		t.Errorf("role = %v, want RoleWarn", role)
	}
}

// Status must never depend on color alone: piped output, NO_COLOR, a dumb
// terminal, and a red/green-colorblind reader all lose the color and keep
// the glyph.
func TestEveryStatusCarriesAGlyph(t *testing.T) {
	var texts []string
	for _, s := range []*agent.Spec{spec(true), spec(false)} {
		for _, installed := range []bool{true, false} {
			text, _ := AgentStatus(s, installed)
			texts = append(texts, text)
		}
	}
	unknown, _ := UnknownAgentStatus()
	texts = append(texts, unknown)

	for _, text := range texts {
		if !strings.ContainsAny(text, "✓✗⚠") {
			t.Errorf("status %q carries no glyph, so it depends on color alone", text)
		}
	}
}
