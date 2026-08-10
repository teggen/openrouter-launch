# Sorting the models table, and the INPUT/OUTPUT rename

Date: 2026-08-10
Status: approved

## What the owner asked for

1. The models table must be sortable by any of its columns
   (MODEL │ CONTEXT │ PROMPT/M │ COMPLETION/M │ TOOLS).
2. The sort configuration lives in the `ctrl+f` filters screen, which is
   renamed **Filter & Sort**.
3. The two price columns are retitled **INPUT** and **OUTPUT** rather than
   PROMPT and COMPLETION.

Three owner decisions taken at design review, each of which had a defensible
opposite:

- **A column sort overrides search relevance.** With a sort chosen, the search
  box only narrows the list; the column orders it.
- **`orl models` sorts too**, via `--sort` / `--desc`. The sort is therefore
  domain code in `internal/openrouter`, not TUI code.
- **The sort persists** in `config.json`, like the four filters, not
  session-only like the search box.

## The rename

`ui.ModelHeaders` becomes:

```go
var ModelHeaders = []string{"MODEL", "CONTEXT", "INPUT/M", "OUTPUT/M", "TOOLS"}
```

That is the whole of it. `orl models`, the TUI picker, and `tableFrame` all
read the same slice, which is exactly why it was extracted into `internal/ui`
in the first place. Two comments follow it: `catalogDropOrder`'s trailing
`// COMPLETION/M, PROMPT/M, CONTEXT, TOOLS`, and `--max-price`'s help text
("maximum USD per million **output** tokens"). The flag name, the config key,
and `openrouter.Model`'s field names are **not** renamed — `PromptPricePerM`
and `CompletionPricePerM` mirror OpenRouter's own JSON (`pricing.prompt`,
`pricing.completion`), and renaming the Go fields would put our vocabulary at
odds with the wire format for no user-visible gain.

The rename makes the five-column table about five columns narrower, which can
only help Landmine 33/34's shedding arithmetic. Nothing there is tuned to a
specific width.

## The sort primitive — `internal/openrouter/sort.go`

```go
type SortKey string

const (
	SortNone    SortKey = ""         // leave the order alone
	SortModel   SortKey = "model"
	SortContext SortKey = "context"
	SortInput   SortKey = "input"
	SortOutput  SortKey = "output"
	SortTools   SortKey = "tools"
)

// Sort is a column plus a direction. The zero value sorts nothing.
type Sort struct {
	Key  SortKey
	Desc bool
}

// ParseSortKey resolves a user-supplied name, case-insensitively.
func ParseSortKey(s string) (SortKey, error)

// SortModels returns models ordered by s, leaving the input untouched.
func SortModels(models []Model, s Sort) []Model
```

Four rules, each of which is a plausible "cleanup" away from being wrong:

**1. `SortNone` is the zero value and means "do not reorder".** Today's
behaviour — catalog order in the CLI, `Rank`'s relevance order in the picker —
is what you get from a `Sort{}`. Every existing caller therefore keeps its
current output by construction, and the feature is opt-in at both surfaces.

**2. Unknown pricing sorts LAST in both directions.** This is Landmine 4
("unknown pricing is never free") restated for ordering. A model whose price
failed to parse carries `PricingUnknown` and renders `?`; comparing it as
`0.0` would put it at the top of a cheapest-first list, which is precisely the
false "this is free" claim the landmine forbids — and it would put it at the
*bottom* of an expensive-first list, so a naive implementation is wrong in one
direction and accidentally right in the other. `?` rows go last whichever way
the arrow points, on both INPUT and OUTPUT.

**3. The sort is stable.** `sort.SliceStable`, so equal keys keep the order
they arrived in: relevance in the picker, catalog order in the CLI. That is
what makes `--sort tools` (a boolean with two values and hundreds of ties)
produce a useful listing rather than an arbitrary one, and it is what keeps
the output deterministic across runs.

**4. Ascending is the natural order of the underlying value.** MODEL A→Z by
ID, CONTEXT small→large, INPUT/OUTPUT cheap→expensive, TOOLS without→with.
There is deliberately no per-column "smart" default direction: picking CONTEXT
shows the smallest windows first until you flip the direction row. Predictable
beats clever here, and the flip is one keystroke.

MODEL compares `strings.ToLower(ID)`, not `Name` — the ID is what the column
shows and what `-m` takes.

## The TUI

`filterState` gains one field, `sort openrouter.Sort`, and `recompute`
composes it outermost:

```go
m.visible = openrouter.SortModels(
	Rank(openrouter.Apply(m.all, m.filters.catalogFilter()), m.filters.search),
	m.filters.sort)
```

Sort outside `Rank` is the owner's decision made mechanical: with a column
chosen, relevance survives only as the tie-break inside a stable sort. Putting
`SortModels` *inside* `Rank`'s argument would invert that — the search would
re-rank whatever the sort had just ordered — and it is the kind of swap that
looks equivalent at a glance, so it gets its own test.

`filterScreenModel.matches()` is unchanged and deliberately does **not** sort:
it counts, and a count cannot depend on order.

### The screen

```
  Filter & Sort

  ›  Tools          ON           only models that can call tools
     Free           off          only models priced at $0
     Min context    128K         hide models with a smaller context window
     Max price      any          hide models above this price per million tokens
     Sort by        OUTPUT/M     order the table by this column
     Direction      ascending    which end of the sort comes first

     12 of 318 models match

  ↑/↓ move · space toggle/cycle · enter apply · esc cancel
```

The two new rows are ordinary `filterRow` entries — `label`, `explain`,
`value`, `cycle` — so the property that a row cannot half-land as a binding
with no explanation next to it still holds, and `space` remains the only
editing key on the screen.

- **Sort by** cycles `relevance → MODEL → CONTEXT → INPUT/M → OUTPUT/M →
  TOOLS → relevance`. The idle value is spelled `relevance`, not `none` or
  `off`: it describes what the ordering actually is.
- **Direction** toggles `ascending` / `descending`. It is shown, and
  cycleable, even while Sort by is `relevance`, where it has no effect —
  hiding or disabling it would make the screen's row list depend on its own
  state, and the row explains itself either way.

Six rows in one flat list, no section heading: the explanations already carry
the distinction, and this screen has to fit a 24-line terminal.

The column values reuse `ui.ModelHeaders`, so the row shows the same string
the table's header shows and the rename cannot drift out of the two places.

### Everything else about the screen is unchanged

The esc latch, `ctrl+c` ending the session, `esc` restoring the filters the
screen opened with, `ctrl+f` having no `len(m.visible) == 0` guard, the live
match count built from `Rank(Apply(...))` — all of Landmine 37 survives
untouched. The cursor bound is `len(filterRows)`, which is already computed,
so two more rows need no arithmetic changed.

### The picker

- Footer: `ctrl+f filter&sort · ctrl+s save profile · esc back`. `hintLines`
  packs it and `chromeHeight()` measures the result, so the longer hint costs
  a list row only on a terminal narrow enough to wrap it (Landmine 33c).
- Status line: `filterState.label()` appends `· sort:OUTPUT/M ↑` when a sort
  is active, and nothing at all when it is `relevance`. The picker has always
  said what it is doing to the list; ordering is now part of that.
  `clampLine` already bounds the line (Landmine 35).

## The CLI

```bash
orl models --sort output --desc
orl models --sort context anthropic
```

`--sort` takes a `SortKey` name; `--desc` reverses it. The merge mirrors
`MergeFilters` exactly — persisted value as the baseline, a flag the user
actually typed wins, decided by `cmd.Flags().Changed`:

```go
func MergeSort(persisted config.Sort, flags openrouter.Sort,
	changed func(string) bool) openrouter.Sort
```

placed beside `MergeFilters` in `internal/launch/filters.go`, with
`FlagSort`/`FlagDesc` name constants next to the existing ones.

Two error paths, deliberately different:

- **`--sort bogus`** fails the command with a message listing the valid keys.
  A typo on the command line must not silently produce catalog order.
- **A bad value in `config.json`** degrades to `relevance`. A hand-edited or
  future-version config must not make `orl models` unusable, and the TUI
  writes only valid values back.

## Persistence

`config.Config` gains a top-level field:

```go
// Sort is the persisted models-table ordering.
type Sort struct {
	Column string `json:"column,omitempty"`
	Desc   bool   `json:"desc,omitempty"`
}

Sort Sort `json:"sort"`
```

Top-level, **not** a field inside `Filters`: `config.Filters` maps to
`openrouter.Filter`, which is a predicate set consumed by `Apply`, and an
ordering is not a predicate. Folding it in would put a non-filter through
`MergeFilters` and `FilterFrom` and muddle both.

`Column` is a plain `string` rather than a `SortKey` so the config package
keeps no dependency on `internal/openrouter` — the same reason `Filters` holds
plain fields today. Validation happens at the boundary, in `ParseSortKey`.

The TUI writes it on the existing session-exit path that already writes
`cfg.Filters`; no new write site (Landmine 6's table is unchanged — this
touches write site 2's *contents*, not its count).

## Testing

The project's recurring review finding is tests that pass for the wrong
reason, so each row names the mutation it must catch. Every one is
mutation-checked before being trusted: break the behaviour, watch the named
test fail, revert.

| Test | Fails when |
|---|---|
| `SortModels` orders each of the five columns, both directions | a comparator reads the wrong field, or `Desc` is ignored |
| unknown pricing sorts last ascending **and** descending | `PricingUnknown` is compared as `0.0` |
| equal keys keep their incoming order | `sort.Slice` replaces `sort.SliceStable` |
| `SortModels` does not mutate its argument | it sorts the caller's slice in place |
| `Sort{}` returns catalog order | `SortNone` grows a comparator |
| `ParseSortKey` accepts the five names case-insensitively, rejects others | a name is dropped, or the error path is silent |
| picker: sort composes OUTSIDE `Rank` | the two are swapped |
| picker: sort survives a search edit and a filter change | `recompute` drops the sort on one path |
| filters screen: Sort by cycles through all six values and wraps | a cycle entry is missing |
| filters screen: Direction toggles and is applied on `enter` | the row is wired to the wrong field |
| filters screen: view names both new rows, values, and explanations | a row loses its explanation |
| filters screen: title reads `Filter & Sort` | the rename is reverted |
| picker: footer says `ctrl+f filter&sort` | ditto |
| picker: `label()` shows the sort only when one is active | it prints `sort:relevance` on a fresh session |
| `orl models --sort output --desc` row order | the CLI wires the flag to the wrong field, or drops `--desc` |
| `orl models --sort bogus` errors | an invalid key falls through to catalog order |
| a bad `sort.column` in config degrades to relevance | the CLI errors on a config it should tolerate |
| `MergeSort`: persisted baseline, typed flag wins | `Changed` is not consulted |
| config round-trips `Sort` | the JSON tag is wrong |
| `ui.ModelHeaders` is INPUT/M, OUTPUT/M | the rename is reverted |

The sort-composition test is the load-bearing one. It needs a fixture where
relevance order and column order genuinely differ — a search whose best match
is *not* the cheapest model — because a fixture where they agree passes with
the composition inverted and proves nothing.

## Out of scope

- Sorting the other listings (`orl agents`, `orl profile list`). They have a
  handful of rows in a meaningful order already.
- A click/keypress on the table header to sort. The picker's header is not
  interactive, and adding a per-column chord reopens exactly the
  terminal-dependent keymap surface Landmine 37 closed.
- Reverse-stepping the cycles. `space` forward, wrapping to the idle value, is
  the screen's existing idiom.
- Renaming `Model.PromptPricePerM` / `CompletionPricePerM` or the
  `--max-price` flag. See the rename section.
- Secondary sort keys. Stability gives a useful implicit second key already.
