package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// writeSitePattern matches every raw filesystem-write primitive Landmine 6
// cares about. os.Create( is anchored on the opening paren (rather than a
// bare \bos\.Create\b) so it cannot also match os.CreateTemp. That anchor is
// not a nicety: an earlier revision of this check used the looser pattern and
// silently missed every CreateTemp hit — which is the atomic-write shape
// config.Save, writeDroidSettingsFile and writeClineProvidersFile all use, so
// the pattern that felt more permissive in fact saw fewer of the write sites
// it exists to find.
var writeSitePattern = regexp.MustCompile(
	`\bos\.WriteFile\b|\bos\.Create\(|\bos\.OpenFile\b|\bos\.MkdirAll\b|\bos\.Rename\b|\bos\.CreateTemp\b`,
)

// writeSiteAllowlist is this repository's half of the Landmine 6 enumeration
// (amended twice in Phase 4b to a four-site form, then to five when cline
// became a ConfigWriter — see the write-site table in CLAUDE.md, and the one
// in docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md).
//
// Landmine 6 is still a FIVE-site claim; the extraction split it across two
// modules. Sites 1 and 2 are here. Sites 3, 4 and 5 moved into
// github.com/teggen/agentlaunch and are covered by
// TestDependencyWriteSitesAreExhaustivelyEnumerated below, because this
// test's walk cannot follow them — it walks "." and a dependency is not
// under it.
//
// A write primitive anywhere else in this tree is a Critical defect — an
// agent-owned or otherwise unsanctioned write site slipping in unnoticed is
// exactly what this invariant exists to prevent.
var writeSiteAllowlist = map[string]bool{
	"internal/openrouter/cache.go": true, // site 1: models.json cache
	"internal/config/config.go":    true, // site 2: config.json
}

// dependencyWriteSiteAllowlist is the other half: the three sanctioned sites
// that now live in github.com/teggen/agentlaunch, keyed by their path within
// that module.
//
// The module runs this same check over its own tree, so why repeat it here?
// Because the claim users are given is about openrouter-launch — README's
// table enumerates the paths THIS tool writes, and CLAUDE.md's says "exactly
// five files". A sixth site introduced in the module, or a version bump that
// pulled one in, would satisfy every test in both repositories while making
// that claim false. This is the only check that sees both halves at once.
var dependencyWriteSiteAllowlist = map[string]bool{
	"launch/handoff.go": true, // site 3: Staged materializer
	"agent/droid.go":    true, // site 4: droid's ConfigWriter
	// Site 5: cline's ConfigWriter. Note the inversion relative to site 4 —
	// this one only ever RESTORES. Cline itself does the writing (-k is
	// persisted into its provider store), and the write primitive here exists
	// to put the user's file back, which is the narrower of the two powers.
	"agent/cline.go": true,
}

// TestWriteSitesAreExhaustivelyEnumerated pins Landmine 6 as a regression
// tripwire instead of leaving it as a grep a human has to remember to run:
// it walks every non-test .go file in this repository and asserts that a raw
// write primitive appears only in the sanctioned files above, and that each
// of those still has one (an entry the allowlist keeps around after its write
// moved elsewhere would silently understate the real enumeration).
func TestWriteSitesAreExhaustivelyEnumerated(t *testing.T) {
	seen, stray, err := scanWriteSites(".", writeSiteAllowlist)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(stray) > 0 {
		t.Errorf("write primitive found outside the sanctioned Landmine 6 sites: %v", stray)
	}
	for f := range writeSiteAllowlist {
		if !seen[f] {
			t.Errorf("sanctioned write site %q has no write primitive anymore — narrow writeSiteAllowlist", f)
		}
	}
}

// TestDependencyWriteSitesAreExhaustivelyEnumerated is the half of Landmine 6
// that lives outside this repository.
//
// Three of the five sanctioned write sites moved into
// github.com/teggen/agentlaunch. That module runs this same check over its
// own tree — but it cannot make the claim this repository publishes, which is
// about openrouter-launch as a whole: README enumerates the paths this tool
// writes and CLAUDE.md says "exactly five files". A sixth site added upstream,
// or pulled in by a version bump here, would leave every test in both
// repositories green while making both of those statements false.
//
// It resolves the dependency's source through `go list -m`, so it checks the
// version actually being built against — the local checkout under a
// workspace, the module cache otherwise — rather than a copy that could drift.
func TestDependencyWriteSitesAreExhaustivelyEnumerated(t *testing.T) {
	const module = "github.com/teggen/agentlaunch"

	// A context with a deadline, not exec.Command: `go list -m` can reach the
	// module proxy on a cold cache, and a hung download would otherwise hang
	// the whole run rather than failing it.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Dir}}", module).Output()
	if err != nil {
		// Deliberately fatal rather than skipped. A skip here fails OPEN: the
		// enumeration would go unchecked on exactly the runs where the
		// dependency could not be resolved, and report success doing it.
		t.Fatalf("resolving %s: %v", module, err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		t.Fatalf("go list -m gave no directory for %s; it cannot be checked", module)
	}

	seen, stray, err := scanWriteSites(dir, dependencyWriteSiteAllowlist)
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if len(stray) > 0 {
		t.Errorf("write primitive found in %s outside the sanctioned Landmine 6 sites: %v; "+
			"the write-site tables in README.md and CLAUDE.md are now understated", module, stray)
	}
	for f := range dependencyWriteSiteAllowlist {
		if !seen[f] {
			t.Errorf("sanctioned write site %q in %s has no write primitive anymore — "+
				"narrow dependencyWriteSiteAllowlist", f, module)
		}
	}
}

// scanWriteSites walks root and reports which allowlisted files hold a write
// primitive and which non-allowlisted ones do. Paths are reported relative to
// root and slash-separated, so an allowlist reads the same on every platform
// and does not depend on where the tree happens to be checked out — which
// matters for the dependency, whose directory is a module-cache path
// containing a version string.
func scanWriteSites(root string, allow map[string]bool) (seen map[string]bool, stray []string, err error) {
	seen = make(map[string]bool, len(allow))
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !writeSitePattern.Match(data) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if allow[key] {
			seen[key] = true
		} else {
			stray = append(stray, key)
		}
		return nil
	})
	return seen, stray, err
}
