package typescript

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

func TestNewtypeAnnotationDoesNotDuplicateNamespaceValueImport(t *testing.T) {
	for _, typeFirst := range []bool{false, true} {
		annotation := &ir.Import{
			Path: "trb/std/path/index", Standard: true, Runtime: true, Namespace: true, Alias: "paths",
			QualifiedRoot: "Path", Symbols: []string{"Path"}, SymbolKinds: map[string]string{"Path": "newtype"},
		}
		runtime := *annotation
		runtime.RuntimeRequired = true
		runtime.QualifiedRoot = ""
		statements := []ir.Statement{&runtime, annotation}
		if typeFirst {
			statements = []ir.Statement{annotation, &runtime}
		}
		generated, err := GenerateProject([]*ir.Program{{ModulePath: "main", Mode: "typescript", Statements: statements}})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(generated[0], "* as paths from") != 1 || strings.Contains(generated[0], "import type * as paths") {
			t.Fatalf("namespace import duplicated (type first: %v):\n%s", typeFirst, generated[0])
		}
	}
}

func TestFileHandleTypeOverridesNominalImportName(t *testing.T) {
	file := stdlib.FileResourceType()
	generator := &generator{
		modulePath: "main", declarationNames: map[identity.Declaration]string{file.Declaration: "Handle"},
	}
	const handle = "{ readonly fd: number; readonly path: string }"
	if got := generator.tsType(file); got != handle {
		t.Fatalf("scoped File type = %q, want %q", got, handle)
	}
	if got := generator.tsTypeWithIdentity(file, &typescriptTypeIdentity{name: "File"}); got != handle {
		t.Fatalf("scoped File expression type = %q, want %q", got, handle)
	}
	userFile := file
	userFile.Declaration.Module = "main"
	if got := generator.tsType(userFile); got != "File" {
		t.Fatalf("unrelated File type = %q, want File", got)
	}
}

func TestTypeNameUsesLocalCanonicalDeclarationIdentity(t *testing.T) {
	generator := &generator{modulePath: "main", typeAliases: map[string]string{}, typeMappings: map[string]string{}}
	box := types.Type{
		Kind: types.Named, Name: "Box", Args: []types.Type{types.FromName("String")},
		Declaration: identity.Declaration{Module: "main", Name: "Services::Box", Kind: identity.Record},
	}
	value := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{box}}
	if got := generator.tsType(value); got != "Array<ServicesBox<string>>" {
		t.Fatalf("TypeScript semantic type = %q, want Array<ServicesBox<string>>", got)
	}
}

func TestExternalDeclarationIdentityKeepsAuthoredImportAlias(t *testing.T) {
	generator := &generator{
		modulePath: "main", typeAliases: map[string]string{"Box": "models"}, typeMappings: map[string]string{},
	}
	box := types.FromName("Box")
	box.Declaration = identity.Declaration{Module: "models/box", Name: "Box", Kind: identity.Record}
	if got := generator.tsType(box); got != "models.Box" {
		t.Fatalf("external TypeScript type = %q, want models.Box", got)
	}
}
