package packageextensionhost

import (
	"reflect"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/types"
)

func TestDeclarationCatalogRoundTripPreservesCompilerSemantics(t *testing.T) {
	original := declaration.NewCatalog()
	product := declaration.NewType("Product", "Model")
	product.SourceModule = "models/product"
	product.TypeParameters = []string{"T"}
	product.InstanceMembers["names"] = declaration.Member{
		Name: "names", Kind: declaration.Property, Provider: "test/provider",
		Return: types.Type{
			Kind: types.Array, Name: "Array",
			Args: []types.Type{{Kind: types.String, Name: "String"}},
		},
	}
	product.ClassMembers["all"] = declaration.Member{
		Name: "all", Kind: declaration.Method, Class: true, Provider: "test/provider",
		Return: types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{{Kind: types.Named, Name: "Product"}}},
	}
	original.Types[product.Name] = product

	wire, err := ExportDeclarationCatalog("test/provider", original)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportDeclarationCatalog(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported, original) {
		t.Fatalf("declaration catalog changed during protocol round trip:\noriginal=%#v\nimported=%#v", original, imported)
	}
}

func TestDeclarationCatalogRejectsMemberFromAnotherProvider(t *testing.T) {
	catalog := declaration.NewCatalog()
	value := declaration.NewType("Value", "")
	value.InstanceMembers["read"] = declaration.Member{
		Name: "read", Kind: declaration.Method, Provider: "other/provider",
		Return: types.FromName("String"),
	}
	catalog.Types[value.Name] = value

	_, err := ExportDeclarationCatalog("test/provider", catalog)
	if err == nil || !strings.Contains(err.Error(), "belongs to provider other/provider") {
		t.Fatalf("unexpected provider ownership error: %v", err)
	}
}
