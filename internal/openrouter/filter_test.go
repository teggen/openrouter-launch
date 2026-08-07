package openrouter

import "testing"

func filterFixture() []Model {
	return []Model{
		{ID: "anthropic/claude-opus-4.6", Name: "Anthropic: Claude Opus 4.6",
			ContextLength: 200000, PromptPricePerM: 15, CompletionPricePerM: 75,
			SupportsTools: true, Provider: "anthropic"},
		{ID: "qwen/qwen3-coder:free", Name: "Qwen: Qwen3 Coder (free)",
			ContextLength: 262144, SupportsTools: true, Provider: "qwen"},
		{ID: "openai/o1-mini", Name: "OpenAI: o1-mini",
			ContextLength: 128000, PromptPricePerM: 1.1, CompletionPricePerM: 4.4,
			SupportsTools: false, Provider: "openai"},
	}
}

func ids(models []Model) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.ID
	}
	return out
}

func equalIDs(got []Model, want []string) bool {
	g := ids(got)
	if len(g) != len(want) {
		return false
	}
	for i := range g {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

func TestApplyNoFilterReturnsAll(t *testing.T) {
	got := Apply(filterFixture(), Filter{})
	if len(got) != 3 {
		t.Errorf("got %d models, want 3", len(got))
	}
}

func TestApplyToolsOnly(t *testing.T) {
	got := Apply(filterFixture(), Filter{ToolsOnly: true})
	want := []string{"anthropic/claude-opus-4.6", "qwen/qwen3-coder:free"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplyFreeOnly(t *testing.T) {
	got := Apply(filterFixture(), Filter{FreeOnly: true})
	want := []string{"qwen/qwen3-coder:free"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplyProvider(t *testing.T) {
	got := Apply(filterFixture(), Filter{Provider: "openai"})
	want := []string{"openai/o1-mini"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplyMinContext(t *testing.T) {
	got := Apply(filterFixture(), Filter{MinContext: 200000})
	want := []string{"anthropic/claude-opus-4.6", "qwen/qwen3-coder:free"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplyMaxPriceUsesCompletionPrice(t *testing.T) {
	got := Apply(filterFixture(), Filter{MaxPrice: 5})
	want := []string{"qwen/qwen3-coder:free", "openai/o1-mini"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplyMaxPriceExcludesUnknownPricing(t *testing.T) {
	models := []Model{
		{ID: "acme/mystery", PricingUnknown: true},
		{ID: "acme/cheap", CompletionPricePerM: 1},
	}
	got := Apply(models, Filter{MaxPrice: 5})
	want := []string{"acme/cheap"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplyFreeOnlyExcludesUnknownPricing(t *testing.T) {
	models := []Model{
		{ID: "acme/mystery", PricingUnknown: true},
		{ID: "acme/free"},
	}
	got := Apply(models, Filter{FreeOnly: true})
	want := []string{"acme/free"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplySearchIsCaseInsensitive(t *testing.T) {
	got := Apply(filterFixture(), Filter{Search: "OPUS"})
	want := []string{"anthropic/claude-opus-4.6"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplySearchMatchesName(t *testing.T) {
	got := Apply(filterFixture(), Filter{Search: "coder"})
	want := []string{"qwen/qwen3-coder:free"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplyCombinesFilters(t *testing.T) {
	got := Apply(filterFixture(), Filter{ToolsOnly: true, FreeOnly: true})
	want := []string{"qwen/qwen3-coder:free"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestFindByID(t *testing.T) {
	m, ok := FindByID(filterFixture(), "openai/o1-mini")
	if !ok {
		t.Fatal("model not found")
	}
	if m.Provider != "openai" {
		t.Errorf("provider = %q, want openai", m.Provider)
	}
	if _, ok := FindByID(filterFixture(), "nope/nope"); ok {
		t.Error("unexpectedly found a missing model")
	}
}

func TestSuggest(t *testing.T) {
	got := Suggest(filterFixture(), "claude", 5)
	if len(got) != 1 || got[0] != "anthropic/claude-opus-4.6" {
		t.Errorf("got %v, want [anthropic/claude-opus-4.6]", got)
	}
}

func TestSuggestRespectsLimit(t *testing.T) {
	if got := Suggest(filterFixture(), "", 2); len(got) != 2 {
		t.Errorf("got %d suggestions, want 2", len(got))
	}
}

func TestApplySearchMatchesIDOnly(t *testing.T) {
	// Isolates the ID branch: search term in ID but not in Name.
	models := []Model{
		{ID: "vendor/uniqueslug-model", Name: "Generic Model"},
	}
	got := Apply(models, Filter{Search: "uniqueslug"})
	want := []string{"vendor/uniqueslug-model"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplySearchMatchesNameOnly(t *testing.T) {
	// Isolates the Name branch: search term in Name but not in ID.
	models := []Model{
		{ID: "vendor/model", Name: "Distinctive AI Name"},
	}
	got := Apply(models, Filter{Search: "distinctive"})
	want := []string{"vendor/model"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplyMaxPriceAtCeiling(t *testing.T) {
	// Model priced exactly at the ceiling should be kept.
	models := []Model{
		{ID: "vendor/atceiling", CompletionPricePerM: 10},
		{ID: "vendor/above", CompletionPricePerM: 10.01},
	}
	got := Apply(models, Filter{MaxPrice: 10})
	want := []string{"vendor/atceiling"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestApplyProviderCaseInsensitive(t *testing.T) {
	// Provider matching should be case-insensitive.
	models := []Model{
		{ID: "openai/gpt-4", Provider: "openai"},
	}
	got := Apply(models, Filter{Provider: "OpenAI"})
	want := []string{"openai/gpt-4"}
	if !equalIDs(got, want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}
