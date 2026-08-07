package main

import (
	"errors"
	"fmt"
	"testing"
)

// fakeExitCoder mimics the ExitCode() method that *exec.ExitError exposes
// (promoted from its embedded *os.ProcessState). os.ProcessState has no
// public constructor, so a real *exec.ExitError can only be obtained by
// actually running a process to nonzero exit - which tests here must not do.
// exitCode is written against the structural ExitCode() int contract rather
// than the concrete *exec.ExitError type specifically so this fake can stand
// in for it.
type fakeExitCoder struct{ code int }

func (e fakeExitCoder) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e fakeExitCoder) ExitCode() int { return e.code }

func TestExitCodeNilIsZero(t *testing.T) {
	if got := exitCode(nil); got != 0 {
		t.Errorf("exitCode(nil) = %d, want 0", got)
	}
}

func TestExitCodeGenericErrorFallsBackToOne(t *testing.T) {
	if got := exitCode(errors.New("boom")); got != 1 {
		t.Errorf("exitCode(generic error) = %d, want 1", got)
	}
}

func TestExitCodeExtractsFromWrappedExitCoder(t *testing.T) {
	err := fmt.Errorf("run claude: %w", fakeExitCoder{code: 3})
	if got := exitCode(err); got != 3 {
		t.Errorf("exitCode = %d, want 3 (the wrapped child's exit code)", got)
	}
}

func TestExitCodeZeroCoderIsPreserved(t *testing.T) {
	// Defensive: a real caller only reaches exitCode with a non-nil err, but
	// if an ExitCode() of 0 ever did arrive wrapped in a non-nil error, it
	// must not be silently promoted to the generic fallback of 1.
	err := fmt.Errorf("run claude: %w", fakeExitCoder{code: 0})
	if got := exitCode(err); got != 0 {
		t.Errorf("exitCode = %d, want 0", got)
	}
}
