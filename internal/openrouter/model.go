// Package openrouter fetches, caches, and filters the OpenRouter model catalog.
package openrouter

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Model is a normalized catalog entry. Prices are USD per million tokens and
// are only meaningful when PricingUnknown is false.
type Model struct {
	ID                  string
	Name                string
	Description         string
	ContextLength       int
	PromptPricePerM     float64
	CompletionPricePerM float64
	// PricingUnknown reports that a price could not be parsed. Unknown
	// pricing must never be mistaken for free pricing.
	PricingUnknown bool
	SupportsTools  bool
	Provider       string
}

// IsFree reports whether both prompt and completion tokens cost nothing.
// Unknown pricing is never free.
func (m Model) IsFree() bool {
	if m.PricingUnknown {
		return false
	}
	return m.PromptPricePerM == 0 && m.CompletionPricePerM == 0
}

type apiPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

type apiModel struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	ContextLength       int        `json:"context_length"`
	Pricing             apiPricing `json:"pricing"`
	SupportedParameters []string   `json:"supported_parameters"`
}

type apiModelList struct {
	Data []apiModel `json:"data"`
}

// DecodeModels parses a /models response body into normalized models.
func DecodeModels(data []byte) ([]Model, error) {
	var list apiModelList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}

	models := make([]Model, 0, len(list.Data))
	for _, m := range list.Data {
		provider, _, _ := strings.Cut(m.ID, "/")
		prompt, promptOK := perMillion(m.Pricing.Prompt)
		completion, completionOK := perMillion(m.Pricing.Completion)
		models = append(models, Model{
			ID:                  m.ID,
			Name:                m.Name,
			Description:         m.Description,
			ContextLength:       m.ContextLength,
			PromptPricePerM:     prompt,
			CompletionPricePerM: completion,
			PricingUnknown:      !promptOK || !completionOK,
			SupportsTools:       slices.Contains(m.SupportedParameters, "tools"),
			Provider:            provider,
		})
	}
	return models, nil
}

// perMillion converts a per-token USD price string to USD per million tokens,
// reporting whether the value parsed. Rounding to six decimals removes float
// noise so prices compare, sort, and render exactly: 0.00000097 scales to
// 0.97000000000000008438, which must be 0.97. (Not every price is noisy —
// 0.000015 scales to exactly 15 — which is why the test pins a value that is.)
//
// A parse failure returns ok=false rather than an error: one malformed entry
// must not make the whole catalog undecodable. The caller records the
// uncertainty on the model so it is never displayed or filtered as free.
//
// Parsing is not enough on its own: ParseFloat accepts several strings that
// are not prices. The endpoint sends "-1" for entries whose cost cannot be
// stated in advance (the openrouter/auto routers), and accepts "NaN" and
// "Inf" as a matter of Go's grammar. Each has to be rejected HERE, because
// every downstream honesty check keys off the ok=false this returns:
//
//   - a negative price fails FormatPrice's ==0 test and passes its <0.005
//     test, so it renders "<$0.01" — the Landmine 4 false claim exactly;
//   - it also slips under any --max-price ceiling and heads an ascending
//     price sort, while unknownLast stays silent because the flag is clear;
//   - NaN compares false against everything, which makes lessBy's ordering
//     relation non-transitive rather than merely wrong.
//
// "Unknown" is the truthful reading in the case that actually occurs: a
// router's price genuinely is not known until the request lands somewhere.
func perMillion(raw string) (float64, bool) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, false
	}
	return math.Round(v*1e6*1e6) / 1e6, true
}
