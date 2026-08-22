package typeprovider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/schemalock"
)

func TestORMDeclarationsCrossVersionedExtensionBoundary(t *testing.T) {
	root := t.TempDir()
	lock := schemalock.New("sqlite")
	lock.Tables["categories"] = schemalock.Table{
		Columns: map[string]schemalock.Column{
			"id":   {Type: "Integer", PrimaryKey: true, Generated: true},
			"name": {Type: "String"},
		},
		UniqueConstraints: map[string]schemalock.UniqueConstraint{
			schemalock.ConstraintKey(true, []string{"id"}): {Columns: []string{"id"}, Primary: true},
		},
	}
	foreignKey := schemalock.ForeignKey{Column: "category_id", ReferencedTable: "categories", ReferencedColumn: "id"}
	lock.Tables["products"] = schemalock.Table{
		Columns: map[string]schemalock.Column{
			"id":          {Type: "Integer", PrimaryKey: true, Generated: true},
			"category_id": {Type: "Integer"},
			"name":        {Type: "String"},
			"price":       {Type: "Float", Nullable: true},
		},
		ForeignKeys: map[string]schemalock.ForeignKey{
			schemalock.ForeignKeyKey(foreignKey.Column, foreignKey.ReferencedTable, foreignKey.ReferencedColumn): foreignKey,
		},
		UniqueConstraints: map[string]schemalock.UniqueConstraint{
			schemalock.ConstraintKey(true, []string{"id"}): {Columns: []string{"id"}, Primary: true},
		},
	}
	lockPath := filepath.Join(root, "db", "schema.lock.json")
	if err := lock.Write(lockPath); err != nil {
		t.Fatal(err)
	}
	program, diagnostics := parser.Parse([]byte(`import { Model, belongs_to, has_many } from trb/orm

class Category < Model
	has_many(Product)
end

class Product < Model
	belongs_to(Category)
end
`))
	if len(diagnostics) > 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	program.ModulePath = "models/catalog"
	unrelated, diagnostics := parser.Parse([]byte("class InternalService\n\tsecret(\"not-for-orm\")\nend\n"))
	if len(diagnostics) > 0 {
		t.Fatalf("unrelated parse diagnostics: %#v", diagnostics)
	}
	unrelated.ModulePath = "services/internal"
	programs := []*ast.Program{unrelated, program}
	context := Context{
		ProjectRoot: root,
		PackageOptions: map[string][]byte{
			"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3","schemaLock":"db/schema.lock.json"}`),
		},
	}
	input, err := ormDeclarationInput(programs, context)
	if err != nil {
		t.Fatal(err)
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedInput), "application.sqlite3") {
		t.Fatalf("ORM declaration input exposed its database location: %s", encodedInput)
	}
	var decodedInput ormintegration.DeclarationInput
	if err := json.Unmarshal(encodedInput, &decodedInput); err != nil {
		t.Fatal(err)
	}
	if err := ormintegration.ValidateDeclarationInput(decodedInput); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodedInput, input) {
		t.Fatal("ORM declaration input changed across its JSON boundary")
	}
	if len(decodedInput.Project.Modules) != 1 || decodedInput.Project.Modules[0].ModulePath != program.ModulePath {
		t.Fatalf("ORM input included unrelated project declarations: %#v", decodedInput.Project.Modules)
	}
	provided, err := loadORMDeclarations(programs, context)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(provided)
	if err != nil {
		t.Fatal(err)
	}
	var decoded packageextension.DeclarationCatalog
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := packageextension.ValidateDeclarationCatalog(decoded); err != nil {
		t.Fatal(err)
	}

	product := declaredProtocolType(t, decoded, "Product")
	category := declaredProtocolMember(t, product.InstanceMembers, "category")
	if category.Kind != "property" || category.RuntimeOperation != "trb.orm.association.value.belongs_to" || protocolTypeString(category.Return) != "DbResult<Category?>" {
		t.Fatalf("association property did not cross the protocol: %#v", category)
	}
	pluck := declaredProtocolMember(t, product.ClassMembers, "pluck")
	namePluck, found := declaredProtocolAlternative(pluck, "name")
	if len(pluck.Alternatives) != 4 || !found || protocolTypeString(namePluck.Return) != "DbResult<Array<String>>" {
		t.Fatalf("literal-dependent terminal did not cross the protocol: %#v", pluck)
	}
	transaction := declaredProtocolMember(t, declaredProtocolType(t, decoded, "Database").ClassMembers, "transaction")
	if transaction.Block == nil || !transaction.Block.Structured || protocolTypeString(transaction.Block.Return) != "T" || protocolTypeString(transaction.Block.ResultBoundary) != "DbError" {
		t.Fatalf("transaction Result boundary did not cross the protocol: %#v", transaction)
	}
	findEach := declaredProtocolMember(t, product.ClassMembers, "find_each")
	if findEach.Block == nil || !findEach.Block.Structured || protocolTypeString(findEach.Block.Parameters[0]) != "Product" || protocolTypeString(findEach.Block.ResultBoundary) != "DbError" {
		t.Fatalf("streaming Result boundary did not cross the protocol: %#v", findEach)
	}

	imported, err := packageextensionhost.ImportDeclarationCatalog(decoded)
	if err != nil {
		t.Fatal(err)
	}
	original, err := ormintegration.Declarations(decodedInput)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported, original) {
		t.Fatal("declaration protocol changed the ORM catalog semantics")
	}
	loaded, err := loadORM(programs, context)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported, loaded) {
		t.Fatal("ORM provider did not load through the declaration protocol")
	}
}

func declaredProtocolType(t *testing.T, catalog packageextension.DeclarationCatalog, name string) packageextension.DeclaredType {
	t.Helper()
	for _, declared := range catalog.Types {
		if declared.Name == name {
			return declared
		}
	}
	t.Fatalf("missing declaration type %s", name)
	return packageextension.DeclaredType{}
}

func declaredProtocolMember(t *testing.T, members []packageextension.DeclaredMember, name string) packageextension.DeclaredMember {
	t.Helper()
	for _, member := range members {
		if member.Name == name {
			return member
		}
	}
	t.Fatalf("missing declaration member %s", name)
	return packageextension.DeclaredMember{}
}

func declaredProtocolAlternative(member packageextension.DeclaredMember, literal string) (packageextension.DeclaredSignature, bool) {
	for _, alternative := range member.Alternatives {
		if len(alternative.Parameters) == 0 || len(alternative.Parameters[0].LiteralValues) == 0 {
			continue
		}
		if alternative.Parameters[0].LiteralValues[0] == literal {
			return alternative, true
		}
	}
	return packageextension.DeclaredSignature{}, false
}

func protocolTypeString(typ packageextension.Type) string {
	if typ.Kind == "" {
		return ""
	}
	name := typ.Name
	if name == "" {
		name = typ.Kind
	}
	if len(typ.Arguments) > 0 {
		values := make([]string, len(typ.Arguments))
		for index, argument := range typ.Arguments {
			values[index] = protocolTypeString(argument)
		}
		name += "<" + strings.Join(values, ", ") + ">"
	}
	if typ.Nullable {
		name += "?"
	}
	return name
}

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
