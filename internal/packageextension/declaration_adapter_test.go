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
					"Client": {
						Kind: "class", Type: DeclarationAdapterType{Kind: "named", Name: "Client"},
						InstanceMembers: map[string]DeclarationAdapterExport{
							"run": {Kind: "function", Type: DeclarationAdapterType{Kind: "string", Name: "String"}},
						},
						ClassMembers: map[string]DeclarationAdapterExport{
							"create": {Kind: "function", Type: DeclarationAdapterType{Kind: "named", Name: "Client"}},
						},
					},
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
	if !strings.Contains(string(data), `"instanceMembers"`) || !strings.Contains(string(data), `"classMembers"`) {
		t.Fatalf("class member identity was not encoded: %s", data)
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
			exported.Kind = "record"
			exported.Type = DeclarationAdapterType{Kind: "named", Name: "run"}
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

func TestDeclarationAdapterCatalogRejectsMalformedSemanticTypes(t *testing.T) {
	tests := []struct {
		name    string
		typ     DeclarationAdapterType
		message string
	}{
		{name: "missing name", typ: DeclarationAdapterType{Kind: "string"}, message: "type kind string requires a name"},
		{name: "noncanonical name", typ: DeclarationAdapterType{Kind: "bool", Name: "Bool"}, message: "type kind bool requires name Boolean"},
		{name: "array arity", typ: DeclarationAdapterType{Kind: "array", Name: "Array"}, message: "type kind array requires exactly one argument"},
		{name: "hash arity", typ: DeclarationAdapterType{Kind: "hash", Name: "Hash", Arguments: []DeclarationAdapterType{{Kind: "string", Name: "String"}}}, message: "type kind hash requires exactly two arguments"},
		{name: "range arity", typ: DeclarationAdapterType{Kind: "range", Name: "Range", Arguments: []DeclarationAdapterType{{Kind: "int", Name: "Integer"}, {Kind: "int", Name: "Integer"}}}, message: "type kind range requires exactly one argument"},
		{name: "function return", typ: DeclarationAdapterType{Kind: "function", Name: "Function"}, message: "type kind function requires a return type"},
		{name: "union alternatives", typ: DeclarationAdapterType{Kind: "union", Name: "Union", Arguments: []DeclarationAdapterType{{Kind: "string", Name: "String"}}}, message: "type kind union requires at least two alternatives"},
		{name: "scalar arguments", typ: DeclarationAdapterType{Kind: "int", Name: "Integer", Arguments: []DeclarationAdapterType{{Kind: "string", Name: "String"}}}, message: "type kind int cannot have arguments"},
		{name: "integer literal", typ: DeclarationAdapterType{Kind: "int_literal", Name: "9007199254740992"}, message: "portable Integer literal"},
		{name: "string literal", typ: DeclarationAdapterType{Kind: "string_literal", Name: "plain"}, message: "quoted String literal"},
		{name: "noncanonical string literal", typ: DeclarationAdapterType{Kind: "string_literal", Name: `"\u0061"`}, message: "canonical String literal"},
		{name: "nullable void", typ: DeclarationAdapterType{Kind: "void", Name: "Void", Nullable: true}, message: "type kind void cannot be nullable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := DeclarationAdapterCatalog{
				ProtocolVersion: DeclarationAdapterProtocolVersion,
				Modules: map[string]DeclarationAdapterModule{
					"library": {Exports: map[string]DeclarationAdapterExport{"run": {Kind: "function", Type: test.typ}}},
				},
			}
			if err := ValidateDeclarationAdapterCatalog(catalog); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}

func TestDeclarationAdapterCatalogRejectsKindSpecificExportShapes(t *testing.T) {
	stringType := DeclarationAdapterType{Kind: "string", Name: "String"}
	functionMember := DeclarationAdapterExport{Kind: "function", Type: stringType}
	tests := []struct {
		name     string
		exported DeclarationAdapterExport
		message  string
	}{
		{name: "record parameters", exported: DeclarationAdapterExport{Kind: "record", Type: DeclarationAdapterType{Kind: "named", Name: "Value"}, Parameters: []DeclarationAdapterType{stringType}}, message: "kind record cannot declare call parameters"},
		{name: "function fields", exported: DeclarationAdapterExport{Kind: "function", Type: stringType, Fields: []DeclarationAdapterField{{Name: "value", Type: stringType}}}, message: "fields are only valid for records and classes"},
		{name: "record members", exported: DeclarationAdapterExport{Kind: "record", Type: DeclarationAdapterType{Kind: "named", Name: "Value"}, Members: map[string]DeclarationAdapterExport{"build": functionMember}}, message: "kind record cannot declare members"},
		{name: "legacy class members", exported: DeclarationAdapterExport{Kind: "class", Type: DeclarationAdapterType{Kind: "named", Name: "Value"}, Members: map[string]DeclarationAdapterExport{"build": functionMember}}, message: "kind class uses instanceMembers or classMembers"},
		{name: "instance members on function", exported: DeclarationAdapterExport{Kind: "function", Type: stringType, InstanceMembers: map[string]DeclarationAdapterExport{"run": functionMember}}, message: "instanceMembers and classMembers are only valid for classes"},
		{name: "nested instance members", exported: DeclarationAdapterExport{Kind: "class", Type: DeclarationAdapterType{Kind: "named", Name: "Value"}, InstanceMembers: map[string]DeclarationAdapterExport{"run": {Kind: "function", Type: stringType, InstanceMembers: map[string]DeclarationAdapterExport{"nested": functionMember}}}}, message: "cannot declare nested members"},
		{name: "overlapping instance and class members", exported: DeclarationAdapterExport{Kind: "class", Type: DeclarationAdapterType{Kind: "named", Name: "Value"}, InstanceMembers: map[string]DeclarationAdapterExport{"run": functionMember}, ClassMembers: map[string]DeclarationAdapterExport{"run": functionMember}}, message: "both instance member and class member"},
		{name: "overlapping field and member", exported: DeclarationAdapterExport{Kind: "class", Type: DeclarationAdapterType{Kind: "named", Name: "Value"}, Fields: []DeclarationAdapterField{{Name: "run", Type: stringType}}, InstanceMembers: map[string]DeclarationAdapterExport{"run": functionMember}}, message: "both field and instance member"},
		{name: "component arity", exported: DeclarationAdapterExport{Kind: "component", Type: DeclarationAdapterType{Kind: "named", Name: "ReactNode"}, Parameters: []DeclarationAdapterType{stringType, stringType}}, message: "at most one props parameter"},
		{name: "component variadic", exported: DeclarationAdapterExport{Kind: "component", Type: DeclarationAdapterType{Kind: "named", Name: "ReactNode"}, Parameters: []DeclarationAdapterType{stringType}, Variadic: true}, message: "component cannot be variadic"},
		{name: "component type parameters", exported: DeclarationAdapterExport{Kind: "component", Type: DeclarationAdapterType{Kind: "named", Name: "ReactNode"}, TypeParameters: []string{"T"}}, message: "component cannot declare type parameters"},
		{name: "required variadic", exported: DeclarationAdapterExport{Kind: "function", Type: stringType, Parameters: []DeclarationAdapterType{stringType}, Required: 1, Variadic: true}, message: "variadic parameter cannot be required"},
		{name: "class self type", exported: DeclarationAdapterExport{Kind: "class", Type: DeclarationAdapterType{Kind: "named", Name: "Other"}}, message: "kind class requires a named self type"},
		{name: "type alias self type", exported: DeclarationAdapterExport{Kind: "type_alias", Type: DeclarationAdapterType{Kind: "named", Name: "Other"}, AliasTarget: &stringType}, message: "kind type_alias requires a named self type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := DeclarationAdapterCatalog{
				ProtocolVersion: DeclarationAdapterProtocolVersion,
				Modules: map[string]DeclarationAdapterModule{
					"library": {Exports: map[string]DeclarationAdapterExport{"Value": test.exported}},
				},
			}
			if err := ValidateDeclarationAdapterCatalog(catalog); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}
