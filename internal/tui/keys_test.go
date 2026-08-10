package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// runeKey is a plain printable keypress.
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// altKey is a printable key with Alt held. bubbletea renders this as
// "alt+t", which is a distinct String() from "t" — the property the picker's
// key handling depends on.
func altKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true}
}

// typeKey is a named key: tea.KeyEnter, tea.KeyEsc, tea.KeyBackspace, ...
func typeKey(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

// typeRunes turns a string into one keypress per rune.
func typeRunes(s string) []tea.KeyMsg {
	out := make([]tea.KeyMsg, 0, len(s))
	for _, r := range s {
		out = append(out, runeKey(r))
	}
	return out
}

// press feeds keys to a model in order and returns it with its concrete type
// restored. Update returns tea.Model, so without this every test would need
// its own type assertion, and a model that accidentally returned a different
// type would surface as a confusing panic instead of a named failure.
func press[M tea.Model](t *testing.T, m M, keys ...tea.KeyMsg) M {
	t.Helper()
	msgs := make([]tea.Msg, len(keys))
	for i, k := range keys {
		msgs[i] = k
	}
	return send(t, m, msgs...)
}

// send is press for messages a screen raises itself rather than reads from
// the terminal. escTimeoutMsg is why it exists: escLatch defers esc, so a
// test that fed only the keypress would observe a screen that has not
// decided anything yet.
func send[M tea.Model](t *testing.T, m M, msgs ...tea.Msg) M {
	t.Helper()
	var cur tea.Model = m
	for _, msg := range msgs {
		cur, _ = cur.Update(msg)
	}
	out, ok := cur.(M)
	if !ok {
		t.Fatalf("Update returned %T, want %T", cur, m)
	}
	return out
}

// realEsc is the pair of events a genuine esc produces: the keypress, then
// the tick that resolves it once the alt-chord window has closed with no
// rune claiming it.
func realEsc() []tea.Msg { return []tea.Msg{typeKey(tea.KeyEsc), escTimeoutMsg{}} }

// Sanity check on the assumption the whole picker key map rests on. If
// bubbletea ever changed this rendering, the picker's chord handling would
// silently start typing letters into the search box.
func TestAltKeysRenderDistinctlyFromPlainKeys(t *testing.T) {
	if got := altKey('t').String(); got != "alt+t" {
		t.Fatalf("altKey('t').String() = %q, want %q", got, "alt+t")
	}
	if got := runeKey('t').String(); got != "t" {
		t.Fatalf("runeKey('t').String() = %q, want %q", got, "t")
	}
}
