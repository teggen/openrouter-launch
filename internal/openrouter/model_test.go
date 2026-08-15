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

func TestDecodeModelsUnknownPricingIsNotFree(t *testing.T) {
	data := []byte(`{"data":[{"id":"acme/mystery","name":"Acme: Mystery","context_length":1000,"pricing":{"prompt":"n/a","completion":"0"},"supported_parameters":["tools"]}]}`)

	models, err := DecodeModels(data)
	if err != nil {
		t.Fatalf("DecodeModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	if !models[0].PricingUnknown {
		t.Error("an unparseable price should set PricingUnknown")
	}
	if models[0].IsFree() {
		t.Error("a model with unknown pricing must never report as free")
	}
}

// TestDecodeModelsMalformedCompletionPriceIsUnknown is the symmetric
// counterpart to TestDecodeModelsUnknownPricingIsNotFree, which only
// malforms the prompt price. Without this fixture, a mutation of
// `PricingUnknown = !promptOK || !completionOK` down to `!promptOK` alone
// would pass every existing test while a malformed completion price read as
// known-zero and wrongly passed --free and --max-price filters.
func TestDecodeModelsMalformedCompletionPriceIsUnknown(t *testing.T) {
	data := []byte(`{"data":[{"id":"acme/mystery2","name":"Acme: Mystery Two","context_length":1000,"pricing":{"prompt":"0.000015","completion":"n/a"},"supported_parameters":["tools"]}]}`)

	models, err := DecodeModels(data)
	if err != nil {
		t.Fatalf("DecodeModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	if !models[0].PricingUnknown {
		t.Error("a malformed completion price should set PricingUnknown, even with a valid prompt price")
	}
	if models[0].IsFree() {
		t.Error("a model with unknown completion pricing must never report as free")
	}
}

func TestDecodeModelsMissingPricingIsUnknown(t *testing.T) {
	data := []byte(`{"data":[{"id":"acme/bare","name":"Acme: Bare","context_length":1000}]}`)

	models, err := DecodeModels(data)
	if err != nil {
		t.Fatalf("DecodeModels: %v", err)
	}
	if !models[0].PricingUnknown {
		t.Error("absent pricing should set PricingUnknown")
	}
	if models[0].IsFree() {
		t.Error("a model with absent pricing must never report as free")
	}
}

func TestDecodeModelsWellFormedPricingIsKnown(t *testing.T) {
	for _, m := range loadFixture(t) {
		if m.PricingUnknown {
			t.Errorf("%s: PricingUnknown set for a well-formed price", m.ID)
		}
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

// TestDecodeModelsPricingRoundsAwayFloatNoise pins the math.Round in
// perMillion. The values are the point: 0.000015 and 0.0000011 (the fixture's
// prices, asserted elsewhere) scale to exact binary floats and pass with the
// rounding deleted, so neither can catch its removal. These two cannot —
// 0.00000097 scales to 0.97000000000000008438 and 0.0000029 to
// 2.9000000000000003553.
func TestDecodeModelsPricingRoundsAwayFloatNoise(t *testing.T) {
	data := []byte(`{"data":[{"id":"acme/cheap","name":"Acme: Cheap","context_length":8000,"pricing":{"prompt":"0.00000097","completion":"0.0000029"},"supported_parameters":["tools"]}]}`)

	models, err := DecodeModels(data)
	if err != nil {
		t.Fatalf("DecodeModels: %v", err)
	}
	if got := models[0].PromptPricePerM; got != 0.97 {
		t.Errorf("prompt price = %.20g, want exactly 0.97", got)
	}
	if got := models[0].CompletionPricePerM; got != 2.9 {
		t.Errorf("completion price = %.20g, want exactly 2.9", got)
	}
}
