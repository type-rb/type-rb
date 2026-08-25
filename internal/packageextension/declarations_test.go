package packageextension

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeclarationCatalogRoundTripsAsVersionedData(t *testing.T) {
	catalog := DeclarationCatalog{
		ProtocolVersion: DeclarationProtocolVersion,
		Provider:        "test.provider",
		ClassBodyDeclarationRules: []DeclaredClassBodyDeclarationRule{{
			Package: "test.provider", Function: "response",
			Owner: DeclaredReference{ModulePath: "endpoints/create", Name: "CreateEndpoint"},
		}},
		Types: []DeclaredType{{
			Name: "Product",
			InstanceMembers: []DeclaredMember{{
				Name: "category", Kind: "property", RuntimeOperation: "test.association",
				Return: Type{Kind: "named", Name: "Result", Arguments: []Type{{Kind: "named", Name: "Category", Nullable: true}, {Kind: "named", Name: "Error"}}},
			}},
			ClassMembers: []DeclaredMember{{
				Name: "pluck", Kind: "method", RuntimeOperation: "test.pluck",
				Parameters: []DeclaredParameter{{Name: "id", Type: Type{Kind: "int", Name: "Integer"}, RepresentationBoundary: true}},
				Return:     Type{Kind: "named", Name: "Result", Arguments: []Type{{Kind: "array", Name: "Array", Arguments: []Type{{Kind: "string", Name: "String"}}}, {Kind: "named", Name: "Error"}}},
				Alternatives: []DeclaredSignature{{
					Parameters: []DeclaredParameter{{Name: "column", Type: Type{Kind: "string", Name: "String"}, LiteralValues: []string{"name"}}},
					Return:     Type{Kind: "named", Name: "Result", Arguments: []Type{{Kind: "array", Name: "Array", Arguments: []Type{{Kind: "string", Name: "String"}}}, {Kind: "named", Name: "Error"}}},
				}},
			}},
		}},
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DeclarationCatalog
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeclarationCatalog(decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Types[0].ClassMembers[0].Alternatives[0].Parameters[0].LiteralValues[0] != "name" {
		t.Fatalf("declaration data changed during JSON round trip: %#v", decoded)
	}
	if !decoded.Types[0].ClassMembers[0].Parameters[0].RepresentationBoundary {
		t.Fatalf("representation boundary changed during JSON round trip: %#v", decoded)
	}
	if len(decoded.ClassBodyDeclarationRules) != 1 || decoded.ClassBodyDeclarationRules[0].Owner.Name != "CreateEndpoint" {
		t.Fatalf("class-body declaration rule changed during JSON round trip: %#v", decoded.ClassBodyDeclarationRules)
	}
}

func TestDeclarationCatalogRejectsInvalidVersionAndDuplicateMembers(t *testing.T) {
	invalidVersion := DeclarationCatalog{ProtocolVersion: DeclarationProtocolVersion + 1, Provider: "test.provider"}
	if err := ValidateDeclarationCatalog(invalidVersion); err == nil || !strings.Contains(err.Error(), "unsupported declaration protocol version") {
		t.Fatalf("unexpected version error: %v", err)
	}
	duplicate := DeclarationCatalog{
		ProtocolVersion: DeclarationProtocolVersion, Provider: "test.provider",
		Types: []DeclaredType{{Name: "Value", InstanceMembers: []DeclaredMember{
			{Name: "read", Kind: "method", Return: Type{Kind: "void", Name: "Void"}},
			{Name: "read", Kind: "method", Return: Type{Kind: "void", Name: "Void"}},
		}}},
	}
	if err := ValidateDeclarationCatalog(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate name") {
		t.Fatalf("unexpected duplicate error: %v", err)
	}
	for name, test := range map[string]struct {
		typ     Type
		message string
	}{
		"source definition": {typ: Type{Kind: "named", Name: "Value", Definition: &Definition{ModulePath: "contracts/value"}}, message: "source definition metadata"},
		"record inspection": {typ: Type{Kind: "named", Name: "Value", Record: &Record{}}, message: "record inspection metadata"},
		"unknown kind":      {typ: Type{Kind: "future_kind", Name: "Value"}, message: "unsupported type kind"},
	} {
		invalidType := DeclarationCatalog{
			ProtocolVersion: DeclarationProtocolVersion,
			Provider:        "test.provider",
			Types: []DeclaredType{{
				Name: "Value",
				InstanceMembers: []DeclaredMember{{
					Name: "read", Kind: "method", Return: test.typ,
				}},
			}},
		}
		if err := ValidateDeclarationCatalog(invalidType); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("unexpected %s error: %v", name, err)
		}
	}
	invalidRule := DeclarationCatalog{
		ProtocolVersion: DeclarationProtocolVersion,
		Provider:        "test.provider",
		FunctionArgumentReferenceRules: []DeclaredFunctionArgumentReferenceRule{{
			Package: "test.provider", Function: "connect", Argument: 0,
		}},
	}
	if err := ValidateDeclarationCatalog(invalidRule); err == nil || !strings.Contains(err.Error(), "invalid owner") {
		t.Fatalf("unexpected reference rule error: %v", err)
	}
	invalidClassBodyRule := DeclarationCatalog{
		ProtocolVersion: DeclarationProtocolVersion,
		Provider:        "test.provider",
		ClassBodyDeclarationRules: []DeclaredClassBodyDeclarationRule{{
			Package: "test.provider", Function: "response",
		}},
	}
	if err := ValidateDeclarationCatalog(invalidClassBodyRule); err == nil || !strings.Contains(err.Error(), "invalid owner") {
		t.Fatalf("unexpected class-body declaration rule error: %v", err)
	}
	foreignRule := DeclarationCatalog{
		ProtocolVersion: DeclarationProtocolVersion,
		Provider:        "test.provider",
		FunctionBlockRules: []DeclaredFunctionBlockRule{{
			Package: "other/provider", Function: "connect", TypeArgument: 0,
		}},
	}
	if err := ValidateDeclarationCatalog(foreignRule); err == nil || !strings.Contains(err.Error(), "does not belong to provider") {
		t.Fatalf("unexpected foreign rule error: %v", err)
	}
}
