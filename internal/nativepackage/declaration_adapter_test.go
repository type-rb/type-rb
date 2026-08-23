package nativepackage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/declarationadapterhost"
	"github.com/type-rb/type-rb/internal/packageextension"
)

func TestApplyDeclarationAdapterFilesCorrectsIndexedExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "declarations.json")
	writeDeclarationAdapterFixture(t, path, packageextension.DeclarationAdapterCatalog{
		ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
		Modules: map[string]packageextension.DeclarationAdapterModule{
			"@acme/ui": {
				Exports: map[string]packageextension.DeclarationAdapterExport{
					"Button": {Kind: "component", Type: adapterType("named", "ReactNode")},
				},
				Records: map[string]packageextension.DeclarationAdapterExport{
					"ButtonProps": {Kind: "record", Type: adapterType("named", "ButtonProps"), Fields: []packageextension.DeclarationAdapterField{{Name: "label", Type: adapterType("string", "String")}}},
				},
			},
		},
	})
	catalog := &Catalog{
		FormatVersion: FormatVersion,
		Dependencies:  map[string]string{"@acme/ui": "1.0.0"},
		Modules: map[string]Module{
			"@acme/ui": {Exports: map[string]Export{}, Unsupported: map[string]string{
				"Button": "uses a conditional type", "ButtonProps": "uses a mapped type",
			}},
		},
	}
	if err := ApplyDeclarationAdapterFiles(catalog, []declarationadapterhost.Source{{
		Package: "github.com/acme/ui-types", Mode: "typescript", Path: path, Dependencies: map[string]string{"@acme/ui": "1.0.0"},
	}}); err != nil {
		t.Fatal(err)
	}
	module := catalog.Modules["@acme/ui"]
	if module.Exports["Button"].Kind != "component" || module.Records["ButtonProps"].Fields[0].Name != "label" {
		t.Fatalf("adapter declarations were not applied: %#v", module)
	}
	if _, exists := module.Unsupported["Button"]; exists {
		t.Fatalf("adapter did not replace the unsupported export: %#v", module.Unsupported)
	}
	if _, exists := module.Unsupported["ButtonProps"]; exists {
		t.Fatalf("adapter did not replace the unsupported record: %#v", module.Unsupported)
	}
}

func TestApplyDeclarationAdapterFilesPreservesGenericResultBridge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "declarations.json")
	tData := adapterType("named", "TData")
	tError := adapterType("named", "TError")
	queryFunction := packageextension.DeclarationAdapterType{
		Kind: "function", Name: "Function", Arguments: []packageextension.DeclarationAdapterType{tData},
		ResultBridge: &packageextension.DeclarationAdapterResultBridge{Kind: "result_to_promise_rejection", Error: tError},
	}
	writeDeclarationAdapterFixture(t, path, packageextension.DeclarationAdapterCatalog{
		ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
		Modules: map[string]packageextension.DeclarationAdapterModule{
			"query-library": {
				Exports: map[string]packageextension.DeclarationAdapterExport{
					"useQuery": {Kind: "function", Type: tData, Parameters: []packageextension.DeclarationAdapterType{{Kind: "named", Name: "QueryOptions", Arguments: []packageextension.DeclarationAdapterType{tData, tError}}}, Required: 1, TypeParameters: []string{"TData", "TError"}},
				},
				Records: map[string]packageextension.DeclarationAdapterExport{
					"QueryOptions": {Kind: "record", Type: adapterType("named", "QueryOptions"), TypeParameters: []string{"TData", "TError"}, Fields: []packageextension.DeclarationAdapterField{{Name: "queryFn", Type: queryFunction}}},
				},
			},
		},
	})
	catalog := Empty(map[string]string{"query-library": "1.0.0"})
	if err := ApplyDeclarationAdapterFiles(catalog, []declarationadapterhost.Source{{
		Package: "github.com/acme/query-types", Mode: "typescript", Path: path, Dependencies: map[string]string{"query-library": "1.0.0"},
	}}); err != nil {
		t.Fatal(err)
	}
	module := catalog.Modules["query-library"]
	if module.Exports["useQuery"].TypeParameters[1] != "TError" || module.Records["QueryOptions"].Fields[0].Type.ResultBridge == nil {
		t.Fatalf("generic Result bridge was not retained: %#v", module)
	}
}

func TestApplyDeclarationAdapterFilesRejectsAdapterSpecificBridgeKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "declarations.json")
	writeDeclarationAdapterFixture(t, path, packageextension.DeclarationAdapterCatalog{
		ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
		Modules: map[string]packageextension.DeclarationAdapterModule{
			"query-library": {Records: map[string]packageextension.DeclarationAdapterExport{
				"Options": {Kind: "record", Type: adapterType("named", "Options"), Fields: []packageextension.DeclarationAdapterField{{Name: "queryFn", Type: packageextension.DeclarationAdapterType{
					Kind: "function", Name: "Function", Arguments: []packageextension.DeclarationAdapterType{adapterType("string", "String")},
					ResultBridge: &packageextension.DeclarationAdapterResultBridge{Kind: "ruby_exception", Error: adapterType("string", "String")},
				}}}},
			}},
		},
	})
	err := ApplyDeclarationAdapterFiles(Empty(map[string]string{"query-library": "1.0.0"}), []declarationadapterhost.Source{{
		Package: "provider", Mode: "typescript", Path: path, Dependencies: map[string]string{"query-library": "1.0.0"},
	}})
	if err == nil || !strings.Contains(err.Error(), "unsupported TypeScript resultBridge kind") {
		t.Fatalf("expected adapter-specific bridge diagnostic, got %v", err)
	}
}

func TestApplyDeclarationAdapterFilesRejectsUnownedAndConflictingDeclarations(t *testing.T) {
	root := t.TempDir()
	write := func(name, module string) declarationadapterhost.Source {
		path := filepath.Join(root, name+".json")
		writeDeclarationAdapterFixture(t, path, packageextension.DeclarationAdapterCatalog{
			ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
			Modules: map[string]packageextension.DeclarationAdapterModule{
				module: {Exports: map[string]packageextension.DeclarationAdapterExport{"Button": {Kind: "component", Type: adapterType("named", "ReactNode")}}},
			},
		})
		return declarationadapterhost.Source{Package: name, Mode: "typescript", Path: path, Dependencies: map[string]string{"ui": "1.0.0"}}
	}
	if err := ApplyDeclarationAdapterFiles(Empty(map[string]string{"ui": "1.0.0"}), []declarationadapterhost.Source{write("unowned", "other")}); err == nil || !strings.Contains(err.Error(), "without a matching TypeScript native dependency") {
		t.Fatalf("expected ownership diagnostic, got %v", err)
	}
	err := ApplyDeclarationAdapterFiles(Empty(map[string]string{"ui": "1.0.0"}), []declarationadapterhost.Source{write("first", "ui"), write("second", "ui")})
	if err == nil || !strings.Contains(err.Error(), "both declare export Button") {
		t.Fatalf("expected conflict diagnostic, got %v", err)
	}
	recordPath := filepath.Join(root, "record.json")
	writeDeclarationAdapterFixture(t, recordPath, packageextension.DeclarationAdapterCatalog{
		ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
		Modules: map[string]packageextension.DeclarationAdapterModule{
			"ui": {Records: map[string]packageextension.DeclarationAdapterExport{
				"Button": {Kind: "record", Type: adapterType("named", "Button")},
			}},
		},
	})
	err = ApplyDeclarationAdapterFiles(Empty(map[string]string{"ui": "1.0.0"}), []declarationadapterhost.Source{
		write("first", "ui"),
		{Package: "record", Mode: "typescript", Path: recordPath, Dependencies: map[string]string{"ui": "1.0.0"}},
	})
	if err == nil || !strings.Contains(err.Error(), "once as an export and once as a supporting record") {
		t.Fatalf("expected cross-category conflict diagnostic, got %v", err)
	}
}

func TestLoadWithDeclarationAdaptersMarksChangedAdapterStale(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "declarations.json")
	catalogSource := packageextension.DeclarationAdapterCatalog{
		ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
		Modules: map[string]packageextension.DeclarationAdapterModule{
			"ui": {Exports: map[string]packageextension.DeclarationAdapterExport{"Button": {Kind: "component", Type: adapterType("named", "ReactNode")}}},
		},
	}
	writeDeclarationAdapterFixture(t, path, catalogSource)
	sources := []declarationadapterhost.Source{{Package: "github.com/acme/ui-types", Mode: "typescript", Path: path, Dependencies: map[string]string{"ui": "1.0.0"}}}
	catalog := Empty(map[string]string{"ui": "1.0.0"})
	if err := ApplyDeclarationAdapterFiles(catalog, sources); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, catalog); err != nil {
		t.Fatal(err)
	}
	catalogSource.Modules["ui"] = packageextension.DeclarationAdapterModule{Exports: map[string]packageextension.DeclarationAdapterExport{"Spinner": {Kind: "component", Type: adapterType("named", "ReactNode")}}}
	writeDeclarationAdapterFixture(t, path, catalogSource)
	loaded, err := LoadWithDeclarationAdapters(root, map[string]string{"ui": "1.0.0"}, sources)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.UnavailableReason, "declaration adapters are stale") || len(loaded.Modules) != 0 {
		t.Fatalf("changed adapter did not invalidate the index: %#v", loaded)
	}
}

func adapterType(kind, name string) packageextension.DeclarationAdapterType {
	return packageextension.DeclarationAdapterType{Kind: kind, Name: name}
}

func writeDeclarationAdapterFixture(t *testing.T, path string, catalog packageextension.DeclarationAdapterCatalog) {
	t.Helper()
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
