package resolver

import (
	"testing"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/types"
)

func TestNewtypeContractsResolveInDefiningImportScope(t *testing.T) {
	var modules []Module
	for _, fixture := range []struct{ path, source string }{
		{"example.com/names/index", "newtype Name = String\n"},
		{"other/index", "newtype Name = Integer\n"},
		{"api/index", `import { Name as Label } from names
record Envelope
	value: Label
end
enum Choice
	Value(value: Label)
end
newtype Wrapped = Label
def echo(value: Label): Array<Label>
	return [value]
end
def identity<Label>(value: Label): Label
	return value
end
`},
	} {
		program, diagnostics := parser.Parse([]byte(fixture.source))
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		modules = append(modules, Module{Path: fixture.path, Filename: fixture.path + ".trb", Program: program, PackageAliases: map[string]string{"names": "example.com/names"}})
	}
	catalog, diagnostics := NewCatalog(modules)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	exports := catalog.Modules["api/index"].Exports
	want := identity.Declaration{Module: "example.com/names/index", Name: "Name", Kind: identity.Newtype}
	for _, typ := range []types.Type{
		exports["Envelope"].Fields[0].Type,
		exports["Choice"].EnumVariants[0].Fields[0].Type,
		exports["Wrapped"].NewtypeTarget,
		exports["echo"].Parameters[0].Type,
		exports["echo"].Type.Args[0],
	} {
		if typ.Declaration != want || typ.Name != "Name" {
			t.Errorf("contract type = %#v, want %#v", typ, want)
		}
	}
	for _, typ := range []types.Type{exports["identity"].Type, exports["identity"].Parameters[0].Type} {
		if typ.Name != "Label" || !typ.Declaration.Empty() {
			t.Errorf("generic parameter was replaced by a nominal import: %#v", typ)
		}
	}
	resolved := Result{Catalog: catalog}
	if _, visible := resolved.ImportedType("Name"); visible {
		t.Fatal("canonical contract identity leaked a source-visible import")
	}
	if binding, found := resolved.ImportedTypeIdentity(want); !found || binding.DeclarationIdentity() != want {
		t.Errorf("exact transitive identity = %#v, found=%v", binding, found)
	}
}
