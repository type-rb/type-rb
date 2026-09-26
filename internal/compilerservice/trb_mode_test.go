package compilerservice

import (
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
)

func TestServiceAnalyzesValidTRBEditsWithoutBackendValidation(t *testing.T) {
	unit := compiler.SourceUnit{
		Filename: "models/user.trb", ModulePath: "models/user",
		Source: []byte("record User\n\tname: String\nend\n"),
	}
	service := New([]compiler.SourceUnit{unit}, compiler.Options{Mode: "trb"})
	if initial := service.Analyze(); initial.HasErrors() || initial.Stale {
		t.Fatalf("unexpected initial trb snapshot: %#v", initial)
	}

	unit.Source = []byte("record User\n\tname: String\n\temail: String\nend\n")
	service.SetDocument(unit)
	edited := service.Analyze()
	if edited.HasErrors() || edited.Stale {
		t.Fatalf("a valid trb edit produced errors or a stale snapshot: %#v", edited.Diagnostics)
	}
	context, ok := edited.Context("models/user")
	if !ok || !hasCompletion(context, "User") {
		t.Fatalf("edited trb context does not contain User: %#v", context)
	}
}
