package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func noticeFixture() noticeModel {
	return newNoticeModel(noticeInput{
		Title: "Claude Code is not installed.",
		Lines: []string{"Install it from https://claude.com/claude-code"},
	})
}

func TestNoticeDismissKeysQuit(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		typeKey(tea.KeyEnter), typeKey(tea.KeyEsc), runeKey('q'),
	} {
		if m := press(t, noticeFixture(), k); !m.done {
			t.Errorf("key %q did not dismiss the notice", k.String())
		}
	}
}

// ctrl+c must be distinguishable from the other dismiss keys: the driver
// turns it into an immediate ErrCancelled instead of the notice's own
// routing (noticeThen/noticeThenFatal).
func TestNoticeCtrlCInterruptsDistinctlyFromOtherDismissKeys(t *testing.T) {
	m := press(t, noticeFixture(), typeKey(tea.KeyCtrlC))
	if !m.done || !m.interrupted {
		t.Errorf("done=%v interrupted=%v, want both true", m.done, m.interrupted)
	}
}

func TestNoticeEscDoesNotInterrupt(t *testing.T) {
	m := press(t, noticeFixture(), typeKey(tea.KeyEsc))
	if m.interrupted {
		t.Error("esc marked interrupted; only ctrl+c should")
	}
}

func TestNoticeIgnoresOtherKeys(t *testing.T) {
	if m := press(t, noticeFixture(), runeKey('x')); m.done {
		t.Error("an unrelated key dismissed the notice")
	}
}

// The typed errors carry their payload as data precisely so it can be
// rendered as something other than a line of error text; if View dropped the
// extra lines, NotInstalledError.Hint would never reach the user.
func TestNoticeViewShowsTitleAndEveryLine(t *testing.T) {
	got := noticeFixture().View()
	for _, want := range []string{"not installed", "claude.com/claude-code"} {
		if !strings.Contains(got, want) {
			t.Errorf("View = %q, missing %q", got, want)
		}
	}
}
