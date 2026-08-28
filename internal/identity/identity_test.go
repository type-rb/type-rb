package identity

import "testing"

func TestDeclarationKeySeparatesModuleKindAndQualifiedName(t *testing.T) {
	items := []Declaration{
		{Module: "alpha", Name: "Services::Box", Kind: Record},
		{Module: "beta", Name: "Services::Box", Kind: Record},
		{Module: "alpha", Name: "Services::Box", Kind: Class},
		{Module: "alpha", Name: "Other::Box", Kind: Record},
	}
	seen := map[string]bool{}
	for _, item := range items {
		if key := item.Key(); key == "" || seen[key] {
			t.Fatalf("declaration key is not unique for %#v: %q", item, key)
		} else {
			seen[key] = true
		}
	}
}

func TestDeclarationLeafAndTypeKind(t *testing.T) {
	declaration := Declaration{Module: "main", Name: "Outer::Inner::Box", Kind: Record}
	if got := declaration.LeafName(); got != "Box" {
		t.Fatalf("leaf name = %q, want Box", got)
	}
	if !declaration.Kind.IsType() || Module.IsType() || Function.IsType() {
		t.Fatal("declaration kind type classification is incorrect")
	}
}
