package resolver

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func TestBareImportBindsTheExactMatchingDeclaration(t *testing.T) {
	program, diagnostics := parser.Parse([]byte("import services/secure_random\n"))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	catalog := &Catalog{Modules: map[string]*Module{
		"services/secure_random": {
			Path: "services/secure_random",
			Exports: map[string]Export{
				"SecureRandom": {Name: "SecureRandom", Kind: ModuleExport, Type: types.FromName("SecureRandom")},
				"Token":        {Name: "Token", Kind: ClassExport, Type: types.FromName("Token")},
			},
		},
	}}
	result, resolvedDiagnostics := Resolve(program, Options{Mode: "ruby", Catalog: catalog})
	if len(resolvedDiagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", resolvedDiagnostics)
	}
	if _, ok := result.Symbols["SecureRandom"]; !ok || len(result.Symbols) != 2 { // includes puts prelude
		t.Fatalf("source symbols = %#v", result.Symbols)
	}
	if _, leaked := result.Symbols["Token"]; leaked {
		t.Fatal("bare import leaked an unrelated declaration")
	}
}

func TestMatchesDeclarationRootUsesTheResolverKeyRule(t *testing.T) {
	tests := []struct {
		path string
		name string
		want bool
	}{
		{path: "trb/std/math", name: "Math", want: true},
		{path: "trb/std/json", name: "JSON", want: true},
		{path: "trb/std/secure_random", name: "SecureRandom", want: true},
		{path: "shared/ui/DataTable/index", name: "DataTable", want: true},
		{path: "trb/std/secure_random", name: "SECURE_RANDOM", want: false},
	}
	for _, test := range tests {
		if got := MatchesDeclarationRoot(test.path, test.name); got != test.want {
			t.Errorf("MatchesDeclarationRoot(%q, %q) = %v, want %v", test.path, test.name, got, test.want)
		}
	}
}

func TestEveryPublicStandardPackageRootResolvesBare(t *testing.T) {
	for _, definition := range stdlib.PublicPortablePackages("go") {
		if definition.Root == "" {
			continue
		}
		t.Run(definition.Path, func(t *testing.T) {
			program, diagnostics := parser.Parse([]byte("import " + definition.Path + "\n"))
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			result, resolvedDiagnostics := Resolve(program, Options{Mode: "go"})
			if len(resolvedDiagnostics) != 0 {
				t.Fatalf("resolve diagnostics: %#v", resolvedDiagnostics)
			}
			binding, ok := result.Symbols[definition.Root]
			if !ok || binding.Import == nil || binding.Import.Path != definition.Path {
				t.Fatalf("root binding %s = %#v", definition.Root, binding)
			}
		})
	}
}

func TestNamedImportAliasesBindOnlyTheLocalAlias(t *testing.T) {
	program, diagnostics := parser.Parse([]byte("import { Response as WebResponse } from services/http\n"))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	catalog := &Catalog{Modules: map[string]*Module{
		"services/http": {Path: "services/http", Exports: map[string]Export{
			"Response": {Name: "Response", Kind: ClassExport, Type: types.FromName("Response")},
		}},
	}}
	result, resolvedDiagnostics := Resolve(program, Options{Mode: "ruby", Catalog: catalog})
	if len(resolvedDiagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", resolvedDiagnostics)
	}
	if binding, ok := result.Symbols["WebResponse"]; !ok || binding.Name != "Response" {
		t.Fatalf("aliased binding = %#v", binding)
	}
	if _, leaked := result.Symbols["Response"]; leaked {
		t.Fatal("exact exported name leaked beside its alias")
	}
}

func TestBareStandardRootRejectsImportingItsMemberByName(t *testing.T) {
	program, diagnostics := parser.Parse([]byte("import { sqrt } from trb/std/math\n"))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	_, resolvedDiagnostics := Resolve(program, Options{Mode: "ruby"})
	if len(resolvedDiagnostics) != 1 || !strings.Contains(resolvedDiagnostics[0].Message, "use Math.sqrt") {
		t.Fatalf("resolve diagnostics = %#v", resolvedDiagnostics)
	}
}

func TestActivationEnablesCapabilityWithoutBinding(t *testing.T) {
	program, diagnostics := parser.Parse([]byte("activate trb/platform/ruby/native\n"))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	result, resolvedDiagnostics := Resolve(program, Options{Mode: "ruby"})
	if len(resolvedDiagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", resolvedDiagnostics)
	}
	if !result.NativeSyntax || len(result.Activations) != 1 || len(result.Symbols) != 1 { // puts prelude only
		t.Fatalf("activation result = %#v", result)
	}
}

func TestGeneratedImportsUseASeparateBindingScope(t *testing.T) {
	source := "import { Response as AuthoredResponse } from services/http\nimport { Response } from services/http\n"
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	catalog := &Catalog{Modules: map[string]*Module{
		"services/http": {Path: "services/http", Exports: map[string]Export{
			"Response": {Name: "Response", Kind: ClassExport, Type: types.FromName("Response")},
		}},
	}}
	start := strings.LastIndex(source, "import { Response }")
	result, resolvedDiagnostics := Resolve(program, Options{Mode: "ruby", Catalog: catalog, CompilerGeneratedStart: start})
	if len(resolvedDiagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", resolvedDiagnostics)
	}
	if binding, ok := result.Symbols["AuthoredResponse"]; !ok || binding.Import.CompilerGenerated {
		t.Fatalf("authored binding = %#v", binding)
	}
	if binding, ok := result.GeneratedSymbols["Response"]; !ok || !binding.Import.CompilerGenerated {
		t.Fatalf("generated binding = %#v", binding)
	}
}
