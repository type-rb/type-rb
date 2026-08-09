package orm

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/types"
)

func TestSQLiteIntrospectionAndModelDeclarations(t *testing.T) {
	root, options := sqliteFixture(t)
	schema, err := LoadSchema(root, options)
	if err != nil {
		t.Fatal(err)
	}
	products, ok := schema.Table("products")
	if !ok {
		t.Fatal("products table was not introspected")
	}
	if len(products.Columns) != 5 {
		t.Fatalf("expected five columns, got %#v", products.Columns)
	}
	assertColumn(t, products.Columns[0], "id", types.Int, false, true)
	assertColumn(t, products.Columns[1], "name", types.String, false, false)
	assertColumn(t, products.Columns[2], "price", types.Float, true, false)
	assertColumn(t, products.Columns[3], "active", types.Bool, false, false)
	assertColumn(t, products.Columns[4], "payload", types.Bytes, true, false)

	program := parseModel(t)
	catalog, err := Declarations([]*ast.Program{program}, root, options)
	if err != nil {
		t.Fatal(err)
	}
	product, exists := catalog.Type("Product")
	if !exists {
		t.Fatal("Product declaration was not generated")
	}
	if product.InstanceMembers["price"].Return.String() != "Float?" {
		t.Fatalf("unexpected price type: %s", product.InstanceMembers["price"].Return)
	}
	where := product.ClassMembers["where"]
	if where.Intrinsic != "trb.orm.where" || len(where.Parameters) != 5 || !where.Parameters[0].Keyword || !where.Parameters[0].Optional {
		t.Fatalf("unexpected where declaration: %#v", where)
	}
	if len(where.Alternatives) < 5 || where.Alternatives[0].Parameters[0].StringValues[0] != "id" {
		t.Fatalf("comparison where signatures are missing: %#v", where.Alternatives)
	}
	query, exists := catalog.Type("ProductQuery")
	if !exists || query.InstanceMembers["all"].Return.String() != "Array<Product>" {
		t.Fatalf("unexpected query declaration: %#v", query)
	}
}

func TestManifestAugmentsModelIRWithoutOwningCompilerIR(t *testing.T) {
	root, options := sqliteFixture(t)
	program := parseModel(t)
	manifest, err := Analyze([]*ast.Program{program}, root, options)
	if err != nil {
		t.Fatal(err)
	}
	lowered := &ir.Program{ModulePath: "src/main", Statements: []ir.Statement{&ir.Class{Name: "Product"}}}
	manifest.Augment(lowered)
	product := lowered.Statements[0].(*ir.Class)
	if len(product.Body) != 6 {
		t.Fatalf("expected five fields and where(), got %#v", product.Body)
	}
	field, ok := product.Body[0].(*ir.Field)
	if !ok || field.Name != "@id" || field.Type.Kind != types.Int {
		t.Fatalf("unexpected first field: %#v", product.Body[0])
	}
	where, ok := product.Body[5].(*ir.Method)
	if !ok || !where.External || !where.Class || where.ReturnType.Name != "ProductQuery" {
		t.Fatalf("unexpected where method: %#v", product.Body[5])
	}
	query, ok := lowered.Statements[1].(*ir.Class)
	if !ok || !query.External || query.Name != "ProductQuery" {
		t.Fatalf("unexpected query class: %#v", lowered.Statements[1])
	}
}

func sqliteFixture(t *testing.T) (string, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		price REAL,
		active BOOLEAN NOT NULL,
		payload BLOB
	)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(Config{Adapter: "sqlite", Database: filepath.Base(databasePath)})
	if err != nil {
		t.Fatal(err)
	}
	return root, map[string][]byte{PackageName: encoded}
}

func parseModel(t *testing.T) *ast.Program {
	t.Helper()
	program, diagnostics := parser.Parse([]byte("import { Model } from trb/orm\n\nclass Product < Model\nend\n"))
	if len(diagnostics) > 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	program.ModulePath = "src/main"
	return program
}

func assertColumn(t *testing.T, column Column, name string, kind types.Kind, nullable, primary bool) {
	t.Helper()
	if column.Name != name || column.Type.Kind != kind || column.Nullable != nullable || column.PrimaryKey != primary {
		t.Fatalf("unexpected column: %#v", column)
	}
}
