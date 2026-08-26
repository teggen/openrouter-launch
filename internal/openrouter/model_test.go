package openrouter

import (
	"os"
	"testing"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

func loadFixture(t *testing.T) []catalog.Model {
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
	want := catalog.Model{
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

// TestDecodeModelsNegativePriceIsUnknown pins the "-1" sentinel the live
// /models endpoint uses for entries whose price cannot be stated in advance:
// as of 2026-08-16 openrouter/auto, openrouter/auto-beta, openrouter/fusion,
// openrouter/pareto-code and openrouter/bodybuilder all send it for both
// prompt and completion, because a router's cost depends on where the
// request actually lands.
//
// ParseFloat accepts "-1", so without an explicit guard the model records a
// negative price with PricingUnknown CLEAR, and every downstream honesty
// check then reads it as very nearly free: FormatPrice renders "<$0.01" (a
// negative fails the ==0 test and passes the <0.005 one), --max-price admits
// it, and an ascending price sort heads the list with it while unknownLast
// never fires. That is the exact false claim Landmine 4 exists to prevent,
// reached by typing nothing more exotic than `orl models --sort input`.
func TestDecodeModelsNegativePriceIsUnknown(t *testing.T) {
	data := []byte(`{"data":[{"id":"openrouter/auto","name":"OpenRouter: Auto","context_length":2000000,"pricing":{"prompt":"-1","completion":"-1"},"supported_parameters":["tools"]}]}`)

	models, err := DecodeModels(data)
	if err != nil {
		t.Fatalf("DecodeModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	got := models[0]
	if !got.PricingUnknown {
		t.Error("the -1 price sentinel should set PricingUnknown")
	}
	if got.IsFree() {
		t.Error("a model priced -1 must never report as free")
	}
	// The recorded prices must not stay negative even with the flag set:
	// FormatPrice is the only reader that consults the flag, while the sort
	// comparators read the floats directly.
	if got.PromptPricePerM != 0 || got.CompletionPricePerM != 0 {
		t.Errorf("prices = %v/%v, want 0/0: a rejected price must not keep its negative value",
			got.PromptPricePerM, got.CompletionPricePerM)
	}
	if FormatPrice(got.PromptPricePerM, got.PricingUnknown) != "?" {
		t.Errorf("rendered %q, want %q",
			FormatPrice(got.PromptPricePerM, got.PricingUnknown), "?")
	}
}

// TestDecodeModelsNonFinitePriceIsUnknown closes the rest of the class the
// -1 sentinel opens. ParseFloat also accepts "NaN", "Inf" and "Infinity"
// (case-insensitively, with an optional sign), none of which the endpoint
// emits today. NaN is the one worth the extra condition: every comparison
// against it is false, so a NaN price does not merely sort wrongly, it makes
// lessBy's ordering relation non-transitive and the sort's output arbitrary.
func TestDecodeModelsNonFinitePriceIsUnknown(t *testing.T) {
	for _, raw := range []string{"NaN", "Inf", "+Inf", "-Inf", "Infinity"} {
		t.Run(raw, func(t *testing.T) {
			data := []byte(`{"data":[{"id":"acme/odd","name":"Acme: Odd","context_length":1000,"pricing":{"prompt":"` + raw + `","completion":"0"},"supported_parameters":["tools"]}]}`)

			models, err := DecodeModels(data)
			if err != nil {
				t.Fatalf("DecodeModels: %v", err)
			}
			if !models[0].PricingUnknown {
				t.Errorf("price %q should set PricingUnknown", raw)
			}
			if models[0].PromptPricePerM != 0 {
				t.Errorf("price %q recorded as %v, want 0", raw, models[0].PromptPricePerM)
			}
		})
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
