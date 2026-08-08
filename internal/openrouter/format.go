package openrouter

import "fmt"

// FormatPrice renders a USD-per-million-tokens price for display. Unknown
// pricing renders as "?" so it is never mistaken for free.
func FormatPrice(usdPerM float64, unknown bool) string {
	if unknown {
		return "?"
	}
	if usdPerM == 0 {
		return "free"
	}
	return fmt.Sprintf("$%.2f", usdPerM)
}

// FormatContext renders a context window in thousands of tokens.
func FormatContext(tokens int) string {
	if tokens <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dk", tokens/1000)
}
