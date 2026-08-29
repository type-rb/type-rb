package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/declarationproviderhost"
	"github.com/type-rb/type-rb/internal/packageextension"
)

func TestRubyFixedDeclarationProviderTypesMixinAndKeepsRuntimeLoader(t *testing.T) {
	providerPath := writePagyDeclarationProvider(t)
	packageSource := SourceUnit{
		Filename: "/project/packages/pagy/src/index.trb", ModulePath: "github.com/acme/pagy/index", Package: "pagy",
		ExternalPackage: true, DeclarationProvider: true,
		Source: []byte("activate trb/platform/ruby/native\n\nrequire \"pagy\"\n"),
	}
	application := SourceUnit{
		Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import github.com/acme/pagy
activate trb/platform/ruby/native

class PageExample
	include Pagy::Method

	def first_page(): String
		page_result := pagy(:offset, ["first", "second"], limit: 1)
		pagination := page_result[0]
		records := page_result[1]
		puts(pagination.page)
		return records[0]
	end
end

def main()
	puts(PageExample.new().first_page())
	return
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{application, packageSource}, Options{
		Mode: "ruby", RubyLoader: "require_relative", SourceRoot: "/project/src", ProjectRoot: "/project",
		DeclarationProviders: []declarationproviderhost.Source{{
			Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: providerPath,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	packageArtifact := artifactForModule(artifacts, "github.com/acme/pagy/index")
	if packageArtifact == nil || !strings.Contains(string(packageArtifact.Output), `require "pagy"`) {
		t.Fatalf("provider root did not preserve its native loader: %#v", packageArtifact)
	}
	main := artifactForModule(artifacts, "main")
	if main == nil {
		t.Fatal("main artifact is missing")
	}
	output := string(main.Output)
	for _, want := range []string{
		`require_relative "./github.com/acme/pagy/index"`,
		"include Pagy::Method",
		`page_result = pagy(:offset, ["first", "second"], limit: 1)`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated Ruby is missing %q:\n%s", want, output)
		}
	}
	variables := map[string]string{}
	collectIRVariables(main.IR.Statements, variables)
	for name, want := range map[string]string{
		"page_result": "Tuple<Pagy::Offset, Array<String>>",
		"pagination":  "Pagy::Offset",
		"records":     "Array<String>",
	} {
		if variables[name] != want {
			t.Fatalf("%s type=%q, want %q; all=%v", name, variables[name], want, variables)
		}
	}
}

func TestDeclarationProviderRejectsDuplicateNamedArguments(t *testing.T) {
	providerPath := writePagyDeclarationProvider(t)
	packageSource := SourceUnit{
		Filename: "/project/packages/pagy/src/index.trb", ModulePath: "github.com/acme/pagy/index", Package: "pagy",
		ExternalPackage: true, DeclarationProvider: true,
		Source: []byte("activate trb/platform/ruby/native\n\nrequire \"pagy\"\n"),
	}
	application := SourceUnit{
		Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import github.com/acme/pagy
activate trb/platform/ruby/native
class PageExample
	include Pagy::Method
	def page()
		pagy(:offset, ["first"], limit: 1, limit: 2)
		return
	end
end
`),
	}
	_, err := CompileProject([]SourceUnit{application, packageSource}, Options{
		Mode: "ruby", RubyLoader: "require_relative", SourceRoot: "/project/src", ProjectRoot: "/project",
		DeclarationProviders: []declarationproviderhost.Source{{
			Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: providerPath,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "pagy() receives argument limit more than once") {
		t.Fatalf("expected duplicate named declaration argument diagnostic, got %v", err)
	}
}

func writePagyDeclarationProvider(t *testing.T) string {
	t.Helper()
	typeT := packageextension.Type{Kind: "named", Name: "T"}
	stringType := packageextension.Type{Kind: "string", Name: "String"}
	integerType := packageextension.Type{Kind: "int", Name: "Integer"}
	arrayT := packageextension.Type{Kind: "array", Name: "Array", Arguments: []packageextension.Type{typeT}}
	tuple := packageextension.Type{Kind: "named", Name: "Tuple", Arguments: []packageextension.Type{{Kind: "named", Name: "Pagy::Offset"}, arrayT}}
	catalog := packageextension.DeclarationCatalog{
		ProtocolVersion: packageextension.DeclarationProtocolVersion,
		Provider:        "github.com/acme/pagy",
		Types: []packageextension.DeclaredType{{
			Name: "Pagy::Offset",
			InstanceMembers: []packageextension.DeclaredMember{{
				Name: "page", Kind: "property", Return: integerType,
			}},
		}},
		Modules: []packageextension.DeclaredModule{{
			Name: "Pagy::Method",
			InstanceMembers: []packageextension.DeclaredMember{{
				Name: "pagy", Kind: "method", TypeParameters: []string{"T"}, Return: tuple,
				Parameters: []packageextension.DeclaredParameter{
					{Name: "paginator", Type: stringType, LiteralValues: []string{"offset"}},
					{Name: "collection", Type: arrayT},
					{Name: "limit", Type: integerType, Keyword: true, Optional: true},
				},
			}},
		}},
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "declarations.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
