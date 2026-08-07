package cli

import (
	"context"
	"strings"
	"testing"

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

// useFakeCatalog points the CLI at in-memory models and an isolated cache.
func useFakeCatalog(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	prev := catalogSource
	catalogSource = &fakeCatalog{models: fakeModels()}
	t.Cleanup(func() { catalogSource = prev })
}

func TestModelsCommandListsAll(t *testing.T) {
	useFakeCatalog(t)
	got := runCmd(t, "models")

	for _, id := range []string{"anthropic/claude-opus-4.6", "qwen/qwen3-coder:free", "openai/o1-mini"} {
		if !strings.Contains(got, id) {
			t.Errorf("output missing %s:\n%s", id, got)
		}
	}
}

func TestModelsCommandToolsFilter(t *testing.T) {
	useFakeCatalog(t)
	got := runCmd(t, "models", "--tools")

	if strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--tools should exclude o1-mini:\n%s", got)
	}
	if !strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--tools dropped a tool-capable model:\n%s", got)
	}
}

func TestModelsCommandFreeFilter(t *testing.T) {
	useFakeCatalog(t)
	got := runCmd(t, "models", "--free")

	if !strings.Contains(got, "qwen/qwen3-coder:free") {
		t.Errorf("--free dropped the free model:\n%s", got)
	}
	if strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--free should exclude paid models:\n%s", got)
	}
}

func TestModelsCommandProviderFilter(t *testing.T) {
	useFakeCatalog(t)
	got := runCmd(t, "models", "--provider", "openai")

	if !strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--provider dropped the match:\n%s", got)
	}
	if strings.Contains(got, "anthropic/claude-opus-4.6") {
		t.Errorf("--provider should exclude other vendors:\n%s", got)
	}
}

func TestModelsCommandMinContextFilter(t *testing.T) {
	useFakeCatalog(t)
	got := runCmd(t, "models", "--min-context", "200000")

	if strings.Contains(got, "openai/o1-mini") {
		t.Errorf("--min-context should exclude the 128k model:\n%s", got)
	}
}

func TestModelsCommandMaxPriceFilter(t *testing.T) {
	useFakeCatalog(t)
	got := runCmd(t, "models", "--max-price", "5")

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

func TestFormatContext(t *testing.T) {
	cases := map[int]string{0: "-", 128000: "128k", 1000000: "1000k"}
	for in, want := range cases {
		if got := formatContext(in); got != want {
			t.Errorf("formatContext(%d) = %q, want %q", in, got, want)
		}
	}
}
