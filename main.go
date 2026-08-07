// Command openrouter-launch starts coding agents against OpenRouter models.
package main

import (
	"errors"
	"os"

	"github.com/teggen/openrouter-launch/internal/cli"
)

func main() {
	os.Exit(exitCode(cli.Execute()))
}

// exitCoder is satisfied by *exec.ExitError (via its embedded
// *os.ProcessState). Matching structurally, instead of asserting the
// concrete *exec.ExitError type, is what makes exitCode unit-testable: Go's
// os.ProcessState has no public constructor, so a real *exec.ExitError can
// only be produced by actually running a process to a nonzero exit, which
// this repo's tests must not do.
type exitCoder interface {
	ExitCode() int
}

// exitCode returns the process exit status for err. On Windows,
// agent.Run (internal/agent/exec_windows.go) waits for the child instead of
// replacing the process, so a nonzero child exit reaches here wrapped in an
// *exec.ExitError; its own exit code is propagated rather than collapsed to
// a generic 1. Any other non-nil error exits 1; nil exits 0.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ec exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 1
}
