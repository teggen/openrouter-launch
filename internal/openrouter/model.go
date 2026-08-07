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

// Model is a normalized catalog entry. Prices are USD per million tokens.
type Model struct {
	ID                  string
	Name                string
	Description         string
	ContextLength       int
	PromptPricePerM     float64
	CompletionPricePerM float64
	SupportsTools       bool
	Provider            string
}

// IsFree reports whether both prompt and completion tokens cost nothing.
func (m Model) IsFree() bool {
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
		models = append(models, Model{
			ID:                  m.ID,
			Name:                m.Name,
			Description:         m.Description,
			ContextLength:       m.ContextLength,
			PromptPricePerM:     perMillion(m.Pricing.Prompt),
			CompletionPricePerM: perMillion(m.Pricing.Completion),
			SupportsTools:       slices.Contains(m.SupportedParameters, "tools"),
			Provider:            provider,
		})
	}
	return models, nil
}

// perMillion converts a per-token USD price string to USD per million tokens.
// Rounding removes float noise so prices compare exactly (0.000015 -> 15, not
// 15.000000000000002). Unparseable values are treated as free.
func perMillion(raw string) float64 {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return math.Round(v*1e6*1e6) / 1e6
}
