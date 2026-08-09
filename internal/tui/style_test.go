package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTruncateLeavesShortStringsAlone(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate = %q, want %q", got, "short")
	}
}

func TestTruncateMarksWhatItCut(t *testing.T) {
	got := truncate("abcdefghij", 5)
	if len([]rune(got)) != 5 {
		t.Errorf("truncate returned %d runes, want 5", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncate = %q, want a trailing ellipsis", got)
	}
}

// Model IDs are ASCII today, but descriptions are not. Counting bytes would
// cut a multibyte rune in half and emit invalid UTF-8 into the terminal.
func TestTruncateCountsRunesNotBytes(t *testing.T) {
	got := truncate("ααααααααα", 5)
	if len([]rune(got)) != 5 {
		t.Errorf("truncate returned %d runes, want 5", len([]rune(got)))
	}
}

// The fixed-height guarantee. OpenRouter descriptions run to several
// paragraphs; if this returned a variable number of lines, the model list
// would reflow on every cursor move.
func TestDescriptionLinesAlwaysReturnsExactlyN(t *testing.T) {
	cases := []string{
		"",
		"short",
		strings.Repeat("word ", 200),
	}
	for _, in := range cases {
		if got := descriptionLines(in, 40, 2); len(got) != 2 {
			t.Errorf("descriptionLines(%d chars) returned %d lines, want 2",
				len(in), len(got))
		}
	}
}

func TestDescriptionLinesWrapsAtTheGivenWidth(t *testing.T) {
	for _, line := range descriptionLines(strings.Repeat("word ", 50), 20, 2) {
		if len([]rune(line)) > 20 {
			t.Errorf("line %q is %d runes, want at most 20", line, len([]rune(line)))
		}
	}
}

func TestDescriptionLinesMarksTruncatedText(t *testing.T) {
	got := descriptionLines(strings.Repeat("word ", 200), 40, 2)
	if !strings.HasSuffix(got[len(got)-1], "…") {
		t.Errorf("last line = %q, want a trailing ellipsis", got[len(got)-1])
	}
}

// Both markers must occupy the same number of display columns, or the
// table's first column resizes as the cursor moves and every column to its
// right shifts with it.
func TestCursorCellWidthIsStable(t *testing.T) {
	if a, b := lipgloss.Width(cursorCell(true)), lipgloss.Width(cursorCell(false)); a != b {
		t.Errorf("marker widths differ: selected=%d unselected=%d", a, b)
	}
}
