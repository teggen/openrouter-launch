package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"github.com/teggen/openrouter-launch/internal/launch"
)

// harness builds a CLI wired to an in-memory catalog, with config and cache
// isolated to a temp dir. It replaces the mutable package globals the CLI
// used to carry for this purpose.
type harness struct {
	svc *launch.Service
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Run stays nil until Task 7: resolveAndRun still calls the package
	// runner, which launch tests stub with captureRun.
	return &harness{svc: &launch.Service{Catalog: &fakeCatalog{models: fakeModels()}}}
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
