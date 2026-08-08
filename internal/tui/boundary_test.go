package tui

import (
	"go/build"
	"testing"
)

// The layering claim: cli imports tui, so tui must not import cli.
//
// The cli edge is also enforced by the compiler today — the reverse import
// would be a cycle — so this test's job is narrower than it looks. It keeps
// the rule enforced if the cli -> tui edge ever disappears, and it covers
// cobra and pflag, which the compiler would accept happily. A tui that knew
// about cobra would have the command layer's concerns leaking into the
// screens, which is the actual thing worth preventing.
func TestTUIDependsOnNeitherCLINorCobra(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}

	banned := []string{
		"github.com/teggen/openrouter-launch/internal/cli",
		"github.com/spf13/cobra",
		"github.com/spf13/pflag",
	}

	lists := map[string][]string{
		"imports":       pkg.Imports,
		"test imports":  pkg.TestImports,
		"xtest imports": pkg.XTestImports,
	}
	for label, list := range lists {
		for _, imp := range list {
			for _, bad := range banned {
				if imp == bad {
					t.Errorf("internal/tui %s include %q", label, bad)
				}
			}
		}
	}
}
