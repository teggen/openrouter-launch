package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The whole resolution table from the design spec, in one place. Each row
// names the mutation it catches, because "handled" and "escaped" are two
// booleans and a latch that returned the wrong pair would still look busy.
func TestEscLatchResolutionTable(t *testing.T) {
	cases := []struct {
		name string
		// armed is whether an esc is pending when msg arrives.
		armed   bool
		msg     tea.Msg
		handled bool
		escaped bool
		// pending is the latch state afterwards.
		pending bool
	}{{
		// The ordinary path: nothing pending, so the latch is transparent and
		// the screen's own key handling runs. A latch that swallowed keys
		// while idle would make the picker inert.
		name:    "idle latch passes an ordinary rune through",
		armed:   false,
		msg:     runeKey('t'),
		handled: false,
	}, {
		// The defect this whole change exists for: bubbletea split ESC+t
		// across two reads, so the t must be swallowed rather than typed.
		name:    "pending esc plus a plain rune swallows both",
		armed:   true,
		msg:     runeKey('t'),
		handled: true,
		escaped: false,
	}, {
		// An esc followed by a real alt chord is two deliberate presses, not
		// one split chord — the alt survived, so no reconstruction is needed.
		name:    "pending esc plus an alt rune is a real esc",
		armed:   true,
		msg:     altKey('t'),
		handled: true,
		escaped: true,
	}, {
		// Only runes can be the tail of a split alt chord. Anything else
		// resolves the esc rather than being mistaken for one.
		name:    "pending esc plus a named key is a real esc",
		armed:   true,
		msg:     typeKey(tea.KeyDown),
		handled: true,
		escaped: true,
	}, {
		name:    "pending esc plus the timeout is a real esc",
		armed:   true,
		msg:     escTimeoutMsg{},
		handled: true,
		escaped: true,
	}, {
		// A tick whose esc was already resolved by a rune. It must be
		// swallowed, not allowed to fall through to the screen — and it must
		// not resolve anything, or the picker would close 40ms after every
		// alt chord.
		name:    "a stale timeout is swallowed and resolves nothing",
		armed:   false,
		msg:     escTimeoutMsg{},
		handled: true,
		escaped: false,
	}, {
		// A resize arriving inside the window must not consume the esc:
		// dropping it here would strand the latch pending forever, and
		// swallowing the resize would freeze the layout.
		name:    "a non-key message passes through and leaves the esc pending",
		armed:   true,
		msg:     tea.WindowSizeMsg{Width: 80, Height: 24},
		handled: false,
		escaped: false,
		pending: true,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var l escLatch
			if tc.armed {
				l.arm()
			}

			handled, escaped := l.step(tc.msg)

			if handled != tc.handled {
				t.Errorf("handled = %v, want %v", handled, tc.handled)
			}
			if escaped != tc.escaped {
				t.Errorf("escaped = %v, want %v", escaped, tc.escaped)
			}
			if l.pending != tc.pending {
				t.Errorf("pending = %v afterwards, want %v", l.pending, tc.pending)
			}
		})
	}
}

// arm must both record the esc and hand back the command that will resolve
// it. A latch that set the flag and returned nil would hang: with no tick
// scheduled, a real esc would wait for the next keypress to take effect.
func TestEscLatchArmSchedulesTheResolvingTick(t *testing.T) {
	var l escLatch

	cmd := l.arm()

	if !l.pending {
		t.Error("arm did not mark the esc pending")
	}
	if cmd == nil {
		t.Fatal("arm returned no command, so nothing would ever resolve the esc")
	}
	if _, ok := cmd().(escTimeoutMsg); !ok {
		t.Errorf("arm's command produced %T, want escTimeoutMsg", cmd())
	}
}
