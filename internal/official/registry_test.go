package official

import "testing"

func TestBundledWebPackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/web")
	if !ok {
		t.Fatal("trb/web is not registered")
	}
	if packageDefinition.Version != "0.1.0" {
		t.Fatalf("version = %q", packageDefinition.Version)
	}
	if packageDefinition.Definition.ModulePath != "trb/web/index" {
		t.Fatalf("module = %q", packageDefinition.Definition.ModulePath)
	}
	if packageDefinition.Definition.Source == "" {
		t.Fatal("package source is empty")
	}
	requestJSON := packageDefinition.Definition.Symbols["request_json"]
	if requestJSON.Intrinsic != "trb.web.request_json" || requestJSON.Return.String() != "Result<T, JsonError>" {
		t.Fatalf("unexpected request_json contract: %#v", requestJSON)
	}
	json := packageDefinition.Definition.Symbols["json"]
	if json.Intrinsic != "trb.web.json" || len(json.Parameters) != 2 || !json.Parameters[1].Optional || json.Return.String() != "Response" {
		t.Fatalf("unexpected json contract: %#v", json)
	}
}
