package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// harness builds a CLI wired to an in-memory catalog, with config and cache
// isolated to a temp dir. It replaces the mutable package globals the CLI
// used to carry for this purpose.
type harness struct {
	svc *launch.Service
	// ran is the command the handoff would have executed.
	ran agent.Command
}

// newHarness builds a harness against the shared fakeCatalog fixture. Most
// tests want this; use newHarnessWith directly when a test needs a
// different catalog (e.g. one that always fails, to exercise the
// stale-cache fallback).
func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, &fakeCatalog{models: fakeModels()})
}

// newHarnessWith builds a harness against catalog, with config and cache
// isolated to a temp dir.
func newHarnessWith(t *testing.T, catalog openrouter.Catalog) *harness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	h := &harness{}
	h.svc = &launch.Service{
		Catalog: catalog,
		Run: func(c agent.Command) error {
			h.ran = c
			return nil
		},
	}
	return h
}

// seedStaleCache writes a catalog cache file older than openrouter.DefaultTTL
// into the isolated XDG_CACHE_HOME, containing the fakeModels() fixture, so
// Service.Snapshot falls back to stale cached data. Call it after the
// harness (or otherwise XDG_CACHE_HOME) is set up.
func seedStaleCache(t *testing.T) {
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
	}{FetchedAt: time.Now().Add(-48 * time.Hour), Models: fakeModels()}) // older than DefaultTTL
	if err != nil {
		t.Fatalf("marshal cache file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
}

// root returns a fresh command tree with both streams writing into out.
func (h *harness) root(out *bytes.Buffer) *cobra.Command {
	root := NewRootCmdWith(h.svc)
	root.SetOut(out)
	root.SetErr(out)
	return root
}

// run executes args, failing the test on error, and returns the output.
func (h *harness) run(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := h.root(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return out.String()
}

// exec executes args and returns the output and the error, for tests that
// assert on failure. Note the output is read AFTER Execute returns.
func (h *harness) exec(args ...string) (string, error) {
	var out bytes.Buffer
	root := h.root(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}
