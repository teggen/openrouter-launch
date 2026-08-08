package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

type fakeCatalog struct{ models []openrouter.Model }

func (f *fakeCatalog) Models(context.Context) ([]openrouter.Model, error) {
	return f.models, nil
}

func fakeModels() []openrouter.Model {
	return []openrouter.Model{
		{ID: "anthropic/claude-opus-4.6", Name: "Anthropic: Claude Opus 4.6",
			ContextLength: 200000, PromptPricePerM: 15, CompletionPricePerM: 75,
			SupportsTools: true, Provider: "anthropic"},
		{ID: "qwen/qwen3-coder:free", Name: "Qwen: Qwen3 Coder (free)",
			ContextLength: 262144, SupportsTools: true, Provider: "qwen"},
		{ID: "openai/o1-mini", Name: "OpenAI: o1-mini",
			ContextLength: 128000, PromptPricePerM: 1.1, CompletionPricePerM: 4.4,
			Provider: "openai"},
	}
}

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

func TestModelsCommandToolsFilter(t *testing.T) {
	h := newHarness(t)
	got := h.run(t, "models", "--tools")

	if strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--tools should exclude o1-mini:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--tools dropped a tool-capable model:\n%s", got)
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

func TestFormatPrice(t *testing.T) {
	cases := map[float64]string{0: "free", 15: "$15.00", 1.1: "$1.10"}
	for in, want := range cases {
		if got := formatPrice(in, false); got != want {
			t.Errorf("formatPrice(%v) = %q, want %q", in, got, want)
		}
	}
	if got := formatPrice(0, true); got != "?" {
		t.Errorf("formatPrice with unknown pricing = %q, want %q", got, "?")
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

func TestFormatContext(t *testing.T) {
	cases := map[int]string{-1: "-", 0: "-", 128000: "128k", 1000000: "1000k"}
	for in, want := range cases {
		if got := formatContext(in); got != want {
			t.Errorf("formatContext(%d) = %q, want %q", in, got, want)
		}
	}
}
