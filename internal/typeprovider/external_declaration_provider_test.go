package typeprovider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declarationproviderhost"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestFixedDeclarationProviderActivatesOnlyThroughRootPackageImport(t *testing.T) {
	path := writeExternalDeclarationProvider(t, "Pagy::Offset")
	source := declarationproviderhost.Source{
		Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: path,
	}
	program := parseProviderProgram(t, "controllers/products", "import acme/pagy\n")
	context := Context{
		PackageAliasesByModule: map[string]map[string]string{program.ModulePath: {"acme/pagy": "github.com/acme/pagy"}},
		DeclarationProviders:   []declarationproviderhost.Source{source},
	}
	catalog, err := Load([]*ast.Program{program}, context)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := catalog.Types["Pagy::Offset"]; !exists {
		t.Fatal("root package import did not activate its fixed declarations")
	}

	unrelated := parseProviderProgram(t, "controllers/unrelated", "import trb/platform/ruby/native\n")
	catalog, err = Load([]*ast.Program{unrelated}, Context{DeclarationProviders: []declarationproviderhost.Source{source}})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := catalog.Types["Pagy::Offset"]; exists {
		t.Fatal("configured but unimported declaration provider leaked into the project")
	}

	submodule := parseProviderProgram(t, "controllers/submodule", "import github.com/acme/pagy/helpers\n")
	catalog, err = Load([]*ast.Program{submodule}, Context{DeclarationProviders: []declarationproviderhost.Source{source}})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := catalog.Types["Pagy::Offset"]; exists {
		t.Fatal("a package submodule import activated root declarations")
	}
}

func TestFixedDeclarationProviderFileParticipatesInIncrementalInputs(t *testing.T) {
	path := writeExternalDeclarationProvider(t, "Pagy::Offset")
	source := declarationproviderhost.Source{Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: path}
	program := parseProviderProgram(t, "main", "import github.com/acme/pagy\n")
	context := Context{DeclarationProviders: []declarationproviderhost.Source{source}}
	previous := CaptureInputs([]*ast.Program{program}, context)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	current := CaptureInputs([]*ast.Program{program}, context)
	if current.CanReuse(previous) {
		t.Fatal("declaration provider edit reused stale declarations")
	}
}

func TestFixedDeclarationProviderRejectsFrameworkDeclarationConflicts(t *testing.T) {
	path := writeExternalDeclarationProvider(t, "ActionController::API")
	program := parseProviderProgram(t, "controller", "import trb/platform/ruby/rails\nimport github.com/acme/pagy\n")
	_, err := Load([]*ast.Program{program}, Context{DeclarationProviders: []declarationproviderhost.Source{{
		Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: path,
	}}})
	if err == nil || !strings.Contains(err.Error(), "conflicts with existing type ActionController::API") {
		t.Fatalf("unexpected conflict error: %v", err)
	}
}

func TestFixedDeclarationProviderRejectsProjectDeclarationConflicts(t *testing.T) {
	path := writeExternalDeclarationProvider(t, "Product")
	program := parseProviderProgram(t, "models/product", "import github.com/acme/pagy\n\nclass Product\nend\n")
	_, err := Load([]*ast.Program{program}, Context{DeclarationProviders: []declarationproviderhost.Source{{
		Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: path,
	}}})
	if err == nil || !strings.Contains(err.Error(), "conflicts with a project declaration") {
		t.Fatalf("unexpected project conflict error: %v", err)
	}
}

func writeExternalDeclarationProvider(t *testing.T, typeName string) string {
	t.Helper()
	catalog := packageextension.DeclarationCatalog{
		ProtocolVersion: packageextension.DeclarationProtocolVersion,
		Provider:        "github.com/acme/pagy",
		Types:           []packageextension.DeclaredType{{Name: typeName}},
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

func parseProviderProgram(t *testing.T, modulePath, source string) *ast.Program {
	t.Helper()
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) > 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	program.ModulePath = modulePath
	return program
}
