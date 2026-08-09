//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ci.yml's audit job runs gosec with -no-fail so this repo's ~19 genuine
// findings stay advisory. That flag also makes gosec exit 0 when it analysed
// nothing at all: on a package with type errors it logs "no ssa result",
// writes a structurally valid SARIF holding `"results": []`, and returns 0 —
// satisfying every downstream control including `if-no-files-found: error`,
// which only ever asked whether a file exists.
//
// check-gosec-analysis.sh is the step that tells those two apart. The
// discriminating pair below is the whole point of this test:
//
//	healthyLog + emptySARIF  -> ALLOW (a genuinely clean tree must stay green)
//	ssaFailureLog + fullSARIF -> REFUSE (findings do not redeem a failed analysis)
//
// A guard that keyed on "the SARIF has no results" would get both backwards.
//
// Landmine for whoever mutation-checks this: check-gosec-analysis.sh is not
// a Go build input, so `go test` will serve a cached PASS from before you
// edited the script. Always pass -count=1 after changing it, or you are
// grading the wrong run.

// Verbatim gosec stderr from `gosec -no-fail -fmt sarif` over this repo
// (timestamps and the file list trimmed; the "Checking file:" shape is what
// the script keys on).
const healthyLog = `[gosec] 2026/08/09 17:06:16 Including rules: default
[gosec] 2026/08/09 17:06:16 Excluding rules: default
[gosec] 2026/08/09 17:06:16 Including analyzers: default
[gosec] 2026/08/09 17:06:16 Excluding analyzers: default
[gosec] 2026/08/09 17:06:16 Import directory: /home/runner/work/openrouter-launch/internal/launch
[gosec] 2026/08/09 17:06:16 Checking package: launch
[gosec] 2026/08/09 17:06:16 Checking file: /home/runner/work/openrouter-launch/internal/launch/handoff.go
[gosec] 2026/08/09 17:06:16 Checking file: /home/runner/work/openrouter-launch/internal/launch/service.go
`

// Verbatim gosec stderr from a package carrying a deliberate type error
// (`undefined: totallyUndefinedSymbol`). Note the exit code was 0 and a
// 1088-byte SARIF was written.
const ssaFailureLog = `[gosec] 2026/08/09 17:10:41 Including rules: default
[gosec] 2026/08/09 17:10:41 Excluding rules: default
[gosec] 2026/08/09 17:10:41 Including analyzers: default
[gosec] 2026/08/09 17:10:41 Excluding analyzers: default
[gosec] 2026/08/09 17:10:41 Import directory: /tmp/broken
[gosec] 2026/08/09 17:10:41 Checking package: main
[gosec] 2026/08/09 17:10:41 Checking file: /tmp/broken/main.go
[gosec] 2026/08/09 17:10:41 Error building the SSA representation of the package main: package main has type errors, skipping SSA analysis, no ssa result
`

// gosec startup with no package ever reached — nothing was analysed, and
// without the "Checking file:" assertion this log is indistinguishable from
// a clean run.
const noFilesLog = `[gosec] 2026/08/09 17:10:41 Including rules: default
[gosec] 2026/08/09 17:10:41 Excluding rules: default
[gosec] 2026/08/09 17:10:41 Including analyzers: default
[gosec] 2026/08/09 17:10:41 Excluding analyzers: default
`

// The SARIF gosec really wrote for the broken package: valid, complete,
// and empty. `if-no-files-found: error` accepts it.
const emptySARIF = `{
	"runs": [{"results": [], "tool": {"driver": {"name": "gosec"}}}],
	"version": "2.1.0"
}`

const fullSARIF = `{
	"runs": [{"results": [{"ruleId": "G306", "level": "error"}], "tool": {"driver": {"name": "gosec"}}}],
	"version": "2.1.0"
}`

func TestGosecAnalysisGuard(t *testing.T) {
	script, err := filepath.Abs(filepath.Join(".github", "scripts", "check-gosec-analysis.sh"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		log   string
		sarif string
		// omitSARIF models the report file never being created, as opposed
		// to being created empty.
		omitSARIF bool
		allow     bool
		why       string
	}{
		{
			name: "clean tree", log: healthyLog, sarif: emptySARIF, allow: true,
			why: "a real analysis that found nothing must pass — otherwise fixing every gosec finding would break CI",
		},
		{
			name: "genuine findings", log: healthyLog, sarif: fullSARIF, allow: true,
			why: "gosec findings are advisory by design and must never fail the job",
		},
		{
			name: "failed analysis, empty report", log: ssaFailureLog, sarif: emptySARIF, allow: false,
			why: "the exact hole: exit 0, valid SARIF, results [], nothing analysed",
		},
		{
			name: "failed analysis, findings anyway", log: ssaFailureLog, sarif: fullSARIF, allow: false,
			why: "AST rules still fire when SSA fails; a non-empty SARIF must not redeem a broken analysis",
		},
		{
			name: "analysed zero files", log: noFilesLog, sarif: emptySARIF, allow: false,
			why: "gosec started and walked nothing; no 'Checking file:' line was ever logged",
		},
		{
			name: "no log at all", log: "", sarif: fullSARIF, allow: false,
			why: "an empty log means gosec never ran, whatever the report says",
		},
		{
			name: "no report at all", log: healthyLog, omitSARIF: true, allow: false,
			why: "a successful analysis that wrote no report is still an audit with no artifact",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/"+map[bool]string{true: "allow", false: "refuse"}[tc.allow], func(t *testing.T) {
			dir := t.TempDir()
			logPath := filepath.Join(dir, "gosec.log")
			if err := os.WriteFile(logPath, []byte(tc.log), 0o600); err != nil {
				t.Fatal(err)
			}
			sarifPath := filepath.Join(dir, "gosec.sarif")
			if !tc.omitSARIF {
				if err := os.WriteFile(sarifPath, []byte(tc.sarif), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			cmd := exec.CommandContext(t.Context(), "bash", script, logPath, sarifPath)
			out, err := cmd.CombinedOutput()
			if tc.allow && err != nil {
				t.Fatalf("%s: guard failed a sound run: %v\n%s", tc.why, err, out)
			}
			if !tc.allow && err == nil {
				t.Fatalf("%s: guard passed it\n%s", tc.why, out)
			}
		})
	}
}

// TestCheckGosecAnalysisScriptIsExecutable pins the file mode, not just the
// content: ci.yml invokes the script by path, which needs the owner-execute
// bit, while TestGosecAnalysisGuard runs it as `bash script` and would stay
// green without it. Same trap as check-tag-branch.sh's.
func TestCheckGosecAnalysisScriptIsExecutable(t *testing.T) {
	path := filepath.Join(".github", "scripts", "check-gosec-analysis.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&0o100 == 0 {
		t.Errorf("%s is not owner-executable (mode %s); ci.yml invokes it by path, not through bash", path, info.Mode())
	}
}
