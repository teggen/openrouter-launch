package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func namePrompt() promptModel {
	return newPromptModel(promptInput{Title: "Save profile", Label: "Name"})
}

func TestPromptCollectsTypedRunes(t *testing.T) {
	m := press(t, namePrompt(), typeRunes("opus-cc")...)
	if m.value != "opus-cc" {
		t.Errorf("value = %q, want %q", m.value, "opus-cc")
	}
}

func TestPromptSpaceIsTypable(t *testing.T) {
	m := press(t, namePrompt(), runeKey('a'), typeKey(tea.KeySpace), runeKey('b'))
	if m.value != "a b" {
		t.Errorf("value = %q, want %q", m.value, "a b")
	}
}

func TestPromptBackspaceRemovesTheLastRune(t *testing.T) {
	m := press(t, namePrompt(), typeRunes("abc")...)
	m = press(t, m, typeKey(tea.KeyBackspace))
	if m.value != "ab" {
		t.Errorf("value = %q, want %q", m.value, "ab")
	}
}

// Byte-wise truncation would leave half a rune behind and emit invalid UTF-8.
func TestPromptBackspaceRemovesAWholeMultibyteRune(t *testing.T) {
	m := press(t, namePrompt(), typeRunes("aé")...)
	m = press(t, m, typeKey(tea.KeyBackspace))
	if m.value != "a" {
		t.Errorf("value = %q, want %q", m.value, "a")
	}
}

func TestPromptBackspaceOnAnEmptyValueIsANoop(t *testing.T) {
	m := press(t, namePrompt(), typeKey(tea.KeyBackspace))
	if m.value != "" || m.submitted || m.cancelled {
		t.Errorf("unexpected state after backspace on empty: %+v", m)
	}
}

func TestPromptEnterSubmits(t *testing.T) {
	m := press(t, namePrompt(), typeRunes("opus-cc")...)
	m = press(t, m, typeKey(tea.KeyEnter))
	if !m.submitted || m.cancelled {
		t.Errorf("submitted=%v cancelled=%v, want submitted", m.submitted, m.cancelled)
	}
}

func TestPromptEscCancels(t *testing.T) {
	m := press(t, namePrompt(), typeKey(tea.KeyEsc))
	if !m.cancelled || m.submitted {
		t.Errorf("submitted=%v cancelled=%v, want cancelled", m.submitted, m.cancelled)
	}
}

// Validation must keep the user in the prompt with the reason visible. If
// enter submitted regardless, ctrl+s on a duplicate profile name would fail
// after the screen closed, with nowhere to correct it.
func TestPromptValidationRejectionKeepsTheUserInThePrompt(t *testing.T) {
	m := newPromptModel(promptInput{
		Label:    "Name",
		Validate: func(string) error { return errors.New("profile already exists: opus-cc") },
	})
	m = press(t, m, typeRunes("opus-cc")...)
	m = press(t, m, typeKey(tea.KeyEnter))

	if m.submitted {
		t.Error("enter submitted a value the validator rejected")
	}
	if !strings.Contains(m.errMsg, "already exists") {
		t.Errorf("errMsg = %q, want the validator's reason", m.errMsg)
	}
	if !strings.Contains(m.View(), "already exists") {
		t.Errorf("View = %q, does not show the validation error", m.View())
	}
}

func TestPromptEditingClearsAStaleValidationError(t *testing.T) {
	m := newPromptModel(promptInput{
		Label:    "Name",
		Validate: func(string) error { return errors.New("nope") },
	})
	m = press(t, m, typeRunes("x")...)
	m = press(t, m, typeKey(tea.KeyEnter))
	if m.errMsg == "" {
		t.Fatal("expected a validation error to be set first")
	}
	m = press(t, m, runeKey('y'))
	if m.errMsg != "" {
		t.Errorf("errMsg = %q, want cleared once the value changed", m.errMsg)
	}
}

func TestPromptValidationAcceptanceSubmits(t *testing.T) {
	m := newPromptModel(promptInput{
		Label:    "Name",
		Validate: func(string) error { return nil },
	})
	m = press(t, m, typeRunes("ok")...)
	m = press(t, m, typeKey(tea.KeyEnter))
	if !m.submitted {
		t.Error("a value the validator accepted did not submit")
	}
}

// The API key must not appear on screen. This is the one prompt whose value
// is a secret.
func TestPromptMaskedViewNeverShowsTheValue(t *testing.T) {
	m := newPromptModel(promptInput{Label: "API key", Masked: true})
	m = press(t, m, typeRunes("sk-or-secret")...)

	got := m.View()
	if strings.Contains(got, "sk-or-secret") {
		t.Errorf("View = %q, leaks the masked value", got)
	}
	if !strings.Contains(got, strings.Repeat("•", len("sk-or-secret"))) {
		t.Errorf("View = %q, want one bullet per rune", got)
	}
}

func TestPromptUnmaskedViewShowsTheValue(t *testing.T) {
	m := press(t, namePrompt(), typeRunes("opus-cc")...)
	if !strings.Contains(m.View(), "opus-cc") {
		t.Errorf("View = %q, does not show the typed value", m.View())
	}
}

// Same bug class as the picker's: without the !Alt guard, a chord would type
// its letter into the field.
func TestPromptAltChordsAreNotTyped(t *testing.T) {
	m := press(t, namePrompt(), altKey('t'))
	if m.value != "" {
		t.Errorf("value = %q, want empty; an alt chord was typed", m.value)
	}
}
