# Models table sorting + INPUT/OUTPUT rename — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the models table sortable by any of its five columns, from the
`ctrl+f` screen (renamed **Filter & Sort**) and from `orl models --sort`, and
retitle the price columns INPUT/M and OUTPUT/M.

**Architecture:** The ordering primitive is domain code in
`internal/openrouter` (`SortKey`, `Sort`, `SortModels`), so the CLI listing and
the TUI picker share one implementation, exactly as they already share
`ui.ModelHeaders` and `openrouter.Apply`. The picker composes it *outside*
`Rank`, so a chosen column overrides search relevance. The chosen sort persists
in `config.json` under a new top-level `sort` key.

**Tech Stack:** Go 1.25, cobra, bubbletea 1.3.x, lipgloss/table.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-08-10-models-sort-design.md`. Read it for
  WHY before changing WHAT.
- **`Launcher.Command` purity, the write-site set, and Landmine 6's table are
  untouched by this plan.** No task adds a write site; the TUI persists through
  the existing `config.Save` path (write site 2).
- **Landmine 4 applies to ordering:** a model with `PricingUnknown` renders
  `"?"` and must sort **last in both directions** on INPUT and OUTPUT. Never
  compare it as `0.0`.
- **Every test gets a mutation check before it is trusted:** break the
  behaviour, watch the *named* test fail, revert. The project's recurring
  review finding is tests that pass for the wrong reason.
- `internal/tui` must not import `internal/cli`, cobra, or pflag (Landmine 13).
- `internal/config` depends on nothing else in this tree — keep it that way
  (that is why `config.Sort.Column` is a plain `string`).
- Run `make test` after each task; `make ci` before the final commit.
- Commit after every task, `develop` branch, conventional-commit subjects.

---

### Task 1: The sort primitive

**Files:**
- Create: `internal/openrouter/sort.go`
- Test: `internal/openrouter/sort_test.go`

**Interfaces:**
- Consumes: `openrouter.Model` (`internal/openrouter/model.go`).
- Produces: `SortKey` (string type) with `SortNone`/`SortModel`/`SortContext`/
  `SortInput`/`SortOutput`/`SortTools`; `var SortKeys []SortKey` (the five
  selectable keys, in `ui.ModelHeaders` order, **excluding** `SortNone`);
  `ParseSortKey(string) (SortKey, error)`; `type Sort struct { Key SortKey;
  Desc bool }`; `SortModels([]Model, Sort) []Model`.

- [ ] **Step 1: Write the failing tests**

Create `internal/openrouter/sort_test.go`. The fixture is local, not
`ortest.Models()`: it needs a `PricingUnknown` entry and a deliberate tie,
neither of which the shared fixture has (and adding them there would perturb
every package that depends on its documented invariants).

```go
package openrouter

import (
	"reflect"
	"testing"
)

// sortFixture is deliberately in NO column's sorted order, so a comparator
// reading the wrong field cannot produce the expected result by accident.
// "unknown/model" carries PricingUnknown, and it is the CHEAPEST-looking
// entry by raw float (0.0) — which is exactly the trap Landmine 4 describes.
func sortFixture() []Model {
	return []Model{
		{ID: "b/mid", ContextLength: 128_000, PromptPricePerM: 3, CompletionPricePerM: 9, SupportsTools: true},
		{ID: "unknown/model", ContextLength: 8_000, PricingUnknown: true},
		{ID: "a/pricey", ContextLength: 200_000, PromptPricePerM: 15, CompletionPricePerM: 75, SupportsTools: true},
		{ID: "c/cheap", ContextLength: 32_000, PromptPricePerM: 1, CompletionPricePerM: 2},
	}
}

func ids(models []Model) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.ID
	}
	return out
}

func TestSortModelsOrdersEveryColumnInBothDirections(t *testing.T) {
	tests := []struct {
		name string
		sort Sort
		want []string
	}{
		{"model asc", Sort{Key: SortModel},
			[]string{"a/pricey", "b/mid", "c/cheap", "unknown/model"}},
		{"model desc", Sort{Key: SortModel, Desc: true},
			[]string{"unknown/model", "c/cheap", "b/mid", "a/pricey"}},
		{"context asc", Sort{Key: SortContext},
			[]string{"unknown/model", "c/cheap", "b/mid", "a/pricey"}},
		{"context desc", Sort{Key: SortContext, Desc: true},
			[]string{"a/pricey", "b/mid", "c/cheap", "unknown/model"}},
		{"input asc", Sort{Key: SortInput},
			[]string{"c/cheap", "b/mid", "a/pricey", "unknown/model"}},
		{"input desc", Sort{Key: SortInput, Desc: true},
			[]string{"a/pricey", "b/mid", "c/cheap", "unknown/model"}},
		{"output asc", Sort{Key: SortOutput},
			[]string{"c/cheap", "b/mid", "a/pricey", "unknown/model"}},
		{"output desc", Sort{Key: SortOutput, Desc: true},
			[]string{"a/pricey", "b/mid", "c/cheap", "unknown/model"}},
		// Ties keep catalog order, which is what makes a two-valued column
		// useful rather than arbitrary.
		{"tools asc", Sort{Key: SortTools},
			[]string{"unknown/model", "c/cheap", "b/mid", "a/pricey"}},
		{"tools desc", Sort{Key: SortTools, Desc: true},
			[]string{"b/mid", "a/pricey", "unknown/model", "c/cheap"}},
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

// Landmine 4 for ordering. Both price columns, both directions, in one test
// so a fix that only handles ascending cannot pass.
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
	in := []Model{
		{ID: "second", ContextLength: 1000},
		{ID: "first", ContextLength: 1000},
		{ID: "third", ContextLength: 1000},
	}
	got := ids(SortModels(in, Sort{Key: SortContext}))
	want := []string{"second", "first", "third"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("equal keys reordered: got %v, want %v (sort.Slice instead of SliceStable?)", got, want)
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
		t.Errorf("ParseSortKey(\"OUTPUT\") = %q, %v; want output, nil", k, err)
	}
	if k, err := ParseSortKey(""); err != nil || k != SortNone {
		t.Errorf("ParseSortKey(\"\") = %q, %v; want SortNone, nil", k, err)
	}
	err := func() error { _, err := ParseSortKey("prompt"); return err }()
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
```

Add `"strings"` to the test file's imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/openrouter/ -run 'Sort|ParseSortKey' -v`
Expected: compile failure — `undefined: SortModels`, `undefined: SortKey`, …

- [ ] **Step 3: Write the implementation**

Create `internal/openrouter/sort.go`:

```go
package openrouter

import (
	"fmt"
	"sort"
	"strings"
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
	for _, valid := range SortKeys {
		if k == valid {
			return k, nil
		}
	}
	names := make([]string, len(SortKeys))
	for i, valid := range SortKeys {
		names[i] = string(valid)
	}
	return SortNone, fmt.Errorf("unknown sort column %q (want one of: %s)",
		s, strings.Join(names, ", "))
}

// Sort is a column plus a direction. Ascending is the natural order of the
// underlying value: IDs A-Z, small contexts and cheap prices first, and
// models WITHOUT tool support before those with it.
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
func SortModels(models []Model, s Sort) []Model {
	out := make([]Model, len(models))
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
func unknownLast(a, b Model, k SortKey) (less, decided bool) {
	if k != SortInput && k != SortOutput {
		return false, false
	}
	if a.PricingUnknown == b.PricingUnknown {
		return false, false
	}
	return !a.PricingUnknown, true
}

func lessBy(k SortKey) func(a, b Model) bool {
	switch k {
	case SortModel:
		// The ID, not the Name: it is what the MODEL column shows and what
		// -m takes.
		return func(a, b Model) bool { return strings.ToLower(a.ID) < strings.ToLower(b.ID) }
	case SortContext:
		return func(a, b Model) bool { return a.ContextLength < b.ContextLength }
	case SortInput:
		return func(a, b Model) bool { return a.PromptPricePerM < b.PromptPricePerM }
	case SortOutput:
		return func(a, b Model) bool { return a.CompletionPricePerM < b.CompletionPricePerM }
	case SortTools:
		return func(a, b Model) bool { return !a.SupportsTools && b.SupportsTools }
	}
	return func(a, b Model) bool { return false }
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/openrouter/ -v -run 'Sort|ParseSortKey'`
Expected: PASS, every subtest.

- [ ] **Step 5: Mutation checks (do all four, revert each)**

1. In `unknownLast`, return `false, false` unconditionally →
   `TestUnknownPricingSortsLastWhicheverWayTheArrowPoints` and the input/output
   rows of the table test must FAIL.
2. Move the `unknownLast` call to *after* the `if s.Desc` swap →
   `TestUnknownPricingSortsLastWhicheverWayTheArrowPoints` must FAIL for
   `desc=true` only. (This is the "right in one direction" bug the spec warns
   about; if it stays green, the test is not pinning the rule.)
3. `sort.SliceStable` → `sort.Slice` → `TestSortModelsIsStable` must FAIL.
   (If it passes, the fixture is too small — Go's pdqsort leaves 3 elements
   alone; grow the tie group to 12 identical-context entries until it fails.)
4. `SortInput`'s comparator reads `CompletionPricePerM` →
   `TestSortModelsOrdersEveryColumnInBothDirections/input_asc` must FAIL.

- [ ] **Step 6: Commit**

```bash
git add internal/openrouter/sort.go internal/openrouter/sort_test.go
git commit -m "feat(openrouter): order the catalog by any column, unknown pricing last"
```

---

### Task 2: The INPUT/OUTPUT rename, and the shared sort label

**Files:**
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/model_test.go`
- Modify: `internal/cli/models_test.go:165`
- Modify: `internal/tui/picker.go:349` (comment only)
- Modify: `internal/tui/picker_test.go:489-502`

**Interfaces:**
- Consumes: `openrouter.SortKeys`, `openrouter.SortKey` (Task 1).
- Produces: `ui.SortLabel(openrouter.SortKey) string` — the header string for
  a sort key, `"relevance"` for `SortNone`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/model_test.go` (import `openrouter`):

```go
func TestModelHeadersUseInputAndOutput(t *testing.T) {
	want := []string{"MODEL", "CONTEXT", "INPUT/M", "OUTPUT/M", "TOOLS"}
	if !reflect.DeepEqual(ui.ModelHeaders, want) {
		t.Errorf("ModelHeaders = %v, want %v", ui.ModelHeaders, want)
	}
}

// The filter&sort screen shows a sort key using the table's own header, so a
// rename cannot leave the two disagreeing. This also pins that SortKeys is in
// ModelHeaders order — a reordering there would otherwise relabel every row
// of that screen silently.
func TestSortLabelMatchesTheTable(t *testing.T) {
	if got := ui.SortLabel(openrouter.SortNone); got != "relevance" {
		t.Errorf("SortLabel(SortNone) = %q, want relevance", got)
	}
	for i, k := range openrouter.SortKeys {
		if got, want := ui.SortLabel(k), ui.ModelHeaders[i]; got != want {
			t.Errorf("SortLabel(%q) = %q, want %q (SortKeys out of ModelHeaders order?)", k, got, want)
		}
	}
	if got := ui.SortLabel(openrouter.SortKey("bogus")); got != "relevance" {
		t.Errorf("SortLabel(bogus) = %q, want relevance", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/ui/ -run 'ModelHeaders|SortLabel' -v`
Expected: FAIL — `undefined: ui.SortLabel`, and the headers assertion.

- [ ] **Step 3: Implement**

In `internal/ui/model.go`, change the headers and append `SortLabel`:

```go
// ModelHeaders are the catalog columns. Shared by `orl models` and the TUI
// picker for the same reason AgentStatus is shared: two renderings of the
// same data that drift are worse than one that is slightly less convenient
// to build.
//
// INPUT/OUTPUT rather than PROMPT/COMPLETION: those are OpenRouter's wire
// names (pricing.prompt / pricing.completion) and they stay on the Model
// fields, but the columns say what a user pays for.
var ModelHeaders = []string{"MODEL", "CONTEXT", "INPUT/M", "OUTPUT/M", "TOOLS"}

// SortLabel is the display name of a sort key: the table's own header for a
// column, "relevance" for SortNone and for anything unrecognised.
//
// Positional by design — openrouter.SortKeys is declared in ModelHeaders
// order and TestSortLabelMatchesTheTable pins that, so a renamed or reordered
// column cannot leave the filter&sort screen naming a different one.
func SortLabel(k openrouter.SortKey) string {
	for i, key := range openrouter.SortKeys {
		if key == k && i < len(ModelHeaders) {
			return ModelHeaders[i]
		}
	}
	return "relevance"
}
```

Then fix the three assertions that name the old headers:

- `internal/cli/models_test.go:165` → `wantColumns(t, out, "MODEL", "CONTEXT", "INPUT/M", "OUTPUT/M", "TOOLS")`
- `internal/tui/picker_test.go:489-490` → `"OUTPUT/M"` in the `Contains` check
  and its message
- `internal/tui/picker_test.go:502` → `[]string{"CONTEXT", "INPUT/M", "OUTPUT/M", "TOOLS"}`
- `internal/tui/picker.go:349` comment → `// OUTPUT/M, INPUT/M, CONTEXT, TOOLS`

- [ ] **Step 4: Run the full suite**

Run: `make test`
Expected: PASS. Any other failure is a fourth place naming the old headers —
fix it the same way.

- [ ] **Step 5: Mutation check**

Revert `ModelHeaders` to `PROMPT/M`/`COMPLETION/M` →
`TestModelHeadersUseInputAndOutput`, `TestSortLabelMatchesTheTable`, and the
two updated assertions must FAIL. Revert the mutation.

- [ ] **Step 6: Commit**

```bash
git add internal/ui internal/cli/models_test.go internal/tui/picker.go internal/tui/picker_test.go
git commit -m "feat(ui): retitle the price columns INPUT/M and OUTPUT/M"
```

---

### Task 3: Persisted sort in the config

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Sort{Column string; Desc bool}` with JSON tags
  `column`/`desc`, and `Config.Sort` under the `sort` key.

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go` (follow the file's existing helper
style for pointing `XDG_CONFIG_HOME` at a temp dir):

```go
func TestSortRoundTripsThroughTheConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sort != (config.Sort{}) {
		t.Errorf("a fresh config sorts by %+v, want the zero value (relevance)", cfg.Sort)
	}

	cfg.Sort = config.Sort{Column: "output", Desc: true}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, err := config.Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), `"sort"`) || !strings.Contains(string(raw), `"column": "output"`) {
		t.Errorf("config file does not carry the sort under the documented keys:\n%s", raw)
	}

	back, err := config.Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if back.Sort != (config.Sort{Column: "output", Desc: true}) {
		t.Errorf("round trip lost the sort: %+v", back.Sort)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/config/ -run TestSortRoundTrips -v`
Expected: FAIL — `cfg.Sort undefined`.

- [ ] **Step 3: Implement**

In `internal/config/config.go`, after the `Filters` type:

```go
// Sort is the persisted models-table ordering.
//
// Column is a plain string rather than an openrouter.SortKey on purpose: this
// package deliberately depends on nothing else in the tree, and an
// unrecognised value must degrade to "relevance" at the boundary
// (launch.SortFrom) rather than fail a config load. A hand-edited or
// future-version config may not make the listing unusable.
type Sort struct {
	Column string `json:"column,omitempty"`
	Desc   bool   `json:"desc,omitempty"`
}
```

and add the field to `Config`, after `Filters`:

```go
	Sort      Sort      `json:"sort"`
```

`defaults()` is unchanged: the zero `Sort` is relevance, which is the
behaviour every existing user already has.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/config/ -run TestSortRoundTrips -v`
Expected: PASS.

- [ ] **Step 5: Mutation check**

Change the JSON tag to `json:"sorting"` → the test must FAIL on the raw-file
assertion. Revert.

- [ ] **Step 6: Commit**

```bash
git add internal/config
git commit -m "feat(config): persist the models-table sort"
```

---

### Task 4: `SortFrom` and `MergeSort`

**Files:**
- Modify: `internal/launch/filters.go`
- Test: `internal/launch/filters_test.go`

**Interfaces:**
- Consumes: `config.Sort` (Task 3), `openrouter.Sort`/`ParseSortKey` (Task 1).
- Produces: `launch.FlagSort = "sort"`, `launch.FlagDesc = "desc"`,
  `launch.SortFrom(config.Sort) openrouter.Sort`,
  `launch.MergeSort(config.Sort, openrouter.Sort, func(string) bool) openrouter.Sort`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/launch/filters_test.go`:

```go
func TestSortFromDegradesAnUnknownColumnToRelevance(t *testing.T) {
	got := launch.SortFrom(config.Sort{Column: "prompt", Desc: true})
	if got != (openrouter.Sort{}) {
		t.Errorf("SortFrom(unknown column) = %+v, want the zero Sort", got)
	}
	if got := launch.SortFrom(config.Sort{Column: "output", Desc: true}); got !=
		(openrouter.Sort{Key: openrouter.SortOutput, Desc: true}) {
		t.Errorf("SortFrom(valid) = %+v", got)
	}
}

func TestMergeSortPrefersTypedFlagsOverThePersistedSort(t *testing.T) {
	persisted := config.Sort{Column: "context", Desc: true}
	flags := openrouter.Sort{Key: openrouter.SortInput, Desc: false}

	none := func(string) bool { return false }
	if got := launch.MergeSort(persisted, flags, none); got !=
		(openrouter.Sort{Key: openrouter.SortContext, Desc: true}) {
		t.Errorf("with no flag typed, MergeSort = %+v, want the persisted sort", got)
	}

	onlySort := func(name string) bool { return name == launch.FlagSort }
	if got := launch.MergeSort(persisted, flags, onlySort); got !=
		(openrouter.Sort{Key: openrouter.SortInput, Desc: true}) {
		t.Errorf("--sort alone = %+v, want the flag's key and the persisted direction", got)
	}

	// The predicate is what makes an explicit --desc=false distinguishable
	// from an absent --desc, exactly as for --tools.
	onlyDesc := func(name string) bool { return name == launch.FlagDesc }
	if got := launch.MergeSort(persisted, flags, onlyDesc); got !=
		(openrouter.Sort{Key: openrouter.SortContext, Desc: false}) {
		t.Errorf("--desc=false alone = %+v, want the persisted key ascending", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/launch/ -run 'SortFrom|MergeSort' -v`
Expected: FAIL — `undefined: launch.SortFrom`.

- [ ] **Step 3: Implement**

In `internal/launch/filters.go`, extend the flag-name block and append:

```go
const (
	FlagTools      = "tools"
	FlagFree       = "free"
	FlagMinContext = "min-context"
	FlagMaxPrice   = "max-price"
	FlagSort       = "sort"
	FlagDesc       = "desc"
)

// SortFrom converts the persisted ordering into a catalog sort. An
// unrecognised column degrades to relevance rather than erroring: config.Sort
// holds a plain string, and a hand-edited or future-version value must not
// make `orl models` unusable. The command line is the opposite case — see
// newModelsCmd, where a typo is a hard error.
func SortFrom(s config.Sort) openrouter.Sort {
	key, err := openrouter.ParseSortKey(s.Column)
	if err != nil {
		return openrouter.Sort{}
	}
	return openrouter.Sort{Key: key, Desc: s.Desc}
}

// MergeSort returns the persisted sort overridden by each flag the user
// explicitly set, with the same changed-predicate rule MergeFilters uses.
func MergeSort(persisted config.Sort, flags openrouter.Sort,
	changed func(string) bool) openrouter.Sort {

	out := SortFrom(persisted)
	if changed(FlagSort) {
		out.Key = flags.Key
	}
	if changed(FlagDesc) {
		out.Desc = flags.Desc
	}
	return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/launch/ -run 'SortFrom|MergeSort' -v`
Expected: PASS.

- [ ] **Step 5: Mutation check**

Make `MergeSort` ignore `changed` and always take the flags →
`TestMergeSortPrefersTypedFlagsOverThePersistedSort`'s first case must FAIL.
Make `SortFrom` return `openrouter.Sort{Key: openrouter.SortKey(s.Column)}`
without parsing → the degrade case must FAIL. Revert both.

- [ ] **Step 6: Commit**

```bash
git add internal/launch
git commit -m "feat(launch): merge the persisted sort with --sort/--desc"
```

---

### Task 5: `orl models --sort` / `--desc`

**Files:**
- Modify: `internal/cli/models.go`
- Test: `internal/cli/models_test.go`

**Interfaces:**
- Consumes: `launch.MergeSort`, `launch.FlagSort`, `launch.FlagDesc`,
  `openrouter.ParseSortKey`, `openrouter.SortModels`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/cli/models_test.go`, following the file's existing harness
(`newHarness(t)` / running the root command with args and capturing output —
copy the exact shape from the tests already there):

```go
// rowOrder returns the model IDs in the order the table printed them.
func rowOrder(t *testing.T, out string, ids ...string) []string {
	t.Helper()
	var got []string
	for _, line := range strings.Split(out, "\n") {
		for _, id := range ids {
			if strings.Contains(line, id) {
				got = append(got, id)
			}
		}
	}
	return got
}

func TestModelsSortsByOutputPrice(t *testing.T) {
	h := newHarness(t)

	out := h.run(t, "models", "--sort", "output")
	want := []string{"qwen/qwen3-coder:free", "openai/o1-mini", "anthropic/claude-opus-4.6"}
	if got := rowOrder(t, out, want...); !reflect.DeepEqual(got, want) {
		t.Errorf("--sort output printed %v, want %v\n%s", got, want, out)
	}

	out = h.run(t, "models", "--sort", "output", "--desc")
	rev := []string{"anthropic/claude-opus-4.6", "openai/o1-mini", "qwen/qwen3-coder:free"}
	if got := rowOrder(t, out, rev...); !reflect.DeepEqual(got, rev) {
		t.Errorf("--sort output --desc printed %v, want %v\n%s", got, rev, out)
	}
}

func TestModelsRejectsAnUnknownSortColumn(t *testing.T) {
	h := newHarness(t)
	_, err := h.runErr(t, "models", "--sort", "prompt")
	if err == nil {
		t.Fatal("--sort prompt must error rather than silently printing catalog order")
	}
	if !strings.Contains(err.Error(), "output") {
		t.Errorf("error %q does not name the valid columns", err)
	}
}

func TestModelsToleratesAnUnknownSortColumnInTheConfig(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(t, &config.Config{Sort: config.Sort{Column: "prompt"}})

	out := h.run(t, "models")
	// Catalog order, unchanged — a bad config value degrades, never errors.
	want := []string{"anthropic/claude-opus-4.6", "qwen/qwen3-coder:free", "openai/o1-mini"}
	if got := rowOrder(t, out, want...); !reflect.DeepEqual(got, want) {
		t.Errorf("bad config sort changed the order: got %v, want %v", got, want)
	}
}

func TestModelsUsesThePersistedSortWithNoFlag(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(t, &config.Config{Sort: config.Sort{Column: "context"}})

	out := h.run(t, "models")
	want := []string{"openai/o1-mini", "anthropic/claude-opus-4.6", "qwen/qwen3-coder:free"}
	if got := rowOrder(t, out, want...); !reflect.DeepEqual(got, want) {
		t.Errorf("persisted sort ignored: got %v, want %v\n%s", got, want, out)
	}
}
```

If the harness has no `runErr`/`writeConfig` equivalent, use whatever the
neighbouring tests in `models_test.go` and `profile_test.go` already use to
run a failing command and to seed a config — do not add a second harness.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/cli/ -run TestModels -v`
Expected: FAIL — `unknown flag: --sort`.

- [ ] **Step 3: Implement**

In `internal/cli/models.go`:

```go
func newModelsCmd(a *app) *cobra.Command {
	var flagFilter openrouter.Filter
	var flagSort string
	var flagDesc bool
```

inside `RunE`, after the filter merge:

```go
			// A typo on the command line is fatal, unlike the same value in
			// the config: the user is standing right here, and silently
			// printing catalog order would look like the sort was applied.
			key, err := openrouter.ParseSortKey(flagSort)
			if err != nil {
				return err
			}
			sortBy := launch.MergeSort(cfg.Sort,
				openrouter.Sort{Key: key, Desc: flagDesc}, cmd.Flags().Changed)
```

and where the rows are built:

```go
			models := openrouter.SortModels(openrouter.Apply(snap.Models, filter), sortBy)
```

Register the flags with the others, and correct `--max-price`'s wording:

```go
	cmd.Flags().Float64Var(&flagFilter.MaxPrice, launch.FlagMaxPrice, 0,
		"maximum USD per million output tokens")
	cmd.Flags().StringVar(&flagSort, launch.FlagSort, "",
		"sort by column: model, context, input, output, tools")
	cmd.Flags().BoolVar(&flagDesc, launch.FlagDesc, false,
		"reverse the sort (largest, priciest, or Z-A first)")
```

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/cli/ -run TestModels -v`
Expected: PASS.

- [ ] **Step 5: Mutation checks**

1. Drop the `SortModels` wrapper (apply the filter only) →
   `TestModelsSortsByOutputPrice` must FAIL.
2. Wire `--desc` to the `Key` field instead / ignore it → the `--desc` case
   must FAIL.
3. Make the `ParseSortKey` error non-fatal (`key, _ :=`) →
   `TestModelsRejectsAnUnknownSortColumn` must FAIL.

- [ ] **Step 6: Commit**

```bash
git add internal/cli
git commit -m "feat(cli): sort orl models with --sort and --desc"
```

---

### Task 6: The picker sorts

**Files:**
- Modify: `internal/tui/filters.go`
- Modify: `internal/tui/picker.go` (`recompute`, `pickerHints`)
- Test: `internal/tui/filters_test.go`, `internal/tui/picker_test.go`

**Interfaces:**
- Consumes: `openrouter.Sort`, `openrouter.SortModels`, `ui.SortLabel`,
  `launch.SortFrom`.
- Produces: `filterState.sort openrouter.Sort`;
  `filterStateFrom(config.Filters, config.Sort) filterState`;
  `filterState.persistedSort() config.Sort`; `nextSortKey(openrouter.SortKey)
  openrouter.SortKey`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/picker_test.go`:

```go
// The load-bearing composition test. The fixture is chosen so relevance order
// and column order genuinely DISAGREE: searching "o" matches all three, and
// Rank puts openai/o1-mini first (ID prefix) while cheapest-output puts the
// free model first. A fixture where the two agree passes with the composition
// inverted and proves nothing.
func TestPickerSortAppliesOutsideRank(t *testing.T) {
	m := newPickerModel(pickerInput{
		Models:  ortest.Models(),
		Filters: filterState{search: "o"},
		Width:   120, Height: 40,
	})
	if got := m.visible[0].ID; got != "openai/o1-mini" {
		t.Fatalf("relevance order changed: first is %q, want openai/o1-mini", got)
	}

	m = newPickerModel(pickerInput{
		Models:  ortest.Models(),
		Filters: filterState{search: "o", sort: openrouter.Sort{Key: openrouter.SortOutput}},
		Width:   120, Height: 40,
	})
	want := []string{"qwen/qwen3-coder:free", "openai/o1-mini", "anthropic/claude-opus-4.6"}
	for i, id := range want {
		if m.visible[i].ID != id {
			t.Fatalf("sorted+searched order = %v, want %v (is SortModels inside Rank?)",
				modelIDs(m.visible), want)
		}
	}
}

func TestPickerKeepsTheSortAcrossASearchEdit(t *testing.T) {
	m := newPickerModel(pickerInput{
		Models:  ortest.Models(),
		Filters: filterState{sort: openrouter.Sort{Key: openrouter.SortOutput, Desc: true}},
		Width:   120, Height: 40,
	})
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	p := next.(pickerModel)
	if got := p.visible[0].ID; got != "anthropic/claude-opus-4.6" {
		t.Errorf("after typing, first row is %q, want the priciest — recompute dropped the sort", got)
	}
}

func TestPickerFooterAdvertisesFilterAndSort(t *testing.T) {
	m := newPickerModel(pickerInput{Models: ortest.Models(), Width: 120, Height: 40})
	if !strings.Contains(m.View(), "ctrl+f filter&sort") {
		t.Errorf("footer does not advertise filter&sort:\n%s", m.View())
	}
}
```

(`modelIDs` may already exist in the package; if not, add the two-line helper
next to the test.)

Append to `internal/tui/filters_test.go`:

```go
func TestLabelNamesTheSortOnlyWhenOneIsActive(t *testing.T) {
	if got := (filterState{}).label(); got != "no filters" {
		t.Errorf("idle label = %q, want %q — relevance is not worth a status line", got, "no filters")
	}
	got := filterState{sort: openrouter.Sort{Key: openrouter.SortOutput}}.label()
	if !strings.Contains(got, "OUTPUT/M") || !strings.Contains(got, "no filters") {
		t.Errorf("label = %q, want it to keep \"no filters\" and name OUTPUT/M", got)
	}
	desc := filterState{
		toolsOnly: true,
		sort:      openrouter.Sort{Key: openrouter.SortContext, Desc: true},
	}.label()
	if !strings.Contains(desc, "tools") || !strings.Contains(desc, "CONTEXT") ||
		!strings.Contains(desc, "↓") {
		t.Errorf("label = %q, want tools, CONTEXT and a descending arrow", desc)
	}
}

func TestNextSortKeyCyclesEveryColumnAndReturnsToRelevance(t *testing.T) {
	seen := []openrouter.SortKey{}
	k := openrouter.SortNone
	for i := 0; i < len(openrouter.SortKeys)+1; i++ {
		k = nextSortKey(k)
		seen = append(seen, k)
	}
	want := append(append([]openrouter.SortKey{}, openrouter.SortKeys...), openrouter.SortNone)
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("cycle = %v, want %v", seen, want)
	}
	// A value from a hand-edited config is not in the cycle; one press must
	// still land somewhere sane rather than looping on itself.
	if got := nextSortKey(openrouter.SortKey("bogus")); got != openrouter.SortKeys[0] {
		t.Errorf("nextSortKey(bogus) = %q, want %q", got, openrouter.SortKeys[0])
	}
}

func TestSortRoundTripsThroughTheFilterState(t *testing.T) {
	in := config.Sort{Column: "input", Desc: true}
	f := filterStateFrom(config.Filters{}, in)
	if f.sort != (openrouter.Sort{Key: openrouter.SortInput, Desc: true}) {
		t.Fatalf("filterStateFrom lost the sort: %+v", f.sort)
	}
	if got := f.persistedSort(); got != in {
		t.Errorf("persistedSort = %+v, want %+v", got, in)
	}
	if got := filterStateFrom(config.Filters{}, config.Sort{Column: "prompt"}).sort; got !=
		(openrouter.Sort{}) {
		t.Errorf("an unknown persisted column became %+v, want relevance", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/tui/ -run 'Sort|Label|FilterAndSort' -v`
Expected: FAIL — `unknown field sort in struct literal`, `undefined: nextSortKey`.

- [ ] **Step 3: Implement**

`internal/tui/filters.go` — add the field, the constructor argument, the
persisted form, the cycle, and the label clause (import `openrouter`,
`config`, `launch`, `ui`):

```go
type filterState struct {
	search     string
	toolsOnly  bool
	freeOnly   bool
	minContext int
	maxPrice   float64
	// sort orders the visible list. The zero value is "relevance", which is
	// the ordering the picker had before sorting existed: catalog order, or
	// best-match-first while searching.
	sort openrouter.Sort
}

func filterStateFrom(f config.Filters, s config.Sort) filterState {
	return filterState{
		toolsOnly:  f.ToolsOnly,
		freeOnly:   f.FreeOnly,
		minContext: f.MinContext,
		maxPrice:   f.MaxPrice,
		// launch.SortFrom, not a local parse: an unrecognised persisted
		// column must degrade to relevance in exactly one place, shared with
		// the CLI.
		sort: launch.SortFrom(s),
	}
}

// persistedSort is the sort's persisted form. Unlike the search box, the sort
// survives the session: a user who prefers cheapest-first should not have to
// say so every run.
func (f filterState) persistedSort() config.Sort {
	return config.Sort{Column: string(f.sort.Key), Desc: f.sort.Desc}
}

// nextSortKey advances the "Sort by" row: relevance, then each catalog column
// in header order, then back to relevance. A value not in the cycle — a
// hand-edited config — lands on the first column, since columns have no
// ordering for a "never silently widen" rule to preserve.
func nextSortKey(cur openrouter.SortKey) openrouter.SortKey {
	for i, k := range openrouter.SortKeys {
		if k == cur {
			if i+1 < len(openrouter.SortKeys) {
				return openrouter.SortKeys[i+1]
			}
			return openrouter.SortNone
		}
	}
	return openrouter.SortKeys[0]
}
```

and in `label()`, replace the final `return` with:

```go
	label := "no filters"
	if len(parts) > 0 {
		label = strings.Join(parts, " · ")
	}
	// The sort is appended rather than folded into parts: it is not a filter,
	// and "no filters" must survive next to it or the line would claim the
	// list is unfiltered only when it is also unsorted.
	if f.sort.Key != openrouter.SortNone {
		arrow := "↑"
		if f.sort.Desc {
			arrow = "↓"
		}
		label += " · sort:" + ui.SortLabel(f.sort.Key) + " " + arrow
	}
	return label
```

`internal/tui/picker.go` — `recompute` and the footer:

```go
// The order matters, in two steps. The four catalog filters run through
// openrouter.Apply; then the search runs through Rank, which orders by match
// quality; then the chosen column runs through SortModels, OUTSIDE Rank, so a
// column the user picked deliberately beats relevance and relevance survives
// only as the stable sort's tie-break. Sorting inside Rank's argument would
// invert that and looks identical at a glance.
func (m *pickerModel) recompute(keepID string) {
	m.visible = openrouter.SortModels(
		Rank(openrouter.Apply(m.all, m.filters.catalogFilter()), m.filters.search),
		m.filters.sort)
	m.cursor = indexOfModel(m.visible, keepID)
	m.clampScroll()
}
```

```go
var pickerHints = []string{
	"ctrl+f filter&sort", "ctrl+s save profile", "esc back",
}
```

- [ ] **Step 4: Run the package**

Run: `go test ./internal/tui/`
Expected: PASS. `filterStateFrom`'s new argument breaks its existing call
sites — fix them by passing `config.Sort{}` in tests and `cfg.Sort` in
`tui.go`'s `run` (Task 7 covers `run` properly; passing `cfg.Sort` here is
that same line).

- [ ] **Step 5: Mutation checks**

1. Move `SortModels` inside `Rank`'s argument →
   `TestPickerSortAppliesOutsideRank` must FAIL.
2. Delete the sort clause from `label()` →
   `TestLabelNamesTheSortOnlyWhenOneIsActive` must FAIL.
3. Make `label()` print the sort unconditionally → the same test must FAIL on
   its first case.

- [ ] **Step 6: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): order the picker by the chosen column, outside Rank"
```

---

### Task 7: The Filter & Sort screen, and persistence

**Files:**
- Modify: `internal/tui/filterscreen.go`
- Modify: `internal/tui/tui.go` (`run`, `session`, `persistFilters`)
- Test: `internal/tui/filterscreen_test.go`, `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: everything from Task 6.
- Produces: two new `filterRows` entries (indices 4 and 5); `session.savedSort`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/filterscreen_test.go`:

```go
func TestFilterScreenTitleAndRowsCoverSorting(t *testing.T) {
	m := newFilterScreenModel(filterScreenInput{
		Models: ortest.Models(), Width: 100, Height: 30,
	})
	view := m.View()
	for _, want := range []string{
		"Filter & Sort",
		"Sort by", "relevance", "order the table by this column",
		"Direction", "ascending", "which end of the sort comes first",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestFilterScreenCyclesTheSortRows(t *testing.T) {
	m := newFilterScreenModel(filterScreenInput{Models: ortest.Models(), Width: 100, Height: 30})

	// Down to "Sort by" (row 4), space once: relevance -> MODEL.
	for i := 0; i < 4; i++ {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(filterScreenModel)
	}
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = next.(filterScreenModel)
	if m.filters.sort.Key != openrouter.SortModel {
		t.Fatalf("space on Sort by gave %q, want model", m.filters.sort.Key)
	}

	// Down to "Direction" (row 5), space once: ascending -> descending.
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(filterScreenModel)
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = next.(filterScreenModel)
	if !m.filters.sort.Desc {
		t.Fatal("space on Direction did not flip to descending")
	}
	if m.filters.sort.Key != openrouter.SortModel {
		t.Errorf("Direction changed the column to %q — the rows are wired to the same field",
			m.filters.sort.Key)
	}

	// enter carries both edits back.
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(filterScreenModel)
	if !m.choice.Applied ||
		m.choice.Filters.sort != (openrouter.Sort{Key: openrouter.SortModel, Desc: true}) {
		t.Errorf("enter returned %+v, applied=%v", m.choice.Filters.sort, m.choice.Applied)
	}
}

func TestFilterScreenCancelDiscardsSortEdits(t *testing.T) {
	opened := filterState{sort: openrouter.Sort{Key: openrouter.SortContext}}
	m := newFilterScreenModel(filterScreenInput{
		Filters: opened, Models: ortest.Models(), Width: 100, Height: 30,
	})
	for i := 0; i < 4; i++ {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(filterScreenModel)
	}
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = next.(filterScreenModel)
	final, _ := m.cancel()
	if got := final.(filterScreenModel).choice.Filters.sort; got != opened.sort {
		t.Errorf("cancel leaked the sort edit: %+v, want %+v", got, opened.sort)
	}
}
```

Append to `internal/tui/tui_test.go` — the driver must persist the sort. Model
it on whatever existing test covers filter persistence (search
`persistFilters`/`savedFilters` in that file); if none exists, write:

```go
func TestSessionPersistsTheSort(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// ...build the stub screens the other driver tests use, with pick
	// returning pickBack and Filters carrying a chosen sort...
	// then, after Run returns:
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sort != (config.Sort{Column: "output", Desc: true}) {
		t.Errorf("session did not persist the sort: %+v", cfg.Sort)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/tui/ -run 'FilterScreen|PersistsTheSort' -v`
Expected: FAIL — the view has no "Sort by" row, and `cfg.Sort` stays zero.

- [ ] **Step 3: Implement**

`internal/tui/filterscreen.go` — two more entries at the end of `filterRows`:

```go
}, {
	label:   "Sort by",
	explain: "order the table by this column",
	value:   func(f filterState) string { return ui.SortLabel(f.sort.Key) },
	cycle:   func(f *filterState) { f.sort.Key = nextSortKey(f.sort.Key) },
}, {
	// Shown even while Sort by is relevance, where it does nothing: hiding it
	// would make the screen's row list depend on its own state, and the row
	// explains itself either way.
	label:   "Direction",
	explain: "which end of the sort comes first",
	value: func(f filterState) string {
		if f.sort.Desc {
			return "descending"
		}
		return "ascending"
	},
	cycle: func(f *filterState) { f.sort.Desc = !f.sort.Desc },
}}
```

and the title:

```go
	b.WriteString(titleStyle.Render("Filter & Sort") + "\n\n")
```

Update the `filterRow` doc comment: it says "adding a fifth filter is a table
entry" — make it "adding a row is a table entry", since two of them are now
sort rows, and note that the label/explain/value/cycle shape is what keeps a
binding from landing with no explanation next to it.

`internal/tui/tui.go`:

```go
	s := &session{
		ctx: ctx, opts: opts, sc: sc, cfg: cfg,
		filters:      filterStateFrom(cfg.Filters, cfg.Sort),
		savedFilters: cfg.Filters,
		savedSort:    cfg.Sort,
		...
	}
```

add `savedSort config.Sort` to the `session` struct beside `savedFilters`, and
extend the persistence function (rename it, since it is no longer only
filters):

```go
// persistView writes the filter and sort state if either changed, re-reading
// the config first. The re-read is not boilerplate: ctrl+s can add a profile
// during the very session whose view state is being written, and saving a
// config captured at start would delete it. launch.recordSelection re-reads
// for the same reason.
func (s *session) persistView() error {
	filters, sortBy := s.filters.persisted(), s.filters.persistedSort()
	if filters == s.savedFilters && sortBy == s.savedSort {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Filters = filters
	cfg.Sort = sortBy
	return config.Save(cfg)
}
```

and in `finish`, call `s.persistView()` with the notice title updated to
`"Could not save the filter and sort settings"`.

- [ ] **Step 4: Run the package**

Run: `go test ./internal/tui/`
Expected: PASS.

- [ ] **Step 5: Mutation checks**

1. Wire the Direction row's `cycle` to `f.sort.Key` →
   `TestFilterScreenCyclesTheSortRows` must FAIL on its "same field" check.
2. Drop `cfg.Sort = sortBy` from `persistView` → `TestSessionPersistsTheSort`
   must FAIL.
3. Drop `sortBy == s.savedSort` from the dirty check (leaving only the filter
   comparison) → `TestSessionPersistsTheSort` must FAIL when the filters are
   unchanged, which is exactly the case it sets up.

- [ ] **Step 6: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): add sorting to the ctrl+f screen, now Filter & Sort"
```

---

### Task 8: Docs, landmine, and the full gate

**Files:**
- Modify: `README.md:165`
- Modify: `HANDOFF.md` (state line, test count, Landmines, Phase 2 section)
- Modify: `CLAUDE.md` only if a claim there went stale (check the picker
  description)

- [ ] **Step 1: README**

Line 165's table row becomes:

```markdown
| `openrouter-launch models` | list models; `--tools --free --provider --min-context --max-price --sort --desc` |
```

Add one line under it documenting the sort keys:

```markdown
`--sort` takes `model`, `context`, `input`, `output`, or `tools`; `--desc`
reverses it. The interactive picker sorts by the same columns from its
`ctrl+f` **Filter & Sort** screen, and remembers the choice.
```

- [ ] **Step 2: HANDOFF.md**

- Update the **Last updated / State** line to lead with this change.
- Update the **Phase 2** row: the `ctrl+f` screen is now Filter & Sort.
- Update the **Tests** row with the real new count. Get it with
  `go test ./... -list '.*' | grep -c '^Test'` and record the delta and what
  produced it, in that row's existing style.
- Add **Landmine 38**, in the established voice:

> **38. Sorting composes OUTSIDE `Rank`, and unknown pricing sorts last in
> both directions.** Two independent traps in one small feature. (a)
> `pickerModel.recompute` is
> `SortModels(Rank(Apply(...), search), sort)` — a chosen column beats search
> relevance, and relevance survives only as the stable sort's tie-break.
> Sorting inside `Rank`'s argument type-checks, looks identical at a glance,
> and inverts the owner's decision; `TestPickerSortAppliesOutsideRank` needs a
> fixture where the two orders genuinely disagree, or it passes either way.
> (b) A model with `PricingUnknown` carries `0.0` in both price fields and
> renders `?`, so a numeric comparison heads a cheapest-first list with models
> whose price is simply unknown — Landmine 4's false "it's free" claim by a new
> route. `unknownLast` runs BEFORE the `Desc` swap, which is why the naive
> version is wrong ascending and accidentally right descending; the test
> asserts both directions for exactly that reason.

- Update the **Where things are** list with the new spec and plan paths.

- [ ] **Step 3: Run the full gate**

Run: `make ci`
Expected: green — fmt, vet, lint on 3 GOOS, actionlint, tidy, cross-build,
security, race, coverage above the 80% floor, and the isolated run.

If coverage dropped below the floor, the gap is in `sort.go`'s `lessBy`
default branch or `ParseSortKey`'s error path; add the missing case to
`sort_test.go` rather than lowering the floor.

- [ ] **Step 4: Commit and push**

```bash
git add README.md HANDOFF.md docs/superpowers/plans/2026-08-10-models-sort.md
git commit -m "docs: hand off the models-table sorting change"
git push origin develop
```

---

## Self-review

**Spec coverage:** rename → Task 2; sort primitive with all four rules →
Task 1; TUI composition and label → Task 6; the two screen rows, the title,
and the footer → Tasks 6 and 7; CLI flags and both error paths → Task 5;
persistence → Tasks 3, 4, 7; every row of the spec's test table appears in
some task's tests; docs and the landmine → Task 8.

**Types:** `Sort{Key, Desc}` is used identically in Tasks 1, 4, 5, 6, 7.
`config.Sort{Column, Desc}` (string column) crosses Tasks 3, 4, 6, 7.
`filterStateFrom` gains its second parameter in Task 6 and its call site in
`run` is fixed in the same task, with Task 7 owning the rest of `run`'s
change.

**Out of scope, per the spec:** sorting `orl agents`/`orl profile list`,
header-click sorting, reverse cycling, renaming `Model.PromptPricePerM`, and
secondary sort keys.
