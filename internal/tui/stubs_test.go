package tui

import (
	"errors"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// stubLauncher stands in for a real agent so tests never depend on what is
// installed on the machine running them: agent.Claude consults PATH and the
// user's home directory, and Claude Code is genuinely installed on the
// development machine.
//
// Every method has a POINTER receiver. The capability interfaces are detected
// by type assertion, and a value receiver on a *Spec.Launcher holding a
// pointer would still satisfy them, but a pointer receiver on a value would
// not — the assertion would silently fail and route the test through the
// wrong branch. This exact mistake was caught in Phase 2's Task 4.
type stubLauncher struct {
	name        string
	display     string
	installed   bool
	installHint string
	compatErr   error
	command     agent.Command
	commandErr  error
}

func (s *stubLauncher) Name() string        { return s.name }
func (s *stubLauncher) DisplayName() string { return s.display }

func (s *stubLauncher) Command(agent.Request) (agent.Command, error) {
	if s.commandErr != nil {
		return agent.Command{}, s.commandErr
	}
	return s.command, nil
}

// Installable
func (s *stubLauncher) CheckInstalled() bool { return s.installed }
func (s *stubLauncher) InstallHint() string  { return s.installHint }

// Compatible
func (s *stubLauncher) CheckModel(openrouter.Model) error { return s.compatErr }

// stubSpec builds a supported, installed agent spec.
func stubSpec(name string) *agent.Spec {
	return &agent.Spec{
		Name: name,
		Launcher: &stubLauncher{
			name: name, display: name, installed: true,
			command: agent.Command{Path: "/bin/" + name, Args: []string{name}},
		},
		Status: agent.Status{Supported: true},
	}
}

// unsupportedSpec builds an agent that cannot be pointed at OpenRouter.
func unsupportedSpec(name, reason string) *agent.Spec {
	s := stubSpec(name)
	s.Status = agent.Status{Supported: false, Reason: reason}
	return s
}

// platformBlockedLauncher additionally implements agent.PlatformSupported,
// reporting the agent as unable to run on this platform — the guard
// launch.Plan checks right after CheckSupported, distinct from
// unsupportedSpec's Status.Supported guard.
type platformBlockedLauncher struct {
	stubLauncher
	reason string
}

func (p *platformBlockedLauncher) Supported() error { return errors.New(p.reason) }

// platformBlockedSpec builds a supported (Status.Supported true), installed
// agent whose launcher itself refuses to run on this platform.
func platformBlockedSpec(name, reason string) *agent.Spec {
	return &agent.Spec{
		Name: name,
		Launcher: &platformBlockedLauncher{
			stubLauncher: stubLauncher{
				name: name, display: name, installed: true,
				command: agent.Command{Path: "/bin/" + name, Args: []string{name}},
			},
			reason: reason,
		},
		Status: agent.Status{Supported: true},
	}
}

var (
	_ agent.Launcher          = (*stubLauncher)(nil)
	_ agent.Installable       = (*stubLauncher)(nil)
	_ agent.Compatible        = (*stubLauncher)(nil)
	_ agent.PlatformSupported = (*platformBlockedLauncher)(nil)
)
