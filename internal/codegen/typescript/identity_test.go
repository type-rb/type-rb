package typescript

import (
	"testing"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/types"
)

func TestTypeNameUsesLocalCanonicalDeclarationIdentity(t *testing.T) {
	generator := &generator{modulePath: "main", typeAliases: map[string]string{}, typeMappings: map[string]string{}}
	box := types.Type{
		Kind: types.Named, Name: "Box", Args: []types.Type{types.FromName("String")},
		Declaration: identity.Declaration{Module: "main", Name: "Services::Box", Kind: identity.Record},
	}
	value := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{box}}
	if got := generator.tsType(value); got != "Array<Services.Box<string>>" {
		t.Fatalf("TypeScript semantic type = %q, want Array<Services.Box<string>>", got)
	}
}

func TestExternalDeclarationIdentityKeepsAuthoredImportAlias(t *testing.T) {
	generator := &generator{
		modulePath: "main", typeAliases: map[string]string{"Box": "models"}, typeMappings: map[string]string{},
	}
	box := types.FromName("Box")
	box.Declaration = identity.Declaration{Module: "models/box", Name: "Box", Kind: identity.Record}
	if got := generator.tsType(box); got != "models.Box" {
		t.Fatalf("external TypeScript type = %q, want models.Box", got)
	}
}
