package orm

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
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
	if len(where.Alternatives) < 5 || where.Alternatives[0].Parameters[0].LiteralValues[0] != "id" {
		t.Fatalf("comparison where signatures are missing: %#v", where.Alternatives)
	}
	if product.ClassMembers["find"].Return.String() != "Product?" {
		t.Fatalf("unexpected find declaration: %#v", product.ClassMembers["find"])
	}
	findEach := product.ClassMembers["find_each"]
	if findEach.Block == nil || len(findEach.Block.Parameters) != 1 || findEach.Block.Parameters[0].String() != "Product" {
		t.Fatalf("unexpected find_each declaration: %#v", findEach)
	}
	query, exists := catalog.Type("ProductQuery")
	if !exists || query.InstanceMembers["all"].Return.String() != "Array<Product>" {
		t.Fatalf("unexpected query declaration: %#v", query)
	}
	findInBatches := query.InstanceMembers["find_in_batches"]
	if findInBatches.Block == nil || findInBatches.Block.Parameters[0].String() != "Array<Product>" {
		t.Fatalf("unexpected find_in_batches declaration: %#v", findInBatches)
	}
	for _, name := range []string{"to_sql", "explain"} {
		if query.InstanceMembers[name].Return.String() != "String" {
			t.Fatalf("unexpected %s declaration: %#v", name, query.InstanceMembers[name])
		}
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
	if len(product.Body) != 9 {
		t.Fatalf("expected five fields and four ORM class methods, got %#v", product.Body)
	}
	field, ok := product.Body[0].(*ir.Field)
	if !ok || field.Name != "@id" || field.Type.Kind != types.Int {
		t.Fatalf("unexpected first field: %#v", product.Body[0])
	}
	where, ok := product.Body[5].(*ir.Method)
	if !ok || !where.External || !where.Class || where.ReturnType.Name != "ProductQuery" {
		t.Fatalf("unexpected where method: %#v", product.Body[5])
	}
	methods := map[string]bool{}
	for _, statement := range product.Body[5:] {
		method, ok := statement.(*ir.Method)
		if ok && method.External && method.Class {
			methods[method.Name] = true
		}
	}
	for _, name := range []string{"where", "find", "find_each", "find_in_batches"} {
		if !methods[name] {
			t.Fatalf("missing generated ORM class method %s: %#v", name, product.Body)
		}
	}
	query, ok := lowered.Statements[1].(*ir.Class)
	if !ok || !query.External || query.Name != "ProductQuery" {
		t.Fatalf("unexpected query class: %#v", lowered.Statements[1])
	}
	queryMethods := map[string]bool{}
	for _, statement := range query.Body {
		method, ok := statement.(*ir.Method)
		if ok {
			queryMethods[method.Name] = true
		}
	}
	for _, name := range []string{"to_sql", "explain"} {
		if !queryMethods[name] {
			t.Fatalf("missing generated ORM query method %s: %#v", name, query.Body)
		}
	}
}

func TestSQLiteAssociationsUseDeclaredForeignKeys(t *testing.T) {
	root, options := sqliteAssociationFixture(t, true)
	program := parseAssociationModels(t)
	schema, err := LoadSchema(root, options)
	if err != nil {
		t.Fatal(err)
	}
	products, ok := schema.Table("products")
	if !ok || len(products.ForeignKeys) != 1 || products.ForeignKeys[0].Column != "category_id" || products.ForeignKeys[0].ReferencedTable != "categories" {
		t.Fatalf("unexpected product foreign keys: %#v", products.ForeignKeys)
	}
	catalog, err := Declarations([]*ast.Program{program}, root, options)
	if err != nil {
		t.Fatal(err)
	}
	product, _ := catalog.Type("Product")
	category, _ := catalog.Type("Category")
	if product.InstanceMembers["category"].Return.String() != "CategoryQuery" {
		t.Fatalf("unexpected belongs_to declaration: %#v", product.InstanceMembers["category"])
	}
	if category.InstanceMembers["products"].Return.String() != "ProductQuery" {
		t.Fatalf("unexpected has_many declaration: %#v", category.InstanceMembers["products"])
	}
	manifest, err := Analyze([]*ast.Program{program}, root, options)
	if err != nil {
		t.Fatal(err)
	}
	productModel, _ := manifest.Model("Product")
	belongsTo, ok := productModel.Association("category")
	if !ok || belongsTo.SourceColumn != "category_id" || belongsTo.TargetColumn != "id" {
		t.Fatalf("unexpected belongs_to association: %#v", belongsTo)
	}
	categoryModel, _ := manifest.Model("Category")
	hasMany, ok := categoryModel.Association("products")
	if !ok || hasMany.SourceColumn != "id" || hasMany.TargetColumn != "category_id" {
		t.Fatalf("unexpected has_many association: %#v", hasMany)
	}
}

func TestSQLiteAssociationRejectsMissingForeignKey(t *testing.T) {
	root, options := sqliteAssociationFixture(t, false)
	_, err := Analyze([]*ast.Program{parseAssociationModels(t)}, root, options)
	if err == nil || !strings.Contains(err.Error(), "requires foreign key products.category_id -> categories.id") {
		t.Fatalf("expected missing foreign key diagnostic, got %v", err)
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

func sqliteAssociationFixture(t *testing.T, foreignKey bool) (string, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	constraint := ""
	if foreignKey {
		constraint = ", FOREIGN KEY (category_id) REFERENCES categories(id)"
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, category_id INTEGER NOT NULL, name TEXT NOT NULL` + constraint + `)`); err != nil {
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

func parseAssociationModels(t *testing.T) *ast.Program {
	t.Helper()
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
	program.ModulePath = "src/models"
	return program
}

func assertColumn(t *testing.T, column Column, name string, kind types.Kind, nullable, primary bool) {
	t.Helper()
	if column.Name != name || column.Type.Kind != kind || column.Nullable != nullable || column.PrimaryKey != primary {
		t.Fatalf("unexpected column: %#v", column)
	}
}
