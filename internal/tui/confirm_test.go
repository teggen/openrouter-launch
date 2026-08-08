package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func questionModel() confirmModel {
	return newConfirmModel(confirmInput{
		Title:    "Before launching",
		Lines:    []string{"warning: model may not be fully compatible"},
		Question: "Launch anyway?",
	})
}

func ackModel() confirmModel {
	return newConfirmModel(confirmInput{
		Title: "Before launching",
		Lines: []string{"warning: using cached data from 3h ago"},
	})
}

func TestConfirmQuestionModeYesAnswersYes(t *testing.T) {
	m := press(t, questionModel(), runeKey('y'))
	if !m.done || !m.answer {
		t.Errorf("done=%v answer=%v, want both true", m.done, m.answer)
	}
}

// Enter must default to NO, matching the CLI's [y/N] prompt. If enter
// defaulted to yes, a stray keypress would launch against an incompatible
// model.
func TestConfirmQuestionModeEnterDefaultsToNo(t *testing.T) {
	m := press(t, questionModel(), typeKey(tea.KeyEnter))
	if !m.done {
		t.Fatal("enter did not resolve the prompt")
	}
	if m.answer {
		t.Error("enter answered yes; the CLI prompt defaults to no")
	}
}

func TestConfirmQuestionModeEscAnswersNo(t *testing.T) {
	m := press(t, questionModel(), typeKey(tea.KeyEsc))
	if !m.done || m.answer {
		t.Errorf("done=%v answer=%v, want done and no", m.done, m.answer)
	}
}

// A `default:` branch that quit on any key would make every stray keystroke
// an answer.
func TestConfirmQuestionModeIgnoresUnrelatedKeys(t *testing.T) {
	m := press(t, questionModel(), runeKey('x'), runeKey('1'))
	if m.done {
		t.Error("an unrelated key resolved the prompt")
	}
}

func TestConfirmAcknowledgeModeEnterProceeds(t *testing.T) {
	m := press(t, ackModel(), typeKey(tea.KeyEnter))
	if !m.done || !m.answer {
		t.Errorf("done=%v answer=%v, want both true", m.done, m.answer)
	}
}

func TestConfirmAcknowledgeModeEscBacksOut(t *testing.T) {
	m := press(t, ackModel(), typeKey(tea.KeyEsc))
	if !m.done || m.answer {
		t.Errorf("done=%v answer=%v, want done and no", m.done, m.answer)
	}
}

// The two modes must actually differ: y/n are meaningless without a question
// and must not resolve an acknowledgement.
func TestConfirmAcknowledgeModeIgnoresYesAndNo(t *testing.T) {
	m := press(t, ackModel(), runeKey('y'), runeKey('n'))
	if m.done {
		t.Error("y/n resolved an acknowledgement screen")
	}
}

// ctrl+c must be distinguishable from esc/n: the driver turns it into an
// immediate ErrCancelled rather than treating it as a plain "no" (see
// stepConfirm).
func TestConfirmQuestionModeCtrlCInterruptsDistinctlyFromEsc(t *testing.T) {
	m := press(t, questionModel(), typeKey(tea.KeyCtrlC))
	if !m.done || !m.interrupted {
		t.Errorf("done=%v interrupted=%v, want both true", m.done, m.interrupted)
	}
}

func TestConfirmAcknowledgeModeCtrlCInterruptsDistinctlyFromEsc(t *testing.T) {
	m := press(t, ackModel(), typeKey(tea.KeyCtrlC))
	if !m.done || !m.interrupted {
		t.Errorf("done=%v interrupted=%v, want both true", m.done, m.interrupted)
	}
}

func TestConfirmEscDoesNotInterrupt(t *testing.T) {
	m := press(t, questionModel(), typeKey(tea.KeyEsc))
	if m.interrupted {
		t.Error("esc marked interrupted; only ctrl+c should")
	}
}

func TestConfirmViewShowsEveryLineAndTheQuestion(t *testing.T) {
	got := questionModel().View()
	for _, want := range []string{
		"Before launching",
		"model may not be fully compatible",
		"Launch anyway?",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("View = %q, missing %q", got, want)
		}
	}
}

func TestConfirmAcknowledgeViewOffersLaunchAndBack(t *testing.T) {
	got := ackModel().View()
	if strings.Contains(got, "y/N") {
		t.Errorf("View = %q, offers a yes/no answer with no question asked", got)
	}
	for _, want := range []string{"enter", "esc"} {
		if !strings.Contains(got, want) {
			t.Errorf("View = %q, missing %q", got, want)
		}
	}
}
