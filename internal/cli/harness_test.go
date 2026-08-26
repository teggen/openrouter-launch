package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/catalog"
	"github.com/teggen/openrouter-launch/internal/catalog/catalogtest"
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
	"github.com/teggen/openrouter-launch/internal/tui"
)

// harness builds a CLI wired to an in-memory catalog, with config and cache
// isolated to a temp dir. It replaces the mutable package globals the CLI
// used to carry for this purpose.
type harness struct {
	svc *launch.Service
	// reg is the registry the command tree is built from. It is a field
	// because a test that stubs a launcher's LookPath must mutate the SAME
	// *Spec the tree resolves — registries are values now, so a second one
	// built alongside would silently leave the stub unused.
	reg *agent.Registry
	// ran is the command the handoff would have executed.
	ran agent.Command
	// tuiPlan and tuiErr are what the injected TUI returns; tuiOpts records
	// what the CLI passed it.
	tuiPlan  launch.Plan
	tuiErr   error
	tuiOpts  []tui.Options
	tuiCalls int
}

// newHarness builds a harness against the shared catalogtest fixture catalog. Most
// tests want this; use newHarnessWith directly when a test needs a
// different catalog (e.g. one that always fails, to exercise the
// stale-cache fallback).
func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, catalogtest.NewCatalog())
}

// newHarnessWith builds a harness against catalog, with config and cache
// isolated to a temp dir.
func newHarnessWith(t *testing.T, source catalog.Catalog) *harness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	// tuiErr defaults to ErrCancelled, not nil: a test that reaches the TUI
	// without configuring tuiPlan would otherwise have handoff call Launch on
	// a zero-value launch.Plan, and recordSelection would dereference
	// p.Spec.Name and panic. Cancelling is the safe default outcome.
	h := &harness{tuiErr: tui.ErrCancelled, reg: openRouterRegistry()}
	// newService is the production wiring, so these tests exercise the real
	// cache, the real settings store and the real config dir — only the
	// catalog source and the process handoff are substituted.
	h.svc = newService(source)
	h.svc.Run = func(c agent.Command) error {
		h.ran = c
		return nil
	}
	return h
}

// seedStaleCache writes a catalog cache file older than openrouter.DefaultTTL
// into the isolated XDG_CACHE_HOME, containing the catalogtest fixture, so
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
		Schema    int             `json:"schema"`
		FetchedAt time.Time       `json:"fetched_at"`
		Models    []catalog.Model `json:"models"`
	}{Schema: openrouter.CacheSchema, FetchedAt: time.Now().Add(-48 * time.Hour), Models: catalogtest.Models()}) // older than DefaultTTL
	if err != nil {
		t.Fatalf("marshal cache file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
}

// root returns a fresh command tree with both streams writing into out.
func (h *harness) root(out *bytes.Buffer) *cobra.Command {
	root := newRootCmd(h.svc, h.reg, func(_ context.Context, o tui.Options) (launch.Plan, error) {
		h.tuiCalls++
		h.tuiOpts = append(h.tuiOpts, o)
		return h.tuiPlan, h.tuiErr
	})
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

// testHome points home-directory lookups at a fresh temp dir on every
// platform, so a test that needs an agent's binary to look ABSENT is not at
// the mercy of what is installed on the machine (Landmine 8).
//
// USERPROFILE is the load-bearing one: os.UserHomeDir reads it rather than
// HOME on Windows, so setting HOME alone isolates nothing there. This test
// passed on Windows anyway — but only because the runner happens to have no
// ~/.local/bin/claude, which is passing for the wrong reason.
//
// internal/agent has its own copy: a _test.go helper cannot be imported
// across packages, and exporting one from production code to serve tests
// would be worse.
func testHome(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("APPDATA", filepath.Join(dir, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	return dir
}

// tableRows parses a rendered ui.Table back into its cells, rejoining the
// lines of a wrapped cell. Row 0 is the header.
//
// Assertions go through this rather than matching substrings against the
// raw output. Once a cell can wrap, "does the output contain X" depends on
// where the wrap landed — which is not a property any of these tests means
// to assert, and which would make them fail for a cosmetic reason or, worse,
// pass because a phrase happened to survive intact.
//
// It assumes a row's first cell is never legitimately empty, which holds
// for every table here: NAME and MODEL are always set.
func tableRows(t *testing.T, out string) [][]string {
	t.Helper()

	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "│") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "│"), "│")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if len(rows) > 0 && cells[0] == "" {
			// A continuation line of a wrapped row.
			last := rows[len(rows)-1]
			for i := range cells {
				if i < len(last) && cells[i] != "" {
					last[i] = strings.TrimSpace(last[i] + " " + cells[i])
				}
			}
			continue
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		t.Fatalf("no table rows in output:\n%s", out)
	}
	return rows
}

// tableRow returns the body row whose first cell is name.
//
// It fails rather than returning a short row when the table has fewer
// columns than its header, so a dropped column reports itself instead of
// panicking out of an index expression several lines later — which takes
// the whole test binary with it and hides every other failure.
func tableRow(t *testing.T, out, name string) []string {
	t.Helper()
	rows := tableRows(t, out)
	for _, row := range rows[1:] {
		if row[0] != name {
			continue
		}
		if len(row) != len(rows[0]) {
			t.Fatalf("row %q has %d cells, header has %d:\n%s",
				name, len(row), len(rows[0]), out)
		}
		return row
	}
	t.Fatalf("no row named %q in output:\n%s", name, out)
	return nil
}

// wantColumns fails unless the table's header has exactly these columns.
// Position-indexed assertions are only meaningful against a known header.
func wantColumns(t *testing.T, out string, want ...string) {
	t.Helper()
	got := tableRows(t, out)[0]
	if len(got) != len(want) {
		t.Fatalf("header = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("header = %q, want %q", got, want)
		}
	}
}

// mustLookup resolves a name in the harness's own registry.
func (h *harness) mustLookup(t *testing.T, name string) *agent.Spec {
	t.Helper()
	spec, err := h.reg.Lookup(name)
	if err != nil {
		t.Fatalf("Lookup(%q): %v", name, err)
	}
	return spec
}

// stubClaudePath makes the harness's Claude launcher resolve without a real
// binary on this machine.
//
// It needs no t.Cleanup to put the previous LookPath back: each harness holds
// its own registry, so the launcher it mutates is not shared with any other
// test. Restoring was mandatory while the registry was a package global.
func (h *harness) stubClaudePath(t *testing.T) {
	t.Helper()
	claude, ok := h.mustLookup(t, "claude").Launcher.(*agent.Claude)
	if !ok {
		t.Fatalf("claude launcher has unexpected type")
	}
	claude.LookPath = func(string) (string, error) { return "/usr/local/bin/claude", nil }
}
