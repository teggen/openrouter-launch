package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/teggen/openrouter-launch/internal/ui"
)

var (
	// theme is bound to os.Stdout because that is where bubbletea renders.
	// Under `go test` stdout is a pipe, so a View comes back free of escape
	// codes and string assertions keep working without any test setup.
	theme = ui.NewTheme(os.Stdout)

	titleStyle  = lipgloss.NewStyle().Bold(true)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "249"})
	// Sourced from the shared palette so the screens and the tables cannot
	// drift apart. Same values these carried before, now declared once.
	dimStyle  = theme.Style(ui.RoleDim)
	warnStyle = theme.Style(ui.RoleWarn).Bold(true)
)

// cursorCell renders the selection marker for a table's leading column,
// where the table itself supplies the padding. One column wide either way,
// so the columns to its right do not shift as the cursor moves.
func cursorCell(selected bool) string {
	if selected {
		return "›"
	}
	return " "
}

// truncate shortens s to at most n runes, marking the cut with an ellipsis.
// Runes, not bytes: descriptions are not ASCII, and a byte-wise cut would
// emit invalid UTF-8.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// wrap breaks s into lines of at most width runes on word boundaries. A word
// longer than width is left intact; the caller truncates it.
func wrap(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	var cur string
	for _, word := range strings.Fields(s) {
		switch {
		case cur == "":
			cur = word
		case runeLen(cur)+1+runeLen(word) <= width:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// descriptionLines renders s as exactly n lines of at most width runes,
// padding with blanks and marking overflow with an ellipsis.
//
// Exactly n is the point. OpenRouter descriptions run to several paragraphs
// for some models, and a variable-height pane would reflow the model list on
// every cursor move.
func descriptionLines(s string, width, n int) []string {
	out := make([]string, n)
	lines := wrap(s, width)
	for i := 0; i < n; i++ {
		if i >= len(lines) {
			continue
		}
		if i == n-1 && len(lines) > n {
			out[i] = truncate(lines[i]+" …", width)
			continue
		}
		out[i] = truncate(lines[i], width)
	}
	return out
}
