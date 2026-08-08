package tui

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// ErrNoTerminal reports that the session cannot run interactively.
var ErrNoTerminal = errors.New(
	"the interactive picker needs a terminal; pass --model <slug> instead")

// isTTY reports whether an interactive session is possible. It is a package
// variable so tests can exercise both branches without a real terminal.
var isTTY = func() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

// isTerminal reports whether f is a character device. This is the stdlib
// check; golang.org/x/term would do the same thing at the cost of a direct
// dependency.
//
// A package variable, like isTTY, rather than a plain function: it lets a
// test replace it with a recorder to pin that isTTY consults both os.Stdin
// and os.Stdout — not one of them twice — which is not otherwise observable
// from outside the package.
var isTerminal = func(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// runProgram runs one screen to completion and returns it with its concrete
// type restored.
func runProgram[M tea.Model](m M, opts ...tea.ProgramOption) (M, error) {
	out, err := tea.NewProgram(m, opts...).Run()
	if err != nil {
		var zero M
		return zero, err
	}
	final, ok := out.(M)
	if !ok {
		var zero M
		return zero, fmt.Errorf("tui: screen returned %T, want %T", out, m)
	}
	return final, nil
}

// liveScreens wires each screen to its own bubbletea program.
//
// One program per screen, rather than one program with a screen enum, is what
// keeps Service.Plan's IO out of every event loop: the driver calls it
// between programs, in ordinary Go. It is also what makes teardown before the
// process handoff structural — the last program has provably returned before
// Run returns.
//
// The picker takes the alt screen because it is the one view that wants the
// whole terminal and should leave no scrollback behind. The others run
// inline, so their final render stays on screen as a wizard trail.
//
// extra is appended to every screen's own program options. Run calls
// liveScreens with none, so production is unaffected; it exists so tests can
// drive the real closures below — the seam between the driver and the screen
// models — with tea.WithInput/tea.WithOutput pointed at buffers instead of
// the terminal, with no TTY and no bubbletea program left dangling.
func liveScreens(extra ...tea.ProgramOption) (screens, error) {
	if !isTTY() {
		return screens{}, ErrNoTerminal
	}

	return screens{
		root: func(in rootInput) (rootChoice, error) {
			m, err := runProgram(newRootModel(in), extra...)
			if err != nil {
				return rootChoice{}, err
			}
			return m.choice, nil
		},

		pick: func(in pickerInput) (pickerChoice, error) {
			m, err := runProgram(newPickerModel(in), pickerProgramOptions(extra)...)
			if err != nil {
				return pickerChoice{}, err
			}
			if !m.done {
				// The program ended without a decision — a signal, or the
				// terminal going away. Treat it as backing out rather than as
				// a selection, and carry the filters so they still persist.
				return pickerChoice{Kind: pickBack, Filters: m.filters}, nil
			}
			return m.choice, nil
		},

		prompt: func(in promptInput) (string, bool, error) {
			m, err := runProgram(newPromptModel(in), extra...)
			if err != nil {
				return "", false, err
			}
			if m.interrupted {
				// ctrl+c ends the session immediately; see ErrCancelled and
				// the design spec's error table.
				return "", false, ErrCancelled
			}
			return m.value, m.submitted, nil
		},

		confirm: func(in confirmInput) (bool, error) {
			m, err := runProgram(newConfirmModel(in), extra...)
			if err != nil {
				return false, err
			}
			if m.interrupted {
				return false, ErrCancelled
			}
			return m.answer, nil
		},

		notice: func(in noticeInput) error {
			m, err := runProgram(newNoticeModel(in), extra...)
			if err != nil {
				return err
			}
			if m.interrupted {
				return ErrCancelled
			}
			return nil
		},
	}, nil
}

// pickerProgramOptions is the picker's program options: the alt screen plus
// whatever the caller adds. Named rather than inlined so it is one thing a
// test can reason about instead of a literal buried in a closure.
func pickerProgramOptions(extra []tea.ProgramOption) []tea.ProgramOption {
	return append([]tea.ProgramOption{tea.WithAltScreen()}, extra...)
}
