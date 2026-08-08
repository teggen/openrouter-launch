package tui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/openrouter/ortest"
)

func TestIsTerminalRejectsARegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-tty")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	if isTerminal(f) {
		t.Error("isTerminal reported a regular file as a terminal")
	}
}

// Without a terminal the picker cannot run at all, and bubbletea's own
// failure would be obscure. The message must name the way forward.
func TestRunWithoutATerminalNamesTheFlagToUseInstead(t *testing.T) {
	original := isTTY
	t.Cleanup(func() { isTTY = original })
	isTTY = func() bool { return false }

	_, err := Run(context.Background(), Options{Service: nil})
	if err == nil {
		t.Fatal("Run with no terminal returned no error")
	}
	if !strings.Contains(err.Error(), "--model") {
		t.Errorf("err = %q, does not point at --model", err)
	}
	// The terminal check must come before the Service check, or a CLI that
	// forgot the service would report the wrong problem.
	if strings.Contains(err.Error(), "Service") {
		t.Errorf("err = %q, reports the service instead of the missing terminal", err)
	}
}

func TestLiveScreensRequiresATerminal(t *testing.T) {
	original := isTTY
	t.Cleanup(func() { isTTY = original })
	isTTY = func() bool { return false }

	if _, err := liveScreens(); err == nil {
		t.Error("liveScreens succeeded with no terminal")
	}
}

func TestLiveScreensProvidesEveryScreen(t *testing.T) {
	original := isTTY
	t.Cleanup(func() { isTTY = original })
	isTTY = func() bool { return true }

	sc, err := liveScreens()
	if err != nil {
		t.Fatalf("liveScreens: %v", err)
	}
	// A nil screen would panic on first use, deep inside a session, rather
	// than failing here.
	if sc.root == nil || sc.pick == nil || sc.prompt == nil ||
		sc.confirm == nil || sc.notice == nil {
		t.Errorf("liveScreens left a screen nil: %+v", sc)
	}
}

func TestRunPropagatesTheTerminalErrorRatherThanPanicking(t *testing.T) {
	original := isTTY
	t.Cleanup(func() { isTTY = original })
	isTTY = func() bool { return false }

	_, err := Run(context.Background(), Options{})
	if err == nil || errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want a terminal error", err)
	}
}

// isTTY's own implementation checks os.Stdin and os.Stdout, in that order,
// exactly once each. A mutation that checks one of them twice (e.g. os.Stdout
// twice) is otherwise invisible: under go test neither stream is usually a
// terminal, so both the correct and the mutated form return false. Spying on
// isTerminal — now a package variable for exactly this reason — pins the
// calls directly instead of relying on which files happen to be terminals.
func TestIsTTYReadsStdinAndStdoutNotOneOfThemTwice(t *testing.T) {
	original := isTerminal
	t.Cleanup(func() { isTerminal = original })

	var calls []*os.File
	isTerminal = func(f *os.File) bool {
		calls = append(calls, f)
		return true
	}

	isTTY()

	if len(calls) != 2 {
		t.Fatalf("isTerminal called %d times, want 2: %v", len(calls), calls)
	}
	if calls[0] != os.Stdin {
		t.Errorf("first call = %v, want os.Stdin", calls[0])
	}
	if calls[1] != os.Stdout {
		t.Errorf("second call = %v, want os.Stdout", calls[1])
	}
}

// /dev/null is a character device — ModeCharDevice is set — even though it
// is not an interactive terminal in any meaningful sense. That is exactly
// what makes it useful here: it is a cheap, portable way to pin isTerminal's
// TRUE branch (an inverted bitmask, e.g. `== 0` instead of `!= 0`, would
// otherwise only show up on the false branch, which
// TestIsTerminalRejectsARegularFile already covers).
func TestIsTerminalAcceptsACharacterDevice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.DevNull is not a character device on windows")
	}
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer f.Close()

	if !isTerminal(f) {
		t.Error("isTerminal reported a character device as not a terminal")
	}
}

// liveScreensHeadless wires production's liveScreens — the seam between the
// driver and the real bubbletea programs — to fake input and output, so its
// closures run with no terminal at all. This is the thing the driver's own
// tests (tui_test.go) cannot exercise: they wire screens to canned closures,
// never touching runProgram, tea.NewProgram, or the option list liveScreens
// builds for each screen.
func liveScreensHeadless(t *testing.T, input, output *bytes.Buffer, extra ...tea.ProgramOption) screens {
	t.Helper()
	original := isTTY
	t.Cleanup(func() { isTTY = original })
	isTTY = func() bool { return true }

	opts := append([]tea.ProgramOption{
		tea.WithInput(input), tea.WithOutput(output), tea.WithoutSignalHandler(),
	}, extra...)
	sc, err := liveScreens(opts...)
	if err != nil {
		t.Fatalf("liveScreens: %v", err)
	}
	return sc
}

func TestLiveScreensPromptReturnsTheTypedValueOnEnter(t *testing.T) {
	in := bytes.NewBufferString("sk-or-secret\r")
	var out bytes.Buffer
	sc := liveScreensHeadless(t, in, &out)

	value, ok, err := sc.prompt(promptInput{Title: "API key", Label: "API key", Masked: true})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if !ok || value != "sk-or-secret" {
		t.Errorf("value=%q ok=%v, want %q and true", value, ok, "sk-or-secret")
	}
}

// These three pin the full chain end to end, through a real bubbletea
// program: the model sets interrupted on ctrl+c, and the production closure
// in liveScreens turns that into ErrCancelled. Neither the model-level tests
// (picker_test.go, confirm_test.go, prompt_test.go, notice_test.go) nor the
// driver-level tests in tui_test.go exercise this translation — the former
// stop at the model's field, the latter script the closures directly and
// never touch program.go at all.
func TestLiveScreensPromptCtrlCReturnsErrCancelled(t *testing.T) {
	in := bytes.NewBufferString("\x03") // ctrl+c
	var out bytes.Buffer
	sc := liveScreensHeadless(t, in, &out)

	if _, _, err := sc.prompt(promptInput{Label: "Name"}); !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestLiveScreensConfirmCtrlCReturnsErrCancelled(t *testing.T) {
	in := bytes.NewBufferString("\x03")
	var out bytes.Buffer
	sc := liveScreensHeadless(t, in, &out)

	if _, err := sc.confirm(confirmInput{Title: "T", Question: "Launch anyway?"}); !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestLiveScreensNoticeCtrlCReturnsErrCancelled(t *testing.T) {
	in := bytes.NewBufferString("\x03")
	var out bytes.Buffer
	sc := liveScreensHeadless(t, in, &out)

	if err := sc.notice(noticeInput{Title: "T"}); !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

// Catches "prompt returns m.value, true instead of m.value, m.submitted":
// esc must never report submitted, or the driver would write an empty value
// into the user's config.
func TestLiveScreensPromptReturnsNotSubmittedOnEsc(t *testing.T) {
	in := bytes.NewBufferString("\x1b")
	var out bytes.Buffer
	sc := liveScreensHeadless(t, in, &out)

	_, ok, err := sc.prompt(promptInput{Title: "API key", Label: "API key"})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if ok {
		t.Error("ok = true after esc, want false")
	}
}

// Catches "confirm returns !m.answer": both directions must come back
// correctly, or an inverted return would launch on "no" and back out on "yes".
func TestLiveScreensConfirmQuestionModeAnswers(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"y answers yes", "y", true},
		{"n answers no", "n", false},
		{"esc answers no", "\x1b", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := bytes.NewBufferString(c.input)
			var out bytes.Buffer
			sc := liveScreensHeadless(t, in, &out)

			got, err := sc.confirm(confirmInput{Title: "Before launching", Question: "Launch anyway?"})
			if err != nil {
				t.Fatalf("confirm: %v", err)
			}
			if got != c.want {
				t.Errorf("confirm(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

// Catches "notice never runs its program": if the closure skipped
// runProgram, no output — and none of the notice's own title text — would
// ever reach the buffer.
func TestLiveScreensNoticeActuallyRuns(t *testing.T) {
	in := bytes.NewBufferString("\r")
	var out bytes.Buffer
	sc := liveScreensHeadless(t, in, &out)

	if err := sc.notice(noticeInput{Title: "Distinctive Notice Title", Lines: []string{"a line"}}); err != nil {
		t.Fatalf("notice: %v", err)
	}
	if !strings.Contains(out.String(), "Distinctive Notice Title") {
		t.Errorf("notice produced no output containing its title; output = %q", out.String())
	}
}

func TestLiveScreensRootReturnsTheChosenAgent(t *testing.T) {
	spec := stubSpec("claude")
	in := bytes.NewBufferString("\r")
	var out bytes.Buffer
	sc := liveScreensHeadless(t, in, &out)

	choice, err := sc.root(rootInput{
		Agents: []*agent.Spec{spec}, Installed: func(*agent.Spec) bool { return true },
	})
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	if choice.Kind != choiceAgent || choice.Agent == nil || choice.Agent.Name != "claude" {
		t.Errorf("choice = %+v, want the agent selected", choice)
	}
}

// The reviewer's own proof that this is testable at all: driving a real
// program with scripted keystrokes and no terminal, headlessly.
func TestLiveScreensPickReturnsTheHighlightedModelOnEnter(t *testing.T) {
	in := bytes.NewBufferString("\x1b[B\r") // down, enter
	var out bytes.Buffer
	sc := liveScreensHeadless(t, in, &out)

	choice, err := sc.pick(pickerInput{Agent: stubSpec("claude"), Models: ortest.Models(), Height: 24, Width: 100})
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if choice.Kind != pickModel || choice.ModelID != "qwen/qwen3-coder:free" {
		t.Errorf("choice = %+v, want the second model picked", choice)
	}
}

func TestLiveScreensPickEscReturnsBackWithLiveFilters(t *testing.T) {
	in := bytes.NewBufferString("\x1bf\x1b") // alt+f, esc
	var out bytes.Buffer
	sc := liveScreensHeadless(t, in, &out)

	choice, err := sc.pick(pickerInput{Agent: stubSpec("claude"), Models: ortest.Models(), Height: 24, Width: 100})
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if choice.Kind != pickBack {
		t.Fatalf("kind = %v, want pickBack", choice.Kind)
	}
	if !choice.Filters.freeOnly {
		t.Error("esc dropped the live filter state")
	}
}

// Catches both "pick drops the !m.done guard" and "pick returns a zero
// pickerChoice": a filter is set live, then the program is ended from
// outside — via tea.WithFilter substituting tea.Quit for the next message,
// standing in for a signal or a lost terminal — before any key resolves the
// picker. m.done stays false the whole time. The correct closure must still
// report pickBack carrying the live filter, not a zero-value choice, and
// must not hang: nothing here waits on input that never arrives.
func TestLiveScreensPickReturnsBackNotZeroWhenUndecided(t *testing.T) {
	in := bytes.NewBufferString("\x1bfx") // alt+f, then a harmless key
	var out bytes.Buffer

	seen := 0
	quitAfterSecondKey := func(_ tea.Model, msg tea.Msg) tea.Msg {
		if _, ok := msg.(tea.KeyMsg); ok {
			seen++
			if seen == 2 {
				return tea.Quit()
			}
		}
		return msg
	}
	sc := liveScreensHeadless(t, in, &out, tea.WithFilter(quitAfterSecondKey))

	choice, err := sc.pick(pickerInput{Agent: stubSpec("claude"), Models: ortest.Models(), Height: 24, Width: 100})
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if choice.Kind != pickBack {
		t.Errorf("kind = %v, want pickBack", choice.Kind)
	}
	if !choice.Filters.freeOnly {
		t.Error("an undecided exit lost the live filter state (returned a zero pickerChoice)")
	}
}

// Catches "picker loses tea.WithAltScreen()": the alt-screen enable sequence
// is written to the program's output at startup, unconditionally, so it is
// observable without a real terminal. Only the picker should write it.
func TestLiveScreensOnlyThePickerEntersTheAltScreen(t *testing.T) {
	const altScreenSeq = "\x1b[?1049h"

	var pickOut bytes.Buffer
	pickSC := liveScreensHeadless(t, bytes.NewBufferString("\x1b"), &pickOut)
	if _, err := pickSC.pick(pickerInput{Agent: stubSpec("claude"), Models: ortest.Models(), Height: 24, Width: 100}); err != nil {
		t.Fatalf("pick: %v", err)
	}
	if !strings.Contains(pickOut.String(), altScreenSeq) {
		t.Error("the picker's program did not enter the alt screen")
	}

	others := map[string]*bytes.Buffer{"root": {}, "prompt": {}, "confirm": {}, "notice": {}}

	rootSC := liveScreensHeadless(t, bytes.NewBufferString("\x1b"), others["root"])
	if _, err := rootSC.root(rootInput{
		Agents: []*agent.Spec{stubSpec("claude")}, Installed: func(*agent.Spec) bool { return true },
	}); err != nil {
		t.Fatalf("root: %v", err)
	}

	promptSC := liveScreensHeadless(t, bytes.NewBufferString("\x1b"), others["prompt"])
	if _, _, err := promptSC.prompt(promptInput{Label: "Name"}); err != nil {
		t.Fatalf("prompt: %v", err)
	}

	confirmSC := liveScreensHeadless(t, bytes.NewBufferString("\x1b"), others["confirm"])
	if _, err := confirmSC.confirm(confirmInput{Title: "T"}); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	noticeSC := liveScreensHeadless(t, bytes.NewBufferString("\x1b"), others["notice"])
	if err := noticeSC.notice(noticeInput{Title: "T"}); err != nil {
		t.Fatalf("notice: %v", err)
	}

	for name, buf := range others {
		if strings.Contains(buf.String(), altScreenSeq) {
			t.Errorf("%s entered the alt screen; only the picker should", name)
		}
	}
}
