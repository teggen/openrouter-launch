package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// confirmInput is what the confirm screen renders.
type confirmInput struct {
	Title string
	// Lines are the warning messages, rendered in order, already carrying
	// their "warning: " prefix. The driver adds it, because the driver is
	// what knows these are warnings.
	Lines []string
	// Question is the planner's own wording, carried on the Warning so a
	// caller cannot ask "Launch anyway?" about a warning that is not about
	// launching. Empty puts the screen in acknowledge mode.
	Question string
}

type confirmModel struct {
	in     confirmInput
	answer bool
	done   bool
}

func newConfirmModel(in confirmInput) confirmModel { return confirmModel{in: in} }

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.in.Question == "" {
		// Acknowledge mode: there is nothing to answer, only to read.
		switch key.String() {
		case "enter":
			m.answer, m.done = true, true
			return m, tea.Quit
		case "esc", "ctrl+c", "q":
			m.answer, m.done = false, true
			return m, tea.Quit
		}
		return m, nil
	}

	// Question mode defaults to no, matching the CLI's [y/N] prompt: a stray
	// keypress must never launch against a model the user was being warned
	// about.
	switch key.String() {
	case "y", "Y":
		m.answer, m.done = true, true
		return m, tea.Quit
	case "n", "N", "esc", "ctrl+c", "enter":
		m.answer, m.done = false, true
		return m, tea.Quit
	}
	return m, nil
}

func (m confirmModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.in.Title) + "\n\n")
	for _, line := range m.in.Lines {
		b.WriteString("  " + warnStyle.Render(line) + "\n")
	}
	b.WriteString("\n")
	if m.in.Question == "" {
		b.WriteString(dimStyle.Render("  enter: launch · esc: back") + "\n")
		return b.String()
	}
	b.WriteString("  " + m.in.Question + " " + dimStyle.Render("[y/N]") + "\n")
	return b.String()
}
