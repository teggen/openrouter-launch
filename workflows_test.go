package main

import (
	"os"
	"path/filepath"
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

// Anchored to the actual `version:` field, not a bare substring search. A
// plain strings.Contains passes for two wrong reasons: v2.12.2 is a PREFIX of
// v2.12.25, so a genuinely divergent pin satisfies it; and the string
// appearing anywhere — a comment, or a leftover after the whole
// golangci-lint-action step is deleted — satisfies it too.
func TestCIPinsTheMakefilesGolangciLintVersion(t *testing.T) {
	want := makefileVar(t, "GOLANGCI_VERSION")
	ci, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("reading ci.yml: %v", err)
	}
	re := regexp.MustCompile(`(?m)^\s*version:\s*` + regexp.QuoteMeta(want) + `\s*$`)
	if !re.Match(ci) {
		t.Errorf("ci.yml has no `version: %s` field pinning golangci-lint (the Makefile's GOLANGCI_VERSION); local and CI would run different linters", want)
	}
}

// TestWorkflowActionsArePinnedToShas guards the supply chain: this repo's
// release workflow holds contents:write and the tool writes an API key to
// disk, so a mutable tag on a third-party action is the wrong risk. A pin is
// a 40-hex SHA; `uses: actions/checkout@v7` must fail this.
func TestWorkflowActionsArePinnedToShas(t *testing.T) {
	uses := regexp.MustCompile(`(?m)uses:\s+([^\s@]+)@(\S+)`)
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)

	// Globbed rather than listed, so a workflow added later is covered
	// automatically instead of depending on someone remembering to extend
	// this slice.
	paths, err := filepath.Glob(".github/workflows/*.yml")
	if err != nil {
		t.Fatalf("globbing workflows: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no workflows found under .github/workflows/ — this test would pass vacuously")
	}
	for _, path := range paths {
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

// Anchored to the `version:` field for the same two reasons its golangci-lint
// sibling is: v2.17.1 is a PREFIX of v2.17.10, and a bare substring is also
// satisfied by the string surviving in a comment after the whole goreleaser
// step is deleted. Both holes were demonstrated on the real tree.
func TestReleaseWorkflowPinsTheMakefilesGoreleaserVersion(t *testing.T) {
	want := makefileVar(t, "GORELEASER_VERSION")
	wf, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatalf("reading release.yml: %v", err)
	}
	re := regexp.MustCompile(`(?m)^\s*version:\s*` + regexp.QuoteMeta(want) + `\s*$`)
	if !re.Match(wf) {
		t.Errorf("release.yml has no `version: %s` field pinning goreleaser (the Makefile's GORELEASER_VERSION); `make snapshot` and the published release would be built by different versions", want)
	}
}
