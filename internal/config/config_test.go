package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func withTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "openrouter-launch", "config.json")
}

func TestPathUsesXDGConfigHome(t *testing.T) {
	want := withTempConfig(t)
	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	withTempConfig(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Filters.ToolsOnly {
		t.Error("ToolsOnly should default to true")
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("got %d profiles, want 0", len(cfg.Profiles))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withTempConfig(t)

	cfg := &Config{
		APIKey:    "sk-or-test",
		LastAgent: "claude",
		LastModel: "anthropic/claude-opus-4.6",
		Filters:   Filters{ToolsOnly: true, MinContext: 128000, MaxPrice: 5},
		Profiles: []Profile{
			{Name: "opus-cc", Agent: "claude", Model: "anthropic/claude-opus-4.6", Args: []string{"--resume"}},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.APIKey != "sk-or-test" {
		t.Errorf("APIKey = %q", got.APIKey)
	}
	if got.LastModel != "anthropic/claude-opus-4.6" {
		t.Errorf("LastModel = %q", got.LastModel)
	}
	if got.Filters.MinContext != 128000 || got.Filters.MaxPrice != 5 {
		t.Errorf("Filters = %+v", got.Filters)
	}
	if len(got.Profiles) != 1 || got.Profiles[0].Name != "opus-cc" {
		t.Fatalf("Profiles = %+v", got.Profiles)
	}
	if len(got.Profiles[0].Args) != 1 || got.Profiles[0].Args[0] != "--resume" {
		t.Errorf("Args = %v", got.Profiles[0].Args)
	}
}

// TestSaveLoadRoundTripToolsOnlyFalse pins that an explicit false survives
// the round trip. Load seeds defaults() (which sets ToolsOnly true) and then
// unmarshals the file over it; this only overrides the default because
// Filters' fields lack `omitempty`. Adding omitempty later - a
// natural-looking cleanup - would make an explicit false vanish from the
// encoded JSON and silently flip back to true on the next Load.
func TestSaveLoadRoundTripToolsOnlyFalse(t *testing.T) {
	withTempConfig(t)

	cfg := &Config{Filters: Filters{ToolsOnly: false}}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Filters.ToolsOnly {
		t.Error("explicit ToolsOnly=false should round-trip as false, not be overridden by the true default")
	}
}

func TestSaveWritesFileMode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	path := withTempConfig(t)

	if err := Save(&Config{APIKey: "secret"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

// TestSaveCreatesConfigDirPrivate pins the directory around the API key.
// The file itself is 0600 (above), but a group- or world-traversable parent
// still leaks its existence, size, and mtime — and on macOS, where every
// user shares the `staff` group, group access is other users' access. The
// assertion is on the group/other bits rather than an exact mode so a
// developer with a stricter umask does not see a spurious failure; under the
// usual 0022 it is exactly 0700.
func TestSaveCreatesConfigDirPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	path := withTempConfig(t)

	if err := Save(&Config{APIKey: "secret"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("config dir mode = %o, want no group or other access (0700)", perm)
	}
}

func TestSavePreservesProfileOrder(t *testing.T) {
	withTempConfig(t)

	cfg := &Config{Profiles: []Profile{
		{Name: "zebra", Agent: "claude", Model: "a/b"},
		{Name: "alpha", Agent: "claude", Model: "c/d"},
	}}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Profiles[0].Name != "zebra" || got.Profiles[1].Name != "alpha" {
		t.Errorf("order not preserved: %+v", got.Profiles)
	}
}

func TestResolveAPIKeyPrefersEnvironment(t *testing.T) {
	withTempConfig(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-env")

	got, err := ResolveAPIKey(&Config{APIKey: "sk-or-file"})
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got != "sk-or-env" {
		t.Errorf("got %q, want the environment value", got)
	}
}

func TestResolveAPIKeyFallsBackToConfig(t *testing.T) {
	withTempConfig(t)
	t.Setenv("OPENROUTER_API_KEY", "")

	got, err := ResolveAPIKey(&Config{APIKey: "sk-or-file"})
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if got != "sk-or-file" {
		t.Errorf("got %q, want the config value", got)
	}
}

func TestResolveAPIKeyErrorsWhenAbsent(t *testing.T) {
	withTempConfig(t)
	t.Setenv("OPENROUTER_API_KEY", "")

	_, err := ResolveAPIKey(&Config{})
	if !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("got %v, want ErrNoAPIKey", err)
	}
}

func TestSortRoundTripsThroughTheConfigFile(t *testing.T) {
	path := withTempConfig(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sort != (Sort{}) {
		t.Errorf("a fresh config sorts by %+v, want the zero value (relevance)", cfg.Sort)
	}

	cfg.Sort = Sort{Column: "output", Desc: true}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{`"sort"`, `"column": "output"`, `"desc": true`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("config file is missing %s:\n%s", want, raw)
		}
	}

	back, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if back.Sort != (Sort{Column: "output", Desc: true}) {
		t.Errorf("round trip lost the sort: %+v", back.Sort)
	}
}

// APIKey and RecordSelection are the two adapters internal/launch is wired
// with. They are thin, but they are the ONLY place this tool's settings store
// meets the planner, and the planner can no longer test them: it does not
// know this package exists.

func TestAPIKeyPrefersTheEnvironment(t *testing.T) {
	withTempConfig(t)
	if err := Save(&Config{APIKey: "sk-from-file"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	t.Setenv(APIKeyEnvVar, "sk-from-env")

	got, err := APIKey()
	if err != nil {
		t.Fatalf("APIKey: %v", err)
	}
	if got != "sk-from-env" {
		t.Errorf("APIKey = %q, want the environment's value", got)
	}
}

func TestAPIKeyFallsBackToTheSavedKey(t *testing.T) {
	withTempConfig(t)
	if err := Save(&Config{APIKey: "sk-from-file"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	t.Setenv(APIKeyEnvVar, "")

	got, err := APIKey()
	if err != nil {
		t.Fatalf("APIKey: %v", err)
	}
	if got != "sk-from-file" {
		t.Errorf("APIKey = %q, want the saved key", got)
	}
}

// TestAPIKeyReportsErrNoAPIKeyUnwrapped is what the TUI's key prompt hangs
// on: launch.Plan returns this error untouched, and internal/tui branches on
// errors.Is to prompt in place rather than end the session. Wrapping it here
// with extra context would keep errors.Is working but put that context in
// front of the user at the prompt.
func TestAPIKeyReportsErrNoAPIKeyUnwrapped(t *testing.T) {
	withTempConfig(t)
	t.Setenv(APIKeyEnvVar, "")

	_, err := APIKey()
	if !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("APIKey returned %v, want ErrNoAPIKey", err)
	}
}

func TestRecordSelectionPersists(t *testing.T) {
	withTempConfig(t)

	if err := RecordSelection("claude", "anthropic/claude-opus-4.6"); err != nil {
		t.Fatalf("RecordSelection: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LastAgent != "claude" || cfg.LastModel != "anthropic/claude-opus-4.6" {
		t.Errorf("LastAgent/LastModel = %q/%q", cfg.LastAgent, cfg.LastModel)
	}
}

// TestRecordSelectionDoesNotClobberEditsSincePlanning is the reason this
// re-reads rather than taking a *Config from the caller. In the TUI a profile
// can be added with ctrl+s after the plan is built and before the launch, and
// a stale in-memory config written back over it would delete the profile the
// user just saved — silently, in the same keystroke that started the agent.
func TestRecordSelectionDoesNotClobberEditsSincePlanning(t *testing.T) {
	withTempConfig(t)

	// A config as it was when planning began.
	if err := Save(&Config{APIKey: "sk-or-test"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// An edit made between planning and launching.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.AddProfile(Profile{Name: "opus-cc", Agent: "claude", Model: "anthropic/claude-opus-4.6"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := RecordSelection("claude", "anthropic/claude-opus-4.6"); err != nil {
		t.Fatalf("RecordSelection: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Profiles) != 1 {
		t.Errorf("profiles = %+v, want the one added between planning and launching", got.Profiles)
	}
	if got.APIKey != "sk-or-test" {
		t.Errorf("APIKey = %q, want the saved key preserved", got.APIKey)
	}
}
