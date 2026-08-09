package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// .goreleaser.yaml names the version symbols by full import path in its
// ldflags. Renaming the package or one of the variables would still build,
// test, vet, lint, and publish cleanly — and every released binary would
// silently report the "dev" placeholders. Nothing else in the tree connects
// the YAML to the Go declarations, so this test is that connection.
const versionImportPath = "github.com/teggen/openrouter-launch/internal/version"

func TestGoreleaserLdflagsMatchVersionSymbols(t *testing.T) {
	cfg, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("reading .goreleaser.yaml: %v", err)
	}
	declared := versionVarsDeclared(t)

	for _, name := range []string{"Version", "Commit", "Date"} {
		if !declared[name] {
			t.Errorf("internal/version declares no var %q, but .goreleaser.yaml injects it", name)
		}
		want := "-X " + versionImportPath + "." + name + "="
		if !strings.Contains(string(cfg), want) {
			t.Errorf(".goreleaser.yaml has no ldflag %q — released binaries would report the dev placeholder", want)
		}
	}
}

// versionVarsDeclared returns the package-level var names in internal/version,
// skipping _test.go files so a variable declared only in a test cannot make
// this assertion pass.
//
// This walks the directory itself rather than calling parser.ParseDir, which
// Go 1.25 deprecated (it ignores build tags when grouping files into
// packages). The replacement keeps the two properties this test depends on —
// non-test .go files only, package-level vars only — and adds no dependency;
// the suggested alternative, golang.org/x/tools/go/packages, would.
func versionVarsDeclared(t *testing.T) map[string]bool {
	t.Helper()
	const dir = "internal/version"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	found := map[string]bool{}
	parsed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		parsed++
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, n := range vs.Names {
					found[n.Name] = true
				}
			}
		}
	}

	// Distinguishes "the package moved or was renamed" from "a variable was
	// renamed" — both break the ldflags, but they are different diagnoses.
	if parsed == 0 {
		t.Fatalf("parsed no non-test .go files in %s — the ldflag target is gone", dir)
	}
	return found
}
