package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// noticeInput is an error the user must read before going back. The planner's
// typed errors carry their payload as fields — NotInstalledError.Hint,
// UnknownModelError.Suggestions, UnsupportedAgentError.Reason — so callers
// render them as lines rather than as one formatted error string.
type noticeInput struct {
	Title string
	Lines []string
}

type noticeModel struct {
	in   noticeInput
	done bool
}

func newNoticeModel(in noticeInput) noticeModel { return noticeModel{in: in} }

func (m noticeModel) Init() tea.Cmd { return nil }

func (m noticeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "enter", "esc", "ctrl+c", "q":
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m noticeModel) View() string {
	var b strings.Builder
	b.WriteString(warnStyle.Render(m.in.Title) + "\n\n")
	for _, line := range m.in.Lines {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("  enter: back") + "\n")
	return b.String()
}
