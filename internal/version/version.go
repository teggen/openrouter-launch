// Package version reports the build identity of the openrouter-launch binary.
package version

import "runtime"

// Version, Commit, and Date are overwritten at link time by the release
// build. Keep them in this package, with these exact names: .goreleaser.yaml
// names all three by full import path in its ldflags, so a rename here would
// still build, test, and publish cleanly while every released binary silently
// reported the placeholders below. TestGoreleaserLdflagsMatchVersionSymbols
// (goreleaser_test.go, package main) pins the two together.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String renders the one-line identity cobra prints for --version.
func String() string {
	return Version + " (commit " + Commit + ", built " + Date + ", " + runtime.Version() + ")"
}
