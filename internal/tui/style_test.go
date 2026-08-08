package tui

import (
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/openrouter"
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

func TestModelRowShowsIDContextAndBothPrices(t *testing.T) {
	got := modelRow(openrouter.Model{
		ID: "anthropic/claude-opus-4.6", ContextLength: 200000,
		PromptPricePerM: 15, CompletionPricePerM: 75, SupportsTools: true,
	})
	for _, want := range []string{"anthropic/claude-opus-4.6", "200k", "$15.00", "$75.00", "tools"} {
		if !strings.Contains(got, want) {
			t.Errorf("modelRow = %q, missing %q", got, want)
		}
	}
}

func TestModelRowOmitsTheToolsMarkerWhenUnsupported(t *testing.T) {
	got := modelRow(openrouter.Model{ID: "openai/o1-mini", ContextLength: 128000})
	if strings.Contains(got, "tools") {
		t.Errorf("modelRow = %q, marks a tool-less model as supporting tools", got)
	}
}

// Landmine 4 at the render layer: a model whose price failed to parse is not
// a free model, and rendering it as "free" would be an actively wrong claim
// about what a launch costs.
func TestModelRowNeverRendersUnknownPricingAsFree(t *testing.T) {
	got := modelRow(openrouter.Model{ID: "x/y", ContextLength: 1000, PricingUnknown: true})
	if strings.Contains(got, "free") {
		t.Errorf("modelRow = %q, renders unknown pricing as free", got)
	}
	if !strings.Contains(got, "?") {
		t.Errorf("modelRow = %q, want %q for unknown pricing", got, "?")
	}
}

// Both gutters must be the same width or every row shifts horizontally as
// the cursor moves through the list.
func TestCursorGutterWidthIsStable(t *testing.T) {
	if a, b := len([]rune(cursorGutter(true))), len([]rune(cursorGutter(false))); a != b {
		t.Errorf("gutter widths differ: selected=%d unselected=%d", a, b)
	}
}
