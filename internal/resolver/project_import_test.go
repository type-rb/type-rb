package resolver

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/parser"
)

func TestProjectImportModuleCandidatesUsePortablePaths(t *testing.T) {
	for _, test := range []struct {
		path string
		want []string
	}{
		{path: "models/user", want: []string{"models/user", "models/user/index"}},
		{path: "./models/user.trb", want: []string{"models/user", "models/user/index"}},
	} {
		got, valid := ProjectImportModuleCandidates(test.path)
		if !valid || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("ProjectImportModuleCandidates(%q)=(%v, %t), want (%v, true)", test.path, got, valid, test.want)
		}
	}

	for _, importPath := range []string{
		".", "..", "../outside", "/outside", `..\outside`, `folder\module`, `C:\outside`, "C:/outside",
	} {
		if candidates, valid := ProjectImportModuleCandidates(importPath); valid {
			t.Fatalf("ProjectImportModuleCandidates(%q)=(%v, true), want invalid", importPath, candidates)
		}
	}
}

func TestCatalogBackedRubyImportFallsBackOnlyToOpaqueRuby(t *testing.T) {
	root := t.TempDir()
	rubyPath := filepath.Join(root, "helper.rb")
	if err := os.WriteFile(rubyPath, []byte("class Helper; end\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	program, parseDiagnostics := parser.Parse([]byte("import { Helper } from helper\n"))
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	result, diagnostics := Resolve(program, Options{
		Mode: "ruby", SourceRoot: root, Filename: filepath.Join(root, "main.trb"),
		Catalog: &Catalog{Modules: map[string]*Module{}},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", diagnostics)
	}
	if len(result.Imports) != 1 {
		t.Fatalf("imports=%#v", result.Imports)
	}
	for _, imported := range result.Imports {
		if imported.Filename != rubyPath || imported.Exports["Helper"].Kind != ClassExport {
			t.Fatalf("opaque Ruby import=%#v", imported)
		}
	}
}
