//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The release workflow's branch guard decides whether a tag may publish.
// Getting it backwards would either block every legitimate release or, worse,
// let a stable tag cut on develop publish as though it came from main. Git
// records no "branch this tag was pushed from", so the guard tests
// reachability — and this test builds a repo whose four tags discriminate
// every case.
//
//	main:    base ────────────────── v0.1.0 (stable, on main)
//	          ├── develop: beta work  v0.2.0-beta.1 (prerelease, on develop)
//	          │                       v0.2.0        (stable, NOT on main)
//	          └── hotfix:  stray work v0.3.0-beta.1 (prerelease, NOT on develop)
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

	cases := []struct {
		tag   string
		allow bool
		why   string
	}{
		{"v0.1.0", true, "stable tag reachable from main"},
		{"v0.2.0-beta.1", true, "prerelease tag reachable from develop"},
		{"v0.2.0", false, "stable tag cut on develop must be refused"},
		{"v0.3.0-beta.1", false, "prerelease tag not reachable from develop must be refused"},
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
