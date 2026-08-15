package openrouter

import "testing"

func TestFormatPrice(t *testing.T) {
	cases := map[float64]string{0: "free", 15: "$15.00", 1.1: "$1.10"}
	for in, want := range cases {
		if got := FormatPrice(in, false); got != want {
			t.Errorf("FormatPrice(%v) = %q, want %q", in, got, want)
		}
	}
}

// Unknown pricing must never render as free: a model whose price failed to
// parse is not a free model, and "free" would be an actively wrong claim
// about what a launch costs.
func TestFormatPriceRendersUnknownPricingAsQuestionMark(t *testing.T) {
	if got := FormatPrice(0, true); got != "?" {
		t.Errorf("FormatPrice(0, unknown) = %q, want %q", got, "?")
	}
	if got := FormatPrice(15, true); got != "?" {
		t.Errorf("FormatPrice(15, unknown) = %q, want %q", got, "?")
	}
}

func TestFormatContext(t *testing.T) {
	cases := map[int]string{-1: "-", 0: "-", 128000: "128k", 1000000: "1000k"}
	for in, want := range cases {
		if got := FormatContext(in); got != want {
			t.Errorf("FormatContext(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatContextBelowOneThousand(t *testing.T) {
	// tokens/1000 truncates, so every one of these rendered "0k" — which
	// reads as no context at all rather than a small one.
	for _, tokens := range []int{1, 512, 999} {
		if got := FormatContext(tokens); got != "<1k" {
			t.Errorf("FormatContext(%d) = %q, want %q", tokens, got, "<1k")
		}
	}
	// The boundary and the existing behaviour are unchanged.
	if got := FormatContext(1000); got != "1k" {
		t.Errorf("FormatContext(1000) = %q, want %q", got, "1k")
	}
	if got := FormatContext(0); got != "-" {
		t.Errorf("FormatContext(0) = %q, want %q", got, "-")
	}
}

func TestFormatPriceBelowTheTwoDecimalFloor(t *testing.T) {
	// "$0.00" for a real price is Landmine 4's misreading one rounding step
	// removed: it is not free, and must not look free.
	for _, price := range []float64{0.0001, 0.004} {
		if got := FormatPrice(price, false); got != "<$0.01" {
			t.Errorf("FormatPrice(%v) = %q, want %q", price, got, "<$0.01")
		}
	}
	// Free, unknown, and ordinary prices are untouched.
	if got := FormatPrice(0, false); got != "free" {
		t.Errorf("FormatPrice(0) = %q, want %q", got, "free")
	}
	if got := FormatPrice(0, true); got != "?" {
		t.Errorf("FormatPrice(unknown) = %q, want %q", got, "?")
	}
	if got := FormatPrice(0.005, false); got != "$0.01" {
		t.Errorf("FormatPrice(0.005) = %q, want %q", got, "$0.01")
	}
}
