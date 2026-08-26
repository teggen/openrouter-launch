package openrouter

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

// sortFixture is deliberately in NO column's sorted order, so a comparator
// reading the wrong field cannot produce the expected result by accident.
//
// Two properties are load-bearing and must survive any edit:
//
//   - "c/asymmetric" is the CHEAPEST input and the DEAREST output, so the two
//     price columns disagree. A fixture whose prompt and completion prices
//     rank models identically passes with the two comparators swapped — that
//     version of this fixture was written first and proved nothing.
//   - "unknown/model" carries PricingUnknown, and is the cheapest-LOOKING
//     entry by raw float (0.0), which is the trap Landmine 4 describes.
func sortFixture() []catalog.Model {
	return []catalog.Model{
		{ID: "b/mid", ContextLength: 128_000, PromptPricePerM: 3, CompletionPricePerM: 9, SupportsTools: true},
		{ID: "unknown/model", ContextLength: 8_000, PricingUnknown: true},
		{ID: "a/pricey", ContextLength: 200_000, PromptPricePerM: 15, CompletionPricePerM: 75, SupportsTools: true},
		{ID: "c/asymmetric", ContextLength: 32_000, PromptPricePerM: 1, CompletionPricePerM: 90},
	}
}

// ids lives in filter_test.go, shared by both files in this package.

func TestSortModelsOrdersEveryColumnInBothDirections(t *testing.T) {
	tests := []struct {
		name string
		sort Sort
		want []string
	}{
		{"model asc", Sort{Key: SortModel},
			[]string{"a/pricey", "b/mid", "c/asymmetric", "unknown/model"}},
		{"model desc", Sort{Key: SortModel, Desc: true},
			[]string{"unknown/model", "c/asymmetric", "b/mid", "a/pricey"}},
		{"context asc", Sort{Key: SortContext},
			[]string{"unknown/model", "c/asymmetric", "b/mid", "a/pricey"}},
		{"context desc", Sort{Key: SortContext, Desc: true},
			[]string{"a/pricey", "b/mid", "c/asymmetric", "unknown/model"}},
		// input and output disagree, on purpose: c/asymmetric is the cheapest
		// input and the dearest output.
		{"input asc", Sort{Key: SortInput},
			[]string{"c/asymmetric", "b/mid", "a/pricey", "unknown/model"}},
		{"input desc", Sort{Key: SortInput, Desc: true},
			[]string{"a/pricey", "b/mid", "c/asymmetric", "unknown/model"}},
		{"output asc", Sort{Key: SortOutput},
			[]string{"b/mid", "a/pricey", "c/asymmetric", "unknown/model"}},
		{"output desc", Sort{Key: SortOutput, Desc: true},
			[]string{"c/asymmetric", "a/pricey", "b/mid", "unknown/model"}},
		// Ties keep catalog order, which is what makes a two-valued column
		// useful rather than arbitrary.
		{"tools asc", Sort{Key: SortTools},
			[]string{"unknown/model", "c/asymmetric", "b/mid", "a/pricey"}},
		{"tools desc", Sort{Key: SortTools, Desc: true},
			[]string{"b/mid", "a/pricey", "unknown/model", "c/asymmetric"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ids(SortModels(sortFixture(), tt.sort))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SortModels(%+v) = %v, want %v", tt.sort, got, tt.want)
			}
		})
	}
}

// Landmine 4 for ordering. Both price columns, both directions, in one test so
// a fix that only handles ascending cannot pass.
func TestUnknownPricingSortsLastWhicheverWayTheArrowPoints(t *testing.T) {
	for _, key := range []SortKey{SortInput, SortOutput} {
		for _, desc := range []bool{false, true} {
			got := ids(SortModels(sortFixture(), Sort{Key: key, Desc: desc}))
			if last := got[len(got)-1]; last != "unknown/model" {
				t.Errorf("key=%s desc=%v: last is %q, want unknown/model (got %v)",
					key, desc, last, got)
			}
		}
	}
}

func TestSortModelsIsStable(t *testing.T) {
	// Forty entries spread over four context values, scrambled, so the sort
	// must genuinely permute the slice.
	//
	// A single all-equal tie group is NOT a stability probe, however large:
	// with every comparison false, pdqsort detects an already-ordered run and
	// returns it untouched, so that version of this test passes with
	// sort.Slice in place. It was written that way first and caught nothing.
	in := make([]catalog.Model, 0, 40)
	for i := 0; i < 40; i++ {
		in = append(in, catalog.Model{
			ID:            fmt.Sprintf("m%02d", i),
			ContextLength: 1000 * (1 + (i*7)%4),
		})
	}

	got := SortModels(in, Sort{Key: SortContext})
	for i := 1; i < len(got); i++ {
		if got[i-1].ContextLength > got[i].ContextLength {
			t.Fatalf("not sorted at index %d: %v", i, ids(got))
		}
	}
	// Within a context group the IDs must still ascend, since they were
	// generated in that order. That is what "stable" buys, and it is the only
	// thing sort.Slice takes away.
	last := map[int]string{}
	for _, m := range got {
		if prev, seen := last[m.ContextLength]; seen && m.ID < prev {
			t.Errorf("context %d: %q came after %q — ties were reordered (sort.Slice instead of SliceStable?)",
				m.ContextLength, m.ID, prev)
		}
		last[m.ContextLength] = m.ID
	}
}

func TestSortModelsDoesNotMutateItsArgument(t *testing.T) {
	in := sortFixture()
	before := ids(in)
	SortModels(in, Sort{Key: SortModel})
	if after := ids(in); !reflect.DeepEqual(before, after) {
		t.Errorf("caller's slice was reordered: %v -> %v", before, after)
	}
}

func TestZeroSortLeavesCatalogOrder(t *testing.T) {
	in := sortFixture()
	if got, want := ids(SortModels(in, Sort{})), ids(in); !reflect.DeepEqual(got, want) {
		t.Errorf("Sort{} reordered the catalog: got %v, want %v", got, want)
	}
}

func TestParseSortKey(t *testing.T) {
	for _, in := range []string{"model", "CONTEXT", " input ", "output", "tools", ""} {
		if _, err := ParseSortKey(in); err != nil {
			t.Errorf("ParseSortKey(%q) errored: %v", in, err)
		}
	}
	if k, err := ParseSortKey("OUTPUT"); err != nil || k != SortOutput {
		t.Errorf("ParseSortKey(%q) = %q, %v; want output, nil", "OUTPUT", k, err)
	}
	if k, err := ParseSortKey(""); err != nil || k != SortNone {
		t.Errorf("ParseSortKey(%q) = %q, %v; want SortNone, nil", "", k, err)
	}

	_, err := ParseSortKey("prompt")
	if err == nil {
		t.Fatal("ParseSortKey(\"prompt\") must error: a typo may not silently mean catalog order")
	}
	// The message has to name the alternatives; "invalid" alone leaves the
	// user guessing which words are legal.
	for _, want := range []string{"model", "context", "input", "output", "tools"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

func TestSortKeysAreTheSelectableColumnsOnly(t *testing.T) {
	want := []SortKey{SortModel, SortContext, SortInput, SortOutput, SortTools}
	if !reflect.DeepEqual(SortKeys, want) {
		t.Errorf("SortKeys = %v, want %v", SortKeys, want)
	}
	for _, k := range SortKeys {
		if k == SortNone {
			t.Error("SortNone must not be in SortKeys: it is the idle value, not a column")
		}
	}
}
