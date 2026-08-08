package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// ErrCancelled reports that the user backed out. The CLI maps it to a silent
// exit 0: cancelling a picker is not a failure.
var ErrCancelled = errors.New("cancelled")

// Options configures one interactive session.
type Options struct {
	// Service is required.
	Service *launch.Service
	// Agent skips the root screen and opens the picker for that agent, as
	// `openrouter-launch claude` does.
	Agent *agent.Spec
	// ExtraArgs is everything after --. It reaches the launched command and
	// is captured into any profile saved with ctrl+s.
	ExtraArgs []string
	// Refresh is --refresh. It is spent exactly once; see session.takeRefresh.
	Refresh bool
	// AssumeYes is --yes. It skips the confirm screen, never the warnings:
	// the caller still renders those.
	AssumeYes bool

	// The three fields below are injection points defaulting to the real
	// registry. They exist so tests never consult PATH or the user's home
	// directory — agent.Claude's install check does both.
	Agents    []*agent.Spec
	Installed func(*agent.Spec) bool
	Lookup    func(string) (*agent.Spec, error)
}

func (o Options) agents() []*agent.Spec {
	if o.Agents != nil {
		return o.Agents
	}
	return agent.List()
}

func (o Options) installed() func(*agent.Spec) bool {
	if o.Installed != nil {
		return o.Installed
	}
	return agent.Installed
}

func (o Options) lookup(name string) (*agent.Spec, error) {
	if o.Lookup != nil {
		return o.Lookup(name)
	}
	return agent.Lookup(name)
}

// screens is the set of interactive steps, as functions rather than direct
// calls, so the driver's navigation can be tested with no terminal and no
// bubbletea program. Production wires these to real programs in program.go.
//
// prompt returns (value, submitted, error): submitted is false when the user
// pressed esc, which is a normal outcome rather than an error.
type screens struct {
	root    func(rootInput) (rootChoice, error)
	pick    func(pickerInput) (pickerChoice, error)
	prompt  func(promptInput) (string, bool, error)
	confirm func(confirmInput) (bool, error)
	notice  func(noticeInput) error
}

// Run drives an interactive session and returns an approved plan.
//
// Run NEVER launches. The caller calls launch.Service.Launch after Run
// returns, so every bubbletea program has torn down and the terminal is out
// of raw mode before syscall.Exec replaces the process. Launching from inside
// a screen would hand a raw-mode terminal to the agent.
func Run(ctx context.Context, opts Options) (launch.Plan, error) {
	sc, err := liveScreens()
	if err != nil {
		return launch.Plan{}, err
	}
	return run(ctx, opts, sc)
}

type state int

const (
	stateRoot state = iota
	statePicker
	statePlan
	stateConfirm
	stateDone
)

type session struct {
	ctx  context.Context
	opts Options
	sc   screens
	cfg  *config.Config

	filters      filterState
	savedFilters config.Filters

	spec    *agent.Spec
	modelID string
	extra   []string
	plan    launch.Plan

	// refreshLeft is the unspent --refresh.
	refreshLeft bool

	models []openrouter.Model
	loaded bool

	// fromPicker records whether the current plan attempt came through the
	// picker, so an error screen knows where "back" is.
	fromPicker bool
	// lastModelID preselects the picker when it opens or reopens.
	lastModelID string
	// keyPrompted guards the API-key retry against looping.
	keyPrompted bool
}

func run(ctx context.Context, opts Options, sc screens) (launch.Plan, error) {
	if opts.Service == nil {
		return launch.Plan{}, errors.New("tui: Options.Service is required")
	}
	cfg, err := config.Load()
	if err != nil {
		return launch.Plan{}, err
	}

	s := &session{
		ctx: ctx, opts: opts, sc: sc, cfg: cfg,
		filters:      filterStateFrom(cfg.Filters),
		savedFilters: cfg.Filters,
		refreshLeft:  opts.Refresh,
		extra:        opts.ExtraArgs,
		lastModelID:  cfg.LastModel,
	}

	st := stateRoot
	if opts.Agent != nil {
		s.spec = opts.Agent
		st = statePicker
	}

	for st != stateDone {
		var err error
		switch st {
		case stateRoot:
			st, err = s.stepRoot()
		case statePicker:
			st, err = s.stepPicker()
		case statePlan:
			st, err = s.stepPlan()
		case stateConfirm:
			st, err = s.stepConfirm()
		default:
			// A state constant added later without a case here would leave st
			// and err unchanged and spin this loop forever; report it as the
			// bug it is instead of hanging at 100% CPU with no output.
			return s.finish(launch.Plan{}, fmt.Errorf("tui: unhandled state %d", st))
		}
		if err != nil {
			return s.finish(launch.Plan{}, err)
		}
	}
	return s.finish(s.plan, nil)
}

func (s *session) stepRoot() (state, error) {
	choice, err := s.sc.root(rootInput{
		Profiles:  s.cfg.Profiles,
		Agents:    s.opts.agents(),
		Installed: s.opts.installed(),
		LastAgent: s.cfg.LastAgent,
	})
	if err != nil {
		return stateDone, err
	}

	switch choice.Kind {
	case choiceProfile:
		spec, lerr := s.opts.lookup(choice.Profile.Agent)
		if lerr != nil {
			// A profile is a stored reference and can rot: its agent may have
			// been renamed or removed since it was saved.
			return s.noticeThen(noticeInput{
				Title: "Profile " + choice.Profile.Name + " names an unknown agent",
				Lines: []string{lerr.Error()},
			}, stateRoot)
		}
		s.spec = spec
		s.modelID = choice.Profile.Model
		s.extra = choice.Profile.Args
		s.fromPicker = false
		return statePlan, nil

	case choiceAgent:
		s.spec = choice.Agent
		s.modelID = ""
		s.extra = s.opts.ExtraArgs
		return statePicker, nil
	}
	return stateDone, ErrCancelled
}

func (s *session) stepPicker() (state, error) {
	if !s.loaded {
		snap, err := s.opts.Service.Snapshot(s.ctx, s.takeRefresh())
		if err != nil {
			return stateDone, err
		}
		s.models = snap.Models
		s.loaded = true
	}

	choice, err := s.sc.pick(pickerInput{
		Agent:    s.spec,
		Models:   s.models,
		Filters:  s.filters,
		Selected: s.lastModelID,
	})
	if err != nil {
		return stateDone, err
	}
	// Taken on every outcome, including pickBack: filters are persisted
	// whether or not the session goes on to launch.
	s.filters = choice.Filters

	switch choice.Kind {
	case pickSaveProfile:
		s.lastModelID = choice.ModelID
		if err := s.saveProfile(choice.ModelID); err != nil {
			return stateDone, err
		}
		return statePicker, nil

	case pickModel:
		s.modelID = choice.ModelID
		s.lastModelID = choice.ModelID
		s.fromPicker = true
		return statePlan, nil
	}

	// pickBack.
	if s.opts.Agent != nil {
		// The session started at the picker; there is no root to return to.
		return stateDone, ErrCancelled
	}
	return stateRoot, nil
}

func (s *session) stepPlan() (state, error) {
	plan, err := s.opts.Service.Plan(s.ctx, launch.Request{
		Spec:      s.spec,
		ModelID:   s.modelID,
		ExtraArgs: s.extra,
		Refresh:   s.takeRefresh(),
	})
	if err != nil {
		// plan.Warnings carries anything accumulated before the fatal guard
		// that produced err — see launch.Plan's doc comment — and must reach
		// the user, not just err.
		return s.handlePlanError(plan.Warnings, err)
	}

	s.plan = plan
	if len(plan.Warnings) == 0 || s.opts.AssumeYes {
		return stateDone, nil
	}
	return stateConfirm, nil
}

func (s *session) stepConfirm() (state, error) {
	lines := make([]string, 0, len(s.plan.Warnings))
	var questions []string
	for _, w := range s.plan.Warnings {
		// The "warning: " prefix is added here because this is the layer that
		// knows these are warnings; Warning.Message deliberately omits it so
		// each caller can render in its own idiom.
		lines = append(lines, "warning: "+w.Message)
		if w.Question != "" {
			questions = append(questions, w.Question)
		}
	}

	// With no question this is an acknowledgement: the full list with an
	// enter/esc footer. Only one warning kind carries a question today, but
	// asking each in turn matches what the CLI does and needs no revisiting
	// when a second one appears.
	if len(questions) == 0 {
		ok, err := s.sc.confirm(confirmInput{Title: "Before launching", Lines: lines})
		if err != nil {
			return stateDone, err
		}
		if !ok {
			return s.retreat(s.backState())
		}
		return stateDone, nil
	}

	for _, q := range questions {
		ok, err := s.sc.confirm(confirmInput{
			Title: "Before launching", Lines: lines, Question: q,
		})
		if err != nil {
			return stateDone, err
		}
		if !ok {
			return s.retreat(s.backState())
		}
	}
	return stateDone, nil
}

func (s *session) handlePlanError(warnings []launch.Warning, err error) (state, error) {
	var lines []string
	for _, w := range warnings {
		// The "warning: " prefix is added here because this is the layer that
		// knows these are warnings; Warning.Message deliberately omits it.
		lines = append(lines, "warning: "+w.Message)
	}

	// The API key is the one failure this screen can fix in place.
	if errors.Is(err, config.ErrNoAPIKey) {
		return s.promptForAPIKey(err)
	}

	var notInstalled *launch.NotInstalledError
	if errors.As(err, &notInstalled) {
		// Back to root, not the picker: a different model cannot fix a
		// missing binary, but a different agent can. With no root to return
		// to (Options.Agent set), this is fatal rather than a cancellation —
		// see noticeThenFatal.
		return s.noticeThenFatal(noticeInput{
			Title: notInstalled.DisplayName + " is not installed.",
			Lines: append(lines, notInstalled.Hint),
		}, err)
	}

	var unknown *launch.UnknownModelError
	if errors.As(err, &unknown) {
		lines = append(lines, "The catalog has no model with that slug.")
		if len(unknown.Suggestions) > 0 {
			lines = append(lines, "", "Did you mean:")
			for _, sug := range unknown.Suggestions {
				lines = append(lines, "  "+sug)
			}
		}
		return s.noticeThen(noticeInput{
			Title: "Unknown model " + unknown.ModelID,
			Lines: lines,
		}, s.backState())
	}

	var unsupported *launch.UnsupportedAgentError
	if errors.As(err, &unsupported) {
		// Fatal, not a cancellation, when there is no root to return to — see
		// noticeThenFatal.
		return s.noticeThenFatal(noticeInput{
			Title: unsupported.Agent + " cannot be pointed at OpenRouter",
			Lines: append(lines, unsupported.Reason),
		}, err)
	}

	var platform *launch.UnsupportedPlatformError
	if errors.As(err, &platform) {
		// Fatal, not a cancellation, when there is no root to return to — see
		// noticeThenFatal.
		return s.noticeThenFatal(noticeInput{
			Title: platform.Agent + " cannot run on this platform",
			Lines: append(lines, platform.Error()),
		}, err)
	}

	// Anything else — a catalog load failure with no cache, an unreadable
	// config — cannot be resolved from inside the picker. Still surface any
	// warnings accumulated before the failure: a stale catalog is frequently
	// the reason the guard below it failed, and a plain error here gives no
	// other way to see that. With no warnings this stays a plain failure, so
	// it does not gain a screen it never had before.
	if len(lines) > 0 {
		if nerr := s.sc.notice(noticeInput{
			Title: "Could not plan the launch",
			Lines: append(lines, err.Error()),
		}); nerr != nil {
			return stateDone, nerr
		}
	}
	return stateDone, err
}

func (s *session) promptForAPIKey(planErr error) (state, error) {
	if s.keyPrompted {
		// A key was already collected and the retry still reports none, so
		// the save must have failed or the value was unusable. Prompting
		// again would loop forever.
		return stateDone, planErr
	}

	key, ok, err := s.sc.prompt(promptInput{
		Title:  "An OpenRouter API key is needed to launch",
		Label:  "API key",
		Masked: true,
		Validate: func(v string) error {
			if strings.TrimSpace(v) == "" {
				return errors.New("a key is required — get one at https://openrouter.ai/keys")
			}
			return nil
		},
	})
	if err != nil {
		return stateDone, err
	}
	if !ok {
		return s.retreat(s.backState())
	}

	cfg, err := config.Load()
	if err != nil {
		return stateDone, err
	}
	cfg.APIKey = strings.TrimSpace(key)
	if serr := config.Save(cfg); serr != nil {
		if nerr := s.warn("Could not save the API key", serr); nerr != nil {
			return stateDone, nerr
		}
	}
	s.cfg = cfg
	// Set only now, on the path that actually saved a key and is about to
	// retry Plan with it. The guard above bounds THAT retry, not how many
	// times this screen may be offered — setting it earlier would burn the
	// user's one chance the moment they pressed esc.
	s.keyPrompted = true
	return statePlan, nil
}

func (s *session) saveProfile(modelID string) error {
	name, ok, err := s.sc.prompt(promptInput{
		Title: "Save profile",
		Label: "Name",
		Validate: func(v string) error {
			// Re-read so the validator sees profiles added since the session
			// began, and reuse config's own duplicate rule rather than
			// reimplementing it.
			cfg, lerr := config.Load()
			if lerr != nil {
				return lerr
			}
			return cfg.AddProfile(config.Profile{
				Name: v, Agent: s.spec.Name, Model: modelID,
			})
		},
	})
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return s.warn("Could not save the profile", err)
	}
	// ExtraArgs is captured so ctrl+s after
	// `openrouter-launch claude -- --resume` favorites the invocation the
	// user is actually performing, matching `profile add ... -- <args>`.
	if err := cfg.AddProfile(config.Profile{
		Name: name, Agent: s.spec.Name, Model: modelID, Args: s.opts.ExtraArgs,
	}); err != nil {
		return s.warn("Could not save the profile", err)
	}
	if err := config.Save(cfg); err != nil {
		return s.warn("Could not save the profile", err)
	}
	s.cfg = cfg
	return nil
}

// takeRefresh spends the --refresh flag. Service.Snapshot runs twice per
// launch — once to fill the picker, once inside Plan — and passing Refresh to
// both would make two HTTP round trips for one launch. Whichever call happens
// first spends it; on the profile path there is no picker, so Plan does.
func (s *session) takeRefresh() bool {
	r := s.refreshLeft
	s.refreshLeft = false
	return r
}

// rootOrDone is where to go when the problem is the agent itself. With
// Options.Agent set there is no root screen to return to.
func (s *session) rootOrDone() state {
	if s.opts.Agent != nil {
		return stateDone
	}
	return stateRoot
}

// backState is where to go when choosing differently might help: the picker,
// when that is where the choice came from.
func (s *session) backState() state {
	if s.fromPicker {
		return statePicker
	}
	return s.rootOrDone()
}

// retreat moves to next, turning "nowhere left to go back to" into a
// cancellation rather than a silent success with an empty plan.
func (s *session) retreat(next state) (state, error) {
	if next == stateDone {
		return stateDone, ErrCancelled
	}
	return next, nil
}

// noticeThen shows a notice and then moves to next. A notice screen failing
// is fatal: without it the user cannot see why anything stopped.
func (s *session) noticeThen(in noticeInput, next state) (state, error) {
	if err := s.sc.notice(in); err != nil {
		return stateDone, err
	}
	return s.retreat(next)
}

// noticeThenFatal shows the notice and returns to the root screen, or ends
// the session with err when there is no root to return to.
//
// The error rather than ErrCancelled is the point: these three conditions
// mean the agent genuinely cannot be launched, and reporting that as a clean
// cancellation would make `openrouter-launch claude` exit 0 with the binary
// missing while `openrouter-launch claude -m <slug>` exits 1 for the very
// same condition.
func (s *session) noticeThenFatal(in noticeInput, err error) (state, error) {
	if nerr := s.sc.notice(in); nerr != nil {
		return stateDone, nerr
	}
	if s.opts.Agent != nil {
		return stateDone, err
	}
	return stateRoot, nil
}

// warn shows a notice and continues. A persistence failure costs the user a
// convenience, not the launch, so it is reported rather than fatal.
func (s *session) warn(title string, err error) error {
	return s.sc.notice(noticeInput{Title: title, Lines: []string{err.Error()}})
}

// finish persists the filter state and returns. It runs on every exit —
// launch or cancel — because the picker's filters are a remembered view, not
// a property of a successful launch.
func (s *session) finish(p launch.Plan, err error) (launch.Plan, error) {
	if perr := s.persistFilters(); perr != nil {
		// Best effort: this is the last thing on screen, and a failure to
		// show it must not mask the real result.
		_ = s.sc.notice(noticeInput{
			Title: "Could not save the filter settings",
			Lines: []string{perr.Error()},
		})
	}
	return p, err
}

// persistFilters writes the filter state if it changed, re-reading the config
// first. The re-read is not boilerplate: ctrl+s can add a profile during the
// very session whose filters are being written, and saving a config captured
// at start would delete it. launch.recordSelection re-reads for the same
// reason.
func (s *session) persistFilters() error {
	next := s.filters.persisted()
	if next == s.savedFilters {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Filters = next
	return config.Save(cfg)
}
