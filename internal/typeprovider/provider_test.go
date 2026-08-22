package typeprovider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestInputSnapshotTracksORMProgramsAndSchemaLock(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "schema.lock.json")
	if err := os.WriteFile(lockPath, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}
	context := Context{
		ProjectRoot: root,
		PackageOptions: map[string][]byte{
			"trb/orm": []byte(`{"schemaLock":"schema.lock.json"}`),
		},
	}
	activation := &ast.Program{Statements: []ast.Statement{
		&ast.ImportStatement{Path: "trb/orm"},
	}}
	model := ormModelProgram("Product")
	firstIrrelevant := &ast.Program{Statements: []ast.Statement{
		&ast.ExpressionStatement{Expression: &ast.Literal{Kind: ast.IntegerLiteral, Raw: "1"}},
	}}
	initial := CaptureInputs([]*ast.Program{activation, model, firstIrrelevant}, context)

	secondIrrelevant := &ast.Program{Statements: []ast.Statement{
		&ast.ExpressionStatement{Expression: &ast.Literal{Kind: ast.IntegerLiteral, Raw: "2"}},
	}}
	unchanged := CaptureInputs([]*ast.Program{activation, model, secondIrrelevant}, context)
	if !unchanged.CanReuse(initial) {
		t.Fatal("irrelevant program edit invalidated ORM provider inputs")
	}

	changedModel := CaptureInputs([]*ast.Program{activation, ormModelProgram("Order"), secondIrrelevant}, context)
	if changedModel.CanReuse(initial) {
		t.Fatal("ORM model edit did not invalidate provider inputs")
	}

	if err := os.WriteFile(lockPath, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	changedLock := CaptureInputs([]*ast.Program{activation, model, secondIrrelevant}, context)
	if changedLock.CanReuse(initial) {
		t.Fatal("ORM schema lock edit did not invalidate provider inputs")
	}
}

func TestInputSnapshotDoesNotReuseORMDatabaseIntrospection(t *testing.T) {
	context := Context{
		ProjectRoot: t.TempDir(),
		PackageOptions: map[string][]byte{
			"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`),
		},
	}
	programs := []*ast.Program{{Statements: []ast.Statement{
		&ast.ImportStatement{Path: "trb/orm"},
	}}}
	first := CaptureInputs(programs, context)
	second := CaptureInputs(programs, context)
	if second.CanReuse(first) {
		t.Fatal("live ORM database inputs were considered reusable")
	}
}

func TestInputSnapshotTracksRailsSchemaCreation(t *testing.T) {
	root := t.TempDir()
	programs := []*ast.Program{{Statements: []ast.Statement{
		&ast.ImportStatement{Path: "trb/platform/ruby/rails"},
	}}}
	missing := CaptureInputs(programs, Context{ProjectRoot: root})
	if !CaptureInputs(programs, Context{ProjectRoot: root}).CanReuse(missing) {
		t.Fatal("missing optional Rails schema was not reusable")
	}
	if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "db", "schema.rb"), []byte("schema"), 0o644); err != nil {
		t.Fatal(err)
	}
	if CaptureInputs(programs, Context{ProjectRoot: root}).CanReuse(missing) {
		t.Fatal("Rails schema creation did not invalidate provider inputs")
	}
}

func TestJobsProviderInputsIgnoreUnrelatedPrograms(t *testing.T) {
	activation := &ast.Program{Statements: []ast.Statement{
		&ast.ImportStatement{Path: "trb/jobs"},
	}}
	job := &ast.Program{Statements: []ast.Statement{
		&ast.ClassStatement{Name: "ImportJob", Superclass: &ast.Identifier{Name: "Job"}},
	}}
	first := CaptureInputs([]*ast.Program{activation, job, &ast.Program{}}, Context{})
	second := CaptureInputs([]*ast.Program{activation, job, &ast.Program{Statements: []ast.Statement{
		&ast.ExpressionStatement{Expression: &ast.Literal{Kind: ast.IntegerLiteral, Raw: "1"}},
	}}}, Context{})
	if !second.CanReuse(first) {
		t.Fatal("unrelated program edit invalidated Jobs provider inputs")
	}
	changed := CaptureInputs([]*ast.Program{activation, &ast.Program{Statements: []ast.Statement{
		&ast.ClassStatement{Name: "ExportJob", Superclass: &ast.Identifier{Name: "Job"}},
	}}, &ast.Program{}}, Context{})
	if changed.CanReuse(first) {
		t.Fatal("Jobs class edit did not invalidate provider inputs")
	}
}

func ormModelProgram(name string) *ast.Program {
	return &ast.Program{Statements: []ast.Statement{
		&ast.ClassStatement{Name: name, Superclass: &ast.Identifier{Name: "Model"}},
	}}
}
