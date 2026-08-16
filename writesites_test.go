package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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

// writeSiteAllowlist is the exhaustive Landmine 6 enumeration (amended twice
// in Phase 4b to a four-site form, then to five when cline became a
// ConfigWriter — see the write-site table in CLAUDE.md, and the one in
// docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md): the
// only files in the module allowed to call a raw write primitive. A write
// primitive anywhere else in the tree is a Critical defect — an
// agent-owned or otherwise unsanctioned write site slipping in unnoticed is
// exactly what this invariant exists to prevent.
var writeSiteAllowlist = map[string]bool{
	"internal/openrouter/cache.go": true, // site 1: models.json cache
	"internal/config/config.go":    true, // site 2: config.json
	"internal/launch/handoff.go":   true, // site 3: Staged materializer
	"internal/agent/droid.go":      true, // site 4: droid's ConfigWriter
	// Site 5: cline's ConfigWriter. Note the inversion relative to site 4 —
	// this one only ever RESTORES. Cline itself does the writing (-k is
	// persisted into its provider store), and the write primitive here exists
	// to put the user's file back, which is the narrower of the two powers.
	"internal/agent/cline.go": true,
}

// TestWriteSitesAreExhaustivelyEnumerated pins Landmine 6 as a regression
// tripwire instead of leaving it as a grep a human has to remember to run:
// it walks every non-test .go file in the module and asserts that a raw
// write primitive appears only in the five sanctioned files above, and
// that each of those five still has one (an entry the allowlist keeps
// around after its write moved elsewhere would silently understate the
// real enumeration).
func TestWriteSitesAreExhaustivelyEnumerated(t *testing.T) {
	seen := make(map[string]bool, len(writeSiteAllowlist))
	var stray []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
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
		rel := filepath.ToSlash(path)
		if writeSiteAllowlist[rel] {
			seen[rel] = true
		} else {
			stray = append(stray, rel)
		}
		return nil
	})
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
