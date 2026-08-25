package resolver

import (
	"fmt"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/types"
)

func TestCatalogTypeAliasResolvesAliasesFromNewCatalog(t *testing.T) {
	program := &ast.Program{Statements: []ast.Statement{
		&ast.TypeAliasStatement{Name: "UserID", Target: ast.TypeRef{Name: "Integer"}},
	}}
	catalog, diagnostics := NewCatalog([]Module{{
		Path: "domain/user", Filename: "domain/user.trb", Program: program,
	}})
	if len(diagnostics) != 0 {
		t.Fatalf("NewCatalog diagnostics=%#v", diagnostics)
	}

	resolved, ok := (Result{Catalog: catalog}).CatalogTypeAlias("UserID")
	if !ok || resolved.Name != "UserID" || resolved.AliasTarget.Name != "Integer" {
		t.Fatalf("CatalogTypeAlias(UserID)=(%#v, %t), want the catalog alias", resolved, ok)
	}
}

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

func TestContractTypeFollowsSelectedImportedValueContract(t *testing.T) {
	imported := &Import{
		Kind:    ProjectImport,
		Path:    "app/mapper",
		Symbols: []string{"load_payload"},
		Exports: map[string]Export{
			"load_payload": {Name: "load_payload", Kind: FunctionExport, Type: types.FromName("Payload")},
			"unselected":   {Name: "unselected", Kind: FunctionExport, Type: types.FromName("OtherPayload")},
		},
	}
	node := &ast.ImportStatement{}
	result := Result{
		Imports: map[*ast.ImportStatement]*Import{node: imported},
		Catalog: &Catalog{Modules: map[string]*Module{
			"contracts/payload": {
				Path: "contracts/payload",
				Exports: map[string]Export{
					"Payload": {
						Name: "Payload", Kind: RecordExport, Type: types.FromName("Payload"),
						Fields: []RecordField{{Name: "metadata", Type: types.FromName("Metadata")}},
					},
					"Metadata": {Name: "Metadata", Kind: RecordExport, Type: types.FromName("Metadata")},
				},
			},
			"contracts/other": {
				Path: "contracts/other",
				Exports: map[string]Export{
					"OtherPayload": {Name: "OtherPayload", Kind: RecordExport, Type: types.FromName("OtherPayload")},
				},
			},
		}},
	}

	for _, name := range []string{"Payload", "Metadata"} {
		resolved, ok := result.ContractType(name)
		if !ok || resolved.Import == nil || resolved.Import.RuntimePath() != "contracts/payload" || resolved.Export == nil || resolved.Export.Name != name {
			t.Fatalf("ContractType(%s)=(%#v, %t), want the selected contract type", name, resolved, ok)
		}
	}
	if _, ok := result.ContractType("OtherPayload"); ok {
		t.Fatal("ContractType resolved a type referenced only by an unselected export")
	}
}

func TestContractTypeDoesNotReplaceDirectOpaqueType(t *testing.T) {
	imported := &Import{
		Kind:    NativeImport,
		Path:    "native/http",
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
			"project/response": {
				Path: "project/response",
				Exports: map[string]Export{
					"Response": {Name: "Response", Kind: RecordExport, Type: types.FromName("Response")},
				},
			},
		}},
	}

	if resolved, ok := result.ContractType("Response"); ok {
		t.Fatalf("ContractType replaced a directly owned opaque type with %#v", resolved)
	}
	resolved, ok := result.InferredType("Response")
	if !ok || resolved.Import != imported || resolved.Export == nil || resolved.Export.Kind != ClassExport {
		t.Fatalf("InferredType(Response)=(%#v, %t), want the directly owned opaque type", resolved, ok)
	}
}

func BenchmarkCatalogTypeAlias(b *testing.B) {
	const modules = 512
	units := make([]Module, 0, modules)
	for index := 0; index < modules; index++ {
		name := fmt.Sprintf("Alias%03d", index)
		units = append(units, Module{
			Path:     fmt.Sprintf("module_%03d", index),
			Filename: fmt.Sprintf("module_%03d.trb", index),
			Program: &ast.Program{Statements: []ast.Statement{
				&ast.TypeAliasStatement{Name: name, Target: ast.TypeRef{Name: "Integer"}},
			}},
		})
	}
	catalog, diagnostics := NewCatalog(units)
	if len(diagnostics) != 0 {
		b.Fatalf("NewCatalog diagnostics=%#v", diagnostics)
	}
	result := Result{Catalog: catalog}
	b.ResetTimer()
	for b.Loop() {
		if _, ok := result.CatalogTypeAlias("Alias511"); !ok {
			b.Fatal("CatalogTypeAlias did not resolve Alias511")
		}
	}
}
