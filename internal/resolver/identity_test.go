package resolver

import (
	"testing"

	"github.com/type-rb/type-rb/internal/identity"
)

func TestBindingExposesCanonicalDeclarationAndDispatchIdentity(t *testing.T) {
	imported := &Import{Path: "models", ModulePath: "models/index"}
	exported := &Export{Name: "Worker", Kind: ClassExport}
	member := &Member{Name: "values", Kind: FunctionExport, Class: false}
	binding := Binding{Import: imported, Name: member.Name, Export: exported, Member: member}

	wantOwner := identity.Declaration{Module: "models/index", Name: "Worker", Kind: identity.Class}
	if got := binding.DeclarationIdentity(); got != wantOwner {
		t.Fatalf("declaration identity = %#v, want %#v", got, wantOwner)
	}
	wantDispatch := identity.Dispatch{Owner: wantOwner, Name: "values", Class: false}
	if got := binding.DispatchIdentity(); got != wantDispatch {
		t.Fatalf("dispatch identity = %#v, want %#v", got, wantDispatch)
	}
}
