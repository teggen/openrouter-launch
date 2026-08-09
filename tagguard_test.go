//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The release workflow's branch guard decides whether a tag may publish.
// Getting it backwards would either block every legitimate release or, worse,
// let a stable tag cut on develop publish as though it came from main. Git
// records no "branch this tag was pushed from", so the guard tests
// reachability — and this test builds a repo whose five tags discriminate
// every case.
//
//	main:    base ─────────────── v0.1.0 (stable, on main)
//	          │              │    ├── develop: beta work  v0.2.0-beta.1 (prerelease, on develop)
//	          │              │    │                        v0.2.0        (stable, NOT on main)
//	          │              │    └── hotfix:  stray work  v0.3.0-beta.1 (prerelease, NOT on develop)
//	          └── main advances ── v1.1.0+build-3 (stable; hyphen is in
//	                                build metadata, not a prerelease marker;
//	                                reachable from main only)
//
// Landmine for whoever mutation-checks this: check-tag-branch.sh is not a Go
// build input, so `go test` will happily serve a cached PASS/FAIL from
// before you edited the script. Always pass -count=1 when re-running after
// changing the script, or you are grading the wrong run.
func TestTagBranchGuard(t *testing.T) {
	script, err := filepath.Abs(filepath.Join(".github", "scripts", "check-tag-branch.sh"))
	if err != nil {
		t.Fatal(err)
	}

	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	git("init", "-q", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
	git("commit", "-q", "--allow-empty", "-m", "base")
	git("tag", "v0.1.0")

	git("checkout", "-q", "-b", "develop")
	git("commit", "-q", "--allow-empty", "-m", "beta work")
	git("tag", "v0.2.0-beta.1")
	git("tag", "v0.2.0")

	git("checkout", "-q", "-b", "hotfix", "main")
	git("commit", "-q", "--allow-empty", "-m", "stray work")
	git("tag", "v0.3.0-beta.1")

	// Advances main past the point where develop and hotfix diverged, so this
	// new commit is reachable from main only — a true discriminator. If the
	// hyphen check ever stops stripping build metadata first, this tag would
	// be (mis)classified prerelease, checked against develop instead of
	// main, found unreachable there, and wrongly refused.
	git("checkout", "-q", "main")
	git("commit", "-q", "--allow-empty", "-m", "main advances after both branches diverged")
	git("tag", "v1.1.0+build-3")

	cases := []struct {
		tag   string
		allow bool
		why   string
	}{
		{"v0.1.0", true, "stable tag reachable from main"},
		{"v0.2.0-beta.1", true, "prerelease tag reachable from develop"},
		{"v0.2.0", false, "stable tag cut on develop must be refused"},
		{"v0.3.0-beta.1", false, "prerelease tag not reachable from develop must be refused"},
		{"v1.1.0+build-3", true, "stable tag whose build metadata contains a hyphen must not be mistaken for a prerelease"},
	}

	for _, tc := range cases {
		t.Run(tc.tag+"/"+map[bool]string{true: "allow", false: "refuse"}[tc.allow], func(t *testing.T) {
			cmd := exec.Command("bash", script, tc.tag, "main", "develop")
			cmd.Dir = repo
			out, err := cmd.CombinedOutput()
			if tc.allow && err != nil {
				t.Fatalf("%s: guard refused a valid tag: %v\n%s", tc.why, err, out)
			}
			if !tc.allow && err == nil {
				t.Fatalf("%s: guard allowed it\n%s", tc.why, out)
			}
		})
	}
}

// TestCheckTagBranchScriptIsExecutable pins the file mode, not just its
// content. release.yml invokes the script by path
// (.github/scripts/check-tag-branch.sh ...), which requires the
// owner-execute bit; TestTagBranchGuard above invokes it via `bash script`,
// which works regardless of the bit and so cannot catch this. Losing the bit
// (e.g. an editor or a `git apply` that doesn't preserve mode) keeps this
// suite green while breaking the actual release.
func TestCheckTagBranchScriptIsExecutable(t *testing.T) {
	path := filepath.Join(".github", "scripts", "check-tag-branch.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&0o100 == 0 {
		t.Errorf("%s is not owner-executable (mode %s); release.yml invokes it by path, not through bash", path, info.Mode())
	}
}
