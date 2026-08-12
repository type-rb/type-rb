package nativepackage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyProviderFilesCorrectsIndexedExport(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "native-types.json")
	writeProviderFixture(t, path, Provider{
		FormatVersion: FormatVersion,
		Modules: map[string]Module{
			"@acme/ui": {
				Exports: map[string]Export{
					"Button": {Kind: "component", Type: Type{Kind: "named", Name: "ReactNode"}, Parameters: []Type{{Kind: "named", Name: "ButtonProps"}}, Required: 1},
				},
				Records: map[string]Export{
					"ButtonProps": {Kind: "record", Type: Type{Kind: "named", Name: "ButtonProps"}, Fields: []Field{{Name: "label", Type: Type{Kind: "string", Name: "String"}}}},
				},
			},
		},
	})
	catalog := &Catalog{
		FormatVersion: FormatVersion,
		Dependencies:  map[string]string{"@acme/ui": "1.0.0"},
		Modules: map[string]Module{
			"@acme/ui": {Exports: map[string]Export{}, Unsupported: map[string]string{
				"Button":      "uses a conditional type",
				"ButtonProps": "uses a mapped type",
			}},
		},
	}
	if err := ApplyProviderFiles(catalog, []ProviderSource{{Package: "github.com/acme/ui-types", Path: path, Dependencies: map[string]string{"@acme/ui": "1.0.0"}}}); err != nil {
		t.Fatal(err)
	}
	module := catalog.Modules["@acme/ui"]
	if module.Exports["Button"].Kind != "component" || module.Records["ButtonProps"].Fields[0].Name != "label" {
		t.Fatalf("provider declarations were not applied: %#v", module)
	}
	if _, exists := module.Unsupported["Button"]; exists {
		t.Fatalf("provider did not replace the inferred unsupported export: %#v", module.Unsupported)
	}
	if _, exists := module.Unsupported["ButtonProps"]; exists {
		t.Fatalf("provider did not replace the inferred unsupported record: %#v", module.Unsupported)
	}
}

func TestApplyProviderFilesAcceptsGenericNativeContracts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "native-types.json")
	aliasTarget := Type{Kind: "union", Name: "Union", Args: []Type{
		{Kind: "named", Name: "PendingResult", Args: []Type{{Kind: "named", Name: "TData"}, {Kind: "named", Name: "TError"}}},
		{Kind: "named", Name: "SuccessResult", Args: []Type{{Kind: "named", Name: "TData"}, {Kind: "named", Name: "TError"}}},
	}}
	writeProviderFixture(t, path, Provider{
		FormatVersion: FormatVersion,
		Modules: map[string]Module{
			"query-library": {
				Exports: map[string]Export{
					"Client": {Kind: "class", Type: Type{Kind: "named", Name: "Client"}},
					"QueryResult": {
						Kind: "type_alias", Type: Type{Kind: "named", Name: "QueryResult", Args: []Type{{Kind: "named", Name: "TData"}, {Kind: "named", Name: "TError"}}},
						TypeParameters: []string{"TData", "TError"}, AliasTarget: &aliasTarget,
					},
					"useQuery": {
						Kind: "function", TypeParameters: []string{"TData", "TError"},
						Type:       Type{Kind: "named", Name: "QueryResult", Args: []Type{{Kind: "named", Name: "TData"}, {Kind: "named", Name: "TError"}}},
						Parameters: []Type{{Kind: "named", Name: "QueryOptions", Args: []Type{{Kind: "named", Name: "TData"}, {Kind: "named", Name: "TError"}}}}, Required: 1,
					},
				},
				Records: map[string]Export{
					"QueryOptions": {
						Kind: "record", Type: Type{Kind: "named", Name: "QueryOptions"}, TypeParameters: []string{"TData", "TError"},
						Fields: []Field{{Name: "queryFn", Type: Type{Kind: "function", Name: "Function", Args: []Type{{Kind: "named", Name: "TData"}}}}},
					},
					"PendingResult": {Kind: "record", Type: Type{Kind: "named", Name: "PendingResult"}, TypeParameters: []string{"TData", "TError"}, Fields: []Field{{Name: "status", Type: Type{Kind: "string_literal", Name: `"pending"`}}}},
					"SuccessResult": {Kind: "record", Type: Type{Kind: "named", Name: "SuccessResult"}, TypeParameters: []string{"TData", "TError"}, Fields: []Field{{Name: "status", Type: Type{Kind: "string_literal", Name: `"success"`}}, {Name: "data", Type: Type{Kind: "named", Name: "TData"}}}},
				},
			},
		},
	})
	catalog := Empty(map[string]string{"query-library": "1.0.0"})
	if err := ApplyProviderFiles(catalog, []ProviderSource{{Package: "github.com/acme/query-types", Path: path, Dependencies: map[string]string{"query-library": "1.0.0"}}}); err != nil {
		t.Fatal(err)
	}
	module := catalog.Modules["query-library"]
	if module.Exports["useQuery"].TypeParameters[0] != "TData" || module.Exports["QueryResult"].AliasTarget == nil || module.Records["QueryOptions"].TypeParameters[1] != "TError" {
		t.Fatalf("generic provider contracts were not retained: %#v", module)
	}
}

func TestApplyProviderFilesRejectsUnownedModule(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "native-types.json")
	writeProviderFixture(t, path, Provider{
		FormatVersion: FormatVersion,
		Modules: map[string]Module{
			"other-package": {Exports: map[string]Export{"run": {Kind: "function", Type: Type{Kind: "void", Name: "Void"}}}},
		},
	})
	catalog := Empty(map[string]string{"@acme/ui": "1.0.0"})
	err := ApplyProviderFiles(catalog, []ProviderSource{{Package: "github.com/acme/ui-types", Path: path, Dependencies: map[string]string{"@acme/ui": "1.0.0"}}})
	if err == nil || !strings.Contains(err.Error(), "without a matching native dependency") {
		t.Fatalf("expected unowned module diagnostic, got %v", err)
	}
}

func TestApplyProviderFilesRejectsConflictingProviders(t *testing.T) {
	root := t.TempDir()
	sources := []ProviderSource{}
	for _, name := range []string{"first", "second"} {
		path := filepath.Join(root, name+".json")
		writeProviderFixture(t, path, Provider{
			FormatVersion: FormatVersion,
			Modules: map[string]Module{
				"ui": {Exports: map[string]Export{"Button": {Kind: "component", Type: Type{Kind: "named", Name: "ReactNode"}}}},
			},
		})
		sources = append(sources, ProviderSource{Package: name, Path: path, Dependencies: map[string]string{"ui": "1.0.0"}})
	}
	err := ApplyProviderFiles(Empty(map[string]string{"ui": "1.0.0"}), sources)
	if err == nil || !strings.Contains(err.Error(), "both declare export Button") {
		t.Fatalf("expected provider conflict diagnostic, got %v", err)
	}
}

func TestLoadWithProvidersMarksChangedProviderStale(t *testing.T) {
	root := t.TempDir()
	providerPath := filepath.Join(root, "provider.json")
	provider := Provider{
		FormatVersion: FormatVersion,
		Modules: map[string]Module{
			"ui": {Exports: map[string]Export{"Button": {Kind: "component", Type: Type{Kind: "named", Name: "ReactNode"}}}},
		},
	}
	writeProviderFixture(t, providerPath, provider)
	sources := []ProviderSource{{Package: "github.com/acme/ui-types", Path: providerPath, Dependencies: map[string]string{"ui": "1.0.0"}}}
	catalog := Empty(map[string]string{"ui": "1.0.0"})
	if err := ApplyProviderFiles(catalog, sources); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, catalog); err != nil {
		t.Fatal(err)
	}
	provider.Modules["ui"] = Module{Exports: map[string]Export{"Spinner": {Kind: "component", Type: Type{Kind: "named", Name: "ReactNode"}}}}
	writeProviderFixture(t, providerPath, provider)
	loaded, err := LoadWithProviders(root, map[string]string{"ui": "1.0.0"}, sources)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.UnavailableReason, "provider types are stale") || len(loaded.Modules) != 0 {
		t.Fatalf("changed provider did not invalidate the index: %#v", loaded)
	}
}

func writeProviderFixture(t *testing.T, path string, provider Provider) {
	t.Helper()
	data, err := json.MarshalIndent(provider, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
