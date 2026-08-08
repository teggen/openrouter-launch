package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/openrouter/ortest"
)

// countingCatalog records how many times the catalog was actually fetched,
// which is how the refresh-spent-once invariant is checked.
type countingCatalog struct {
	models []openrouter.Model
	calls  int
}

func (c *countingCatalog) Models(context.Context) ([]openrouter.Model, error) {
	c.calls++
	return c.models, nil
}

// newTestService isolates config and cache to a temp dir and provides an API
// key, so Plan reaches its final guard instead of stopping at the key check.
func newTestService(t *testing.T) (*launch.Service, *countingCatalog) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(config.APIKeyEnvVar, "test-key")

	cat := &countingCatalog{models: ortest.Models()}
	return &launch.Service{
		Catalog: cat,
		// Never called: the driver returns a plan and the caller launches.
		Run: func(agent.Command) error { return errors.New("the driver must not launch") },
	}, cat
}

type promptResult struct {
	value string
	ok    bool
}

// script queues one return value per screen call and records every input.
// A call with nothing left queued fails the test rather than returning a zero
// value, so a driver that visits a screen it should not have visited is
// reported as that, not as a confusing downstream assertion.
type script struct {
	t *testing.T

	root    []rootChoice
	pick    []pickerChoice
	prompt  []promptResult
	confirm []bool

	rootIn    []rootInput
	pickIn    []pickerInput
	promptIn  []promptInput
	confirmIn []confirmInput
	noticeIn  []noticeInput
}

func (s *script) screens() screens {
	return screens{
		root: func(in rootInput) (rootChoice, error) {
			s.t.Helper()
			s.rootIn = append(s.rootIn, in)
			if len(s.root) == 0 {
				s.t.Fatalf("root screen called %d times, more than scripted", len(s.rootIn))
			}
			out := s.root[0]
			s.root = s.root[1:]
			return out, nil
		},
		pick: func(in pickerInput) (pickerChoice, error) {
			s.t.Helper()
			s.pickIn = append(s.pickIn, in)
			if len(s.pick) == 0 {
				s.t.Fatalf("picker called %d times, more than scripted", len(s.pickIn))
			}
			out := s.pick[0]
			s.pick = s.pick[1:]
			return out, nil
		},
		prompt: func(in promptInput) (string, bool, error) {
			s.t.Helper()
			s.promptIn = append(s.promptIn, in)
			if len(s.prompt) == 0 {
				s.t.Fatalf("prompt called %d times, more than scripted", len(s.promptIn))
			}
			out := s.prompt[0]
			s.prompt = s.prompt[1:]
			return out.value, out.ok, nil
		},
		confirm: func(in confirmInput) (bool, error) {
			s.t.Helper()
			s.confirmIn = append(s.confirmIn, in)
			if len(s.confirm) == 0 {
				s.t.Fatalf("confirm called %d times, more than scripted", len(s.confirmIn))
			}
			out := s.confirm[0]
			s.confirm = s.confirm[1:]
			return out, nil
		},
		notice: func(in noticeInput) error {
			s.noticeIn = append(s.noticeIn, in)
			return nil
		},
	}
}

// stubOptions wires the injection points to stubs so nothing consults the
// real registry, PATH, or the user's home directory.
func stubOptions(svc *launch.Service, specs ...*agent.Spec) Options {
	byName := map[string]*agent.Spec{}
	for _, sp := range specs {
		byName[sp.Name] = sp
	}
	return Options{
		Service:   svc,
		Agents:    specs,
		Installed: func(*agent.Spec) bool { return true },
		Lookup: func(name string) (*agent.Spec, error) {
			if sp, ok := byName[name]; ok {
				return sp, nil
			}
			return nil, agent.ErrUnknownAgent
		},
	}
}

func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func TestRunRequiresAService(t *testing.T) {
	sc := (&script{t: t}).screens()
	if _, err := run(context.Background(), Options{}, sc); err == nil {
		t.Error("run with a nil Service returned no error")
	}
}

func TestRunCancelsFromRoot(t *testing.T) {
	svc, _ := newTestService(t)
	s := &script{t: t, root: []rootChoice{{Kind: choiceCancel}}}

	_, err := run(context.Background(), stubOptions(svc, stubSpec("claude")), s.screens())
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

func TestRunAgentThenModelReturnsAnApprovedPlan(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
	}

	plan, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if plan.Model.ID != "anthropic/claude-opus-4.6" {
		t.Errorf("plan.Model.ID = %q, want the picked model", plan.Model.ID)
	}
	if plan.Command.Path == "" {
		t.Error("plan carries no command")
	}
}

// Selecting a profile launches immediately; the picker must never open.
func TestRunProfileSkipsThePicker(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t: t,
		root: []rootChoice{{Kind: choiceProfile, Profile: config.Profile{
			Name: "opus-cc", Agent: "claude", Model: "anthropic/claude-opus-4.6",
			Args: []string{"--resume"},
		}}},
	}

	plan, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.pickIn) != 0 {
		t.Errorf("the picker opened %d times on the profile path", len(s.pickIn))
	}
	if plan.Model.ID != "anthropic/claude-opus-4.6" {
		t.Errorf("plan.Model.ID = %q", plan.Model.ID)
	}
}

func TestRunEscFromThePickerReturnsToRoot(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t: t,
		root: []rootChoice{
			{Kind: choiceAgent, Agent: spec},
			{Kind: choiceCancel},
		},
		pick: []pickerChoice{{Kind: pickBack}},
	}

	_, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.rootIn) != 2 {
		t.Errorf("root screen opened %d times, want 2 (esc must go back to it)", len(s.rootIn))
	}
}

// With Options.Agent set the session started at the picker, so there is no
// root to go back to and esc must cancel outright.
func TestRunEscFromThePickerCancelsWhenAnAgentWasGiven(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	opts := stubOptions(svc, spec)
	opts.Agent = spec

	s := &script{t: t, pick: []pickerChoice{{Kind: pickBack}}}

	_, err := run(context.Background(), opts, s.screens())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.rootIn) != 0 {
		t.Errorf("root screen opened %d times, want 0", len(s.rootIn))
	}
}

// A profile is a stored reference and can rot: its agent may have been
// renamed or removed since it was saved.
func TestRunProfileNamingAnUnknownAgentReturnsToRoot(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t: t,
		root: []rootChoice{
			{Kind: choiceProfile, Profile: config.Profile{Name: "old", Agent: "removed-agent", Model: "x"}},
			{Kind: choiceCancel},
		},
	}

	_, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.noticeIn) != 1 {
		t.Fatalf("notices shown = %d, want 1", len(s.noticeIn))
	}
	if len(s.rootIn) != 2 {
		t.Errorf("root opened %d times, want 2 (the notice must return to root)", len(s.rootIn))
	}
}

// THE double-fetch test. Snapshot runs twice per launch — once to fill the
// picker, once inside Plan — so passing --refresh to both would make two HTTP
// round trips for one launch.
func TestRunSpendsRefreshExactlyOnce(t *testing.T) {
	svc, cat := newTestService(t)
	spec := stubSpec("claude")
	opts := stubOptions(svc, spec)
	opts.Refresh = true

	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
	}

	if _, err := run(context.Background(), opts, s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if cat.calls != 1 {
		t.Errorf("catalog fetched %d times, want 1; --refresh was spent more than once", cat.calls)
	}
}

// On the profile path there is no picker, so Plan spends the refresh instead
// — it must still be spent, or --refresh would do nothing for a profile.
func TestRunProfilePathSpendsRefreshOnPlan(t *testing.T) {
	svc, cat := newTestService(t)
	spec := stubSpec("claude")
	opts := stubOptions(svc, spec)
	opts.Refresh = true

	s := &script{t: t, root: []rootChoice{{Kind: choiceProfile, Profile: config.Profile{
		Name: "p", Agent: "claude", Model: "anthropic/claude-opus-4.6",
	}}}}

	if _, err := run(context.Background(), opts, s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if cat.calls != 1 {
		t.Errorf("catalog fetched %d times, want 1", cat.calls)
	}
}

func TestRunPersistsChangedFiltersOnLaunch(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{{
			Kind: pickModel, ModelID: "anthropic/claude-opus-4.6",
			Filters: filterState{freeOnly: true, minContext: 128_000},
		}},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := loadTestConfig(t).Filters
	if !got.FreeOnly || got.MinContext != 128_000 {
		t.Errorf("saved filters = %+v, want the picker's state", got)
	}
}

// Filters are a remembered view, not a property of a successful launch, so
// backing out still saves them.
func TestRunPersistsChangedFiltersWhenCancelled(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t: t,
		root: []rootChoice{
			{Kind: choiceAgent, Agent: spec},
			{Kind: choiceCancel},
		},
		pick: []pickerChoice{{Kind: pickBack, Filters: filterState{freeOnly: true}}},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if !loadTestConfig(t).Filters.FreeOnly {
		t.Error("filters changed during a cancelled session were not saved")
	}
}

// An unchanged session must not touch the config at all. The temp dir starts
// empty, so the file appearing is proof of a write.
func TestRunDoesNotWriteConfigWhenNothingChanged(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{t: t, root: []rootChoice{{Kind: choiceCancel}}}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	path, err := config.Path()
	if err != nil {
		t.Fatalf("config.Path: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("config file exists after a session that changed nothing (stat err = %v)", err)
	}
}

// ctrl+s captures ExtraArgs, so saving after
// `openrouter-launch claude -- --resume` favorites the invocation the user is
// actually performing.
func TestRunSavesAProfileCapturingExtraArgs(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	opts := stubOptions(svc, spec)
	opts.ExtraArgs = []string{"--resume"}

	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickSaveProfile, ModelID: "anthropic/claude-opus-4.6"},
			{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"},
		},
		prompt: []promptResult{{value: "opus-cc", ok: true}},
	}

	if _, err := run(context.Background(), opts, s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}

	cfg := loadTestConfig(t)
	p, ok := cfg.Profile("opus-cc")
	if !ok {
		t.Fatalf("profile not saved; config has %+v", cfg.Profiles)
	}
	if p.Agent != "claude" || p.Model != "anthropic/claude-opus-4.6" {
		t.Errorf("profile = %+v, want the picker's agent and model", p)
	}
	if len(p.Args) != 1 || p.Args[0] != "--resume" {
		t.Errorf("profile.Args = %v, want the session's extra args", p.Args)
	}
}

// Reopening the picker after a save must not dump the user back at the top of
// a 400-model list.
func TestRunReopensThePickerPreselectingTheSavedModel(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickSaveProfile, ModelID: "openai/o1-mini"},
			{Kind: pickModel, ModelID: "openai/o1-mini"},
		},
		prompt: []promptResult{{value: "mini", ok: true}},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.pickIn) != 2 {
		t.Fatalf("picker opened %d times, want 2", len(s.pickIn))
	}
	if s.pickIn[1].Selected != "openai/o1-mini" {
		t.Errorf("reopened picker preselected %q, want the saved model", s.pickIn[1].Selected)
	}
}

func TestRunCancellingTheProfileNameKeepsThePickerOpen(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickSaveProfile, ModelID: "openai/o1-mini"},
			{Kind: pickBack},
		},
		prompt: []promptResult{{ok: false}},
	}
	s.root = append(s.root, rootChoice{Kind: choiceCancel})

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(loadTestConfig(t).Profiles) != 0 {
		t.Error("a cancelled name prompt still saved a profile")
	}
}

// incompatibleSpec produces exactly one advisory warning from Plan.
func incompatibleSpec() *agent.Spec {
	s := stubSpec("claude")
	s.Launcher.(*stubLauncher).compatErr = agent.ErrIncompatibleModel
	return s
}

func TestRunSkipsConfirmWhenThereAreNoWarnings(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.confirmIn) != 0 {
		t.Errorf("confirm screen shown %d times with no warnings", len(s.confirmIn))
	}
}

func TestRunAsksTheWarningsOwnQuestion(t *testing.T) {
	svc, _ := newTestService(t)
	spec := incompatibleSpec()
	s := &script{
		t:       t,
		root:    []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick:    []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		confirm: []bool{true},
	}

	plan, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.confirmIn) != 1 {
		t.Fatalf("confirm shown %d times, want 1", len(s.confirmIn))
	}
	// The wording comes from the planner, not from this package.
	if s.confirmIn[0].Question != "Launch anyway?" {
		t.Errorf("question = %q, want the planner's own wording", s.confirmIn[0].Question)
	}
	if len(s.confirmIn[0].Lines) == 0 ||
		!strings.HasPrefix(s.confirmIn[0].Lines[0], "warning: ") {
		t.Errorf("lines = %v, want the warning text with its prefix", s.confirmIn[0].Lines)
	}
	if len(plan.Warnings) == 0 {
		t.Error("the returned plan dropped its warnings; the CLI renders them to stderr")
	}
}

func TestRunDecliningTheConfirmReturnsToThePicker(t *testing.T) {
	svc, _ := newTestService(t)
	spec := incompatibleSpec()
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"},
			{Kind: pickBack},
		},
		confirm: []bool{false},
	}
	s.root = append(s.root, rootChoice{Kind: choiceCancel})

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.pickIn) != 2 {
		t.Errorf("picker opened %d times, want 2 (declining must go back to it)", len(s.pickIn))
	}
}

func TestRunAssumeYesSkipsTheConfirmScreen(t *testing.T) {
	svc, _ := newTestService(t)
	spec := incompatibleSpec()
	opts := stubOptions(svc, spec)
	opts.AssumeYes = true

	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
	}

	plan, err := run(context.Background(), opts, s.screens())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.confirmIn) != 0 {
		t.Errorf("confirm shown %d times under --yes", len(s.confirmIn))
	}
	// --yes suppresses the interruption, never the information: the CLI still
	// renders these to stderr.
	if len(plan.Warnings) == 0 {
		t.Error("--yes dropped the warnings from the plan")
	}
}

func TestRunNotInstalledShowsTheHintAndReturnsToRoot(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	spec.Launcher.(*stubLauncher).installed = false
	spec.Launcher.(*stubLauncher).installHint = "Install it from https://example.test/install"

	s := &script{
		t: t,
		root: []rootChoice{
			{Kind: choiceAgent, Agent: spec},
			{Kind: choiceCancel},
		},
		pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.noticeIn) != 1 {
		t.Fatalf("notices = %d, want 1", len(s.noticeIn))
	}
	if !strings.Contains(strings.Join(s.noticeIn[0].Lines, " "), "example.test/install") {
		t.Errorf("notice = %+v, does not carry NotInstalledError.Hint", s.noticeIn[0])
	}
	// A different model cannot fix a missing binary, but a different agent
	// can, so this returns to root rather than the picker.
	if len(s.rootIn) != 2 {
		t.Errorf("root opened %d times, want 2", len(s.rootIn))
	}
	if len(s.pickIn) != 1 {
		t.Errorf("picker opened %d times, want 1", len(s.pickIn))
	}
}

func TestRunUnknownModelFromAProfileListsSuggestions(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t: t,
		root: []rootChoice{
			// openrouter.Suggest matches by literal substring containment,
			// not fuzzy similarity, so the stale slug must actually be a
			// substring of the real one for Plan to offer it back.
			{Kind: choiceProfile, Profile: config.Profile{
				Name: "stale", Agent: "claude", Model: "anthropic/claude-opus-4",
			}},
			{Kind: choiceCancel},
		},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.noticeIn) != 1 {
		t.Fatalf("notices = %d, want 1", len(s.noticeIn))
	}
	joined := strings.Join(s.noticeIn[0].Lines, "\n")
	if !strings.Contains(joined, "anthropic/claude-opus-4.6") {
		t.Errorf("notice = %q, does not offer UnknownModelError.Suggestions", joined)
	}
}

func TestRunUnsupportedAgentFromAProfileExplainsWhy(t *testing.T) {
	svc, _ := newTestService(t)
	spec := unsupportedSpec("copilot", "cannot be pointed at a custom endpoint")
	s := &script{
		t: t,
		root: []rootChoice{
			{Kind: choiceProfile, Profile: config.Profile{
				Name: "old", Agent: "copilot", Model: "anthropic/claude-opus-4.6",
			}},
			{Kind: choiceCancel},
		},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.noticeIn) != 1 {
		t.Fatalf("notices = %d, want 1", len(s.noticeIn))
	}
	if !strings.Contains(strings.Join(s.noticeIn[0].Lines, " "), "custom endpoint") {
		t.Errorf("notice = %+v, does not carry the registry's stated reason", s.noticeIn[0])
	}
}

func TestRunPromptsForTheAPIKeyAndRetries(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv(config.APIKeyEnvVar, "") // force the key guard

	spec := stubSpec("claude")
	s := &script{
		t:      t,
		root:   []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick:   []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		prompt: []promptResult{{value: "sk-or-typed", ok: true}},
	}

	plan, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.promptIn) != 1 {
		t.Fatalf("prompt shown %d times, want 1", len(s.promptIn))
	}
	if !s.promptIn[0].Masked {
		t.Error("the API key prompt was not masked")
	}
	if plan.Command.Path == "" {
		t.Error("the retry after the key prompt did not produce a command")
	}
	if got := loadTestConfig(t).APIKey; got != "sk-or-typed" {
		t.Errorf("saved API key = %q, want the typed value", got)
	}
}

func TestRunCancellingTheAPIKeyPromptDoesNotLaunch(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv(config.APIKeyEnvVar, "")

	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"},
			{Kind: pickBack},
		},
		prompt: []promptResult{{ok: false}},
	}
	s.root = append(s.root, rootChoice{Kind: choiceCancel})

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
}

// The retry must happen at most once. A key that saves but still does not
// resolve would otherwise prompt forever; the script's Fatal on an
// over-called screen turns that hang into a failure.
func TestRunDoesNotLoopWhenTheSavedKeyStillDoesNotResolve(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv(config.APIKeyEnvVar, "")

	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		// The fake bypasses Validate, so an empty key reaches the config and
		// ResolveAPIKey rejects it again on the retry.
		prompt: []promptResult{{value: "", ok: true}},
	}

	_, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if err == nil {
		t.Fatal("run succeeded with an unusable API key")
	}
	if len(s.promptIn) != 1 {
		t.Errorf("prompt shown %d times, want exactly 1", len(s.promptIn))
	}
}
