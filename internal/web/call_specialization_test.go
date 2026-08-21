package web

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/packageextension"
)

func TestBindSpecializationProtocolRoundTripsAsData(t *testing.T) {
	request := packageextension.SpecializeCallRequest{
		ProtocolVersion: packageextension.ProtocolVersion,
		Provider:        "trb.web.bind",
		CallSite:        packageextension.CallSite{ID: "42", ModulePath: "routes/todos"},
		TypeArguments: []packageextension.Type{{
			Kind: "named", Name: "Input", Definition: &packageextension.Definition{ModulePath: "routes/todos"},
			Record: &packageextension.Record{Fields: []packageextension.Field{
				{Name: "params", Type: packageextension.Type{Kind: "named", Name: "Params", Definition: &packageextension.Definition{ModulePath: "routes/todos"}}},
				{Name: "body", Type: packageextension.Type{Kind: "named", Name: "Payload", Definition: &packageextension.Definition{ModulePath: "contracts/payload", ImportPath: "contracts/payload"}}},
			}},
		}},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded packageextension.SpecializeCallRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	response, err := packageextension.SpecializeCall(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if response.GeneratedSource == nil || response.Replacement == nil {
		t.Fatalf("incomplete response: %#v", response)
	}
	for _, expected := range []string{"def __trb_specialize_bind_42(", ".params<Params>() catch", ".request.json<Payload>() catch", "EndpointInputError::Params", "EndpointInputError::Body", "Input.new(params:"} {
		if !strings.Contains(response.GeneratedSource.Source, expected) {
			t.Fatalf("generated source is missing %q:\n%s", expected, response.GeneratedSource.Source)
		}
	}
	if len(response.RequiredImports) != 3 {
		t.Fatalf("unexpected required imports: %#v", response.RequiredImports)
	}
}

func TestBindSpecializerRejectsUnsupportedInputShape(t *testing.T) {
	response, err := packageextension.SpecializeCall(packageextension.SpecializeCallRequest{
		ProtocolVersion: packageextension.ProtocolVersion,
		Provider:        "trb.web.bind",
		CallSite:        packageextension.CallSite{ID: "7", ModulePath: "main"},
		TypeArguments:   []packageextension.Type{{Kind: "named", Name: "Input", Record: &packageextension.Record{Fields: []packageextension.Field{{Name: "headers", Type: packageextension.Type{Kind: "string", Name: "String"}}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Issues) != 1 || !strings.Contains(response.Issues[0].Message, `unsupported field "headers"`) {
		t.Fatalf("unexpected response: %#v", response)
	}
}
