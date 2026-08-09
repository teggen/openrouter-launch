package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/ui"
)

var (
	// theme is bound to os.Stdout because that is where bubbletea renders.
	// Under `go test` stdout is a pipe, so a View comes back free of escape
	// codes and string assertions keep working without any test setup.
	theme = ui.NewTheme(os.Stdout)

	titleStyle    = lipgloss.NewStyle().Bold(true)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "249"})
	selectedStyle = lipgloss.NewStyle().Bold(true)
	// Sourced from the shared palette so the screens and the tables cannot
	// drift apart. Same values these carried before, now declared once.
	dimStyle  = theme.Style(ui.RoleDim)
	warnStyle = theme.Style(ui.RoleWarn).Bold(true)
)

// cursorGutter renders the selection marker. Both branches are the same
// width so rows do not shift horizontally as the cursor moves.
func cursorGutter(selected bool) string {
	if selected {
		return "› "
	}
	return "  "
}

// cursorCell is the same marker for a table's leading column, where the
// table itself supplies the padding. One column wide either way, so the
// columns to its right do not shift as the cursor moves.
func cursorCell(selected bool) string {
	if selected {
		return "›"
	}
	return " "
}

// modelRow renders one catalog row:
//
//	anthropic/claude-opus-4.6      200k     $15.00/$75.00    tools
func modelRow(m openrouter.Model) string {
	tools := "     "
	if m.SupportsTools {
		tools = "tools"
	}
	return fmt.Sprintf("%-38s %7s  %8s/%-8s %s",
		truncate(m.ID, 38),
		openrouter.FormatContext(m.ContextLength),
		openrouter.FormatPrice(m.PromptPricePerM, m.PricingUnknown),
		openrouter.FormatPrice(m.CompletionPricePerM, m.PricingUnknown),
		tools)
}

// clampRow truncates row to fit within width, once the 2-column indent and
// 2-column cursor gutter it is about to be wrapped in are accounted for.
// This does not lean on the renderer to cut a too-wide row for us — bubbletea
// already truncates every line to the terminal width on its own — it exists
// so an overflowing row gets an explicit "…" marker instead of being cut
// silently. avail <= 0 (width <= 4, including width == 0 before the first
// WindowSizeMsg, when the terminal size is not yet known) means there is no
// room left after the indent and gutter, so the row renders at its natural
// width instead of being clamped to nothing.
func clampRow(row string, width int) string {
	avail := width - 4
	if avail <= 0 {
		return row
	}
	return truncate(row, avail)
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
