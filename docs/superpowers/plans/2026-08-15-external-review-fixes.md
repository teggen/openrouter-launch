# External review fixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the eight verified defects from the 2026-08-15 external review
triage, without touching anything the triage rejected.

**Architecture:** Nine independent tasks, no shared state, each ending in a
green `make pre-commit` and its own commit. Two change launcher behaviour
(claude, droid), one changes a CLI signal, one changes catalog rendering, two
strengthen tests, three are documentation and dependency bookkeeping. Task
order runs highest user impact first; nothing after Task 1 depends on anything
before it, so tasks may be reordered or dropped individually.

**Tech Stack:** Go 1.26.6 local (`go 1.25` module floor), cobra, bubbletea,
lipgloss. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-08-15-external-review-triage.md` — read
the triage entry (T1–T10) cited by each task before changing code; it records
the evidence and, for three claims, why the reviews' own proposed fixes were
rejected.

## Global Constraints

- **Branch:** `develop`. Do not tag; do not push to `main`.
- **Module floor stays `go 1.25`** (Landmine 25). No task may raise it. If a
  dependency bump would, stop and report instead.
- **Zero-touch, five write sites** (Landmine 6). No task adds a write
  primitive anywhere. `TestWriteSitesAreExhaustivelyEnumerated` must stay green.
- **`Launcher.Command` must stay pure** — no writes, no network, no spawning.
  Task 1 adds a validation branch, which is pure; nothing else in this plan
  touches `Command`.
- **Every new test gets its mutation check**: break the behaviour, watch the
  *named* test fail, revert. A task is not done until its mutation check has
  been run and reported. This is the project's recurring review finding.
- **Tests that need a binary to look absent must call `testHome(t)`**
  (Landmine 8) — real installs exist on this machine.
- **`make pre-commit` must pass before every commit.** `make ci` at the end.
- HANDOFF.md is present on disk but **untracked and gitignored** as of
  2026-08-15. Never `git add` it. Edits to it are local-only by design.

---

### Task 1: `claude` rejects a conflicting model passthrough

Triage T2. Ten of eleven launchers call `rejectModelFlag`; claude does not, so
a passthrough `--model` reaches Claude Code's argv alongside the managed one.

**Files:**
- Modify: `internal/agent/claude.go:63-71` (inside `Command`, after the API-key guard)
- Test: `internal/agent/claude_test.go` (append)

**Interfaces:**
- Consumes: `rejectModelFlag(agentName string, args []string) error` from `internal/agent/args.go:14` — already in package scope, no import needed.
- Produces: nothing new. `Claude.Command`'s signature is unchanged.

- [ ] **Step 1: Write the failing test**

Append to `internal/agent/claude_test.go`. `slices` and `testModel`/`stubLookPath`
are already in that file's scope.

```go
// TestClaudeCommandRejectsConflictingExtras pins claude into the rule the
// other ten launchers already follow. Claude Code's own --model outranks the
// managed one on argv, and the ANTHROPIC_DEFAULT_*_MODEL env vars keep
// pointing at ours, so accepting both would run the session and its subagents
// on different models while every report says the managed one.
func TestClaudeCommandRejectsConflictingExtras(t *testing.T) {
	c := &Claude{LookPath: stubLookPath("/usr/local/bin/claude")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"-mx/y"}, {"--model", "x/y"}, {"--model=x/y"},
	} {
		if _, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}

	// The rule is about the conflict, not a ban on passthrough: everything
	// that does not touch the managed model must still reach argv, in order.
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "k",
		ExtraArgs: []string{"--resume", "--verbose"}})
	if err != nil {
		t.Fatalf("benign extras rejected: %v", err)
	}
	want := []string{"--model", "anthropic/claude-opus-4.6", "--resume", "--verbose"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %v, want %v", cmd.Args, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/agent/ -run TestClaudeCommandRejectsConflictingExtras -v`
Expected: FAIL, four `accepted, want conflict error` lines (one per form).

- [ ] **Step 3: Add the guard**

In `internal/agent/claude.go`, inside `Command`, directly after the API-key
check and before `c.findPath()`:

```go
	// Claude Code's own --model wins on argv, and it would win only there:
	// the ANTHROPIC_DEFAULT_*_MODEL and CLAUDE_CODE_SUBAGENT_MODEL vars below
	// still carry the managed model, so a passthrough --model splits the
	// session between two models while the tool reports one. Landmine 3's
	// failure class, on argv — same reason the other ten launchers reject it.
	if err := rejectModelFlag("claude", req.ExtraArgs); err != nil {
		return Command{}, err
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/agent/ -run TestClaude -v`
Expected: PASS, including the pre-existing `TestClaudeCommandArgs` (which
passes `--resume` and must be unaffected).

- [ ] **Step 5: Run the mutation check**

Comment out the four-line guard from Step 3. Run
`go test ./internal/agent/ -run TestClaudeCommandRejectsConflictingExtras -v`
and confirm it FAILS. Restore the guard. Report both outcomes in the commit
body or the task report.

- [ ] **Step 6: Commit**

```bash
make pre-commit
git add internal/agent/claude.go internal/agent/claude_test.go
git commit -m "fix(agent): reject a conflicting --model passthrough for claude

claude was the only supported launcher not calling rejectModelFlag, so
\`orl claude -- --model other/model\` put two --model flags on argv while
ANTHROPIC_DEFAULT_*_MODEL kept pointing at the managed one. Mutation check
run: removing the guard fails TestClaudeCommandRejectsConflictingExtras."
```

---

### Task 2: droid refuses a `customModels` it cannot understand

Triage T4. A valid-JSON settings file whose `customModels` is not an array is
silently overwritten by `Apply` and its key deleted by `restore`.

**Files:**
- Modify: `internal/agent/droid.go:139-150` (`foreignDroidModels`), `:89` (call in `Apply`), `:110` (call in `restore`)
- Test: `internal/agent/droid_test.go` (append)

**Interfaces:**
- Produces: `foreignDroidModels(path string, settings map[string]any) ([]any, error)` — signature changes from `(map[string]any) []any`. Both existing callers live in `Apply` and its `restore` closure and both already have `path` in scope.

- [ ] **Step 1: Write the failing test**

Append to `internal/agent/droid_test.go`, modelled on the existing
`TestDroidApplyRefusesUnparseableFile`:

```go
// TestDroidApplyRefusesWrongShapedCustomModels extends "never clobber what we
// cannot understand" from the whole-file parse to the one field we rewrite.
// A customModels that is valid JSON of another type used to fail the type
// assertion silently: Apply replaced the user's value and restore then took
// the len(kept)==0 branch and deleted the key outright.
func TestDroidApplyRefusesWrongShapedCustomModels(t *testing.T) {
	home := testHome(t)
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.local.json")
	original := `{"customModels":"see the other file","model":"custom:theirs"}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &Droid{}
	if _, err := d.Apply(Request{Model: testModel(), APIKey: "sk"}); err == nil {
		t.Fatal("Apply accepted a customModels it could not understand")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original {
		t.Errorf("file was modified:\n got %s\nwant %s", raw, original)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/agent/ -run TestDroidApplyRefusesWrongShapedCustomModels -v`
Expected: FAIL at `Apply accepted a customModels it could not understand`.

- [ ] **Step 3: Make the shape a hard error**

Replace `foreignDroidModels` in `internal/agent/droid.go`:

```go
// foreignDroidModels returns customModels entries we do not own, in their
// original order. A user editing the file mid-session keeps their entries.
//
// A customModels that is present but not a list is an error rather than an
// empty result. Treating it as empty would let Apply overwrite the user's
// value and restore then delete the key, losing it for good — which breaks
// both readDroidSettingsFile's "never clobber what we cannot understand" rule
// and Apply's promise to return the restore that undoes exactly what it did.
func foreignDroidModels(path string, settings map[string]any) ([]any, error) {
	raw, present := settings["customModels"]
	if !present {
		return nil, nil
	}
	models, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("droid: customModels in %s is %T, not a list; refusing to modify it", path, raw)
	}
	var kept []any
	for _, item := range models {
		if entry, ok := item.(map[string]any); ok && entry["displayName"] == droidMarker {
			continue
		}
		kept = append(kept, item)
	}
	return kept, nil
}
```

In `Apply`, replace `kept := foreignDroidModels(settings)` with:

```go
	kept, err := foreignDroidModels(path, settings)
	if err != nil {
		return nil, err
	}
```

In the `restore` closure, replace `kept := foreignDroidModels(settings)` with:

```go
		kept, err := foreignDroidModels(path, settings)
		if err != nil {
			return err
		}
```

Returning the error from `restore` is correct: `launchConfigWriter` joins it
with the child's error so an `*exec.ExitError` still survives (Landmine 24).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/agent/ -run TestDroid -v`
Expected: PASS for all nine pre-existing `TestDroid*` tests plus the new one.
`TestDroidApplyPreservesForeignEntriesAndPriorDefault` and
`TestDroidRestoreKeepsFileWhenUserAddedEntriesMidSession` are the ones that
prove the happy path still works.

- [ ] **Step 5: Run the mutation check**

Change the new guard back to `models, _ := raw.([]any)` (dropping the `ok`
check). Run `go test ./internal/agent/ -run TestDroidApplyRefusesWrongShapedCustomModels -v`
and confirm it FAILS. Revert.

- [ ] **Step 6: Commit**

```bash
make pre-commit
git add internal/agent/droid.go internal/agent/droid_test.go
git commit -m "fix(agent): refuse a droid customModels that is not a list

A valid-JSON settings.local.json whose customModels was a string failed the
type assertion silently, so Apply overwrote it and restore deleted the key.
Extends the whole-file 'never clobber what we cannot understand' rule to the
one field we rewrite. Mutation check run: restoring the discarded ok fails
TestDroidApplyRefusesWrongShapedCustomModels."
```

---

### Task 3: correct README's Windows caveat

Triage T3. The bullet claims a blocking, green CI leg is advisory and red, and
it ships inside every release archive.

**Files:**
- Modify: `README.md:146-153`

**Interfaces:** none. No test reads README (verified: no `_test.go` file
references it).

- [ ] **Step 1: Replace the bullet**

Delete the existing bullet in "Known caveats" that begins **"The Windows test
leg is advisory and currently red."** and put this in its place. The residual
risk in the old bullet's last sentence is real and is what the new bullet
leads with:

```markdown
- **Nobody has run the binary on real Windows.** All three CI legs (Linux,
  macOS, Windows) are blocking and green — the Windows platform-fixture
  failures were fixed on 2026-08-09, not skipped — but that covers the test
  suite, not the shipped binary. No end-to-end run (catalog fetch, interactive
  TUI, agent launch) has happened on Windows, and exit-code propagation in
  particular is unverified there.
```

- [ ] **Step 2: Verify no other stale CI claim remains**

Run: `grep -n -i "continue-on-error\|advisory\|experimental\|19 platform" README.md`
Expected: no hits. Then confirm the source of truth still says what the new
bullet claims: `grep -n -A6 "matrix:" .github/workflows/ci.yml` must show
`experimental: false` for all three OSes.

- [ ] **Step 3: Commit**

```bash
make pre-commit
git add README.md
git commit -m "docs: correct the README's stale Windows CI caveat

The bullet still described the pre-2026-08-09 state (continue-on-error, 19
platform-fixture failures); all three legs have been blocking and green since.
Kept the part that is still true and led with it: the binary has never been
run on real Windows. v0.3.0's archives carry the old text until the next
release."
```

---

### Task 4: `--desc` with no sort column is an error, not a no-op

Triage T6. `orl models --desc` on a config with no saved sort column prints
catalog order and exits 0.

**Files:**
- Modify: `internal/cli/models.go` (after the `launch.MergeSort` call)
- Test: `internal/cli/models_test.go` (append)

**Interfaces:**
- Consumes: `openrouter.SortNone`, `launch.FlagDesc`, `launch.FlagSort`, and `cmd.Flags().Changed` — all already used in this file.

- [ ] **Step 1: Write the failing test**

Append to `internal/cli/models_test.go`. `h.exec` returns `(string, error)`;
`newHarness` isolates `XDG_CONFIG_HOME` to a temp dir, so the config has no
saved sort column.

```go
// TestModelsDescWithoutASortColumnErrors mirrors the --sort typo rule
// directly above it: SortModels returns catalog order for SortNone whatever
// Desc says, so accepting the flag would print an unsorted table that looks
// sorted.
func TestModelsDescWithoutASortColumnErrors(t *testing.T) {
	h := newHarness(t)

	_, err := h.exec("models", "--desc")
	if err == nil {
		t.Fatal("--desc with no sort column must error rather than silently printing catalog order")
	}
	if !strings.Contains(err.Error(), "--sort") {
		t.Errorf("error %q does not point at the flag that fixes it", err)
	}

	// With a column it is meaningful, and must still work.
	if _, err := h.exec("models", "--sort", "output", "--desc"); err != nil {
		t.Errorf("--sort output --desc rejected: %v", err)
	}
}
```

- [ ] **Step 2: Add a saved-column regression test**

Also append — a saved column must satisfy the requirement, or the guard would
break the persisted-sort path:

```go
// TestModelsDescAloneWorksWithASavedSortColumn keeps the Task 4 guard from
// reading the typed flags only: MergeSort merges the saved column in, so
// --desc alone is meaningful for a user who saved one from the picker.
func TestModelsDescAloneWorksWithASavedSortColumn(t *testing.T) {
	h := newHarness(t)
	if err := config.Save(&config.Config{Sort: config.Sort{Column: "output"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.exec("models", "--desc"); err != nil {
		t.Errorf("--desc rejected despite a saved sort column: %v", err)
	}
}
```

`config` is already imported in `models_test.go` (used by
`TestModelsToleratesAnUnknownSortColumnInTheConfig`).

- [ ] **Step 3: Run both tests to verify the first fails**

Run: `go test ./internal/cli/ -run 'TestModelsDesc' -v`
Expected: `TestModelsDescWithoutASortColumnErrors` FAILS at the `t.Fatal`;
`TestModelsDescAloneWorksWithASavedSortColumn` PASSES already.

- [ ] **Step 4: Add the guard**

In `internal/cli/models.go`, directly after the `sortBy := launch.MergeSort(...)`
assignment:

```go
			// --desc alone cannot do anything: SortModels returns an
			// unchanged copy for SortNone whatever Desc says. Same call as
			// the ParseSortKey typo above — the user is standing right here,
			// and printing catalog order would look like the flag applied.
			// Checked against the MERGED key, so a saved column still counts.
			if sortBy.Key == openrouter.SortNone && cmd.Flags().Changed(launch.FlagDesc) {
				return fmt.Errorf("--%s needs a sort column: add --%s (model, context, input, output, tools)",
					launch.FlagDesc, launch.FlagSort)
			}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run TestModels -v`
Expected: PASS, including the pre-existing `--sort output --desc` ordering
test around `models_test.go:232`.

- [ ] **Step 6: Run the mutation check**

Delete the guard. Confirm `TestModelsDescWithoutASortColumnErrors` FAILS.
Then restore it but check `flagDesc` instead of the merged `sortBy.Key`, and
confirm `TestModelsDescAloneWorksWithASavedSortColumn` FAILS. Revert to the
Step 4 form.

- [ ] **Step 7: Commit**

```bash
make pre-commit
git add internal/cli/models.go internal/cli/models_test.go
git commit -m "fix(cli): error on --desc when no sort column is in effect

orl models --desc on a fresh config printed catalog order and exited 0 —
byte-identical to plain 'models'. Checked against the merged key so a column
saved from the picker still satisfies it. Mutation checks run for both the
guard and the merged-vs-typed distinction."
```

---

### Task 5: render sub-1k contexts and sub-cent prices honestly

Triage T7. `FormatContext` truncates 1..999 tokens to `0k`; `FormatPrice`
renders any nonzero price under half a cent as `$0.00`. Latent today (zero of
413 live models hit either), wrong on their own terms.

**Files:**
- Modify: `internal/openrouter/format.go`
- Test: `internal/openrouter/format_test.go`

**Interfaces:** none changed — `FormatContext(int) string` and
`FormatPrice(float64, bool) string` keep their signatures.

- [ ] **Step 1: Write the failing tests**

Append to `internal/openrouter/format_test.go`:

```go
func TestFormatContextBelowOneThousand(t *testing.T) {
	// tokens/1000 truncates, so every one of these rendered "0k" — which
	// reads as no context at all rather than a small one.
	for _, tokens := range []int{1, 512, 999} {
		if got := FormatContext(tokens); got != "<1k" {
			t.Errorf("FormatContext(%d) = %q, want %q", tokens, got, "<1k")
		}
	}
	// The boundary and the existing behaviour are unchanged.
	if got := FormatContext(1000); got != "1k" {
		t.Errorf("FormatContext(1000) = %q, want %q", got, "1k")
	}
	if got := FormatContext(0); got != "-" {
		t.Errorf("FormatContext(0) = %q, want %q", got, "-")
	}
}

func TestFormatPriceBelowTheTwoDecimalFloor(t *testing.T) {
	// "$0.00" for a real price is Landmine 4's misreading one rounding step
	// removed: it is not free, and must not look free.
	for _, price := range []float64{0.0001, 0.004} {
		if got := FormatPrice(price, false); got != "<$0.01" {
			t.Errorf("FormatPrice(%v) = %q, want %q", price, got, "<$0.01")
		}
	}
	// Free, unknown, and ordinary prices are untouched.
	if got := FormatPrice(0, false); got != "free" {
		t.Errorf("FormatPrice(0) = %q, want %q", got, "free")
	}
	if got := FormatPrice(0, true); got != "?" {
		t.Errorf("FormatPrice(unknown) = %q, want %q", got, "?")
	}
	if got := FormatPrice(0.005, false); got != "$0.01" {
		t.Errorf("FormatPrice(0.005) = %q, want %q", got, "$0.01")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/openrouter/ -run 'TestFormatContextBelowOneThousand|TestFormatPriceBelowTheTwoDecimalFloor' -v`
Expected: FAIL with `= "0k", want "<1k"` and `= "$0.00", want "<$0.01"`.

- [ ] **Step 3: Add the branches**

`internal/openrouter/format.go`:

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/openrouter/ ./internal/ui/ ./internal/cli/ ./internal/tui/ -count=1`
Expected: PASS everywhere. The existing `TestFormatPrice` and
`TestFormatContext` assert only values outside the two new branches (0, 1.1,
15, and -1, 0, 128000, 1000000), so neither needs changing. `<$0.01` is one
column wider than `$0.00`, so the table-width tests are the ones that would
notice:
`TestMaxWidthCapsAnOverflowingTable`, `TestPickerViewClampsRowsToTheAvailableWidth`,
`TestPickerShedsCatalogColumnsOnNarrowTerminals`.

- [ ] **Step 5: Run the mutation check**

Delete the `< 1000` branch, confirm `TestFormatContextBelowOneThousand` FAILS.
Restore. Delete the `< 0.005` branch, confirm
`TestFormatPriceBelowTheTwoDecimalFloor` FAILS. Restore.

- [ ] **Step 6: Commit**

```bash
make pre-commit
git add internal/openrouter/format.go internal/openrouter/format_test.go
git commit -m "fix(openrouter): render sub-1k contexts and sub-cent prices honestly

FormatContext truncated 1..999 tokens to '0k' (no context, not small);
FormatPrice rendered any nonzero price under half a cent as '\$0.00', one
rounding step from the free reading Landmine 4 exists to prevent. No model in
today's 413-model catalog hits either, so this closes a latent case.
Mutation checks run for both branches."
```

---

### Task 6: pin the rounding in `perMillion`, and fix the comment that misnames it

Triage T9 and T10. The doc comment's example is not noisy; no test can fail if
`math.Round` is deleted.

**Files:**
- Modify: `internal/openrouter/model.go:83-96`
- Test: `internal/openrouter/model_test.go` (append)

**Interfaces:** none changed — `perMillion(raw string) (float64, bool)` keeps
its signature and behaviour.

- [ ] **Step 1: Write the failing test**

Append to `internal/openrouter/model_test.go`. Use an inline JSON literal, as
`TestDecodeModelsUnknownPricingIsNotFree` does — adding a fourth model to
`testdata/models.json` would break `TestDecodeModelsFields`' `len(models) != 3`
guard and the index-based assertions in every other fixture test.

```go
// TestDecodeModelsPricingRoundsAwayFloatNoise pins the math.Round in
// perMillion. The values are the point: 0.000015 and 0.0000011 (the fixture's
// prices, asserted elsewhere) scale to exact binary floats and pass with the
// rounding deleted, so neither can catch its removal. These two cannot —
// 0.00000097 scales to 0.97000000000000008438 and 0.0000029 to
// 2.9000000000000003553.
func TestDecodeModelsPricingRoundsAwayFloatNoise(t *testing.T) {
	data := []byte(`{"data":[{"id":"acme/cheap","name":"Acme: Cheap","context_length":8000,"pricing":{"prompt":"0.00000097","completion":"0.0000029"},"supported_parameters":["tools"]}]}`)

	models, err := DecodeModels(data)
	if err != nil {
		t.Fatalf("DecodeModels: %v", err)
	}
	if got := models[0].PromptPricePerM; got != 0.97 {
		t.Errorf("prompt price = %.20g, want exactly 0.97", got)
	}
	if got := models[0].CompletionPricePerM; got != 2.9 {
		t.Errorf("completion price = %.20g, want exactly 2.9", got)
	}
}
```

- [ ] **Step 2: Run it — it must PASS**

Run: `go test ./internal/openrouter/ -run TestDecodeModelsPricingRoundsAwayFloatNoise -v`
Expected: PASS. This test pins behaviour that is already correct, so the
mutation check in Step 4 — not a red-first run — is what proves it has teeth.

- [ ] **Step 3: Fix the doc comment**

In `internal/openrouter/model.go`, replace the first paragraph of
`perMillion`'s comment. Leave the rest of the comment and the body unchanged:

```go
// perMillion converts a per-token USD price string to USD per million tokens,
// reporting whether the value parsed. Rounding to six decimals removes float
// noise so prices compare, sort, and render exactly: 0.00000097 scales to
// 0.97000000000000008438, which must be 0.97. (Not every price is noisy —
// 0.000015 scales to exactly 15 — which is why the test pins a value that is.)
```

- [ ] **Step 4: Run the mutation check**

In `perMillion`, replace `return math.Round(v*1e6*1e6) / 1e6, true` with
`return v * 1e6 * 1e6 / 1e6, true`. Run
`go test ./internal/openrouter/ -run TestDecodeModelsPricingRoundsAwayFloatNoise -v`
and confirm it FAILS with `0.97000000000000008438`. Revert.

Then confirm the gap this closes is real: with the same mutation in place, run
`go test ./internal/openrouter/ -run 'TestDecodeModelsFields|TestDecodeModelsPricingIsExact' -v`
and confirm both still PASS. Revert. Report both halves.

- [ ] **Step 5: Commit**

```bash
make pre-commit
git add internal/openrouter/model.go internal/openrouter/model_test.go
git commit -m "test(openrouter): pin perMillion's rounding with a value that needs it

No existing test could fail if math.Round were deleted: 15, 75, 1.1 and 4.4
all scale exactly with or without it. 0.00000097 does not. Also corrects the
doc comment, which cited 0.000015 -> 15.000000000000002 as the motivating
case; that value is exact. Mutation check run, both directions."
```

---

### Task 7: pin omp's and qwen's *absence* of a credential-shadow check

Triage T8. Only cline's absence is machine-checked.

**Files:**
- Test: `internal/agent/omp_test.go`, `internal/agent/qwen_test.go` (append to each)

**Interfaces:**
- Consumes: `CredentialShadowCheck` (`internal/agent/agent.go:81`), types `OMP` and `Qwen`.

- [ ] **Step 1: Write the tests**

Append to `internal/agent/omp_test.go`:

```go
// TestOMPDoesNotClaimCredentialShadowing pins a declined dependency as a
// decision with a test to answer rather than a comment to overlook. omp's
// stored credentials live in a SQLite database (~/.omp/agent/agent.db) and
// outrank the env key, but reading them would mean taking a sqlite dependency
// for one advisory string. If you are here because you added a check, update
// the README caveat that promises omp gets no warning.
func TestOMPDoesNotClaimCredentialShadowing(t *testing.T) {
	if _, ok := any(&OMP{}).(CredentialShadowCheck); ok {
		t.Error("*OMP implements CredentialShadowCheck; the sqlite dependency was deliberately declined")
	}
}
```

Append to `internal/agent/qwen_test.go`:

```go
// TestQwenDoesNotClaimCredentialShadowing pins the same decision for qwen,
// whose gap is a different shape: a modelProviders.openai[] entry in
// ~/.qwen/settings.json may outrank --auth-type openai, and no detector ships
// because the collision has never been confirmed against a real qwen install.
// A detector added before that evidence exists would be guessing.
func TestQwenDoesNotClaimCredentialShadowing(t *testing.T) {
	if _, ok := any(&Qwen{}).(CredentialShadowCheck); ok {
		t.Error("*Qwen implements CredentialShadowCheck; no detector ships pending live evidence")
	}
}
```

- [ ] **Step 2: Run them**

Run: `go test ./internal/agent/ -run 'DoesNotClaimCredentialShadowing' -v`
Expected: PASS (three tests — cline's plus the two new ones).

- [ ] **Step 3: Run the mutation check**

Add a throwaway method to `internal/agent/omp.go`:
`func (o *OMP) ShadowedCredential() string { return "" }`. Confirm
`TestOMPDoesNotClaimCredentialShadowing` FAILS. Delete it. Repeat for `Qwen`
in `internal/agent/qwen.go` against its own test. Delete.

- [ ] **Step 4: Commit**

```bash
make pre-commit
git add internal/agent/omp_test.go internal/agent/qwen_test.go
git commit -m "test(agent): pin omp's and qwen's absent credential-shadow checks

Both gaps are deliberate and documented in the README, but only cline's
absence was machine-checked. Mutation check run: adding a stub
ShadowedCredential to either type fails its named test."
```

---

### Task 8: retire the stale "four write sites" texts

Triage T5. The allowlist has five entries; three prose sites still say four.

**Files:**
- Modify: `writesites_test.go:40-46` (doc comment only — the allowlist itself is correct)
- Modify: `HANDOFF.md` (local, untracked — **never `git add` it**)

- [ ] **Step 1: Fix the test's doc comment**

In `writesites_test.go`, replace the `TestWriteSitesAreExhaustivelyEnumerated`
doc comment:

```go
// TestWriteSitesAreExhaustivelyEnumerated pins Landmine 6 as a regression
// tripwire instead of leaving it as a grep a human has to remember to run:
// it walks every non-test .go file in the module and asserts that a raw
// write primitive appears only in the five sanctioned files above, and
// that each of those five still has one (an entry the allowlist keeps
// around after its write moved elsewhere would silently understate the
// real enumeration).
```

- [ ] **Step 2: Verify the count is the only thing that was wrong**

Run: `go test . -run TestWriteSitesAreExhaustivelyEnumerated -v` — PASS.
Run the Landmine 6 grep by hand and confirm five files, no more:

```bash
grep -rn "os.WriteFile\|os.Create(\|os.MkdirAll\|os.Rename\|OpenFile\|CreateTemp" \
  --include="*.go" . | grep -v _test | cut -d: -f1 | sort -u
```
Expected, exactly: `./internal/agent/cline.go`, `./internal/agent/droid.go`,
`./internal/config/config.go`, `./internal/launch/handoff.go`,
`./internal/openrouter/cache.go`.

- [ ] **Step 3: Fix HANDOFF.md (local only)**

Three edits in the "Verify the tree is sound" section (~lines 1540, 1561, 1567):

1. `**Write-site verification** (Landmine 6, four sites — see the table above):`
   → `... (Landmine 6, five sites — see the table above):`
2. In "Expected hits, exhaustively", add the missing fifth after the droid
   entry: `, and \`internal/agent/cline.go\` (\`Apply\`/restore, snapshotting
   and restoring \`~/.cline/data/settings/providers.json\` — restore-only, see
   Landmine 36)`. Change "Confirmed exhaustive against exactly these **four
   files**, 2026-08-09 (Task 6)" to "**five files**", and note the cline
   amendment date.
3. `a hit turns up outside \`writeSiteAllowlist\`'s four files, or if one of
   those four stops having a hit` → `five files` / `those five`.

- [ ] **Step 4: Record the two accepted-as-is observations in HANDOFF.md (local only)**

Add to the open items or landmine notes, so the triage's "verified but not
acted on" entries are not rediscovered as fresh findings:

- kimi's `ShadowedCredential` matches `legacyKimiPaths[0]` only, so a legacy
  install at `~/.local/bin/kimi` never warns. Deliberate — the comment scopes
  the heuristic to the uv tools dir and trusts PATH hits — and left alone
  pending live evidence.
- Declining the CLI confirm exits 1; backing out of the TUI exits 0. Both
  intended: a refused instruction versus a completed session.

- [ ] **Step 5: Commit the untracking, and only the untracking**

The staged `HANDOFF.md` deletion and the `.gitignore` entry are the
2026-08-15 owner decision (triage: glm H1). Land them together so the reason
is in one commit message. Confirm `git status --porcelain` shows HANDOFF.md as
neither staged nor untracked (it is ignored) before committing.

```bash
make pre-commit
git add .gitignore writesites_test.go
git status --short   # HANDOFF.md must NOT appear
git commit -m "chore: stop tracking HANDOFF.md, and fix the stale write-site count

HANDOFF.md stays on disk as the local working document and leaves the repo by
owner decision. Note the consequence: README.md and CLAUDE.md both point
readers at it, and 50 Landmine citations in .go comments reference its
numbering, so a fresh clone gets dangling pointers — see the open question in
docs/superpowers/plans/2026-08-15-external-review-fixes.md.

Separately, writesites_test.go's doc comment still said 'four' sanctioned
files beside a five-entry allowlist. The invariant was always correct; only
the prose lagged the cline amendment."
```

---

### Task 9: bump `golang.org/x/sys` to clear the uncalled advisory

Triage T1. GO-2026-5024 (Windows-only, not called) is the one finding
`govulncheck` still reports.

> **Status: attempted, blocked, parked (2026-08-15).** This task was run
> exactly as written below and did not land. Step 1's own expectation held
> only on its literal wording; Step 2's expectation — "the `go 1.25`
> directive is untouched" — did not. `go get golang.org/x/sys@v0.44.0 && go
> mod tidy` rewrites this module's directive from `go 1.25` to `go 1.25.0`,
> deterministically: `go mod tidy` re-derives the directive from the maximum
> precision found anywhere in the module graph, and `x/sys@v0.44.0`'s own
> `go.mod` spells it with three components. `go 1.25` and `go 1.25.0` are the
> same numeric floor, but `actions/setup-go`'s `go-version-file: go.mod`
> resolves them differently — from setup-go v7's `docs/advanced-usage.md`:
>
> > The `go` directive in `go.mod` can specify a patch version or omit it
> > altogether (e.g., `go 1.25.0` or `go 1.25`). If a patch version is
> > specified, that specific patch version will be used. If no patch version
> > is specified, it will search for the latest available patch version…
>
> All six `setup-go` invocations in this repo (`.github/workflows/ci.yml`
> lines 44, 83, 173, 216; `.github/workflows/release.yml` lines 21, 67) have
> no `check-latest`, so the rewrite would pin CI *and releases* to exactly
> go1.25.0 — reintroducing the four reachable stdlib vulnerabilities
> Landmine 25's toolchain bump had just closed — in exchange for clearing one
> uncalled, Windows-only advisory. That is a regression, not a cleanup, so
> the bump was reverted before commit and this task stays open. Full trace,
> including the mechanism check against `actions/setup-go`'s own source:
> `.superpowers/sdd/2026-08-15-external-review-fixes/task-9-report.md`. The
> original design rationale this collides with predates this plan:
> `docs/superpowers/plans/2026-08-09-ci-cd-makefile-readme.md:16` — "`go
> 1.25.0` would pin CI to the oldest 1.25 patch and reproduce the very
> problem this raise was meant to fix." The corresponding triage entry (T1
> in `docs/superpowers/specs/2026-08-15-external-review-triage.md`) has been
> corrected to match; it originally asserted the opposite of what Step 2
> below proved.
>
> The steps below are left as originally written, as the record of the
> rejected change — do not re-run them expecting a different outcome without
> first re-reading the status note above.

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Confirm the floor is safe before bumping**

`x/sys@v0.44.0` declares `go 1.25.0`, so it cannot raise this module's floor
(Landmine 25). Verify rather than trust:

```bash
curl -s https://raw.githubusercontent.com/golang/sys/v0.44.0/go.mod
```
Expected: `go 1.25.0`. If it says anything ≥ 1.26, **stop** and report — the
bump would raise the floor and that is a separate decision.

**What actually happened:** this expectation held (`x/sys@v0.44.0`'s own
floor is `1.25.0`, not ≥ 1.26) — and it is not the whole story. The numeric
floor is safe; what breaks is `go-version-file` resolution, which depends on
how many components *this* module's own directive has after `go mod tidy`
runs, not on `x/sys`'s numeric value alone. See the status note above.

- [ ] **Step 2: Bump and tidy**

```bash
go get golang.org/x/sys@v0.44.0
go mod tidy
git diff go.mod
```
Expected: only the `golang.org/x/sys` indirect line changes, and the `go 1.25`
directive is untouched.

**What actually happened:** it did not hold. `go get` printed `go: upgraded
go 1.25 => 1.25.0` before `go mod tidy` even ran, and the rewrite survived a
hand-revert-and-retidy. See the status note above for why that one-line diff
is a CI and release regression, not a formatting nit — this is where the
task was stopped and the working tree reverted.

- [ ] **Step 3: Verify the advisory is gone and nothing broke**

```bash
govulncheck ./... | tail -20
make ci
```
Expected: govulncheck no longer lists GO-2026-5024; `make ci` exits 0.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): bump golang.org/x/sys to v0.44.0

Clears GO-2026-5024 (Windows-only, not called by this code) — the last
finding govulncheck reported after the local toolchain moved to go1.26.6.
x/sys v0.44.0 declares go 1.25.0, so the Landmine 25 floor is untouched."
```

---

## Final verification

- [ ] `make ci` exits 0
- [ ] `make test-isolated` passes (Landmine 8 — Tasks 2 and 7 add tests that touch HOME)
- [ ] `make test-race` passes
- [ ] `go test . -run TestWriteSitesAreExhaustivelyEnumerated -v` passes
- [ ] `git status --short` shows no stray files and no HANDOFF.md
- [ ] Every mutation check in Tasks 1, 2, 4, 5, 6, 7 has been run and its
      outcome reported — this is the gate the project's recurring review
      finding exists for

## Open question for the owner

HANDOFF.md now leaves the repo (Task 8) while `README.md` ("HANDOFF.md is the
canonical project state"), `CLAUDE.md` ("Read HANDOFF.md first"), and 50
`Landmine N` citations in `.go` comments still point at it. A fresh clone
resolves none of them.

This plan does not attempt a rewrite, because the right answer is an editorial
call rather than a mechanical one — fold the load-bearing landmines into the
code comments that cite them, move them to a tracked `ARCHITECTURE.md`, or
accept the dangling pointers as the cost of keeping the document private.
Deciding it is a separate task with its own spec.
