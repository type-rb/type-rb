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
}
