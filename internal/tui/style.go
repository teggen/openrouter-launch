package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "249"})
	selectedStyle = lipgloss.NewStyle().Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "242", Dark: "246"})
	warnStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"})
)

// cursorGutter renders the selection marker. Both branches are the same
// width so rows do not shift horizontally as the cursor moves.
func cursorGutter(selected bool) string {
	if selected {
		return "› "
	}
	return "  "
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
// Below roughly 75 columns an unclamped row wraps in the terminal, which
// breaks the picker's fixed-list-height guarantee the scrolling math is
// built on. width <= 0 means the terminal size is not yet known (before the
// first WindowSizeMsg), so the row renders at its natural width instead of
// being clamped to nothing.
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
