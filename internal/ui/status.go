package ui

import "github.com/teggen/agentlaunch/agent"

// The glyph is ALWAYS emitted, so a status never depends on color alone —
// it survives NO_COLOR, a pipe, a dumb terminal, and a reader who cannot
// tell red from green.
const (
	glyphOK   = "✓"
	glyphBad  = "✗"
	glyphWarn = "⚠"
)

// AgentStatus is the single source of the status vocabulary. Both the CLI
// listing and the TUI root screen call it, so the two cannot disagree about
// what a state is called or how it is colored.
//
// Unsupported outranks installed-ness on purpose: the binary may well be
// present, but an agent that cannot be pointed at OpenRouter still cannot
// be launched by this tool, and reporting it as "installed" would be a
// wrong claim about what the user can do next.
func AgentStatus(spec *agent.Spec, installed bool) (string, Role) {
	switch {
	case !spec.Status.Supported:
		return glyphWarn + " unsupported", RoleWarn
	case installed:
		return glyphOK + " installed", RoleOK
	default:
		return glyphBad + " not installed", RoleBad
	}
}

// UnknownAgentStatus is the cell for a saved profile naming an agent that is
// not in the registry. It is reachable only from a hand-edited config or an
// agent dropped between releases — `profile add` validates the name — but
// without it that failure stays invisible until launch time.
func UnknownAgentStatus() (string, Role) { return glyphWarn + " unknown agent", RoleWarn }
