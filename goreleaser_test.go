package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
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
func versionVarsDeclared(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "internal/version",
		func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatalf("parsing internal/version: %v", err)
	}
	found := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
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
	}
	return found
}
