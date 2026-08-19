package resolver

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/types"
)

func TestContractTypeAliasFollowsSelectedImportedValueContract(t *testing.T) {
	alias := Export{
		Name:           "AppResult",
		Kind:           TypeAliasExport,
		TypeParameters: []string{"T"},
		AliasTarget: types.Type{
			Kind: types.Named,
			Name: "Result",
			Args: []types.Type{types.FromName("T"), types.FromName("AppError")},
		},
	}
	imported := &Import{
		Kind:    ProjectImport,
		Symbols: []string{"load_value"},
		Exports: map[string]Export{
			"load_value": {Name: "load_value", Kind: FunctionExport, Type: types.Type{Kind: types.Named, Name: "AppResult", Args: []types.Type{types.FromName("Integer")}}},
			"unselected": {Name: "unselected", Kind: FunctionExport, Type: types.FromName("OtherAlias")},
		},
	}
	node := &ast.ImportStatement{}
	result := Result{
		Imports: map[*ast.ImportStatement]*Import{node: imported},
		Catalog: &Catalog{Modules: map[string]*Module{
			"domain/result": {Exports: map[string]Export{
				"AppResult":  alias,
				"OtherAlias": {Name: "OtherAlias", Kind: TypeAliasExport, AliasTarget: types.FromName("String")},
			}},
		}},
	}

	resolved, ok := result.ContractTypeAlias("AppResult")
	if !ok || resolved.Name != "AppResult" {
		t.Fatalf("ContractTypeAlias(AppResult)=(%#v, %t), want the selected contract alias", resolved, ok)
	}
	if _, ok := result.ContractTypeAlias("OtherAlias"); ok {
		t.Fatal("ContractTypeAlias resolved an alias referenced only by an unselected export")
	}
}

func TestContractTypeAliasDoesNotReplaceDirectOpaqueType(t *testing.T) {
	imported := &Import{
		Kind:    NativeImport,
		Symbols: []string{"load_response"},
		Exports: map[string]Export{
			"load_response": {Name: "load_response", Kind: FunctionExport, Type: types.FromName("Response")},
			"Response":      {Name: "Response", Kind: ClassExport, Type: types.FromName("Response")},
		},
	}
	node := &ast.ImportStatement{}
	result := Result{
		Imports: map[*ast.ImportStatement]*Import{node: imported},
		Catalog: &Catalog{Modules: map[string]*Module{
			"project/response": {Exports: map[string]Export{
				"Response": {Name: "Response", Kind: TypeAliasExport, AliasTarget: types.FromName("String")},
			}},
		}},
	}

	if resolved, ok := result.ContractTypeAlias("Response"); ok {
		t.Fatalf("ContractTypeAlias replaced a directly owned opaque type with %#v", resolved)
	}
}
