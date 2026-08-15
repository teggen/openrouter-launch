package openrouter

import "fmt"

// FormatPrice renders a USD-per-million-tokens price for display. Unknown
// pricing renders as "?" so it is never mistaken for free, and a nonzero
// price below the two-decimal floor renders "<$0.01" rather than "$0.00" —
// the same misreading Landmine 4 works against, one rounding step removed.
func FormatPrice(usdPerM float64, unknown bool) string {
	if unknown {
		return "?"
	}
	if usdPerM == 0 {
		return "free"
	}
	if usdPerM < 0.005 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", usdPerM)
}

// FormatContext renders a context window in thousands of tokens. Anything
// under 1k renders "<1k": integer division would truncate it to "0k", which
// reads as no context rather than a small one.
func FormatContext(tokens int) string {
	if tokens <= 0 {
		return "-"
	}
	if tokens < 1000 {
		return "<1k"
	}
	return fmt.Sprintf("%dk", tokens/1000)
}
