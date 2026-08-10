package tui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// erroringCatalog always fails, forcing Service.Snapshot down the
// stale-cache fallback path — the mirror of launch package's own
// erroringCatalog, needed here to reproduce the same stale-plus-cache
// situation from the driver's side.
type erroringCatalog struct{}

func (erroringCatalog) Models(context.Context) ([]openrouter.Model, error) {
	return nil, errors.New("network down")
}

// seedCatalogCache pre-populates the on-disk catalog cache in the JSON shape
// openrouter.Cache expects, without depending on its unexported cacheFile
// type — only the on-disk shape needs to match, as internal/launch's own
// service_test.go helper does. fetchedAt controls freshness: within
// openrouter.DefaultTTL the cache is served without a fetch; older forces
// one. The caller must set XDG_CACHE_HOME before calling this so
// openrouter.CachePath() resolves inside the test's temp dir.
func seedCatalogCache(t *testing.T, fetchedAt time.Time) {
	t.Helper()
	path, err := openrouter.CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(struct {
		FetchedAt time.Time          `json:"fetched_at"`
		Models    []openrouter.Model `json:"models"`
	}{FetchedAt: fetchedAt, Models: ortest.Models()})
	if err != nil {
		t.Fatalf("marshal cache file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
}

type promptResult struct {
	value string
	ok    bool
	// err lets a test script ctrl+c: the driver's prompt closure returns
	// ErrCancelled on ctrl+c (see program.go), never a plain (value, false).
	err error
}

// confirmResult mirrors promptResult: ok is the plain y/n answer, and err
// lets a test script ctrl+c the same way the real confirm closure does.
type confirmResult struct {
	ok  bool
	err error
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
	confirm []confirmResult
	// noticeErr lets a test script ctrl+c on a notice; consumed in the same
	// order as notice calls, defaulting to nil once exhausted (or if never
	// set) so existing scripts that never care about the return need not
	// populate it.
	noticeErr []error

	filters []filterScreenChoice

	rootIn    []rootInput
	pickIn    []pickerInput
	promptIn  []promptInput
	confirmIn []confirmInput
	noticeIn  []noticeInput
	filtersIn []filterScreenInput
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
			return out.value, out.ok, out.err
		},
		confirm: func(in confirmInput) (bool, error) {
			s.t.Helper()
			s.confirmIn = append(s.confirmIn, in)
			if len(s.confirm) == 0 {
				s.t.Fatalf("confirm called %d times, more than scripted", len(s.confirmIn))
			}
			out := s.confirm[0]
			s.confirm = s.confirm[1:]
			return out.ok, out.err
		},
		filters: func(in filterScreenInput) (filterScreenChoice, error) {
			s.t.Helper()
			s.filtersIn = append(s.filtersIn, in)
			if len(s.filters) == 0 {
				s.t.Fatalf("filters screen called %d times, more than scripted", len(s.filtersIn))
			}
			out := s.filters[0]
			s.filters = s.filters[1:]
			return out, nil
		},
		notice: func(in noticeInput) error {
			s.noticeIn = append(s.noticeIn, in)
			if len(s.noticeErr) == 0 {
				return nil
			}
			err := s.noticeErr[0]
			s.noticeErr = s.noticeErr[1:]
			return err
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
//
// The cache is pre-warmed BEFORE asserting a fetch happened. Without that, a
// cold cache forces a fetch regardless of Refresh (cache.Load fetches
// unconditionally when there is no cache file), so cat.calls == 1 would hold
// whether or not stepPlan actually passes Refresh through. Warming the cache
// first removes that confound: the only thing that can now explain a fetch
// is --refresh being honored.
func TestRunProfilePathSpendsRefreshOnPlan(t *testing.T) {
	svc, cat := newTestService(t)
	seedCatalogCache(t, time.Now())
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
		t.Errorf("catalog fetched %d times, want 1; --refresh was not honored", cat.calls)
	}
}

// The mirror of the test above: with a warm cache and Refresh false, the
// catalog must NOT be fetched. Neither this test nor the one above is
// sufficient alone — the first only pins the fetch happening when Refresh is
// true, the second only pins it not happening when Refresh is false. Together
// they pin both halves of "exactly once"; a regression dropping Refresh from
// stepPlan entirely would pass whichever half runs Refresh: false but fail
// the other.
func TestRunProfilePathWithoutRefreshServesTheCache(t *testing.T) {
	svc, cat := newTestService(t)
	seedCatalogCache(t, time.Now())
	spec := stubSpec("claude")
	opts := stubOptions(svc, spec)
	opts.Refresh = false

	s := &script{t: t, root: []rootChoice{{Kind: choiceProfile, Profile: config.Profile{
		Name: "p", Agent: "claude", Model: "anthropic/claude-opus-4.6",
	}}}}

	if _, err := run(context.Background(), opts, s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if cat.calls != 0 {
		t.Errorf("catalog fetched %d times, want 0; a fresh cache should short-circuit the fetch", cat.calls)
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

// The round trip ctrl+f makes: picker → filters screen → picker. Applying a
// filter must carry the new filters into the reopened picker AND land it back
// on the model that was highlighted, or the screen would be unusable for
// comparing two models across a filter change.
func TestRunApplyingFiltersReopensThePickerOnTheSameModel(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickFilters, ModelID: "openai/o1-mini"},
			{Kind: pickModel, ModelID: "openai/o1-mini"},
		},
		filters: []filterScreenChoice{
			{Filters: filterState{toolsOnly: true}, Applied: true},
		},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(s.filtersIn) != 1 {
		t.Fatalf("filters screen opened %d times, want 1", len(s.filtersIn))
	}
	if len(s.pickIn) != 2 {
		t.Fatalf("picker opened %d times, want 2", len(s.pickIn))
	}
	if !s.pickIn[1].Filters.toolsOnly {
		t.Errorf("reopened picker got %+v, want the applied filter", s.pickIn[1].Filters)
	}
	if s.pickIn[1].Selected != "openai/o1-mini" {
		t.Errorf("reopened picker preselected %q, want the model ctrl+f was pressed on",
			s.pickIn[1].Selected)
	}
}

// The filters screen must be handed the live filters, not the saved ones:
// otherwise a filter set earlier in the session vanishes from the panel that
// exists to display it.
func TestRunOpensTheFiltersScreenOnThePickersLiveFilters(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickFilters, Filters: filterState{freeOnly: true}},
			{Kind: pickModel, ModelID: "openai/o1-mini"},
		},
		filters: []filterScreenChoice{{Filters: filterState{freeOnly: true}}},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.filtersIn) != 1 || !s.filtersIn[0].Filters.freeOnly {
		t.Errorf("filters screen opened on %+v, want the picker's live filters", s.filtersIn)
	}
}

// Cancelling the panel must leave the session's filters exactly as the picker
// last reported them. A driver that assigned choice.Filters unconditionally
// would commit the discarded edits.
func TestRunCancellingTheFiltersScreenKeepsThePickersFilters(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickFilters, Filters: filterState{freeOnly: true}},
			{Kind: pickModel, ModelID: "openai/o1-mini"},
		},
		// What a cancel returns: the filters the screen opened with, Applied
		// false. The toolsOnly edit the user made was discarded by the screen.
		filters: []filterScreenChoice{{Filters: filterState{freeOnly: true}}},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := s.pickIn[1].Filters; got != (filterState{freeOnly: true}) {
		t.Errorf("reopened picker got %+v, want the picker's own live filters", got)
	}
}

func TestRunFiltersScreenCtrlCCancelsImmediately(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	s := &script{
		t:       t,
		root:    []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick:    []pickerChoice{{Kind: pickFilters}},
		filters: []filterScreenChoice{{Cancelled: true}},
	}

	_, err := run(context.Background(), stubOptions(svc, spec), s.screens())

	if !errors.Is(err, ErrCancelled) {
		t.Errorf("err = %v, want ErrCancelled", err)
	}
	if len(s.pickIn) != 1 {
		t.Errorf("picker opened %d times, want 1: ctrl+c must not reopen it", len(s.pickIn))
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

// The acknowledge-mode branch of stepConfirm — warnings present, none
// carrying a Question — had no driver coverage at all. It is a whole
// user-visible mode (an enter/esc footer instead of a y/N question), driven
// here by the most common warning: a stale catalog.
func TestRunStepConfirmAcknowledgeModeForAStaleCatalog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(config.APIKeyEnvVar, "test-key")
	seedCatalogCache(t, time.Now().Add(-48*time.Hour))

	svc := &launch.Service{
		Catalog: erroringCatalog{},
		Run:     func(agent.Command) error { return errors.New("the driver must not launch") },
	}
	spec := stubSpec("claude")
	s := &script{
		t:       t,
		root:    []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick:    []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		confirm: []confirmResult{{ok: true}},
	}

	plan, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.confirmIn) != 1 {
		t.Fatalf("confirm shown %d times, want 1", len(s.confirmIn))
	}
	if s.confirmIn[0].Question != "" {
		t.Errorf("question = %q, want empty (acknowledge mode)", s.confirmIn[0].Question)
	}
	joined := strings.Join(s.confirmIn[0].Lines, "\n")
	if !strings.Contains(joined, "could not refresh the model catalog") {
		t.Errorf("confirm lines = %q, missing the stale-catalog warning", joined)
	}
	if plan.Model.ID == "" {
		t.Error("acknowledging the warning did not produce a plan")
	}
}

func TestRunAsksTheWarningsOwnQuestion(t *testing.T) {
	svc, _ := newTestService(t)
	spec := incompatibleSpec()
	s := &script{
		t:       t,
		root:    []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick:    []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		confirm: []confirmResult{{ok: true}},
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
		confirm: []confirmResult{{ok: false}},
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

// With Options.Agent set (the `openrouter-launch claude` path) there is no
// root screen to return to, so a missing binary must end the session with
// NotInstalledError, not fold into a silent ErrCancelled — the CLI maps
// ErrCancelled to exit 0, which would make `openrouter-launch claude` report
// success with the binary missing while `openrouter-launch claude -m <slug>`
// exits 1 for the identical condition.
func TestRunNotInstalledWithAgentSetIsFatal(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	spec.Launcher.(*stubLauncher).installed = false
	spec.Launcher.(*stubLauncher).installHint = "Install it from https://example.test/install"

	opts := stubOptions(svc, spec)
	opts.Agent = spec

	s := &script{t: t, pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}}}

	_, err := run(context.Background(), opts, s.screens())
	if errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want a real error, not ErrCancelled", err)
	}
	var notInstalled *launch.NotInstalledError
	if !errors.As(err, &notInstalled) {
		t.Fatalf("err = %v, want *launch.NotInstalledError", err)
	}
	if len(s.noticeIn) != 1 {
		t.Fatalf("notices = %d, want 1", len(s.noticeIn))
	}
	if !strings.Contains(strings.Join(s.noticeIn[0].Lines, " "), "example.test/install") {
		t.Errorf("notice = %+v, does not carry NotInstalledError.Hint", s.noticeIn[0])
	}
}

// Same shape as TestRunNotInstalledWithAgentSetIsFatal for the
// unsupported-agent branch: with no root to return to, the agent genuinely
// cannot be launched, so this must end the session with
// UnsupportedAgentError rather than a clean cancellation.
func TestRunUnsupportedAgentWithAgentSetIsFatal(t *testing.T) {
	svc, _ := newTestService(t)
	spec := unsupportedSpec("copilot", "cannot be pointed at a custom endpoint")

	opts := stubOptions(svc, spec)
	opts.Agent = spec

	s := &script{t: t, pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}}}

	_, err := run(context.Background(), opts, s.screens())
	if errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want a real error, not ErrCancelled", err)
	}
	var unsupported *launch.UnsupportedAgentError
	if !errors.As(err, &unsupported) {
		t.Fatalf("err = %v, want *launch.UnsupportedAgentError", err)
	}
	if len(s.noticeIn) != 1 {
		t.Fatalf("notices = %d, want 1", len(s.noticeIn))
	}
	if !strings.Contains(strings.Join(s.noticeIn[0].Lines, " "), "custom endpoint") {
		t.Errorf("notice = %+v, does not carry the registry's stated reason", s.noticeIn[0])
	}
}

// Same shape as TestRunNotInstalledWithAgentSetIsFatal and
// TestRunUnsupportedAgentWithAgentSetIsFatal, for the third of the three
// agent-fatal guards in handlePlanError: with no root to return to, a
// platform the agent cannot run on must end the session with
// UnsupportedPlatformError, not a clean ErrCancelled.
func TestRunUnsupportedPlatformWithAgentSetIsFatal(t *testing.T) {
	svc, _ := newTestService(t)
	spec := platformBlockedSpec("droid", "windows is not supported yet")

	opts := stubOptions(svc, spec)
	opts.Agent = spec

	s := &script{t: t, pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}}}

	_, err := run(context.Background(), opts, s.screens())
	if errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want a real error, not ErrCancelled", err)
	}
	var platform *launch.UnsupportedPlatformError
	if !errors.As(err, &platform) {
		t.Fatalf("err = %v, want *launch.UnsupportedPlatformError", err)
	}
	if len(s.noticeIn) != 1 {
		t.Fatalf("notices = %d, want 1", len(s.noticeIn))
	}
	if !strings.Contains(strings.Join(s.noticeIn[0].Lines, " "), "windows is not supported yet") {
		t.Errorf("notice = %+v, does not carry the platform error", s.noticeIn[0])
	}
}

// The regression a naive fix would cause: a user-initiated retreat (declining
// the confirm screen, then backing out of the picker) must still cancel
// cleanly even with Options.Agent set. Only the three agent-fatal guards in
// handlePlanError become fatal; backing out on purpose is still a
// cancellation.
func TestRunDecliningConfirmWithAgentSetStillCancels(t *testing.T) {
	svc, _ := newTestService(t)
	spec := incompatibleSpec()

	opts := stubOptions(svc, spec)
	opts.Agent = spec

	s := &script{
		t: t,
		pick: []pickerChoice{
			{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"},
			{Kind: pickBack},
		},
		confirm: []confirmResult{{ok: false}},
	}

	_, err := run(context.Background(), opts, s.screens())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.pickIn) != 2 {
		t.Errorf("picker opened %d times, want 2 (declining must go back to it)", len(s.pickIn))
	}
}

// The motivating scenario: from a picker reached after declining a confirm,
// ctrl+c used to be aliased to esc (pickBack), which routed back to root —
// requiring a THIRD press (esc from root) to actually leave. ctrl+c must end
// the session on this very next screen instead.
func TestRunPickerCtrlCCancelsImmediatelyEvenAfterADecline(t *testing.T) {
	svc, _ := newTestService(t)
	spec := incompatibleSpec()
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"},
			{Kind: pickBack, Cancelled: true},
		},
		confirm: []confirmResult{{ok: false}},
	}

	_, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.pickIn) != 2 {
		t.Errorf("picker opened %d times, want 2", len(s.pickIn))
	}
	if len(s.rootIn) != 1 {
		t.Errorf("root opened %d times, want 1 — ctrl+c must not reopen it", len(s.rootIn))
	}
}

// ctrl+c on the confirm screen itself must also end the session in one
// press, bypassing backState() (which would otherwise return to the picker).
func TestRunConfirmCtrlCCancelsImmediately(t *testing.T) {
	svc, _ := newTestService(t)
	spec := incompatibleSpec()
	s := &script{
		t:       t,
		root:    []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick:    []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		confirm: []confirmResult{{err: ErrCancelled}},
	}

	_, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.pickIn) != 1 {
		t.Errorf("picker opened %d times, want 1 — ctrl+c must not return to it", len(s.pickIn))
	}
}

// ctrl+c on the API-key prompt must also end the session in one press,
// bypassing the retreat-to-picker that a plain esc gets.
func TestRunAPIKeyPromptCtrlCCancelsImmediately(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv(config.APIKeyEnvVar, "")

	spec := stubSpec("claude")
	s := &script{
		t:      t,
		root:   []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick:   []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		prompt: []promptResult{{err: ErrCancelled}},
	}

	_, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.pickIn) != 1 {
		t.Errorf("picker opened %d times, want 1 — ctrl+c must not retry through it", len(s.pickIn))
	}
}

// ctrl+c on a notice must also end the session in one press, overriding even
// noticeThenFatal's routing (NotInstalledError, exercised here, would
// otherwise return to root since Options.Agent is unset in this scenario).
func TestRunNoticeCtrlCCancelsImmediately(t *testing.T) {
	svc, _ := newTestService(t)
	spec := stubSpec("claude")
	spec.Launcher.(*stubLauncher).installed = false
	spec.Launcher.(*stubLauncher).installHint = "install it"

	s := &script{
		t:         t,
		root:      []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick:      []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		noticeErr: []error{ErrCancelled},
	}

	_, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.rootIn) != 1 {
		t.Errorf("root opened %d times, want 1 — ctrl+c on the notice must not return to it", len(s.rootIn))
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

// launch.Plan's doc comment states the contract: warnings accumulated before
// a fatal guard are returned alongside the error, not discarded, because a
// stale catalog is frequently the reason a later guard failed. This puts a
// user through exactly that: an unknown model AND a stale, offline catalog at
// once, so the notice must explain both — not just the unknown-model half —
// or the one fact that explains the failure (the catalog is stale and could
// not refresh) is silently lost.
func TestRunRendersWarningsAccumulatedBeforeAFatalGuard(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(config.APIKeyEnvVar, "test-key")
	// Older than openrouter.DefaultTTL plus an always-failing catalog forces
	// Cache.Load down the stale-serve path, so Plan appends the stale warning
	// before it ever reaches the unknown-model guard below it.
	seedCatalogCache(t, time.Now().Add(-48*time.Hour))

	svc := &launch.Service{
		Catalog: erroringCatalog{},
		// Never called: the driver returns a plan and the caller launches.
		Run: func(agent.Command) error { return errors.New("the driver must not launch") },
	}
	spec := stubSpec("claude")
	s := &script{
		t: t,
		root: []rootChoice{
			// A substring of "anthropic/claude-opus-4.6" in the cached
			// fixture, so Suggest offers it back — same trick as
			// TestRunUnknownModelFromAProfileListsSuggestions.
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
	if !strings.Contains(joined, "could not refresh the model catalog") {
		t.Errorf("notice = %q, is missing the stale-catalog warning (plan.Warnings was dropped)", joined)
	}
	if !strings.Contains(joined, "anthropic/claude-opus-4.6") {
		t.Errorf("notice = %q, is missing UnknownModelError.Suggestions", joined)
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

// The one deliberate user-facing decision behind promptForAPIKey's title: it
// must tell the user their key is being written to disk, and it must name
// the file config.Path() actually resolves — not a hardcoded
// ~/.config/openrouter-launch/config.json that lies under a non-default
// XDG_CONFIG_HOME. newTestService already points XDG_CONFIG_HOME at a temp
// dir, so a hardcoded path would show up here as a mismatch even without a
// real custom XDG_CONFIG_HOME in play.
func TestRunAPIKeyPromptNamesTheResolvedConfigPathAndSaysItSaves(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv(config.APIKeyEnvVar, "") // force the key guard

	wantPath, err := config.Path()
	if err != nil {
		t.Fatalf("config.Path: %v", err)
	}

	spec := stubSpec("claude")
	s := &script{
		t:      t,
		root:   []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick:   []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		prompt: []promptResult{{value: "sk-or-typed", ok: true}},
	}

	if _, err := run(context.Background(), stubOptions(svc, spec), s.screens()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.promptIn) != 1 {
		t.Fatalf("prompt shown %d times, want 1", len(s.promptIn))
	}

	title := s.promptIn[0].Title
	if !strings.Contains(title, wantPath) {
		t.Errorf("title = %q, missing the resolved config path %q", title, wantPath)
	}
	if !strings.Contains(strings.ToLower(title), "saved") {
		t.Errorf("title = %q, does not tell the user the key will be saved", title)
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

// keyPrompted's guard bounds the retry after a key was actually SAVED, not
// how many times the screen may be offered. Pressing esc must not burn that
// one chance: the user backs out to the picker, picks a model again, and the
// prompt must still appear.
func TestRunEscFromTheKeyPromptOffersItAgainOnASecondAttempt(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv(config.APIKeyEnvVar, "")

	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{
			{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"},
			{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"},
		},
		prompt: []promptResult{
			{ok: false}, // esc: must not disable the retry
			{value: "sk-or-typed", ok: true},
		},
	}

	plan, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.promptIn) != 2 {
		t.Fatalf("prompt shown %d times, want 2 (esc must not disable a later retry)", len(s.promptIn))
	}
	if plan.Command.Path == "" {
		t.Error("the retry after the second key prompt did not produce a command")
	}
}

// launch.Plan's doc comment states the contract: warnings accumulated before
// a fatal guard are returned alongside err, not discarded. promptForAPIKey
// used to drop them on both of its dead-end returns. This puts a user
// through an offline, stale-cache session with no API key: they decline the
// key prompt, and the stale warning — the one fact explaining why the
// catalog may be wrong — must still reach a notice, not vanish.
func TestRunDecliningTheAPIKeyPromptRendersAccumulatedWarnings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(config.APIKeyEnvVar, "")
	seedCatalogCache(t, time.Now().Add(-48*time.Hour))

	svc := &launch.Service{
		Catalog: erroringCatalog{},
		Run:     func(agent.Command) error { return errors.New("the driver must not launch") },
	}
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
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	if len(s.noticeIn) != 1 {
		t.Fatalf("notices = %d, want 1", len(s.noticeIn))
	}
	joined := strings.Join(s.noticeIn[0].Lines, "\n")
	if !strings.Contains(joined, "could not refresh the model catalog") {
		t.Errorf("notice = %q, is missing the stale-catalog warning (dropped on the ErrNoAPIKey branch)", joined)
	}
}

// The mirror of the test above for promptForAPIKey's other dead end: the
// retry-exhausted guard. Same stale-catalog setup, but the saved key still
// does not resolve, so the guard trips instead of the user declining.
func TestRunAPIKeyRetryExhaustedRendersAccumulatedWarnings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(config.APIKeyEnvVar, "")
	seedCatalogCache(t, time.Now().Add(-48*time.Hour))

	svc := &launch.Service{
		Catalog: erroringCatalog{},
		Run:     func(agent.Command) error { return errors.New("the driver must not launch") },
	}
	spec := stubSpec("claude")
	s := &script{
		t:    t,
		root: []rootChoice{{Kind: choiceAgent, Agent: spec}},
		pick: []pickerChoice{{Kind: pickModel, ModelID: "anthropic/claude-opus-4.6"}},
		// The fake bypasses Validate, so an empty key reaches the config and
		// ResolveAPIKey rejects it again on the retry — same trick as
		// TestRunDoesNotLoopWhenTheSavedKeyStillDoesNotResolve.
		prompt: []promptResult{{value: "", ok: true}},
	}

	_, err := run(context.Background(), stubOptions(svc, spec), s.screens())
	if err == nil {
		t.Fatal("run succeeded with an unusable API key")
	}
	if len(s.noticeIn) != 1 {
		t.Fatalf("notices = %d, want 1", len(s.noticeIn))
	}
	joined := strings.Join(s.noticeIn[0].Lines, "\n")
	if !strings.Contains(joined, "could not refresh the model catalog") {
		t.Errorf("notice = %q, is missing the stale-catalog warning (dropped on the ErrNoAPIKey branch)", joined)
	}
}
