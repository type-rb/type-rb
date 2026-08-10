package orm

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
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
	if !products.Columns[0].Generated || !products.Columns[0].HasDefault || !products.Columns[3].HasDefault {
		t.Fatalf("SQLite generated/default metadata is missing: %#v", products.Columns)
	}
	assertUniqueConstraints(t, products.UniqueConstraints, []string{"id"}, []string{"name", "active"})

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
	if product.ClassMembers["find"].Return.String() != "DbResult<Product?>" {
		t.Fatalf("unexpected find declaration: %#v", product.ClassMembers["find"])
	}
	create := product.ClassMembers["create"]
	if create.Return.String() != "DbResult<Product>" || !create.Parameters[0].Optional || create.Parameters[1].Optional || !create.Parameters[2].Optional || !create.Parameters[3].Optional {
		t.Fatalf("unexpected create declaration: %#v", create)
	}
	build := product.ClassMembers["build"]
	if build.Return.String() != "ProductDraft" || !build.Parameters[0].Optional || build.Parameters[1].Optional || !build.Parameters[2].Optional || !build.Parameters[3].Optional {
		t.Fatalf("unexpected build declaration: %#v", build)
	}
	draft, exists := catalog.Type("ProductDraft")
	if !exists || draft.InstanceMembers["save"].Return.String() != "DbResult<Product>" {
		t.Fatalf("unexpected draft declaration: %#v", draft)
	}
	insertAll := product.ClassMembers["insert_all"]
	if insertAll.Return.String() != "DbResult<Integer>" || len(insertAll.Parameters) != 1 || insertAll.Parameters[0].Type.String() != "Array<ProductDraft>" {
		t.Fatalf("unexpected insert_all declaration: %#v", insertAll)
	}
	insertIfAbsent := product.ClassMembers["insert_if_absent"]
	if insertIfAbsent.Return.String() != "DbResult<Boolean>" || len(insertIfAbsent.Parameters) != 2 || len(insertIfAbsent.Parameters[1].LiteralArrays) != 2 {
		t.Fatalf("unexpected insert_if_absent declaration: %#v", insertIfAbsent)
	}
	upsertAll := product.ClassMembers["upsert_all"]
	if upsertAll.Return.String() != "DbResult<Integer>" || len(upsertAll.Parameters) != 3 || upsertAll.Parameters[0].Type.String() != "Array<ProductDraft>" {
		t.Fatalf("unexpected upsert_all declaration: %#v", upsertAll)
	}
	upsert := draft.InstanceMembers["upsert"]
	if upsert.Return.String() != "DbResult<Product>" || len(upsert.Parameters) != 2 || len(upsert.Parameters[0].LiteralArrays) != 2 {
		t.Fatalf("unexpected upsert declaration: %#v", upsert)
	}
	if got := strings.Join(upsert.Parameters[1].LiteralArrayElements, ","); got != "name,price,active,payload" {
		t.Fatalf("unexpected upsert update columns: %q", got)
	}
	update := product.InstanceMembers["update"]
	if update.Return.String() != "DbResult<Product>" || len(update.Parameters) != 4 || !update.Parameters[0].Optional {
		t.Fatalf("unexpected update declaration: %#v", update)
	}
	with := product.InstanceMembers["with"]
	if with.Return.String() != "ProductChanges" || len(with.Parameters) != 4 || !with.Parameters[0].Optional {
		t.Fatalf("unexpected with declaration: %#v", with)
	}
	changes, exists := catalog.Type("ProductChanges")
	if !exists || changes.InstanceMembers["save"].Return.String() != "DbResult<Product>" {
		t.Fatalf("unexpected changes declaration: %#v", changes)
	}
	if product.InstanceMembers["delete"].Return.String() != "DbResult<Boolean>" {
		t.Fatalf("unexpected delete declaration: %#v", product.InstanceMembers["delete"])
	}
	findEach := product.ClassMembers["find_each"]
	if findEach.Return.String() != "DbResult<Integer>" || findEach.Block == nil || !findEach.Block.Structured || len(findEach.Block.Parameters) != 1 || findEach.Block.Parameters[0].String() != "Product" {
		t.Fatalf("unexpected find_each declaration: %#v", findEach)
	}
	query, exists := catalog.Type("ProductQuery")
	if !exists || query.InstanceMembers["all"].Return.String() != "DbResult<Array<Product>>" {
		t.Fatalf("unexpected query declaration: %#v", query)
	}
	findInBatches := query.InstanceMembers["find_in_batches"]
	if findInBatches.Return.String() != "DbResult<Integer>" || findInBatches.Block == nil || !findInBatches.Block.Structured || findInBatches.Block.Parameters[0].String() != "Array<Product>" {
		t.Fatalf("unexpected find_in_batches declaration: %#v", findInBatches)
	}
	if query.InstanceMembers["to_sql"].Return.String() != "String" {
		t.Fatalf("unexpected to_sql declaration: %#v", query.InstanceMembers["to_sql"])
	}
	for name, expected := range map[string]string{
		"first": "DbResult<Product?>", "count": "DbResult<Integer>", "explain": "DbResult<String>",
	} {
		if query.InstanceMembers[name].Return.String() != expected {
			t.Fatalf("unexpected %s declaration: %#v", name, query.InstanceMembers[name])
		}
	}
}

func TestSQLiteAdapterDefinesPortableRuntimeSyntax(t *testing.T) {
	adapter, err := AdapterFor("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.DriverName != "sqlite" || adapter.GoDriverImport != "modernc.org/sqlite" {
		t.Fatalf("unexpected SQLite driver definition: %#v", adapter)
	}
	if got := adapter.QuoteIdentifier(`product"names`); got != `"product""names"` {
		t.Fatalf("QuoteIdentifier() = %q", got)
	}
	if got := adapter.Placeholder(3); got != "?" {
		t.Fatalf("Placeholder() = %q", got)
	}
}

func TestPostgreSQLAdapterDefinesPortableRuntimeSyntax(t *testing.T) {
	adapter, err := AdapterFor("postgresql")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.DriverName != "pgx" || adapter.GoDriverImport != "github.com/jackc/pgx/v5/stdlib" {
		t.Fatalf("unexpected PostgreSQL driver definition: %#v", adapter)
	}
	if got := adapter.QuoteIdentifier(`product"names`); got != `"product""names"` {
		t.Fatalf("QuoteIdentifier() = %q", got)
	}
	if got := adapter.Placeholder(3); got != "$3" {
		t.Fatalf("Placeholder() = %q", got)
	}
}

func TestPostgreSQLColumnTypes(t *testing.T) {
	tests := map[string]types.Kind{
		"int8": types.Int, "float8": types.Float, "text": types.String,
		"uuid": types.String, "jsonb": types.String, "bool": types.Bool, "bytea": types.Bytes,
	}
	for databaseType, want := range tests {
		got, err := postgresqlColumnType(databaseType, databaseType)
		if err != nil {
			t.Fatalf("postgresqlColumnType(%q): %v", databaseType, err)
		}
		if got.Kind != want {
			t.Fatalf("postgresqlColumnType(%q) = %s, want %s", databaseType, got.Kind, want)
		}
	}
	if _, err := postgresqlColumnType("timestamp without time zone", "timestamp"); err == nil {
		t.Fatal("timestamp should remain unsupported until portable time types are defined")
	}
}

func TestMySQLAdapterDefinesPortableRuntimeSyntax(t *testing.T) {
	adapter, err := AdapterFor("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.DriverName != "mysql" || adapter.GoDriverImport != "github.com/go-sql-driver/mysql" {
		t.Fatalf("unexpected MySQL driver definition: %#v", adapter)
	}
	if got := adapter.QuoteIdentifier("product`names"); got != "`product``names`" {
		t.Fatalf("QuoteIdentifier() = %q", got)
	}
	if got := adapter.Placeholder(3); got != "?" {
		t.Fatalf("Placeholder() = %q", got)
	}
}

func TestMySQLColumnTypes(t *testing.T) {
	tests := []struct {
		dataType, databaseType string
		want                   types.Kind
	}{
		{dataType: "bigint", databaseType: "bigint", want: types.Int},
		{dataType: "double", databaseType: "double", want: types.Float},
		{dataType: "varchar", databaseType: "varchar(255)", want: types.String},
		{dataType: "json", databaseType: "json", want: types.String},
		{dataType: "tinyint", databaseType: "tinyint(1)", want: types.Bool},
		{dataType: "blob", databaseType: "blob", want: types.Bytes},
	}
	for _, test := range tests {
		got, err := mysqlColumnType(test.dataType, test.databaseType)
		if err != nil {
			t.Fatalf("mysqlColumnType(%q): %v", test.databaseType, err)
		}
		if got.Kind != test.want {
			t.Fatalf("mysqlColumnType(%q) = %s, want %s", test.databaseType, got.Kind, test.want)
		}
	}
	if _, err := mysqlColumnType("timestamp", "timestamp"); err == nil {
		t.Fatal("timestamp should remain unsupported until portable time types are defined")
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
	if len(product.Body) != 17 {
		t.Fatalf("expected five fields and twelve ORM methods, got %#v", product.Body)
	}
	field, ok := product.Body[0].(*ir.Field)
	if !ok || field.Name != "@id" || field.Type.Kind != types.Int {
		t.Fatalf("unexpected first field: %#v", product.Body[0])
	}
	methods := map[string]bool{}
	var where *ir.Method
	for _, statement := range product.Body[5:] {
		method, ok := statement.(*ir.Method)
		if ok && method.Name == "where" {
			where = method
		}
		if ok && method.External && method.Class {
			methods[method.Name] = true
		}
	}
	if where == nil || !where.External || !where.Class || where.ReturnType.Name != "ProductQuery" {
		t.Fatalf("unexpected where method: %#v", where)
	}
	for _, name := range []string{"where", "find", "create", "build", "insert_all", "insert_if_absent", "upsert_all", "find_each", "find_in_batches"} {
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
	draft, ok := lowered.Statements[2].(*ir.Class)
	if !ok || !draft.External || draft.Name != "ProductDraft" || len(draft.Body) != 2 {
		t.Fatalf("unexpected draft class: %#v", lowered.Statements[2])
	}
	save, ok := draft.Body[0].(*ir.Method)
	if !ok || !save.External || save.Name != "save" || save.ReturnType.String() != "DbResult<Product>" {
		t.Fatalf("unexpected draft save method: %#v", draft.Body[0])
	}
	upsert, ok := draft.Body[1].(*ir.Method)
	if !ok || !upsert.External || upsert.Name != "upsert" || upsert.ReturnType.String() != "DbResult<Product>" || len(upsert.Parameters[0].LiteralArrays) != 2 {
		t.Fatalf("unexpected draft upsert method: %#v", draft.Body[1])
	}
	changes, ok := lowered.Statements[3].(*ir.Class)
	if !ok || !changes.External || changes.Name != "ProductChanges" || len(changes.Body) != 1 {
		t.Fatalf("unexpected changes class: %#v", lowered.Statements[3])
	}
	changeSave, ok := changes.Body[0].(*ir.Method)
	if !ok || !changeSave.External || changeSave.Name != "save" || changeSave.ReturnType.String() != "DbResult<Product>" {
		t.Fatalf("unexpected changes save method: %#v", changes.Body[0])
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
	if product.InstanceMembers["category"].Return.String() != "Category?" {
		t.Fatalf("unexpected belongs_to declaration: %#v", product.InstanceMembers["category"])
	}
	if product.InstanceMembers["category_query"].Return.String() != "CategoryQuery" {
		t.Fatalf("unexpected belongs_to query declaration: %#v", product.InstanceMembers["category_query"])
	}
	if category.InstanceMembers["products"].Return.String() != "Array<Product>" {
		t.Fatalf("unexpected has_many declaration: %#v", category.InstanceMembers["products"])
	}
	if category.InstanceMembers["products_query"].Return.String() != "ProductQuery" {
		t.Fatalf("unexpected has_many query declaration: %#v", category.InstanceMembers["products_query"])
	}
	preload := productQueryMember(t, catalog, "ProductQuery", "preload")
	if len(preload.Parameters) != 1 || len(preload.Parameters[0].LiteralValues) != 1 || preload.Parameters[0].LiteralValues[0] != "category" {
		t.Fatalf("unexpected ProductQuery.preload declaration: %#v", preload)
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

func productQueryMember(t *testing.T, catalog *declaration.Catalog, typeName, name string) declaration.Member {
	t.Helper()
	query, ok := catalog.Type(typeName)
	if !ok {
		t.Fatalf("missing query type %s", typeName)
	}
	return query.InstanceMembers[name]
}

func TestSQLiteAssociationRejectsMissingForeignKey(t *testing.T) {
	root, options := sqliteAssociationFixture(t, false)
	_, err := Analyze([]*ast.Program{parseAssociationModels(t)}, root, options)
	if err == nil || !strings.Contains(err.Error(), "requires foreign key products.category_id -> categories.id") {
		t.Fatalf("expected missing foreign key diagnostic, got %v", err)
	}
}

func TestSQLiteDatabaseEnvironmentIsResolvedWithoutEmbeddingItsValue(t *testing.T) {
	root, _ := sqliteFixture(t)
	databasePath := filepath.Join(root, "application.sqlite3")
	t.Setenv("TRB_TEST_DATABASE_URL", databasePath)
	schema, err := LoadSchema(root, map[string][]byte{
		PackageName: []byte(`{"adapter":"sqlite","database":{"environment":"TRB_TEST_DATABASE_URL"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if schema.Database != databasePath || schema.DatabaseEnvironment != "TRB_TEST_DATABASE_URL" {
		t.Fatalf("unexpected database source: %#v", schema)
	}
	manifest, err := Analyze([]*ast.Program{parseModel(t)}, root, map[string][]byte{
		PackageName: []byte(`{"adapter":"sqlite","database":{"environment":"TRB_TEST_DATABASE_URL"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.DatabaseEnvironment != "TRB_TEST_DATABASE_URL" {
		t.Fatalf("manifest database environment = %q", manifest.DatabaseEnvironment)
	}
}

func TestDatabaseEnvironmentMustBePresent(t *testing.T) {
	t.Setenv("TRB_TEST_MISSING_DATABASE_URL", "")
	_, err := LoadSchema(t.TempDir(), map[string][]byte{
		PackageName: []byte(`{"adapter":"sqlite","database":{"environment":"TRB_TEST_MISSING_DATABASE_URL"}}`),
	})
	if err == nil || !strings.Contains(err.Error(), `database.environment "TRB_TEST_MISSING_DATABASE_URL" is not set or empty`) {
		t.Fatalf("expected missing database environment diagnostic, got %v", err)
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
		active BOOLEAN NOT NULL DEFAULT TRUE,
		payload BLOB,
		UNIQUE (name, active)
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

func assertUniqueConstraints(t *testing.T, constraints []UniqueConstraint, expected ...[]string) {
	t.Helper()
	if len(constraints) != len(expected) {
		t.Fatalf("unique constraints = %#v, want columns %#v", constraints, expected)
	}
	for index, columns := range expected {
		if strings.Join(constraints[index].Columns, ",") != strings.Join(columns, ",") {
			t.Fatalf("unique constraint %d = %#v, want columns %#v", index, constraints[index], columns)
		}
	}
	if len(constraints) > 0 && !constraints[0].Primary {
		t.Fatalf("first unique constraint is not primary: %#v", constraints)
	}
}
