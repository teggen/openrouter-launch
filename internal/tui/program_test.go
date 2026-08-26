package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/teggen/agentlaunch/agent"
	"github.com/teggen/agentlaunch/catalog/catalogtest"
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
	if sc.root == nil || sc.pick == nil || sc.filters == nil || sc.prompt == nil ||
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

// chunkReader hands back one chunk per Read call, so a test can put a read
// boundary exactly where it wants one. bytes.Buffer cannot: it returns as
// much as the caller's buffer holds, which is how the escape ambiguity hides
// from a test that uses it.
type chunkReader struct {
	chunks []string
	n      int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.n >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.n])
	r.n++
	return n, nil
}

// syncBuffer is the output sink every headless program writes to. It is
// mutex-guarded rather than a bare bytes.Buffer because bubbletea's standard
// renderer writes frames from its own goroutine: any test that reads the
// output while the program is still running — which is the whole point of
// waitForOutput — races with that writer. A plain bytes.Buffer passes
// `make test` and fails `make test-race`.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Copy: bytes.Buffer.Bytes aliases the buffer's own storage, so handing
	// it out unlocked would let a caller read a slice the renderer is
	// concurrently appending to — the race this type exists to prevent,
	// reintroduced one level down.
	return append([]byte(nil), b.buf.Bytes()...)
}

// pipeInput is an input side a test can write to WHILE the program runs, so a
// keystroke can be sent in response to something that rendered. The pre-filled
// bytes.Buffer and chunkReader inputs cannot express that: their bytes are all
// available before the program starts, so nothing can prove a frame was drawn
// before a key was consumed.
//
// It delivers one Send per Read call, keeping chunkReader's property that a
// test controls exactly where the read boundaries fall — an escape sequence
// can still be split across reads deliberately (see the alt-chord test).
//
// It is additive, not a replacement: several tests depend on their input
// reaching io.EOF, which a reader that blocks for more input never does.
type pipeInput struct {
	chunks chan string
	closed chan struct{}
	once   sync.Once
}

func newPipeInput(t *testing.T) *pipeInput {
	t.Helper()
	p := &pipeInput{chunks: make(chan string, 16), closed: make(chan struct{})}
	t.Cleanup(p.Close)
	return p
}

// Send queues one chunk, to be returned by exactly one Read call.
func (p *pipeInput) Send(s string) { p.chunks <- s }

// Close makes subsequent reads return io.EOF once the queue drains.
func (p *pipeInput) Close() { p.once.Do(func() { close(p.closed) }) }

func (p *pipeInput) Read(b []byte) (int, error) {
	select {
	case s := <-p.chunks:
		return copy(b, s), nil
	case <-p.closed:
		// Drain anything queued before the close before reporting EOF.
		select {
		case s := <-p.chunks:
			return copy(b, s), nil
		default:
			return 0, io.EOF
		}
	}
}

const (
	// waitInterval and waitTimeout are constants rather than options because
	// no test needs to vary them. teatest exposes WithCheckInterval and
	// WithDuration; adding them here with no caller only gives the linter
	// something to complain about.
	waitInterval = 10 * time.Millisecond
	waitTimeout  = 3 * time.Second
)

// waitForOutput blocks until pred accepts the program's output so far, or
// fails the test on timeout.
//
// The predicate sees ACCUMULATED output, not the current frame: every frame
// the renderer has written is still in the buffer. So predicates must be
// positive — "contains X". A negative one ("no longer shows Y") cannot be
// expressed this way, because the frame that did show Y never leaves the
// buffer. Asserting a disappearance needs a different tool than this one.
func waitForOutput(t *testing.T, out *syncBuffer, pred func([]byte) bool) {
	t.Helper()

	deadline := time.Now().Add(waitTimeout)
	for {
		if pred(out.Bytes()) {
			return
		}
		if time.Now().After(deadline) {
			// The output goes in the failure: a bare "timed out" gives a
			// reader nothing to work from, and the frames are the evidence
			// for why the condition never held.
			t.Fatalf("waitForOutput: condition unmet after %s; output so far:\n%q", waitTimeout, out.String())
		}
		time.Sleep(waitInterval)
	}
}

// liveScreensHeadless wires production's liveScreens — the seam between the
// driver and the real bubbletea programs — to fake input and output, so its
// closures run with no terminal at all. This is the thing the driver's own
// tests (tui_test.go) cannot exercise: they wire screens to canned closures,
// never touching runProgram, tea.NewProgram, or the option list liveScreens
// builds for each screen.
func liveScreensHeadless(t *testing.T, input io.Reader, output *syncBuffer, extra ...tea.ProgramOption) screens {
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
	var out syncBuffer
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
	var out syncBuffer
	sc := liveScreensHeadless(t, in, &out)

	if _, _, err := sc.prompt(promptInput{Label: "Name"}); !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestLiveScreensConfirmCtrlCReturnsErrCancelled(t *testing.T) {
	in := bytes.NewBufferString("\x03")
	var out syncBuffer
	sc := liveScreensHeadless(t, in, &out)

	if _, err := sc.confirm(confirmInput{Title: "T", Question: "Launch anyway?"}); !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestLiveScreensNoticeCtrlCReturnsErrCancelled(t *testing.T) {
	in := bytes.NewBufferString("\x03")
	var out syncBuffer
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
	var out syncBuffer
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
			var out syncBuffer
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
	var out syncBuffer
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
	var out syncBuffer
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
	var out syncBuffer
	sc := liveScreensHeadless(t, in, &out)

	choice, err := sc.pick(pickerInput{Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: 100})
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if choice.Kind != pickModel || choice.ModelID != "qwen/qwen3-coder:free" {
		t.Errorf("choice = %+v, want the second model picked", choice)
	}
}

// Typing into the search box, one keystroke at a time, through a real program:
// bytes on the wire → decode → handleKey accumulation → recompute → rendered
// frame → enter → the closure's return. This is the exercise for pipeInput and
// waitForOutput, and the only test that drives typing end to end.
//
// Scope, honestly: it is an integration test, not a uniquely-sharp regression
// test. Two mutations were tried against it — `+=` to `=` in the search
// accumulation (picker.go:330), and rendering the catalog only after the first
// key — and BOTH are also caught by the model-level tests, because View is pure
// and press()/typeRunes() already drive Update message by message. Do not treat
// a failure here as localised: check picker_test.go first, since it will
// usually have failed too and will say what broke more precisely.
//
// What it does pin that nothing else does is ordering on the wire. The catalog
// is on screen before any input is read, and each keystroke is echoed before
// the next is sent — intermediate frames a later frame overwrites, which only a
// mid-run reader can see. Landmine 17 is about that echo surviving as it grows.
//
// The pre-filled inputs cannot express any of it: handed "qwen" in one read,
// detectOneMsg gathers the whole run of printable bytes into a SINGLE KeyRunes
// message (key.go:697), so the four keystrokes collapse into one.
func TestLiveScreensPickTypesIntoTheSearchBoxOneKeystrokeAtATime(t *testing.T) {
	in := newPipeInput(t)
	var out syncBuffer
	sc := liveScreensHeadless(t, in, &out)

	type result struct {
		choice pickerChoice
		err    error
	}
	done := make(chan result, 1)
	go func() {
		choice, err := sc.pick(pickerInput{
			Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: 100,
		})
		done <- result{choice, err}
	}()

	contains := func(s string) func([]byte) bool {
		return func(b []byte) bool { return bytes.Contains(b, []byte(s)) }
	}

	// Nothing has been sent yet: the catalog must already be drawn.
	waitForOutput(t, &out, contains("anthropic/claude-opus-4.6"))

	// One rune per Send, so each arrives as its own KeyRunes message, and the
	// next is not sent until the echo proves the previous one landed.
	for _, want := range []string{"q", "qw", "qwe", "qwen"} {
		in.Send(want[len(want)-1:])
		waitForOutput(t, &out, contains("search: "+want))
	}

	in.Send("\r")

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("pick: %v", got.err)
		}
		// Four keystrokes narrowed the catalog to one model, and enter takes
		// it. A picker that dropped all but the last rune would search "n",
		// which matches a different set entirely.
		if got.choice.Kind != pickModel || got.choice.ModelID != "qwen/qwen3-coder:free" {
			t.Errorf("choice = %+v, want the model the search narrowed to", got.choice)
		}
	case <-time.After(waitTimeout):
		t.Fatal("pick did not return after the enter was sent")
	}
}

func TestLiveScreensPickEscReturnsBackWithLiveFilters(t *testing.T) {
	in := bytes.NewBufferString("\x1b") // esc
	var out syncBuffer
	sc := liveScreensHeadless(t, in, &out)

	// Draining the input leaves the read loop at io.EOF, which bubbletea
	// tolerates silently (tty.go:100). The esc still resolves, because what
	// resolves it is escLatch's own tick rather than another keypress.
	choice, err := sc.pick(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(),
		Filters: filterState{freeOnly: true}, Height: 24, Width: 100,
	})
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

// THE regression test for the reported defect, and the only one that
// reproduces it: bubbletea must be handed ESC and the letter on SEPARATE
// Read calls. bytes.NewBufferString("\x1bt") delivers both in one read, which
// detectSequence resolves to a single alt+t — so that version passes with
// escLatch deleted and proves nothing.
//
// Without the latch the first read's lone \x1b becomes a bare esc
// (key.go:707) and the picker resolves right there, returning pickBack with
// Cancelled false. With it, the esc is held, the t is recognised as the
// chord's tail and swallowed, and the picker survives to see the ctrl+c —
// which is what makes Cancelled the discriminator between the two.
func TestLiveScreensPickSurvivesAnAltChordSplitAcrossReads(t *testing.T) {
	in := &chunkReader{chunks: []string{"\x1b", "t", "\x03"}} // ESC, t, ctrl+c
	var out syncBuffer
	sc := liveScreensHeadless(t, in, &out)

	choice, err := sc.pick(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: 100,
	})
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if !choice.Cancelled {
		t.Fatal("the picker resolved before the ctrl+c: the split esc was acted on")
	}
	if choice.Filters.search != "" {
		t.Errorf("search = %q, want empty; the chord's letter was typed into the search box",
			choice.Filters.search)
	}
}

// Catches both "pick drops the !m.done guard" and "pick returns a zero
// pickerChoice": the program is ended from outside — via tea.WithFilter
// substituting tea.Quit for the second key, standing in for a signal or a
// lost terminal — before any key resolves the picker. m.done stays false the
// whole time. The correct closure must still report pickBack carrying the
// live filter, not a zero-value choice, and must not hang: nothing here waits
// on input that never arrives.
//
// The two keys must arrive on separate reads. Handed "xy" in one read,
// detectOneMsg gathers the whole run of printable bytes into ONE KeyRunes
// message (key.go:697), the filter's count never reaches 2, and the test
// hangs until the timeout instead of failing.
func TestLiveScreensPickReturnsBackNotZeroWhenUndecided(t *testing.T) {
	in := &chunkReader{chunks: []string{"x", "y"}} // two harmless keys
	var out syncBuffer

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

	choice, err := sc.pick(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(),
		Filters: filterState{freeOnly: true}, Height: 24, Width: 100,
	})
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

// Catches "a screen loses tea.WithAltScreen()", and its opposite: the
// alt-screen enable sequence is written to the program's output at startup,
// unconditionally, so it is observable without a real terminal.
//
// The picker and the filters screen both take it — the filters screen because
// it is a detour FROM the picker, and a screen that ran inline would drop back
// through the main screen on the way in and leave the panel in scrollback on
// the way out. The wizard-trail screens must not.
func TestLiveScreensOnlyThePickerAndFiltersScreenEnterTheAltScreen(t *testing.T) {
	const altScreenSeq = "\x1b[?1049h"

	var pickOut syncBuffer
	pickSC := liveScreensHeadless(t, bytes.NewBufferString("\x1b"), &pickOut)
	if _, err := pickSC.pick(pickerInput{Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: 100}); err != nil {
		t.Fatalf("pick: %v", err)
	}
	if !strings.Contains(pickOut.String(), altScreenSeq) {
		t.Error("the picker's program did not enter the alt screen")
	}

	var filtersOut syncBuffer
	filtersSC := liveScreensHeadless(t, bytes.NewBufferString("\x1b"), &filtersOut)
	if _, err := filtersSC.filters(filterScreenInput{Models: catalogtest.Models(), Height: 24, Width: 100}); err != nil {
		t.Fatalf("filters: %v", err)
	}
	if !strings.Contains(filtersOut.String(), altScreenSeq) {
		t.Error("the filters screen's program did not enter the alt screen")
	}

	others := map[string]*syncBuffer{"root": {}, "prompt": {}, "confirm": {}, "notice": {}}

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

// The filters closure, driven headlessly: space toggles the first row and
// enter applies. Landmine 16 — screen-closure wiring is pinned by driving a
// real program, never by a nil check, which cannot tell a correctly wired
// closure from one that returns a zero choice.
func TestLiveScreensFiltersAppliesTheEditOnEnter(t *testing.T) {
	in := bytes.NewBufferString(" \r") // space, enter
	var out syncBuffer
	sc := liveScreensHeadless(t, in, &out)

	choice, err := sc.filters(filterScreenInput{Models: catalogtest.Models(), Height: 24, Width: 100})
	if err != nil {
		t.Fatalf("filters: %v", err)
	}
	if !choice.Applied {
		t.Fatal("enter did not apply")
	}
	if !choice.Filters.toolsOnly {
		t.Errorf("applied %+v, want the first row toggled on", choice.Filters)
	}
}

// The mirror of the closure's !m.done branch: an exit nobody decided must
// read as a cancel carrying the filters the screen opened with, not a zero
// filterState that would silently clear the session's filters.
func TestLiveScreensFiltersUndecidedExitCancelsWithTheOpeningFilters(t *testing.T) {
	in := &chunkReader{chunks: []string{"x", "y"}}
	var out syncBuffer

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

	opened := filterState{freeOnly: true}
	choice, err := sc.filters(filterScreenInput{
		Filters: opened, Models: catalogtest.Models(), Height: 24, Width: 100,
	})
	if err != nil {
		t.Fatalf("filters: %v", err)
	}
	if choice.Applied {
		t.Error("an undecided exit reported an applied choice")
	}
	if choice.Filters != opened {
		t.Errorf("undecided exit returned %+v, want the opening filters %+v", choice.Filters, opened)
	}
}

func TestLiveScreensFiltersCtrlCCancelsTheSession(t *testing.T) {
	in := bytes.NewBufferString("\x03")
	var out syncBuffer
	sc := liveScreensHeadless(t, in, &out)

	choice, err := sc.filters(filterScreenInput{Models: catalogtest.Models(), Height: 24, Width: 100})
	if err != nil {
		t.Fatalf("filters: %v", err)
	}
	if !choice.Cancelled {
		t.Error("ctrl+c did not cancel")
	}
}

// ctrl+f driven as the RAW BYTE a terminal sends (0x06), not as a
// synthesized tea.KeyMsg. The model-level tests build the message themselves,
// so they assume the byte→KeyMsg→case chain rather than exercising it — and
// that chain is precisely what the reported defect was about.
func TestLiveScreensPickCtrlFByteOpensTheFiltersScreen(t *testing.T) {
	// One byte and no backstop. A trailing ctrl+c looks like a way to make a
	// regression fail fast instead of hanging, and is not: both bytes arrive
	// in one read, so both KeyMsgs are queued, and the model still processes
	// the ctrl+c after ctrl+f's tea.Quit — overwriting the choice and failing
	// the passing case. A regression here surfaces as a test timeout naming
	// this function, which is slower but honest.
	in := bytes.NewBufferString("\x06")
	var out syncBuffer
	sc := liveScreensHeadless(t, in, &out)

	choice, err := sc.pick(pickerInput{
		Agent: stubSpec("claude"), Models: catalogtest.Models(), Height: 24, Width: 100,
	})
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if choice.Kind != pickFilters {
		t.Errorf("kind = %v, want pickFilters: the 0x06 byte did not reach the ctrl+f case",
			choice.Kind)
	}
}
