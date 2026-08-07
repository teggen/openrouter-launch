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
// reporting whether the value parsed. Rounding removes float noise so prices
// compare exactly (0.000015 -> 15, not 15.000000000000002).
//
// A parse failure returns ok=false rather than an error: one malformed entry
// must not make the whole catalog undecodable. The caller records the
// uncertainty on the model so it is never displayed or filtered as free.
func perMillion(raw string) (float64, bool) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return math.Round(v*1e6*1e6) / 1e6, true
}
