package version

import (
	"strings"
	"testing"
)

// TestDefaultsAreDevPlaceholders documents what a plain `go build` reports.
// The release build overwrites all three via -ldflags -X; `go test` never
// applies those, so these defaults are what this test always sees.
func TestDefaultsAreDevPlaceholders(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Version = %q, want %q for a non-release build", Version, "dev")
	}
	if Commit != "none" {
		t.Errorf("Commit = %q, want %q for a non-release build", Commit, "none")
	}
	if Date != "unknown" {
		t.Errorf("Date = %q, want %q for a non-release build", Date, "unknown")
	}
}

// TestStringReportsEveryInjectedField would pass vacuously if it only checked
// the defaults, because "dev"/"none"/"unknown" are also plausible substrings
// of an unrelated string. It assigns three distinct sentinels instead, so
// dropping any one field from String() fails this test by name.
func TestStringReportsEveryInjectedField(t *testing.T) {
	saved := [3]string{Version, Commit, Date}
	t.Cleanup(func() { Version, Commit, Date = saved[0], saved[1], saved[2] })

	Version, Commit, Date = "v9.9.9-sentinel", "cafebabe", "2026-01-02T03:04:05Z"

	got := String()
	for _, want := range []string{Version, Commit, Date} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
}
