package openrouter

import (
	"os"
	"testing"
)

func loadFixture(t *testing.T) []Model {
	t.Helper()
	data, err := os.ReadFile("testdata/models.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	models, err := DecodeModels(data)
	if err != nil {
		t.Fatalf("DecodeModels: %v", err)
	}
	return models
}

func TestDecodeModelsFields(t *testing.T) {
	models := loadFixture(t)
	if len(models) != 3 {
		t.Fatalf("got %d models, want 3", len(models))
	}

	got := models[0]
	want := Model{
		ID:                  "anthropic/claude-opus-4.6",
		Name:                "Anthropic: Claude Opus 4.6",
		Description:         "Flagship reasoning model.",
		ContextLength:       200000,
		PromptPricePerM:     15,
		CompletionPricePerM: 75,
		SupportsTools:       true,
		Provider:            "anthropic",
	}
	if got != want {
		t.Errorf("model[0]:\n got %+v\nwant %+v", got, want)
	}
}

func TestDecodeModelsPricingIsExact(t *testing.T) {
	models := loadFixture(t)
	if got := models[2].PromptPricePerM; got != 1.1 {
		t.Errorf("prompt price = %v, want 1.1", got)
	}
	if got := models[2].CompletionPricePerM; got != 4.4 {
		t.Errorf("completion price = %v, want 4.4", got)
	}
}

func TestIsFree(t *testing.T) {
	models := loadFixture(t)
	if models[0].IsFree() {
		t.Error("paid model reported as free")
	}
	if !models[1].IsFree() {
		t.Error("free model not reported as free")
	}
}

func TestSupportsTools(t *testing.T) {
	models := loadFixture(t)
	if !models[1].SupportsTools {
		t.Error("qwen should support tools")
	}
	if models[2].SupportsTools {
		t.Error("o1-mini should not support tools")
	}
}

func TestDecodeModelsRejectsInvalidJSON(t *testing.T) {
	if _, err := DecodeModels([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
