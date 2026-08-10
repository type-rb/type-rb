package orm

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
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
	if distinct := product.ClassMembers["distinct"]; distinct.Intrinsic != "trb.orm.distinct" || distinct.Return.String() != "ProductQuery" {
		t.Fatalf("unexpected distinct declaration: %#v", distinct)
	}
	not := product.ClassMembers["not"]
	if not.Intrinsic != "trb.orm.not" || not.MinimumArguments != 1 || not.MaximumArguments != 1 || len(not.Parameters) != 5 {
		t.Fatalf("unexpected not declaration: %#v", not)
	}
	if where.Parameters[0].Type.String() != "Array<Integer> | Integer | Range<Integer> | Subquery<Integer>" || where.Parameters[1].Type.String() != "Array<String> | String | Subquery<String>" {
		t.Fatalf("where predicate input types do not include collections and ranges: %#v", where.Parameters)
	}
	if product.ClassMembers["find_by"].Return.String() != "DbResult<Product?>" || product.ClassMembers["exists?"].Return.String() != "DbResult<Boolean>" {
		t.Fatalf("unexpected predicate terminal declarations: %#v", product.ClassMembers)
	}
	if product.ClassMembers["order"].Return.String() != "ProductQuery" || product.ClassMembers["limit"].Return.String() != "ProductQuery" || product.ClassMembers["offset"].Return.String() != "ProductQuery" {
		t.Fatalf("unexpected model query-root declarations: %#v", product.ClassMembers)
	}
	if product.ClassMembers["all"].Return.String() != "DbResult<Array<Product>>" || product.ClassMembers["first"].Return.String() != "DbResult<Product?>" || product.ClassMembers["count"].Return.String() != "DbResult<Integer>" {
		t.Fatalf("unexpected model terminal declarations: %#v", product.ClassMembers)
	}
	if product.ClassMembers["to_sql"].Return.String() != "String" || product.ClassMembers["explain"].Return.String() != "DbResult<String>" {
		t.Fatalf("unexpected model diagnostic declarations: %#v", product.ClassMembers)
	}
	if product.ClassMembers["pluck"].Alternatives[1].Return.String() != "DbResult<Array<String>>" || product.ClassMembers["pick"].Alternatives[2].Return.String() != "DbResult<Float?>" || product.ClassMembers["ids"].Return.String() != "DbResult<Array<Integer>>" {
		t.Fatalf("unexpected projection declarations: %#v", product.ClassMembers)
	}
	if product.ClassMembers["select"].Alternatives[0].Return.String() != "Subquery<Integer>" || product.ClassMembers["select"].Alternatives[2].Return.String() != "Subquery<Float>" {
		t.Fatalf("unexpected subquery declarations: %#v", product.ClassMembers["select"])
	}
	if product.ClassMembers["sum"].Alternatives[0].Return.String() != "DbResult<Integer>" || product.ClassMembers["sum"].Alternatives[1].Return.String() != "DbResult<Float>" {
		t.Fatalf("unexpected sum declaration: %#v", product.ClassMembers["sum"])
	}
	if product.ClassMembers["average"].Alternatives[1].Return.String() != "DbResult<Float?>" {
		t.Fatalf("unexpected average declaration: %#v", product.ClassMembers["average"])
	}
	if product.ClassMembers["minimum"].Alternatives[0].Return.String() != "DbResult<Integer?>" || product.ClassMembers["minimum"].Alternatives[1].Return.String() != "DbResult<String?>" || product.ClassMembers["minimum"].Alternatives[2].Return.String() != "DbResult<Float?>" {
		t.Fatalf("unexpected minimum declaration: %#v", product.ClassMembers["minimum"])
	}
	if !reflect.DeepEqual(product.ClassMembers["maximum"].Parameters[0].LiteralValues, []string{"id", "name", "price"}) {
		t.Fatalf("unexpected maximum columns: %#v", product.ClassMembers["maximum"])
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
	if query.InstanceMembers["not"].Intrinsic != "trb.orm.query.not" || query.InstanceMembers["or"].Parameters[0].Type.String() != "ProductQuery" {
		t.Fatalf("unexpected predicate composition declarations: %#v", query.InstanceMembers)
	}
	if distinct := query.InstanceMembers["distinct"]; distinct.Intrinsic != "trb.orm.query.distinct" || distinct.Return.String() != "ProductQuery" {
		t.Fatalf("unexpected query distinct declaration: %#v", distinct)
	}
	if query.InstanceMembers["find_by"].Return.String() != "DbResult<Product?>" || query.InstanceMembers["exists?"].Return.String() != "DbResult<Boolean>" {
		t.Fatalf("unexpected query predicate terminals: %#v", query.InstanceMembers)
	}
	if query.InstanceMembers["update_all"].Return.String() != "DbResult<Integer>" || query.InstanceMembers["update_all"].MinimumArguments != 1 || query.InstanceMembers["delete_all"].Return.String() != "DbResult<Integer>" {
		t.Fatalf("unexpected relation bulk write declarations: %#v", query.InstanceMembers)
	}
	if query.InstanceMembers["pluck"].Alternatives[1].Return.String() != "DbResult<Array<String>>" || query.InstanceMembers["pick"].Alternatives[2].Return.String() != "DbResult<Float?>" || query.InstanceMembers["ids"].Return.String() != "DbResult<Array<Integer>>" {
		t.Fatalf("unexpected query projection declarations: %#v", query.InstanceMembers)
	}
	for _, name := range []string{"sum", "average", "minimum", "maximum"} {
		if query.InstanceMembers[name].Intrinsic != "trb.orm.query."+name {
			t.Fatalf("unexpected query aggregate declaration: %#v", query.InstanceMembers[name])
		}
	}
	for name, expected := range map[string]string{
		"first": "DbResult<Product?>", "count": "DbResult<Integer>", "explain": "DbResult<String>",
	} {
		if query.InstanceMembers[name].Return.String() != expected {
			t.Fatalf("unexpected %s declaration: %#v", name, query.InstanceMembers[name])
		}
	}
}

func TestAssociationOptionsAndThroughMetadata(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, external_id TEXT NOT NULL UNIQUE, name TEXT NOT NULL);
		CREATE TABLE posts (id INTEGER PRIMARY KEY, author_id TEXT NOT NULL, title TEXT NOT NULL, FOREIGN KEY (author_id) REFERENCES users(external_id));
		CREATE TABLE projects (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE memberships (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, project_id INTEGER NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id), FOREIGN KEY (project_id) REFERENCES projects(id));
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	program, diagnostics := parser.Parse([]byte(`import { Model, belongs_to, has_many } from trb/orm

class User < Model
	has_many(Post, name: :authored_posts, foreign_key: :author_id, references: :external_id, inverse: :author, dependent: :destroy)
	has_many(Membership)
	has_many(Project, through: :memberships)
end

class Post < Model
	belongs_to(User, name: :author, foreign_key: :author_id, references: :external_id, inverse: :authored_posts)
end

class Project < Model
	has_many(Membership)
end

class Membership < Model
	belongs_to(User)
	belongs_to(Project)
end
`))
	if len(diagnostics) > 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	program.ModulePath = "src/models"
	encoded, err := json.Marshal(Config{Adapter: "sqlite", Database: filepath.Base(databasePath)})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Analyze([]*ast.Program{program}, root, map[string][]byte{PackageName: encoded})
	if err != nil {
		t.Fatal(err)
	}
	user, ok := manifest.Model("User")
	if !ok {
		t.Fatal("User model was not discovered")
	}
	authored, ok := user.Association("authored_posts")
	if !ok || authored.SourceColumn != "external_id" || authored.TargetColumn != "author_id" || authored.Inverse != "author" || authored.Dependent != DependentDestroy {
		t.Fatalf("unexpected custom association metadata: %#v", authored)
	}
	projects, ok := user.Association("projects")
	if !ok || projects.Through != "memberships" || projects.Source != "project" || projects.TargetModel != "Project" || !projects.Preloadable {
		t.Fatalf("unexpected through association metadata: %#v", projects)
	}
	post, _ := manifest.Model("Post")
	author, ok := post.Association("author")
	if !ok || author.SourceColumn != "author_id" || author.TargetColumn != "external_id" || author.Inverse != "authored_posts" {
		t.Fatalf("unexpected belongs_to metadata: %#v", author)
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
	if len(product.Body) != 40 {
		t.Fatalf("expected six fields and thirty-four ORM methods, got %#v", product.Body)
	}
	field, ok := product.Body[1].(*ir.Field)
	if !ok || field.Name != "@id" || field.Type.Kind != types.Int {
		t.Fatalf("unexpected first schema field: %#v", product.Body[1])
	}
	methods := map[string]bool{}
	var where *ir.Method
	for _, statement := range product.Body[6:] {
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
	for _, name := range []string{"where", "distinct", "select", "using", "not", "order", "limit", "offset", "all", "first", "count", "to_sql", "explain", "find_by", "exists?", "pluck", "pick", "sum", "average", "minimum", "maximum", "ids", "find", "create", "build", "insert_all", "insert_if_absent", "upsert_all", "find_each", "find_in_batches"} {
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
	for _, name := range []string{"distinct", "select", "group", "not", "or", "find_by", "exists?", "update_all", "delete_all", "pluck", "pick", "sum", "average", "minimum", "maximum", "ids", "to_sql", "explain"} {
		if !queryMethods[name] {
			t.Fatalf("missing generated ORM query method %s: %#v", name, query.Body)
		}
	}
	scope, ok := lowered.Statements[2].(*ir.Class)
	if !ok || !scope.External || scope.Name != "ProductScope" {
		t.Fatalf("unexpected scope class: %#v", lowered.Statements[2])
	}
	draft, ok := lowered.Statements[3].(*ir.Class)
	if !ok || !draft.External || draft.Name != "ProductDraft" || len(draft.Body) != 2 {
		t.Fatalf("unexpected draft class: %#v", lowered.Statements[3])
	}
	save, ok := draft.Body[0].(*ir.Method)
	if !ok || !save.External || save.Name != "save" || save.ReturnType.String() != "DbResult<Product>" {
		t.Fatalf("unexpected draft save method: %#v", draft.Body[0])
	}
	upsert, ok := draft.Body[1].(*ir.Method)
	if !ok || !upsert.External || upsert.Name != "upsert" || upsert.ReturnType.String() != "DbResult<Product>" || len(upsert.Parameters[0].LiteralArrays) != 2 {
		t.Fatalf("unexpected draft upsert method: %#v", draft.Body[1])
	}
	changes, ok := lowered.Statements[4].(*ir.Class)
	if !ok || !changes.External || changes.Name != "ProductChanges" || len(changes.Body) != 1 {
		t.Fatalf("unexpected changes class: %#v", lowered.Statements[4])
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
	if category.InstanceMembers["product"].Return.String() != "Product?" {
		t.Fatalf("unexpected has_one declaration: %#v", category.InstanceMembers["product"])
	}
	if category.InstanceMembers["product_query"].Return.String() != "ProductQuery" {
		t.Fatalf("unexpected has_one query declaration: %#v", category.InstanceMembers["product_query"])
	}
	preload := productQueryMember(t, catalog, "ProductQuery", "preload")
	if preload.Intrinsic != "trb.orm.query.preload" || len(preload.Alternatives) != 2 ||
		!reflect.DeepEqual(preload.Alternatives[0].Parameters[0].LiteralValues, []string{"category"}) ||
		len(preload.Alternatives[1].Parameters) != 2 || preload.Alternatives[1].Parameters[1].Type.String() != "CategoryQuery" {
		t.Fatalf("unexpected ProductQuery.preload declaration: %#v", preload)
	}
	modelPreload := product.ClassMembers["preload"]
	if modelPreload.Intrinsic != "trb.orm.preload" || !modelPreload.Class || len(modelPreload.Alternatives) != 2 ||
		!reflect.DeepEqual(modelPreload.Alternatives[0].Parameters[0].LiteralValues, []string{"category"}) {
		t.Fatalf("unexpected Product.preload declaration: %#v", modelPreload)
	}
	categoryPreload := productQueryMember(t, catalog, "CategoryQuery", "preload")
	if len(categoryPreload.Alternatives) != 4 ||
		categoryPreload.Alternatives[1].Parameters[1].Type.String() != "ProductQuery" ||
		categoryPreload.Alternatives[3].Parameters[1].Type.String() != "ProductQuery" {
		t.Fatalf("unexpected CategoryQuery.preload declaration: %#v", categoryPreload)
	}
	productJoin := productQueryMember(t, catalog, "ProductQuery", "join")
	if productJoin.Intrinsic != "trb.orm.query.join" || len(productJoin.Alternatives) != 2 ||
		!reflect.DeepEqual(productJoin.Alternatives[0].Parameters[0].LiteralValues, []string{"category"}) ||
		len(productJoin.Alternatives[1].Parameters) != 2 || productJoin.Alternatives[1].Parameters[1].Type.String() != "CategoryQuery" {
		t.Fatalf("unexpected ProductQuery.join declaration: %#v", productJoin)
	}
	productModelJoin := product.ClassMembers["left_join"]
	if productModelJoin.Intrinsic != "trb.orm.left_join" || !productModelJoin.Class || len(productModelJoin.Alternatives) != 2 {
		t.Fatalf("unexpected Product.left_join declaration: %#v", productModelJoin)
	}
	productExists := productQueryMember(t, catalog, "ProductQuery", "where_exists")
	if productExists.Intrinsic != "trb.orm.query.where_exists" || len(productExists.Alternatives) != 2 ||
		productExists.Alternatives[1].Parameters[1].Type.String() != "CategoryQuery" {
		t.Fatalf("unexpected ProductQuery.where_exists declaration: %#v", productExists)
	}
	productNotExists := product.ClassMembers["where_not_exists"]
	if productNotExists.Intrinsic != "trb.orm.where_not_exists" || !productNotExists.Class || len(productNotExists.Alternatives) != 2 {
		t.Fatalf("unexpected Product.where_not_exists declaration: %#v", productNotExists)
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
	hasOne, ok := categoryModel.Association("product")
	if !ok || hasOne.SourceColumn != "id" || hasOne.TargetColumn != "category_id" || hasOne.CardinalityVerified {
		t.Fatalf("unexpected unverified has_one association: %#v", hasOne)
	}
}

func TestSQLiteHasOneRecognizesUniqueForeignKey(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE products (
			id INTEGER PRIMARY KEY,
			category_id INTEGER NOT NULL UNIQUE,
			name TEXT NOT NULL,
			FOREIGN KEY (category_id) REFERENCES categories(id)
		);
	`); err != nil {
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
	manifest, err := Analyze([]*ast.Program{parseAssociationModels(t)}, root, map[string][]byte{PackageName: encoded})
	if err != nil {
		t.Fatal(err)
	}
	category, _ := manifest.Model("Category")
	hasOne, ok := category.Association("product")
	if !ok || !hasOne.CardinalityVerified {
		t.Fatalf("has_one did not recognize the unique foreign key: %#v", hasOne)
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
	program, diagnostics := parser.Parse([]byte(`import { Model, belongs_to, has_many, has_one } from trb/orm

class Category < Model
	has_many(Product)
	has_one(Product)
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
