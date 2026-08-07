package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
