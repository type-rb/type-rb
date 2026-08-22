package languageservice

import (
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func TestServiceReusesProjectContextForSessionUpdates(t *testing.T) {
	project := &ir.Program{ModulePath: "models/user", SourcePath: "models/user.trb", Statements: []ir.Statement{
		&ir.Record{Name: "User", Body: []ir.Statement{&ir.RecordField{Name: "name", Type: types.FromName("String")}}},
	}}
	session := &ir.Program{ModulePath: "repl", SourcePath: ".trb-repl.trb", Statements: []ir.Statement{
		&ir.Import{Path: "models/user", Symbols: []string{"User"}},
	}}
	service := New("go")
	service.Update([]*ir.Program{project, session}, session.ModulePath)
	initialCache := service.project
	if initialCache == nil {
		t.Fatal("project context was not cached")
	}

	updatedSession := &ir.Program{ModulePath: session.ModulePath, SourcePath: session.SourcePath, Statements: []ir.Statement{
		&ir.Import{Path: "models/user", Symbols: []string{"User"}},
		&ir.Variable{Name: "current", Type: types.FromName("User")},
	}}
	programs := []*ir.Program{project, updatedSession}
	service.Update(programs, updatedSession.ModulePath)
	if service.project != initialCache {
		t.Fatal("session-only update rebuilt the project context")
	}
	if expected := BuildContext(programs, updatedSession.ModulePath); !reflect.DeepEqual(service.context, expected) {
		t.Fatal("cached project context changed the resulting session context")
	}

	updatedProject := &ir.Program{ModulePath: project.ModulePath, SourcePath: project.SourcePath, Statements: project.Statements}
	service.Update([]*ir.Program{updatedProject, updatedSession}, updatedSession.ModulePath)
	if service.project == initialCache {
		t.Fatal("changed project module did not rebuild the project context")
	}
}
