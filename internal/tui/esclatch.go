package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// escWindow is how long a pending esc waits for the rune that would reveal it
// as half of an alt chord.
//
// The two bytes of ESC+t leave the terminal together and are split only at
// bubbletea's read() boundary, so the real gap is sub-millisecond; 40ms is
// generous for that and imperceptible on a genuine esc. It is the same order
// as the timeout readline and vim use for the identical ambiguity.
const escWindow = 40 * time.Millisecond

// escTimeoutMsg resolves a pending esc that no key arrived to claim.
type escTimeoutMsg struct{}

// escLatch defers an esc until it is clear whether the user pressed esc or an
// alt chord whose two bytes landed in separate reads.
//
// bubbletea 1.3.10 computes canHaveMoreData as numBytes == len(buf) against a
// 256-byte buffer (key.go:579), so a read returning a lone \x1b looks like a
// complete event boundary and detectOneMsg reports a bare KeyEscape
// (key.go:707). The rune from the next read then arrives unmodified, which on
// a type-to-search screen means the chord closes the screen AND types its
// letter. Holding the esc for one short window makes the split case
// indistinguishable from the same-read case.
type escLatch struct {
	pending bool
}

// arm records a pending esc and returns the command that resolves it if no
// key claims it first.
func (l *escLatch) arm() tea.Cmd {
	l.pending = true
	return tea.Tick(escWindow, func(time.Time) tea.Msg { return escTimeoutMsg{} })
}

// step reports whether the latch consumed msg, and whether consuming it
// resolved a pending esc into a real one.
//
// A stale tick — one whose esc a rune already resolved — is consumed and
// resolves nothing. No generation counter guards it: the only way a stale
// tick can reach a pending esc is a second esc pressed inside the window, and
// resolving that one early performs exactly the action it was going to
// perform anyway.
func (l *escLatch) step(msg tea.Msg) (handled, escaped bool) {
	if !l.pending {
		_, stale := msg.(escTimeoutMsg)
		return stale, false
	}

	switch key := msg.(type) {
	case escTimeoutMsg:
		l.pending = false
		return true, true

	case tea.KeyMsg:
		l.pending = false
		// Only a plain rune can be the tail of a split alt chord: had the alt
		// survived, bubbletea would have delivered one alt-modified message
		// instead of two. Swallow both halves — the chord is unbound, so
		// reconstructing it would be a no-op anyway, and the point is that
		// neither the esc nor the letter takes effect.
		if key.Type == tea.KeyRunes && !key.Alt {
			return true, false
		}
		return true, true
	}

	// Anything else (a resize, say) is none of the latch's business and must
	// not consume the esc, which is still waiting for its window to close.
	return false, false
}
