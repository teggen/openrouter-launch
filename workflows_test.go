package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The Makefile installs the tool version developers run locally; the
// workflows pin the version CI runs. When those drift, `make lint` passes
// locally and CI fails (or worse, the reverse) for reasons no error message
// explains. These tests make the drift a test failure instead.

func makefileVar(t *testing.T, name string) string {
	t.Helper()
	src, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("reading Makefile: %v", err)
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\s*:?=\s*(\S+)`)
	m := re.FindSubmatch(src)
	if m == nil {
		t.Fatalf("Makefile declares no %s", name)
	}
	return string(m[1])
}

func TestCIPinsTheMakefilesGolangciLintVersion(t *testing.T) {
	want := makefileVar(t, "GOLANGCI_VERSION")
	ci, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("reading ci.yml: %v", err)
	}
	if !strings.Contains(string(ci), want) {
		t.Errorf("ci.yml does not pin golangci-lint %s (the Makefile's GOLANGCI_VERSION); local and CI would run different linters", want)
	}
}

// TestWorkflowActionsArePinnedToShas guards the supply chain: this repo's
// release workflow holds contents:write and the tool writes an API key to
// disk, so a mutable tag on a third-party action is the wrong risk. A pin is
// a 40-hex SHA; `uses: actions/checkout@v7` must fail this.
func TestWorkflowActionsArePinnedToShas(t *testing.T) {
	uses := regexp.MustCompile(`(?m)uses:\s+([^\s@]+)@(\S+)`)
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)

	for _, path := range []string{".github/workflows/ci.yml"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, m := range uses.FindAllStringSubmatch(string(src), -1) {
			action, ref := m[1], m[2]
			if strings.HasPrefix(action, "./") {
				continue // a local composite action has nothing to pin
			}
			if !sha.MatchString(ref) {
				t.Errorf("%s: %s is pinned to %q, want a full 40-character commit SHA", path, action, ref)
			}
		}
	}
}
