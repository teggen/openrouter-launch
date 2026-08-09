package cli

import (
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/ui"
)

func TestModelsCommandListsAll(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--tools=false")

	for _, id := range []string{"anthropic/claude-opus-4.6", "qwen/qwen3-coder:free", "openai/o1-mini"} {
		if !strings.Contains(got, id) {
			t.Errorf("output missing %s:\n%s", id, got)
		}
	}
}

func TestModelsCommandDefaultsToToolCapableModels(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models")

	// config.defaults() sets ToolsOnly:true because a coding agent without
	// tool calling is unusable, and openai/o1-mini is the only fixture model
	// without tool support.
	if strings.Contains(got, "openai/o1-mini") {
		t.Errorf("bare `models` should honor the saved tools-only default:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("tool-capable models should still be listed:\n%s", got)
	}
}

func TestModelsCommandExplicitToolsFalseOverridesSavedDefault(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--tools=false")

	// --tools=false and an absent --tools are both false by value; only the
	// Changed() check distinguishes them. If that check is dropped, the
	// persisted true wins and o1-mini stays hidden.
	if !strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--tools=false should override the saved ToolsOnly:true:\n%s", got)
	}
}

// TestModelsCommandToolsFilter seeds a persisted Filters.ToolsOnly:false -
// the opposite of config.defaults() - so an explicit --tools has something
// to actually override. Without this, config.defaults() already sets
// ToolsOnly:true, making bare `models` and `models --tools` byte-identical:
// deleting the changed(FlagTools) branch in MergeFilters would leave this
// test green. An explicit --tools overriding a persisted false is the only
// tools-flag direction with no other CLI-level coverage.
func TestModelsCommandToolsFilter(t *testing.T) {
	h := newHarness(t)
	if err := config.Save(&config.Config{Filters: config.Filters{ToolsOnly: false}}); err != nil {
		t.Fatalf("config.Save: %v", err)
	}

	got := h.run(t, "models", "--tools")

	if strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--tools should exclude o1-mini even though the saved filter is false:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--tools dropped a tool-capable model:\n%s", got)
	}
}

// TestModelsCommandRendersStaleCatalogWarning is the regression test for the
// stale-catalog warning `models` renders via renderWarning. Before task 7
// duplicated the render inline, this was covered through the shared
// loadCatalog by a test in the now-deleted catalog_test.go; only the
// launch-side copy got replacement coverage. This is the only user-facing
// signal that `models` may be listing stale data while offline, so a stale
// catalog must both surface the warning AND still list the (stale) models.
func TestModelsCommandRendersStaleCatalogWarning(t *testing.T) {
	h := newHarnessWith(t, erroringCatalog{})
	seedStaleCache(t)

	got := h.run(t, "models", "--tools=false")

	if !strings.Contains(got, "could not refresh the model catalog") {
		t.Errorf("stderr should contain the stale-catalog warning:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("a stale catalog must not suppress the models listing:\n%s", got)
	}
}

func TestModelsCommandFreeFilter(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--free")

	if !strings.Contains(got, "qwen/qwen3-coder:free") {
		t.Errorf("--free dropped the free model:\n%s", got)
	}
	if strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--free should exclude paid models:\n%s", got)
	}
}

func TestModelsCommandProviderFilter(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--provider", "openai", "--tools=false")

	if !strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--provider dropped the match:\n%s", got)
	}
	if strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--provider should exclude other vendors:\n%s", got)
	}
	if strings.Contains(got, "qwen/qwen3-coder:free") {
		t.Errorf("--provider should exclude qwen models:\n%s", got)
	}
}

// --tools=false matters here even though the assertion is about absence:
// without it, o1-mini would be filtered out by the tools default and this
// test would pass whether or not the min-context guard works.
func TestModelsCommandMinContextFilter(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--min-context", "200000", "--tools=false")

	if strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--min-context should exclude the 128k model:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--min-context should include exactly 200k model:\n%s", got)
	}
	if !strings.Contains(got, "qwen/qwen3-coder:free") {
		t.Errorf("--min-context should include 262k model:\n%s", got)
	}
}

func TestModelsCommandMaxPriceFilter(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--max-price", "5", "--tools=false")

	if strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--max-price should exclude the $75 model:\n%s", got)
	}
	if !strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--max-price dropped a cheap model:\n%s", got)
	}
}

func mustLoadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func TestModelsTableMarksToolSupport(t *testing.T) {
	out := ui.NewTheme(new(strings.Builder)).Render(modelsTable([]openrouter.Model{
		{ID: "a/tools", ContextLength: 200000, SupportsTools: true},
		{ID: "a/plain", ContextLength: 128000},
	}))
	wantColumns(t, out, "MODEL", "CONTEXT", "PROMPT/M", "COMPLETION/M", "TOOLS")

	if got := tableRow(t, out, "a/tools")[4]; got != "✓" {
		t.Errorf("tools cell = %q, want %q", got, "✓")
	}
	if got := tableRow(t, out, "a/plain")[4]; got != "" {
		t.Errorf("tools cell = %q, want empty for a tool-less model", got)
	}
}

// Landmine 4 at the render layer: a model whose price failed to parse is
// not free, and rendering it as free is an actively wrong claim about cost.
func TestModelsTableNeverRendersUnknownPricingAsFree(t *testing.T) {
	out := ui.NewTheme(new(strings.Builder)).Render(modelsTable([]openrouter.Model{
		{ID: "x/y", ContextLength: 1000, PricingUnknown: true},
	}))

	row := tableRow(t, out, "x/y")
	if strings.Contains(row[2], "0.00") || strings.Contains(row[3], "0.00") {
		t.Errorf("unknown pricing rendered as free: %q", row)
	}
	if row[2] != "?" || row[3] != "?" {
		t.Errorf("price cells = %q/%q, want %q", row[2], row[3], "?")
	}
}

func TestModelsListingEmitsNoEscapesWhenNotATerminal(t *testing.T) {
	h := newHarness(t)
	if got := h.run(t, "models"); strings.Contains(got, "\x1b") {
		t.Errorf("models emitted ANSI escapes to a buffer:\n%q", got)
	}
}
