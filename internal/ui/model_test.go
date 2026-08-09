package ui

import (
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

func TestModelCellsMatchTheHeaderCount(t *testing.T) {
	got := ModelCells(openrouter.Model{ID: "a/b"})
	if len(got) != len(ModelHeaders) {
		t.Errorf("ModelCells returned %d cells, ModelHeaders has %d", len(got), len(ModelHeaders))
	}
}

func TestModelCellsRenderIDContextAndBothPrices(t *testing.T) {
	got := ModelCells(openrouter.Model{
		ID: "anthropic/claude-opus-4.6", ContextLength: 200000,
		PromptPricePerM: 15, CompletionPricePerM: 75, SupportsTools: true,
	})

	for i, want := range []string{"anthropic/claude-opus-4.6", "200k", "$15.00", "$75.00", "✓"} {
		if got[i] != want {
			t.Errorf("cell %d (%s) = %q, want %q", i, ModelHeaders[i], got[i], want)
		}
	}
}

func TestModelCellsOmitTheToolsMarkerWhenUnsupported(t *testing.T) {
	if got := ModelCells(openrouter.Model{ID: "openai/o1-mini", ContextLength: 128000})[4]; got != "" {
		t.Errorf("tools cell = %q, want empty for a tool-less model", got)
	}
}

// Landmine 4 at the render layer: a model whose price failed to parse is
// not free, and rendering it as free is an actively wrong claim about what
// a launch costs.
func TestModelCellsNeverRenderUnknownPricingAsFree(t *testing.T) {
	got := ModelCells(openrouter.Model{ID: "x/y", ContextLength: 1000, PricingUnknown: true})

	for _, i := range []int{2, 3} {
		if strings.Contains(got[i], "free") || strings.Contains(got[i], "0.00") {
			t.Errorf("cell %d (%s) = %q, renders unknown pricing as free", i, ModelHeaders[i], got[i])
		}
		if got[i] != "?" {
			t.Errorf("cell %d (%s) = %q, want %q", i, ModelHeaders[i], got[i], "?")
		}
	}
}

// The MODEL column is the accent; TOOLS carries the same green as an
// installed agent. A flat mapping would make the catalog look unrelated to
// the listings it sits beside.
func TestModelRoleMapsTheAccentAndToolsColumns(t *testing.T) {
	cases := map[int]Role{0: RoleAccent, 1: RolePlain, 2: RolePlain, 3: RolePlain, 4: RoleOK}
	for col, want := range cases {
		if got := ModelRole(col); got != want {
			t.Errorf("ModelRole(%d) = %v, want %v", col, got, want)
		}
	}
}
