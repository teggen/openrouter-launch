# Picker filters: a discoverable screen, and an unambiguous esc

Date: 2026-08-10
Status: approved

## The two defects

Reported by the owner against `v0.2.0`, running `orl` over SSH
(`TERM=xterm-256color`; the emulator that decides what alt+t emits lives on
the client and is not observable from the server):

1. **The alt chords leak their letter into the search box.** Pressing
   `alt+t` / `alt+f` / `alt+c` / `alt+p` types `t` / `f` / `c` / `p` into the
   picker's search.
2. **The chords are unexplained.** The footer reads
   `alt+t tools · alt+f free · alt+c ctx · alt+p price` and that is the whole
   of the documentation. A new user cannot tell what "ctx" does, what it is
   currently set to, or that `alt+c` cycles rather than toggles.

## Why defect 1 is not in the model

`pickerModel.handleKey` already switches on `key.String()` before the
search-append branch, so `alt+t` cannot fall through to it, and
`TestAltKeysRenderDistinctlyFromPlainKeys` pins the bubbletea rendering that
rests on. The search-append branch additionally guards on `!key.Alt`. The
model is correct; the letter is arriving as a *plain rune*, which means the
`alt` never survived the trip from the terminal to `tea.KeyMsg`.

Exactly two things can do that, and they are fixed by different mechanisms:

| Case | What the terminal sends | What bubbletea 1.3.10 produces | Second symptom |
|---|---|---|---|
| **B — split read** | `ESC` `t` | `esc`, then a plain `t` | picker also closes |
| **C — no ESC** | `t` | a plain `t` | filter never toggles |

Case B is bubbletea's escape ambiguity. `readAnsiInputs` sets
`canHaveMoreData := numBytes == len(buf)` against a 256-byte buffer
(`key.go:579`), so a read that returns a lone `\x1b` is treated as a complete
event boundary and `detectOneMsg` falls through to `key.go:707`:

```go
// We didn't find an escape sequence, nor a valid rune. Was this a
// lone escape character at the end of the input?
if alt && len(b) == 1 {
    return 1, KeyMsg(Key{Type: KeyEscape})
}
```

The `t` in the next read is then an ordinary rune. Over SSH the two bytes
normally share a TCP segment, so the split happens at the `read()` boundary
rather than after a real delay.

Case C is not recoverable by parsing: a bare `t` carries no evidence that Alt
was held. It can only be fixed by moving the chord onto a key the search box
can never claim.

The owner asked for both cases covered without a further diagnostic round
trip, so this design ships both mechanisms.

## Mechanism A — `escLatch`

`esc` stops acting on arrival. It sets a pending flag and returns a
`tea.Tick`; the next event resolves it:

| Next event | Resolution |
|---|---|
| plain rune (`KeyRunes`, `!Alt`) inside the window | the pair was `alt+<rune>` split across reads — **both swallowed** |
| any other `tea.KeyMsg` | the pending esc resolves as a **real esc**; the key that resolved it is dropped |
| `escTimeoutMsg` while pending | the pending esc resolves as a **real esc** |
| anything else (e.g. `tea.WindowSizeMsg`) | passes through, esc stays pending |

`escWindow` is 40 ms. The real gap between the two bytes is sub-millisecond,
so the window is generous; 40 ms is imperceptible on a genuine esc, and it is
the same order as the timeout readline and vim use for the identical
ambiguity.

**No generation counter on the tick.** A tick that fires after its own esc was
already resolved finds `pending == false` and is swallowed. The only way a
stale tick can reach a *pending* esc is if a second esc was pressed inside the
window, and resolving that one early performs exactly the action it was going
to perform anyway. A counter would add state that no observable behaviour
depends on.

The latch is a shared type rather than per-screen logic, because two screens
need it and its resolution table is the part worth testing once:

```go
// step reports whether the latch consumed msg, and whether that resolved a
// pending esc into a real one.
func (l *escLatch) step(msg tea.Msg) (handled, escaped bool)
```

### Where it is applied

- **Picker** — required; this is the reported defect.
- **Filters screen** — `esc` there discards edits, so a stray chord losing
  them is worse than a stray letter.
- **Not** root, confirm, or notice: they are menu screens with no text input,
  where the letter half of a split chord already does nothing.
- **Not** `prompt.go` (API-key entry), deliberately. It has the identical
  text-input hazard, but it advertises no chords, so nothing invites the
  keypress. Deferred rather than overlooked; folding it in later is three
  lines plus its own headless program test.

## Mechanism B — the filters screen

`ctrl+f` opens a screen. A control key is what makes this work in case C: the
search box can never claim it, whatever the terminal does with Alt.

```
  Filters

  ›  Tools          ON     only models that can call tools
     Free           off    only models priced at $0
     Min context    128K   hide models with a smaller context window
     Max price      any    hide models above this price per million tokens

     12 of 318 models match

  ↑/↓ move · space toggle/cycle · enter apply · esc cancel
```

Each row carries its **name**, its **current value**, and **what it actually
does** — which is the whole of defect 2. The live match count recomputes as
you edit, so the effect of a filter is visible before you commit to it.

The rows are a declarative table (`label`, `explain`, `value`, `cycle`),
mirroring how `internal/agent` declares its registry: adding a fifth filter is
a table entry, not a new `case`.

The count uses the **identical expression the picker uses** —
`Rank(openrouter.Apply(all, f.catalogFilter()), f.search)` — not just
`Apply`. `filterState` carries the session's search text, and a count that
ignored it would disagree with the picker's own status line whenever a search
was active.

### Keymap

- `↑`/`↓`, `ctrl+p`/`ctrl+n` — move
- `space` — toggle (Tools, Free) or cycle (Min context, Max price), reusing
  `nextContext`/`nextPrice`, whose never-silently-widen rule is unchanged
- `enter` — apply and return to the picker
- `esc` — cancel, restoring the filters the screen opened with
- `ctrl+c` — end the session, as on every other screen

### Alt chords are dropped

The four `alt+` cases are deleted from the picker. Terminals that *do* deliver
`alt+t` now produce a `tea.KeyMsg` no branch claims, and the search-append
branch's existing `!key.Alt` guard means it types nothing. The chord becomes
inert rather than harmful — which is the correct outcome for a binding that
is no longer advertised.

This is the owner's choice among three options, taken for the smallest
terminal-dependent surface: after this change nothing in the keymap depends on
how the emulator treats Alt.

### `ctrl+f` has no non-empty-list guard

`enter` and `ctrl+s` both return early when `len(m.visible) == 0`, because
neither means anything without a highlighted model. `ctrl+f` must **not** copy
that guard: a filter combination that matches nothing is precisely when the
user needs to reach the filters screen to undo it. A guard there would trap
them with `esc` as the only exit.

## Wiring

The launch pipeline is unchanged; this is entirely within `internal/tui`.

- **`filterscreen.go`** (new) — `filterScreenInput` / `filterScreenModel` /
  `filterScreenChoice`, following the existing one-program-per-screen shape.
- **`picker.go`** — four `alt+` cases deleted; `ctrl+f` returns a new
  `pickFilters` choice kind carrying the highlighted model; `esc` arms the
  latch; footer becomes `ctrl+f filters · ctrl+s save profile · esc back`.
- **`tui.go`** — `screens` gains `filters func(filterScreenInput)
  (filterScreenChoice, error)`; `stepPicker` routes `pickFilters` through it
  and returns to `statePicker`, following the round trip `pickSaveProfile`
  already makes.
- **`program.go`** — the `filters` closure. `pickerProgramOptions` is renamed
  `altScreenOptions`, since two screens now want the alt screen; the filters
  screen takes it so the detour does not flicker back through the main screen
  and leave the panel in scrollback.

`chromeHeight()` already measures `len(m.footer())`, so the shorter footer
needs no constant updated — Landmine 17's budget adapts on its own.

### Undecided exits

`liveScreens.pick` maps `!m.done` to `pickBack` carrying the live filters. The
`filters` closure follows the same rule: an undecided exit returns
`filterScreenChoice{Filters: in.Filters}` with `Applied` false — the filters
the screen opened with, unmodified — so a signal or a lost terminal cannot
silently commit half-made edits.

## Testing

The project's recurring review finding is tests that pass for the wrong
reason, so each of these names the mutation it must catch.

| Test | Fails when |
|---|---|
| `escLatch` resolution table, unit | any row of the table above is wrong |
| picker: split reader, headless program | the latch is removed |
| picker: `ctrl+f` returns `pickFilters` with the highlighted model | `ctrl+f` is unbound, or drops `ModelID` |
| picker: `ctrl+f` works with an empty visible list | someone copies `enter`'s guard onto it |
| picker: `alt+t`/`f`/`c`/`p` change neither filters nor search | a chord is re-added, or the `!key.Alt` guard is dropped |
| picker: `esc` still returns `pickBack` with live filters | the latch never resolves |
| filters screen: `space` toggles and cycles each row | a `cycle` func is wired to the wrong field |
| filters screen: `esc` returns the *original* filters | cancel leaks the live edits |
| filters screen: `enter` returns the edited filters with `Applied` | apply is dropped |
| filters screen: view names every filter, value, and explanation | a row loses its explanation — defect 2 regressing |
| filters screen: match count tracks edits **and** honours search | the count is built from `Apply` alone |
| driver: `pickFilters` opens the screen and reopens the picker | routing is dropped |
| driver: cancelled filters screen leaves `s.filters` untouched | `Applied` is ignored |
| `liveScreens` wires `filters` (headless, per Landmine 16) | the closure is nil |

**The split-reader test is the load-bearing one.**
`bytes.NewBufferString("\x1bt")` hands bubbletea both bytes in one `read()`,
which parses as `alt+t` and would pass with the latch deleted. Reproducing the
defect requires a reader that returns `\x1b` and `t` on **separate `Read`
calls**, and blocks rather than returning `io.EOF` once drained, so the
program ends on its own terms. Without the latch that test must see the picker
return `pickBack` with `t` in the search; with it, neither.

## Out of scope

- `prompt.go`'s latch, as above.
- Any change to `internal/launch`, `internal/agent`, or the write-site set.
  This design adds no write site; filters continue to persist through the
  existing `config.Filters` path on session exit.
- Left/right stepping the cycles backwards. `space` forward through a cycle
  that always returns to unconstrained is enough; a reverse key can be added
  when someone wants it.
