package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// promptInput configures the one-line input screen.
type promptInput struct {
	Title string
	Label string
	// Masked renders the value as bullets. Set for the API key.
	Masked bool
	// Validate rejects a value with an error rendered inline, keeping the
	// user in the prompt so they can correct it. nil accepts anything.
	Validate func(string) error
}

type promptModel struct {
	in        promptInput
	value     string
	errMsg    string
	submitted bool
	cancelled bool
}

func newPromptModel(in promptInput) promptModel { return promptModel{in: in} }

func (m promptModel) Init() tea.Cmd { return nil }

func (m promptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "enter":
		if m.in.Validate != nil {
			if err := m.in.Validate(m.value); err != nil {
				// Stay in the prompt: a rejection the user cannot correct
				// without reopening the screen is a dead end.
				m.errMsg = err.Error()
				return m, nil
			}
		}
		m.submitted = true
		return m, tea.Quit
	case "esc", "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	case "backspace":
		if r := []rune(m.value); len(r) > 0 {
			m.value = string(r[:len(r)-1])
			m.errMsg = ""
		}
		return m, nil
	}

	// Anything else that produces text goes into the field. The !Alt guard
	// keeps a chord from typing its letter.
	switch {
	case key.Type == tea.KeyRunes && !key.Alt:
		m.value += string(key.Runes)
		m.errMsg = ""
	case key.Type == tea.KeySpace:
		m.value += " "
		m.errMsg = ""
	}
	return m, nil
}

func (m promptModel) View() string {
	shown := m.value
	if m.in.Masked {
		shown = strings.Repeat("•", runeLen(m.value))
	}

	var b strings.Builder
	if m.in.Title != "" {
		b.WriteString(titleStyle.Render(m.in.Title) + "\n\n")
	}
	b.WriteString("  " + m.in.Label + ": " + shown + "▎\n")
	if m.errMsg != "" {
		b.WriteString("\n  " + warnStyle.Render(m.errMsg) + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("  enter: accept · esc: cancel") + "\n")
	return b.String()
}
