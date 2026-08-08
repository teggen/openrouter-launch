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
