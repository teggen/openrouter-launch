package openrouter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/teggen/agentlaunch/catalog"
)

// SortKey names a catalog column to order by. The zero value means "do not
// reorder", so every caller that has not opted in keeps catalog order.
type SortKey string

const (
	SortNone    SortKey = ""
	SortModel   SortKey = "model"
	SortContext SortKey = "context"
	SortInput   SortKey = "input"
	SortOutput  SortKey = "output"
	SortTools   SortKey = "tools"
)

// SortKeys are the selectable columns, in ui.ModelHeaders order. SortNone is
// deliberately absent: it is the idle value, not a column, and the surfaces
// that offer a cycle add it themselves at the position they want it.
//
// The order is pinned against ModelHeaders by ui.TestSortLabelMatchesTheTable,
// so renaming or reordering a column cannot silently mislabel this list.
var SortKeys = []SortKey{SortModel, SortContext, SortInput, SortOutput, SortTools}

// ParseSortKey resolves a user-supplied column name, case-insensitively. The
// empty string is SortNone and not an error: it is what an unset flag or a
// fresh config carries.
func ParseSortKey(s string) (SortKey, error) {
	k := SortKey(strings.ToLower(strings.TrimSpace(s)))
	if k == SortNone {
		return SortNone, nil
	}
	names := make([]string, len(SortKeys))
	for i, valid := range SortKeys {
		if k == valid {
			return k, nil
		}
		names[i] = string(valid)
	}
	return SortNone, fmt.Errorf("unknown sort column %q (want one of: %s)",
		s, strings.Join(names, ", "))
}

// Sort is a column plus a direction. Ascending is the natural order of the
// underlying value: IDs A-Z, small contexts and cheap prices first, and models
// WITHOUT tool support before those with it.
type Sort struct {
	Key  SortKey
	Desc bool
}

// SortModels returns models ordered by s, leaving the caller's slice
// untouched.
//
// The sort is STABLE, so equal keys keep the order they arrived in — catalog
// order for the CLI, relevance order for the picker. That is the whole reason
// a two-valued column like TOOLS produces a useful listing.
func SortModels(models []catalog.Model, s Sort) []catalog.Model {
	out := make([]catalog.Model, len(models))
	copy(out, models)
	if s.Key == SortNone {
		return out
	}
	less := lessBy(s.Key)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if result, decided := unknownLast(a, b, s.Key); decided {
			return result
		}
		if s.Desc {
			a, b = b, a
		}
		return less(a, b)
	})
	return out
}

// unknownLast keeps models with unparseable pricing at the BOTTOM of a
// price-sorted list whichever way the arrow points — which is why it is
// decided BEFORE Desc swaps the operands.
//
// This is Landmine 4 ("unknown pricing is never free") restated for ordering.
// Such a model renders "?" and carries 0.0 in both price fields, so comparing
// it numerically would head a cheapest-first list with models whose price is
// simply not known — the same false claim that --free and --max-price already
// refuse to make.
func unknownLast(a, b catalog.Model, k SortKey) (less, decided bool) {
	if k != SortInput && k != SortOutput {
		return false, false
	}
	if a.PricingUnknown == b.PricingUnknown {
		return false, false
	}
	return !a.PricingUnknown, true
}

func lessBy(k SortKey) func(a, b catalog.Model) bool {
	switch k {
	case SortModel:
		// The ID, not the Name: it is what the MODEL column shows and what -m
		// takes.
		return func(a, b catalog.Model) bool { return strings.ToLower(a.ID) < strings.ToLower(b.ID) }
	case SortContext:
		return func(a, b catalog.Model) bool { return a.ContextLength < b.ContextLength }
	case SortInput:
		return func(a, b catalog.Model) bool { return a.PromptPricePerM < b.PromptPricePerM }
	case SortOutput:
		return func(a, b catalog.Model) bool { return a.CompletionPricePerM < b.CompletionPricePerM }
	case SortTools:
		return func(a, b catalog.Model) bool { return !a.SupportsTools && b.SupportsTools }
	}
	// An unrecognised key orders nothing. Unreachable through ParseSortKey,
	// but SortModels takes a Sort a caller could build by hand.
	return func(_, _ catalog.Model) bool { return false }
}
