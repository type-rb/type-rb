package packageextension

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeclarationAdapterCatalogRoundTripsAsModeIndependentData(t *testing.T) {
	catalog := DeclarationAdapterCatalog{
		ProtocolVersion: DeclarationAdapterProtocolVersion,
		Modules: map[string]DeclarationAdapterModule{
			"query-library": {
				Exports: map[string]DeclarationAdapterExport{
					"useQuery": {
						Kind: "function", TypeParameters: []string{"TData", "TError"},
						Type:       DeclarationAdapterType{Kind: "named", Name: "QueryResult", Arguments: []DeclarationAdapterType{{Kind: "named", Name: "TData"}, {Kind: "named", Name: "TError"}}},
						Parameters: []DeclarationAdapterType{{Kind: "named", Name: "QueryOptions", Arguments: []DeclarationAdapterType{{Kind: "named", Name: "TData"}, {Kind: "named", Name: "TError"}}}}, Required: 1,
					},
				},
				Records: map[string]DeclarationAdapterExport{
					"QueryOptions": {
						Kind: "record", Type: DeclarationAdapterType{Kind: "named", Name: "QueryOptions"}, TypeParameters: []string{"TData", "TError"},
						Fields: []DeclarationAdapterField{{Name: "queryFn", Type: DeclarationAdapterType{
							Kind: "function", Name: "Function", Arguments: []DeclarationAdapterType{{Kind: "named", Name: "TData"}},
							ResultBridge: &DeclarationAdapterResultBridge{Kind: "result_to_promise_rejection", Error: DeclarationAdapterType{Kind: "named", Name: "TError"}},
						}}},
					},
				},
			},
		},
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "typescript") {
		t.Fatalf("mode leaked into the declaration protocol: %s", data)
	}
	var decoded DeclarationAdapterCatalog
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeclarationAdapterCatalog(decoded); err != nil {
		t.Fatal(err)
	}
}

func TestDeclarationAdapterCatalogRejectsInvalidBoundaryData(t *testing.T) {
	valid := func() DeclarationAdapterCatalog {
		return DeclarationAdapterCatalog{
			ProtocolVersion: DeclarationAdapterProtocolVersion,
			Modules: map[string]DeclarationAdapterModule{
				"library": {Exports: map[string]DeclarationAdapterExport{
					"run": {Kind: "function", Type: DeclarationAdapterType{Kind: "void", Name: "Void"}},
				}},
			},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*DeclarationAdapterCatalog)
		message string
	}{
		{name: "version", mutate: func(catalog *DeclarationAdapterCatalog) { catalog.ProtocolVersion++ }, message: "unsupported declaration adapter protocol version"},
		{name: "modules", mutate: func(catalog *DeclarationAdapterCatalog) { catalog.Modules = nil }, message: "modules are required"},
		{name: "module name", mutate: func(catalog *DeclarationAdapterCatalog) { catalog.Modules[""] = DeclarationAdapterModule{} }, message: "empty module name"},
		{name: "kind", mutate: func(catalog *DeclarationAdapterCatalog) {
			exported := catalog.Modules["library"].Exports["run"]
			exported.Kind = "dynamic"
			catalog.Modules["library"].Exports["run"] = exported
		}, message: "unsupported kind"},
		{name: "bridge shape", mutate: func(catalog *DeclarationAdapterCatalog) {
			exported := catalog.Modules["library"].Exports["run"]
			exported.Type.ResultBridge = &DeclarationAdapterResultBridge{Kind: "native_failure", Error: DeclarationAdapterType{Kind: "string", Name: "String"}}
			catalog.Modules["library"].Exports["run"] = exported
		}, message: "only valid on function types"},
		{name: "export record overlap", mutate: func(catalog *DeclarationAdapterCatalog) {
			module := catalog.Modules["library"]
			module.Records = map[string]DeclarationAdapterExport{"run": {Kind: "record", Type: DeclarationAdapterType{Kind: "named", Name: "Run"}}}
			catalog.Modules["library"] = module
		}, message: "both an export and a supporting record"},
		{name: "duplicate fields", mutate: func(catalog *DeclarationAdapterCatalog) {
			exported := catalog.Modules["library"].Exports["run"]
			exported.Fields = []DeclarationAdapterField{{Name: "value", Type: DeclarationAdapterType{Kind: "string", Name: "String"}}, {Name: "value", Type: DeclarationAdapterType{Kind: "string", Name: "String"}}}
			catalog.Modules["library"].Exports["run"] = exported
		}, message: "duplicate field value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := valid()
			test.mutate(&catalog)
			if err := ValidateDeclarationAdapterCatalog(catalog); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}
