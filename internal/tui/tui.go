package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/teggen/agentlaunch/agent"
	"github.com/teggen/agentlaunch/catalog"
	"github.com/teggen/agentlaunch/launch"
	"github.com/teggen/openrouter-launch/internal/config"
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

	// Registry is the set of agents this session offers, and the source of
	// every List/Installed/Lookup answer it needs. It is required and
	// injected rather than defaulted: this package has no provider to bind a
	// registry to, and taking it as one value is also what keeps tests off
	// PATH and the user's home directory — agent.Claude's install check
	// reads both. It replaced three separately overridable function fields,
	// which could disagree about which registry they were describing; one of
	// them was in fact bypassed entirely (see rootInput.Lookup).
	Registry *agent.Registry
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
	filters func(filterScreenInput) (filterScreenChoice, error)
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
	savedSort    config.Sort

	spec    *agent.Spec
	modelID string
	extra   []string
	plan    launch.Plan

	// refreshLeft is the unspent --refresh.
	refreshLeft bool

	models []catalog.Model
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
	if opts.Registry == nil {
		return launch.Plan{}, errors.New("tui: Options.Registry is required")
	}
	cfg, err := config.Load()
	if err != nil {
		return launch.Plan{}, err
	}

	s := &session{
		ctx: ctx, opts: opts, sc: sc, cfg: cfg,
		filters:      filterStateFrom(cfg.Filters, cfg.Sort),
		savedFilters: cfg.Filters,
		savedSort:    cfg.Sort,
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
		Agents:    s.opts.Registry.List(),
		Installed: s.opts.Registry.Installed,
		Lookup:    s.opts.Registry.Lookup,
		LastAgent: s.cfg.LastAgent,
	})
	if err != nil {
		return stateDone, err
	}

	switch choice.Kind {
	case choiceProfile:
		spec, lerr := s.opts.Registry.Lookup(choice.Profile.Agent)
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
	// ctrl+c, not esc: it ends the session immediately, before any of the
	// Kind-based routing below — including backState()/rootOrDone() — gets a
	// say, matching every other screen's handling of ctrl+c.
	if choice.Cancelled {
		return stateDone, ErrCancelled
	}

	switch choice.Kind {
	case pickSaveProfile:
		s.lastModelID = choice.ModelID
		if err := s.saveProfile(choice.ModelID); err != nil {
			return stateDone, err
		}
		return statePicker, nil

	case pickFilters:
		// Preselecting the reopened picker is what keeps your place across a
		// filter change; ctrl+f carries the highlighted model for exactly
		// this. It may be empty when the list was filtered to nothing, which
		// indexOfModel already treats as "start at the top".
		s.lastModelID = choice.ModelID
		fc, ferr := s.sc.filters(filterScreenInput{Filters: s.filters, Models: s.models})
		if ferr != nil {
			return stateDone, ferr
		}
		if fc.Cancelled {
			return stateDone, ErrCancelled
		}
		// Only on apply. s.filters already holds what the picker reported, so
		// assigning unconditionally would commit the edits a cancel discarded
		// — the screen returns what it opened with in that case, but relying
		// on that would make the driver's correctness depend on the screen's.
		if fc.Applied {
			s.filters = fc.Filters
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
		return s.promptForAPIKey(err, lines)
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

// promptForAPIKey collects and saves an API key, then retries planning.
//
// lines carries any warnings launch.Plan accumulated before its ErrNoAPIKey
// guard — see handlePlanError and launch.Plan's doc comment, "Callers must
// render Plan.Warnings before inspecting err." Both dead ends below (the
// keyPrompted guard tripping, and the user declining to enter a key) render
// lines before returning, or that fact — often the very reason the catalog
// looked wrong in the first place — reaches no screen at all.
func (s *session) promptForAPIKey(planErr error, lines []string) (state, error) {
	if s.keyPrompted {
		// A key was already collected and the retry still reports none, so
		// the save must have failed or the value was unusable. Prompting
		// again would loop forever.
		return s.noticeThenDone(noticeInput{
			Title: "Could not plan the launch",
			Lines: append(lines, planErr.Error()),
		}, planErr)
	}

	// config.Path() honors XDG_CONFIG_HOME; a hardcoded ~/.config/... path
	// would tell some users their key is going somewhere it is not. When the
	// path cannot be resolved (e.g. os.UserHomeDir failing), the disclosure
	// falls back to wording that names no file rather than either lying or
	// putting a raw error into a prompt title.
	keyPath := "your config file"
	if path, perr := config.Path(); perr == nil {
		keyPath = path
	}

	key, ok, err := s.sc.prompt(promptInput{
		// This is the only credential the tool writes to disk, and the only
		// path around it is the environment variable — see config.go's
		// ResolveAPIKey and the design spec's note on why saving stays
		// unconditional. The prompt discloses that before the user types,
		// rather than saving silently.
		Title:    "An OpenRouter API key is needed to launch.\nIt's saved to " + keyPath + " (mode 0600).",
		Label:    "API key",
		Masked:   true,
		Validate: validateAPIKey,
	})
	if err != nil {
		return stateDone, err
	}
	if !ok {
		if len(lines) == 0 {
			return s.retreat(s.backState())
		}
		return s.noticeThen(noticeInput{
			Title: "Could not plan the launch",
			Lines: lines,
		}, s.backState())
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

// rootOrDone is where backState() falls back to when the current attempt
// did not come from the picker.
//
// Its opts.Agent != nil branch is unreachable from backState(), its sole
// remaining caller: whenever Options.Agent is set, the session starts
// directly at the picker and stepRoot (the only place that clears
// fromPicker) never runs, so fromPicker is always true by the time
// backState() would fall through to this function — backState() returns
// statePicker first instead. The branch stays as defense in depth rather
// than being removed, and this function must not be "fixed" to return the
// error instead — that was the tempting fix for a real bug, and the wrong
// one. With Options.Agent set there is no root to fall back to, so the
// NotInstalledError, UnsupportedAgentError and UnsupportedPlatformError
// branches of handlePlanError dead-ended here, got stateDone, and retreat()
// turned that into ErrCancelled, which the CLI maps to a silent exit 0:
// `openrouter-launch claude` reported success with Claude Code not
// installed, while the identical condition under `-m <slug>` exited 1. The
// fix belongs in noticeThenFatal, which ends the session with the original
// error only when there is no root to return to. Changing rootOrDone would
// break the legitimate retreats that also reach it — declining the confirm
// screen, cancelling the API-key prompt — which must stay cancellations.
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

// noticeThenDone shows a notice and ends the session with err — a genuine
// planning failure, not a user-initiated cancellation, so err rather than
// ErrCancelled is what Run returns. ctrl+c on the notice still takes
// precedence over err, matching every other notice in this file.
func (s *session) noticeThenDone(in noticeInput, err error) (state, error) {
	if nerr := s.sc.notice(in); nerr != nil {
		return stateDone, nerr
	}
	return stateDone, err
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

// finish persists the view state and returns. It runs on every exit — launch
// or cancel — because the picker's filters and sort are a remembered view, not
// a property of a successful launch.
func (s *session) finish(p launch.Plan, err error) (launch.Plan, error) {
	if perr := s.persistView(); perr != nil {
		// Best effort: this is the last thing on screen, and a failure to
		// show it must not mask the real result.
		_ = s.sc.notice(noticeInput{
			Title: "Could not save the filter and sort settings",
			Lines: []string{perr.Error()},
		})
	}
	return p, err
}

// persistView writes the filter and sort state if either changed, re-reading
// the config first. The re-read is not boilerplate: ctrl+s can add a profile
// during the very session whose view state is being written, and saving a
// config captured at start would delete it. launch.recordSelection re-reads
// for the same reason.
//
// Both halves are in the dirty check. Comparing only the filters would drop a
// session that changed nothing but the ordering — the exact case
// TestRunPersistsTheSortWithUnchangedFilters sets up.
func (s *session) persistView() error {
	filters, sortBy := s.filters.persisted(), s.filters.persistedSort()
	if filters == s.savedFilters && sortBy == s.savedSort {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Filters = filters
	cfg.Sort = sortBy
	return config.Save(cfg)
}

// validateAPIKey is the key prompt's Validate. It is a named function rather
// than a closure so it can be tested directly and asserted to be the one the
// prompt carries.
//
// Refusing a control character here is the second half of a fix whose first
// half is in the agent module. strings.TrimSpace does NOT strip NUL — it is
// not whitespace — so without this a pasted key with a stray byte is accepted,
// written to the config file (where encoding/json escapes it to \u0000 and
// hides it completely), and only fails at the NEXT launch with the key long
// since forgotten. A Windows user hit exactly that: launching opencode died
// with "exec: environment variable contains NUL".
//
// Refused, not sanitized. Silently altering a credential trades a clear
// message here for a 401 the user cannot explain later.
func validateAPIKey(v string) error {
	if strings.TrimSpace(v) == "" {
		return errors.New("a key is required — get one at https://openrouter.ai/keys")
	}
	if i := strings.IndexFunc(v, isControlChar); i >= 0 {
		return fmt.Errorf("that key contains a control character (%#02x at position %d) — "+
			"paste it again; copying from a terminal can pick up stray bytes", v[i], i)
	}
	return nil
}

// isControlChar reports whether r is a control character. It mirrors the
// agent module's rule for what an environment value may not contain: C0
// (below 0x20) covers NUL, tab, carriage return and newline; 0x7f is DEL.
//
// Duplicated rather than exported from there on purpose. The module's copy is
// what actually protects the launch and must keep working for every consuming
// tool; this one exists so THIS tool can refuse a bad key at the moment it is
// typed, which is the only point where the user still has the good one in
// their clipboard.
func isControlChar(r rune) bool {
	return r < 0x20 || r == 0x7f
}
