package compiler

import (
	"database/sql"
	"errors"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
	_ "modernc.org/sqlite"
)

func TestPortableORMCompilesLiveSQLiteQuery(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, price REAL, active BOOLEAN NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRB_TEST_DATABASE_URL", databasePath)
	source := []byte(`import { DbResult, Model } from trb/orm

class Product < Model
end

alias ProductList = Array<Product>

def create_product(): DbResult<Product>
	return Product.create(name: "Created", active: true)
end

def save_product(): DbResult<Product>
	draft := Product.build(name: "Saved", active: true)
	return draft.save()
end

def insert_products(): DbResult<Integer>
	return Product.insert_all([
		Product.build(name: "First", active: true),
		Product.build(active: false, name: "Second")
	])
end

def insert_product_if_absent(): DbResult<Boolean>
	draft := Product.build(name: "Unique", active: true)
	return Product.insert_if_absent(draft, unique_by: [:name])
end

def upsert_product(): DbResult<Product>
	draft := Product.build(name: "Unique", price: 12.5, active: true)
	return draft.upsert(unique_by: [:name], update: [:price, :active])
end

def upsert_products(): DbResult<Integer>
	return Product.upsert_all([
		Product.build(name: "First", price: 1.0, active: true),
		Product.build(active: false, name: "Second", price: 2.0)
	], unique_by: [:name], update: [:price, :active])
end

def update_product(product: Product): DbResult<Product>
	return product.update(name: "Updated")
end

def save_product_changes(product: Product): DbResult<Product>
	changes := product.with(name: "Saved update")
	return changes.save()
end

def delete_product(product: Product): DbResult<Boolean>
	return product.delete()
end

def update_all_products(): DbResult<Integer>
	return Product.update_all(name: "Updated")
end

def delete_all_products(): DbResult<Integer>
	return Product.delete_all()
end

def load_products(): DbResult<ProductList>
	return Product.where(name: "Widget").all()
end

def main()
	case load_products()
	when DbResult::Ok(products)
		products.each do |product|
			puts(product.name)
			puts(product.price)
		end
	when DbResult::Err(error)
		puts(error.message)
	end
end
`)
	artifacts, err := CompileProject([]SourceUnit{{
		Filename: filepath.Join(root, "src", "main.trb"), Source: source, ModulePath: "src/main", Package: "main",
	}}, Options{
		Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{
			"trb/orm": []byte(`{"adapter":"sqlite","database":{"environment":"TRB_TEST_DATABASE_URL"}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var output, ormOutput string
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
		if artifact.AST.ModulePath == "src/main" {
			output = string(artifact.Output)
		} else if artifact.AST.ModulePath == "trb/orm/index" {
			ormOutput = string(artifact.Output)
		}
	}
	if output == "" {
		t.Fatal("main artifact was not generated")
	}
	for _, expected := range []string{
		"type Product struct", "type TrbOrmProductQuery struct",
		`trbOrmProductStatement(query, "\"id\", \"name\", \"price\", \"active\"")`,
		"TrbOrmLoadProduct", "type ProductList = []*Product", "orm.DbResult[[]*Product]", "defer orm.TrbOrmCloseDatabase()",
		"orm.NewDbResultErr[[]*Product]", `"database query failed"`, "type ProductDraft struct", "TrbOrmBuildProduct",
		"TrbOrmSaveProductDraft", "TrbOrmCreateProduct", "type ProductChanges struct", "TrbOrmWithProduct",
		"TrbOrmSaveProductChanges", "TrbOrmUpdateProduct", "TrbOrmInsertAllProduct", "TrbOrmDeleteProduct",
		`TrbOrmUpdateAllProduct(TrbOrmProductExecutionScope(`, `[]string{"name"}, []any{"Updated"})`,
		`TrbOrmDeleteAllProduct(TrbOrmProductExecutionScope(`,
		"TrbOrmInsertProductIfAbsent", "TrbOrmUpsertProduct", "trbOrmProductUniqueColumns",
		"TrbOrmUpsertAllProduct", "func(value *float64) any", "if value == nil {",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated Go is missing %q:\n%s", expected, output)
		}
	}
	for _, expected := range []string{
		"modernc.org/sqlite", "func TrbOrmDatabase() (*sql.DB, error)", "trbOrmDatabase.Ping()",
		`os.LookupEnv("TRB_TEST_DATABASE_URL")`,
	} {
		if !strings.Contains(ormOutput, expected) {
			t.Fatalf("generated ORM pool is missing %q:\n%s", expected, ormOutput)
		}
	}
	if strings.Contains(ormOutput, databasePath) {
		t.Fatalf("generated ORM pool exposes the compile-time database value:\n%s", ormOutput)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", output, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid Go: %v\n%s", err, output)
	}
	context := languageservice.BuildContext(programs, "src/main")
	assertORMCompletionContext(t, context)
	assertORMLiteralCompletions(t, context)
}

func TestPortableORMAcceptsNewtypeRepresentationsAtPersistenceBoundaries(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	source := []byte(`import { DbResult, Model } from trb/orm

newtype ProductId = Integer
newtype ProductIds = Array<ProductId>

class Product < Model
end

def load(id: ProductId, ids: ProductIds): DbResult<Product?>
	Product.where(id: ids).where("id", "=", id)
	return Product.find(id)
end
`)
	if _, err := CompileProject([]SourceUnit{{
		Filename: filepath.Join(root, "src", "main.trb"), Source: source, ModulePath: "src/main", Package: "main",
	}}, Options{
		Mode: "go", GoModule: "example.com/orm-newtype", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	}); err != nil {
		t.Fatalf("ORM representation boundary rejected newtypes: %v", err)
	}
}

func TestPortableORMEnumColumnResolvesPackageAlias(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE orders (id INTEGER PRIMARY KEY, status TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	contracts := SourceUnit{
		Filename:        filepath.Join(root, "packages", "contracts", "src", "index.trb"),
		ModulePath:      "github.com/acme/contracts/index",
		Package:         "contracts",
		PackageAliases:  map[string]string{},
		ExternalPackage: true,
		Source: []byte(`enum PackageOrderStatus
	Pending = "PENDING"
	Completed = "COMPLETED"
end
`),
	}
	model := SourceUnit{
		Filename:   filepath.Join(root, "src", "models", "order.trb"),
		ModulePath: "models/order",
		Package:    "models",
		Source: []byte(`import { PackageOrderStatus } from contracts
import { DbResult, Model, enum_column } from trb/orm

class Order < Model
	enum_column(:status, PackageOrderStatus)
end

def create_order(status: PackageOrderStatus): DbResult<Order>
	return Order.create(status: status)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{contracts, model}, Options{
				Mode:              mode,
				GoModule:          "example.com/orm-package-enum",
				RubyLoader:        "require_relative",
				TypeScriptRuntime: "bun",
				SourceRoot:        filepath.Join(root, "src"),
				ProjectRoot:       root,
				PackageAliases:    map[string]string{"contracts": "github.com/acme/contracts"},
				PackageOptions: map[string][]byte{
					"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPortableORMEmitsSharedGoRuntimeOncePerPackage(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
	} {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRB_TEST_DATABASE_URL", databasePath)

	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename:   filepath.Join(root, "src", "models", "product.trb"),
			ModulePath: "models/product",
			Package:    "models",
			Source: []byte(`import { Model } from trb/orm

class Product < Model
end
`),
		},
		{
			Filename:   filepath.Join(root, "src", "models", "user.trb"),
			ModulePath: "models/user",
			Package:    "models",
			Source: []byte(`import { Model } from trb/orm

class User < Model
end
`),
		},
	}, Options{
		Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{
			"trb/orm": []byte(`{"adapter":"sqlite","database":{"environment":"TRB_TEST_DATABASE_URL"}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	outputs := map[string]string{}
	sharedRuntimeCount := 0
	for _, artifact := range artifacts {
		if artifact.AST.Package != "models" {
			continue
		}
		output := string(artifact.Output)
		outputs[artifact.AST.ModulePath] = output
		sharedRuntimeCount += strings.Count(output, "type trbOrmExecutorTarget interface")
		if _, err := parser.ParseFile(token.NewFileSet(), artifact.AST.ModulePath+".go", output, parser.AllErrors); err != nil {
			t.Fatalf("generated %s is invalid Go: %v\n%s", artifact.AST.ModulePath, err, output)
		}
	}
	if sharedRuntimeCount != 1 {
		t.Fatalf("shared ORM runtime was generated %d times, want 1", sharedRuntimeCount)
	}
	if !strings.Contains(outputs["models/product"], "type TrbOrmProductQuery struct") {
		t.Fatal("product-specific ORM runtime was not generated")
	}
	if !strings.Contains(outputs["models/user"], "type TrbOrmUserQuery struct") {
		t.Fatal("user-specific ORM runtime was not generated")
	}
}

func TestPortableORMCompilesExplicitTransactionScope(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	source := []byte(`import { Database, DbError, DbResult, Model, Transaction } from trb/orm

class Product < Model
end

def persist_product(transaction: Transaction, name: String): DbResult<Integer>
	product := try Product.using(transaction).create(name: name)
	return DbResult<Integer>::Ok(product.id)
end

def create_product(): DbResult<Integer>
	return Database.transaction() do |tx|
		products := Product.using(tx)
		product := try products.create(name: "Created")
		locked_products := try products.where(id: product.id).lock().all()
		puts(locked_products.size())
		puts(try persist_product(tx, "Created by helper"))
		product.id
	end
end

def create_nested_product(): DbResult<Integer>
	return Database.transaction() do |tx|
		nested_result := try tx.transaction() do |nested|
			products := Product.using(nested)
			product := try products.create(name: "Nested")
			product.id
		end
		nested_result
	end
end

def create_and_ignore_product(): DbResult<Integer>
	return Database.transaction() do |tx|
		products := Product.using(tx)
		_ignored := try products.create(name: "Ignored result")
		0
	end
end

def main()
	puts(create_product())
	puts(create_nested_product())
	puts(create_and_ignore_product())
end
`)
	artifacts, err := CompileProject([]SourceUnit{{
		Filename: filepath.Join(root, "src", "main.trb"), Source: source, ModulePath: "src/main", Package: "main",
	}}, Options{
		Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	var output, ormOutput string
	for _, artifact := range artifacts {
		switch artifact.AST.ModulePath {
		case "src/main":
			assertORMTransactionScopeIR(t, artifact.IR)
			output = string(artifact.Output)
		case "trb/orm/index":
			ormOutput = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		"func CreateProduct(__trbScope trbcontext.Context) orm.DbResult[int]", "orm.TrbOrmBeginTransaction(__trbScope)",
		"TrbOrmProductUsing(tx)", "TrbOrmProductCreateScoped(products", "defer func()",
		"PersistProduct(__trbScope, tx, \"Created by helper\")",
		"TrbOrmProductLock(TrbOrmProductExecutionScope(TrbOrmProductQueryWhere(products", "trbOrmExecutorForQuery(query.scope, query.transaction, query.lock)",
		"orm.TrbOrmBeginNestedTransaction(tx)", "TrbOrmProductUsing(nested)",
		".Rollback()", ".Commit()", "orm.DbResultErrTag",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated transaction is missing %q:\n%s", expected, output)
		}
	}
	for _, expected := range []string{
		"type TrbOrmTransaction struct", "func TrbOrmBeginTransaction(scope trbcontext.Context)", `"BEGIN IMMEDIATE"`, "func TrbOrmBeginNestedTransaction(parent *TrbOrmTransaction)",
		`"SAVEPOINT " + savepoint`, `"ROLLBACK TO SAVEPOINT " + transaction.savepoint`, `"RELEASE SAVEPOINT " + transaction.savepoint`,
		"func (transaction *TrbOrmTransaction) Commit()", "func (transaction *TrbOrmTransaction) Rollback()",
	} {
		if !strings.Contains(ormOutput, expected) {
			t.Fatalf("generated transaction runtime is missing %q:\n%s", expected, ormOutput)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", output, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid transaction Go: %v\n%s", err, output)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "orm.go", ormOutput, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid ORM transaction Go: %v\n%s", err, ormOutput)
	}
}

func assertORMTransactionScopeIR(t *testing.T, program *ir.Program) {
	t.Helper()
	var method *ir.Method
	for _, statement := range program.Statements {
		candidate, ok := statement.(*ir.Method)
		if ok && candidate.Name == "create_product" {
			method = candidate
			break
		}
	}
	if method == nil || len(method.Body) != 1 {
		t.Fatalf("unexpected transaction method IR: %#v", program.Statements)
	}
	block, ok := method.Body[0].(*ir.StructuredBlock)
	if !ok {
		t.Fatalf("unexpected transaction block IR: %#v", method.Body)
	}
	var product *ir.Variable
	for _, statement := range block.Body {
		candidate, ok := statement.(*ir.Variable)
		if ok && candidate.Name == "product" {
			product = candidate
			break
		}
	}
	if product == nil {
		t.Fatalf("missing propagated product IR: %#v", block.Body)
	}
	result, ok := product.Value.(*ir.Case)
	if !ok {
		t.Fatalf("unexpected propagated product value IR: %#v", product.Value)
	}
	call, ok := result.Value.(*ir.Call)
	if !ok {
		t.Fatalf("unexpected propagated call IR: %#v", result.Value)
	}
	member, ok := call.Callee.(*ir.Member)
	if !ok || member.Reference == nil || member.Reference.Intrinsic != "trb.orm.scope.create" || member.Receiver.ExprType().Name != "ProductScope" {
		t.Fatalf("unexpected scoped create IR: %#v", call.Callee)
	}
}

func assertORMCompletionContext(t *testing.T, context languageservice.Context) {
	t.Helper()
	var product *languageservice.Symbol
	for index := range context.Symbols {
		if context.Symbols[index].Name == "Product" {
			product = &context.Symbols[index]
			break
		}
	}
	if product == nil {
		t.Fatal("Product is missing from completion context")
	}
	classMethods := map[string]bool{}
	for _, member := range product.Members {
		classMethods[member.Name] = true
	}
	for _, name := range []string{"where", "not", "find_by", "exists?", "pluck", "pick", "sum", "average", "minimum", "maximum", "ids", "find", "build", "create", "insert_all", "insert_if_absent", "upsert_all", "find_each", "find_in_batches"} {
		if !classMethods[name] {
			t.Fatalf("Product.%s is missing from completion context: %#v", name, product.Members)
		}
	}
	fields := map[string]bool{}
	for _, member := range context.TypeMembers["Product"] {
		if member.Kind == languageservice.CompletionField {
			fields[member.Name] = true
		}
	}
	if !fields["id"] || !fields["name"] || !fields["price"] {
		t.Fatalf("schema fields are missing from completion context: %#v", context.TypeMembers["Product"])
	}
	modelMethods := map[string]bool{}
	for _, member := range context.TypeMembers["Product"] {
		if member.Kind == languageservice.CompletionMethod {
			modelMethods[member.Name] = true
		}
	}
	if !modelMethods["with"] {
		t.Fatalf("Product.with is missing from completion context: %#v", context.TypeMembers["Product"])
	}
	queryMethods := map[string]bool{}
	for _, member := range context.TypeMembers["ProductQuery"] {
		if member.Kind == languageservice.CompletionMethod {
			queryMethods[member.Name] = true
		}
	}
	for _, name := range []string{"where", "not", "or", "find_by", "exists?", "update_all", "delete_all", "pluck", "pick", "sum", "average", "minimum", "maximum", "ids", "order", "limit", "offset", "all", "first", "count", "to_sql", "explain", "find_each", "find_in_batches"} {
		if !queryMethods[name] {
			t.Fatalf("ProductQuery.%s is missing from completion context: %#v", name, context.TypeMembers["ProductQuery"])
		}
	}
	draftMethods := map[string]bool{}
	for _, member := range context.TypeMembers["ProductDraft"] {
		if member.Kind == languageservice.CompletionMethod {
			draftMethods[member.Name] = true
		}
	}
	if !draftMethods["save"] || !draftMethods["upsert"] {
		t.Fatalf("ProductDraft write methods are missing from completion context: %#v", context.TypeMembers["ProductDraft"])
	}
	changesMethods := map[string]bool{}
	for _, member := range context.TypeMembers["ProductChanges"] {
		if member.Kind == languageservice.CompletionMethod {
			changesMethods[member.Name] = true
		}
	}
	if !changesMethods["save"] {
		t.Fatalf("ProductChanges.save is missing from completion context: %#v", context.TypeMembers["ProductChanges"])
	}
}

func assertORMLiteralCompletions(t *testing.T, context languageservice.Context) {
	t.Helper()
	complete := func(source string) []languageservice.CompletionItem {
		return languageservice.Complete(languageservice.CompletionRequest{Source: source, Cursor: len(source), Mode: "go", Context: context})
	}
	find := func(items []languageservice.CompletionItem, label string) (languageservice.CompletionItem, bool) {
		for _, item := range items {
			if item.Label == label {
				return item, true
			}
		}
		return languageservice.CompletionItem{}, false
	}
	for _, test := range []struct {
		source, label, insert string
	}{
		{source: `Product.where("pr`, label: "price", insert: `price"`},
		{source: `Product.where("price", ">`, label: ">=", insert: `>="`},
		{source: `Product.where().order(price: :d`, label: "desc", insert: "desc"},
		{source: `Product.sum(:pr`, label: "price", insert: "price"},
		{source: `Product.minimum(:na`, label: "name", insert: "name"},
		{source: "query := Product.where(name: \"Widget\")\nquery.where(\"na", label: "name", insert: `name"`},
		{source: `Product.insert_if_absent(Product.build(name: "Widget", active: true), unique_by: [:na`, label: "name", insert: "name"},
		{source: `Product.build(name: "Widget", active: true).upsert(unique_by: [:name], update: [:pr`, label: "price", insert: "price"},
	} {
		items := complete(test.source)
		item, ok := find(items, test.label)
		if !ok {
			t.Fatalf("ORM completion for %q is missing %q: %#v", test.source, test.label, items)
		}
		if item.InsertText != test.insert || item.Kind != languageservice.CompletionValue {
			t.Fatalf("ORM completion for %q is %#v, want insert %q", test.source, item, test.insert)
		}
	}
	active := complete(`Product.where("active", "`)
	for _, label := range []string{"=", "!="} {
		if _, ok := find(active, label); !ok {
			t.Fatalf("Boolean comparison completion is missing %q: %#v", label, active)
		}
	}
	for _, label := range []string{"<", "<=", ">", ">="} {
		if _, ok := find(active, label); ok {
			t.Fatalf("Boolean comparison completion unexpectedly includes %q: %#v", label, active)
		}
	}
	uniqueBy := complete(`Product.insert_if_absent(Product.build(name: "Widget", active: true), unique_by: `)
	for _, label := range []string{"[:id]", "[:name]"} {
		item, ok := find(uniqueBy, label)
		if !ok || item.InsertText != label {
			t.Fatalf("unique_by completion is missing %q: %#v", label, uniqueBy)
		}
	}
}

func TestPortableORMWhereUsesSchemaTypes(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	database.Close()
	_, err = CompileProject([]SourceUnit{{
		Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "src/main", Package: "main",
		Source: []byte("import { Model } from trb/orm\nclass Product < Model\nend\ndef main()\n\tProduct.where(id: \"wrong\").all()\nend\n"),
	}}, Options{
		Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	})
	if err == nil || !strings.Contains(err.Error(), "expected Array<Integer>") {
		t.Fatalf("expected schema type error, got %v", err)
	}
}

func TestPortableORMCreateUsesSchemaTypesAndDefaults(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		price REAL,
		active BOOLEAN NOT NULL DEFAULT TRUE
	)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	compile := func(call string) ([]*Artifact, error) {
		return CompileProject([]SourceUnit{{
			Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "src/main", Package: "main",
			Source: []byte("import { Model } from trb/orm\nclass Product < Model\nend\ndef main()\n\tresult := " + call + "\n\tputs(result)\nend\n"),
		}}, Options{
			Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
			PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
		})
	}
	artifacts, err := compile(`Product.create(name: "Widget")`)
	if err != nil {
		t.Fatal(err)
	}
	if output := string(artifacts[0].Output); !strings.Contains(output, `TrbOrmProductCreateScoped(`) || !strings.Contains(output, `[]string{"name"}, []any{"Widget"})`) {
		t.Fatalf("generated create call does not preserve schema keywords:\n%s", output)
	}
	artifacts, err = compile(`Product.build(name: "Widget").save()`)
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmSaveProductDraft(TrbOrmProductBuildScoped(`) || !strings.Contains(output, `[]string{"name"}, []any{"Widget"})`) {
		t.Fatalf("generated draft save does not preserve schema keywords:\n%s", output)
	}
	artifacts, err = compile(`puts(Product.new().with(name: "Updated").save())`)
	if err != nil {
		t.Fatal(err)
	}
	output = string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmSaveProductChanges(TrbOrmWithProduct(NewProduct(), []string{"name"}, []any{"Updated"}))`) {
		t.Fatalf("generated changes save does not preserve schema keywords:\n%s", output)
	}
	artifacts, err = compile(`puts(Product.insert_all([Product.build(name: "First"), Product.build(name: "Second")]))`)
	if err != nil {
		t.Fatal(err)
	}
	output = string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmInsertAllProduct([]*ProductDraft{TrbOrmProductBuildScoped`) {
		t.Fatalf("generated strict bulk insert does not use typed drafts:\n%s", output)
	}
	artifacts, err = compile(`puts(Product.insert_if_absent(Product.build(name: "Unique"), unique_by: [:name]))`)
	if err != nil {
		t.Fatal(err)
	}
	output = string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmInsertProductIfAbsent(TrbOrmProductBuildScoped(`) || !strings.Contains(output, `[]string{"name"}, []any{"Unique"})`) || !strings.Contains(output, `[]string{"name"})`) {
		t.Fatalf("generated insert_if_absent does not preserve unique_by:\n%s", output)
	}
	artifacts, err = compile(`puts(Product.build(name: "Unique", price: 1.0).upsert(unique_by: [:name], update: [:price]))`)
	if err != nil {
		t.Fatal(err)
	}
	output = string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmUpsertProduct(TrbOrmProductBuildScoped`) || !strings.Contains(output, `[]string{"name"}, []string{"price"}`) {
		t.Fatalf("generated upsert does not preserve conflict and update columns:\n%s", output)
	}
	artifacts, err = compile(`puts(Product.upsert_all([Product.build(name: "First", price: 1.0), Product.build(price: 2.0, name: "Second")], unique_by: [:name], update: [:price]))`)
	if err != nil {
		t.Fatal(err)
	}
	output = string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmUpsertAllProduct([]*ProductDraft{TrbOrmProductBuildScoped`) || !strings.Contains(output, `[]string{"name"}, []string{"price"}`) {
		t.Fatalf("generated upsert_all does not use typed drafts and literal columns:\n%s", output)
	}
	for _, test := range []struct {
		call string
		want string
	}{
		{call: `Product.create(price: 10.0)`, want: "create() is missing required argument name"},
		{call: `Product.create(name: "Widget", price: "wrong")`, want: "has type String, expected Float?"},
		{call: `Product.create(name: "Widget", missing: true)`, want: "create() has no named argument missing"},
		{call: `Product.build(price: 10.0)`, want: "build() is missing required argument name"},
		{call: `Product.build(name: "Widget", price: "wrong")`, want: "has type String, expected Float?"},
		{call: `Product.build(name: "Widget", missing: true)`, want: "build() has no named argument missing"},
		{call: `Product.new().with(id: 2)`, want: "with() has no named argument id"},
		{call: `Product.new().with(price: "wrong")`, want: "has type String, expected Float?"},
		{call: `Product.insert_all([Product.new()])`, want: "expected Array<ProductDraft>"},
		{call: `Product.insert_if_absent(Product.build(name: "Unique"), unique_by: [:price])`, want: "must match one of [:id], [:name]"},
		{call: `Product.build(name: "Unique").upsert(unique_by: [:name], update: [:id])`, want: "must be a non-empty literal array"},
		{call: `Product.build(name: "Unique").upsert(unique_by: [:name], update: [:price, :price])`, want: "must be a non-empty literal array"},
		{call: `Product.upsert_all([Product.new()], unique_by: [:name], update: [:price])`, want: "expected Array<ProductDraft>"},
	} {
		if _, err := compile(test.call); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected write diagnostic %q, got %v", test.want, err)
		}
	}
}

func TestPortableORMComparisonWhereUsesSchemaTypes(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL, price REAL NOT NULL, discount REAL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	compile := func(source string) ([]*Artifact, error) {
		return CompileProject([]SourceUnit{{
			Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: []byte(source),
		}}, Options{
			Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
			PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
		})
	}
	valid := `import { Model } from trb/orm
class Product < Model
end
def main()
	ids := [1, 2, 3]
	bounds := 6...8
	puts(Product.where("price", ">=", 10).where(id: ids).not(id: 3...5).not(id: bounds).where(discount: nil).not(discount: nil).all())
end
`
	artifacts, err := compile(valid)
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		`[]string{"price"}`, `[]string{">="}`, `[]string{"id"}`, `[]string{"IN"}`,
		`[]string{"RANGE_EXCLUSIVE"}`, `trbOrmRange{start: 3, end: 5, exclusive: true}`,
		`return trbOrmRange{start: bounds[0], end: bounds[1], exclusive: bounds[2] != 0}`,
		`if values.Len() == 0 {`, `return "1 = 0"`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated comparison query is missing %q:\n%s", expected, output)
		}
	}
	invalid := []struct {
		source string
		want   string
	}{
		{source: "Product.where(\"missing\", \"=\", 1)", want: `argument 1 to where() must be one of`},
		{source: "Product.where(\"price\", \"contains\", 1.0)", want: `argument 2 to where() must be one of`},
		{source: "Product.where(\"price\", \">=\", \"ten\")", want: `has type String, expected Float`},
		{source: "Product.where(\"discount\", \">=\", nil)", want: `expected Float`},
		{source: `Product.where(id: ["one", "two"])`, want: `expected Array<Integer>`},
		{source: `Product.where(price: 1..3)`, want: `expected Array<Float>`},
	}
	for _, test := range invalid {
		source := "import { Model } from trb/orm\nclass Product < Model\nend\ndef main()\n\t" + test.source + "\nend\n"
		if _, err := compile(source); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected comparison diagnostic %q, got %v", test.want, err)
		}
	}
}

func TestPortableORMComposesTypedQueries(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL, price REAL NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	compileSource := func(source string) ([]*Artifact, error) {
		return CompileProject([]SourceUnit{{
			Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: []byte(source),
		}}, Options{
			Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
			PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
		})
	}
	compile := func(body string) ([]*Artifact, error) {
		return compileSource("import { Model } from trb/orm\nclass Product < Model\nend\ndef run_queries()\n" + body + "\n\treturn\nend\ndef main()\n\trun_queries()\nend\n")
	}
	artifacts, err := compile("\tquery := Product.where(\"price\", \">=\", 10).not(name: \"Deleted\").or(Product.where(name: \"Widget\")).order(price: :desc).limit(5).offset(1)\n\tputs(Product.not(name: \"Deleted\").to_sql())\n\tputs(Product.exists?(name: \"Widget\"))\n\tputs(Product.find_by(name: \"Widget\"))\n\tputs(Product.pluck(:name))\n\tputs(Product.pick(:price))\n\tputs(Product.sum(:price))\n\tputs(Product.average(:price))\n\tputs(Product.minimum(:name))\n\tputs(Product.maximum(:price))\n\tputs(Product.ids())\n\tputs(Product.where(\"price\", \">=\", 10).exists?())\n\tputs(Product.where(\"price\", \">=\", 10).find_by(name: \"Widget\"))\n\tputs(Product.where(\"price\", \">=\", 10).pluck(:name))\n\tputs(Product.where(\"price\", \">=\", 10).pick(:price))\n\tputs(Product.where(\"price\", \">=\", 10).sum(:price))\n\tputs(Product.where(\"price\", \">=\", 10).average(:price))\n\tputs(Product.where(\"price\", \">=\", 10).minimum(:name))\n\tputs(Product.where(\"price\", \">=\", 10).maximum(:price))\n\tputs(Product.where(\"price\", \">=\", 10).ids())\n\tputs(Product.where(name: \"Widget\").update_all(price: 20.0))\n\tputs(Product.where(name: \"Deleted\").delete_all())\n\tputs(query.to_sql())\n\tputs(query.explain())\n\tputs(query.count())\n\tputs(query.first())\n\tputs(query.all())")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		"TrbOrmProductQueryWhere", "TrbOrmProductNot", "TrbOrmProductQueryNot", "TrbOrmProductQueryOr",
		"TrbOrmFirstProduct", "TrbOrmExistsProduct", "trbOrmProductPredicateSQL",
		"TrbOrmUpdateAllProduct", "TrbOrmDeleteAllProduct", "database bulk update failed", "database bulk delete failed",
		"TrbOrmPluckProductName", "TrbOrmPickProductPrice", "database projection query failed",
		"TrbOrmSumProductPrice", "TrbOrmAverageProductPrice", "TrbOrmMinimumProductName", "TrbOrmMaximumProductPrice",
		"database aggregate result was invalid", "AS trb_aggregate", `COALESCE(SUM(\"trb_value\"), 0)`,
		"TrbOrmProductOrder", "TrbOrmProductLimit", "TrbOrmProductOffset",
		"TrbOrmToSQLProduct", "TrbOrmExplainProduct", "EXPLAIN QUERY PLAN", "TrbOrmCountProduct", "TrbOrmFirstProduct", `statement += " ORDER BY "`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated composed query is missing %q:\n%s", expected, output)
		}
	}
	direct, err := compileSource(`import { DbResult, Model } from trb/orm

class Product < Model
end

def process_products(): DbResult<Integer>
	return Product.find_each(batch_size: 2) do |product|
		puts(product.name)
	end
end

def main()
	puts(process_products())
end
`)
	if err != nil {
		t.Fatal(err)
	}
	directOutput := string(direct[0].Output)
	for _, expected := range []string{"func ProcessProducts(__trbScope trbcontext.Context) orm.DbResult[int]", "return func() orm.DbResult[int]", "return orm.NewDbResultOk[int]"} {
		if !strings.Contains(directOutput, expected) {
			t.Fatalf("generated direct-return batch query is missing %q:\n%s", expected, directOutput)
		}
	}
	invalid := []struct {
		body string
		want string
	}{
		{body: "\tProduct.where().order(price: :sideways)", want: `must be one of "asc", "desc"`},
		{body: "\tProduct.where().limit(1.5)", want: "has type Float, expected Integer"},
		{body: "\tProduct.where().to_sql(1)", want: "to_sql() expects at most 0 arguments, got 1"},
		{body: "\tProduct.where().explain(1)", want: "explain() expects at most 0 arguments, got 1"},
		{body: "\tProduct.not()", want: "not() expects at least 1 argument(s), got 0"},
		{body: "\tProduct.not(name: \"Deleted\", price: 1.0)", want: "not() expects at most 1 argument(s), got 2"},
		{body: "\tProduct.find_by()", want: "find_by() expects at least 1 argument(s), got 0"},
		{body: "\tProduct.where(name: \"Widget\").find_by()", want: "find_by() expects at least 1 argument(s), got 0"},
		{body: "\tProduct.where(name: \"Widget\").update_all()", want: "update_all() expects at least 1 argument(s), got 0"},
		{body: "\tProduct.where(name: \"Widget\").update_all(price: \"twenty\")", want: "has type String, expected Float"},
		{body: "\tProduct.pluck(:missing)", want: "must be one of"},
		{body: "\tProduct.pick(:missing)", want: "must be one of"},
		{body: "\tProduct.sum(:name)", want: "must be one of"},
		{body: "\tProduct.average(:active)", want: "must be one of"},
		{body: "\tProduct.maximum(:active)", want: "must be one of"},
		{body: "\tProduct.ids(1)", want: "ids() expects at most 0 arguments, got 1"},
	}
	for _, test := range invalid {
		if _, err := compile(test.body); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected query composition diagnostic %q, got %v", test.want, err)
		}
	}
}

func TestPortableORMPropagatesExecutionScopeThroughImportedInterface(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	sources := []SourceUnit{
		{
			Filename: filepath.Join(root, "src", "application", "repository.trb"), ModulePath: "application/repository", Package: "application",
			Source: []byte(`import { DbResult } from trb/orm

interface Repository
	count(): DbResult<Integer>
end
`),
		},
		{
			Filename: filepath.Join(root, "src", "infrastructure", "repository.trb"), ModulePath: "infrastructure/repository", Package: "infrastructure",
			Source: []byte(`import { DbResult, Model } from trb/orm
import { Repository } from application/repository

class Product < Model
end

class SQLRepository implements Repository
	def count(): DbResult<Integer>
		return Product.count()
	end
end
`),
		},
		{
			Filename: filepath.Join(root, "src", "application", "count.trb"), ModulePath: "application/count", Package: "application",
			Source: []byte(`import { DbResult } from trb/orm
import { Repository } from application/repository

def count_products(repository: Repository): DbResult<Integer>
	return repository.count()
end
`),
		},
	}
	artifacts, err := CompileProject(sources, Options{
		Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	var application string
	for _, artifact := range artifacts {
		if artifact.AST.ModulePath == "application/count" {
			application = string(artifact.Output)
			break
		}
	}
	for _, expected := range []string{
		"func CountProducts(__trbScope trbcontext.Context, repository Repository)",
		"repository.Count(__trbScope)",
	} {
		if !strings.Contains(application, expected) {
			t.Fatalf("generated interface call is missing %q:\n%s", expected, application)
		}
	}
}

func TestPortableORMFindsAndIteratesInPrimaryKeyBatches(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	compile := func(body string) ([]*Artifact, error) {
		source := "import { Model } from trb/orm\nclass Product < Model\nend\ndef run_batches()\n" + body + "\n\treturn\nend\ndef main()\n\trun_batches()\nend\n"
		return CompileProject([]SourceUnit{{
			Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: []byte(source),
		}}, Options{
			Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
			PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
		})
	}
	valid := `	found := Product.find(2)
	puts(found)
	each_result := Product.find_each(batch_size: 2) do |product|
		puts(product.name)
		break
	end
	puts(each_result)
	batch_result := Product.where("id", ">", 0).find_in_batches(batch_size: 2) do |products|
		puts(products.size())
	end
	puts(batch_result)
	inline_result := Product.where(name: "Widget").find_each(batch_size: 2) { |product| puts(product.name) }
	puts(inline_result)
	mut reassigned := Product.where(name: "Widget").find_each(batch_size: 2) { |product| puts(product.name) }
	reassigned = Product.where(name: "Widget").find_each(batch_size: 2) { |product| puts(product.name) }
	puts(reassigned)`
	artifacts, err := compile(valid)
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		"TrbOrmFirstProduct(TrbOrmProductWhere", "func TrbOrmBatchProduct", "__trbBatchLoop", "break __trbBatchLoop", "TrbOrmBatchProduct",
		"orm.DbResult[int]", "orm.NewDbResultOk[int]", "orm.NewDbResultErr[int]", "__trbBatchProcessed",
		`"batch queries do not accept joins, order, limit, offset, or lock"`, "reassigned = func() orm.DbResult[int]",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated batch query is missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "panic(loaded.ErrError") {
		t.Fatalf("generated batch query still exposes database failures through panic:\n%s", output)
	}
	invalid := []struct {
		body string
		want string
	}{
		{body: "\tProduct.find(\"two\")", want: "has type String, expected Integer"},
		{body: "\tProduct.find_each(batch_size: 2)", want: "find_each() requires a block"},
		{body: "\t_result := Product.find_each(batch_size: 1.5) do |product|\n\t\tputs(product.name)\n\tend", want: "has type Float, expected Integer"},
		{body: "\t_result := Product.find_each() do |product|\n\t\tputs(1)\n\tend", want: "block parameter product is not used"},
		{body: "\t_result := Product.find_in_batches() do |left, right|\n\t\tputs(left)\n\t\tputs(right)\n\tend", want: "find_in_batches block expects 1 parameter(s), got 2"},
		{body: "\tProduct.find_each() do |product|\n\t\tputs(product.name)\n\tend", want: "structured block find_each() must be the direct value"},
	}
	for _, test := range invalid {
		if _, err := compile(test.body); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected batch diagnostic %q, got %v", test.want, err)
		}
	}
}

func TestPortableORMAssociationsReturnTypedQueries(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (
		id INTEGER PRIMARY KEY,
		category_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		FOREIGN KEY (category_id) REFERENCES categories(id)
	)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	source := []byte(`import { DbResult, Model, belongs_to, has_many, has_one } from trb/orm

class Category < Model
	has_many(Product)
	has_one(Product)
end

class Product < Model
	belongs_to(Category)
end

def load_associations(): DbResult<Integer>
	all_products := try Product.all()
	first_product := Product.first()
	product_count := try Product.count()
	limited_products := try Product.limit(1).offset(0).all()
	puts(first_product)
	puts(Product.to_sql())
	puts(Product.explain())
	joined := try Product.join(:category, Category.where(name: "Books")).where(name: "TypeRB").all()
	left_joined := try Product.where(name: "TypeRB").left_join(:category).all()
	distinct_categories := try Category.join(:products).distinct().all()
	category_ids := Category.where(name: "Books").select(:id)
	subquery_products := try Product.where(category_id: category_ids).all()
	excluded_products := try Product.where("category_id", "!=", Category.select(:id)).all()
	existing_products := try Product.where_exists(:category, Category.where(name: "Books")).all()
	missing_products := try Product.where_not_exists(:category, Category.where(name: "Missing")).all()
	group_counts := try Product.group(:category_id).count()
	popular_groups := try Product.where(name: "TypeRB").group(:category_id).having(:count, ">=", 1).count()
	group_sums := try Product.group(:category_id).sum(:id)
	large_groups := try Product.group(:category_id).having(:sum, :id, ">=", 1).sum(:id)
	paged_groups := try Product.order(category_id: :desc).limit(1).group(:category_id).count()
	products := try Product.preload(:category).all()
	products.each do |product|
		puts(product.category)
		puts(product.category.loaded?())
		puts(product.category.load())
		puts(product.category.reload())
		puts(product.category_query().count())
	end
	categories := try Category.preload(:products).all()
	categories.each do |category|
		loaded_products := try category.products
		puts(loaded_products.size())
		puts(category.products.loaded?())
		puts(category.products_query().count())
	end
	single_categories := try Category.preload(:product).all()
	single_categories.each do |category|
		puts(category.product)
		puts(category.product_query().count())
	end
	nested_categories := try Category.preload(:products, Product.where(name: "TypeRB").preload(:category)).all()
	nested_categories.each do |category|
		nested_products := try category.products
		nested_products.each do |product|
			puts(product.category)
		end
	end
	return DbResult<Integer>::Ok(all_products.size() + product_count + limited_products.size() + joined.size() + left_joined.size() + distinct_categories.size() + subquery_products.size() + excluded_products.size() + existing_products.size() + missing_products.size() + group_counts.size() + popular_groups.size() + group_sums.size() + large_groups.size() + paged_groups.size() + products.size() + categories.size() + single_categories.size() + nested_categories.size())
end

def main()
	puts(load_associations())
end
`)
	artifacts, err := CompileProject([]SourceUnit{{
		Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: source,
	}}, Options{
		Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		`TrbOrmProductJoin(TrbOrmProductWhere`, `Kind: "INNER JOIN"`, `Kind: "LEFT JOIN"`,
		`TrbOrmCategoryDistinct(TrbOrmCategoryJoin`, `prefix += "DISTINCT "`,
		`Table: "categories"`, `SourceColumn: "category_id"`, `TargetColumn: "id"`,
		`TrbOrmCategoryAssociationPredicate(`, `TrbOrmCategoryWhere`, `__trb_join_key`,
		`TrbOrmSelectCategoryId(TrbOrmCategoryWhere`, `*orm.TrbOrmSubquery[int]`,
		`condition.operator == "IN" || condition.operator == "NOT_IN"`, `operator = " NOT IN "`,
		`TrbOrmProductWhereExists(TrbOrmProductWhere`, `operator := "EXISTS"`, `operator = "NOT EXISTS"`,
		`TrbOrmGroupProductCategoryId(TrbOrmProductExecutionScope(TrbOrmProductWhere`, `TrbOrmHavingProductCategoryId`, `TrbOrmCountGroupedProductCategoryId`,
		`TrbOrmSumGroupedProductCategoryIdId`, `COALESCE(SUM(trb_value), 0)`,
		`grouped.query.orders = nil`, `ORDER BY`, `grouped.limit`,
		`GROUP BY`, `grouped.havingExpression`, `map[int]int`,
		`trbOrmQuoteIdentifier("products")`, `TrbOrmCategoryAssociationPredicate(`, `TrbOrmCategoryWhere`,
		`TrbOrmCategoryQueryWhere(TrbOrmCategoryUsing(product.TrbOrmTransaction()), []string{"id"}, []string{"="}, []any{product.TrbOrmColumnCategoryId()})`,
		`TrbOrmProductQueryWhere(TrbOrmProductUsing(category.TrbOrmTransaction()), []string{"category_id"}, []string{"="}, []any{category.TrbOrmColumnId()})`,
		`TrbOrmProductPreload`, `trbOrmPreloadProductCategory`, `trbOrmPreloadCategoryProducts`,
		`TrbOrmCategoryPreloadProducts`, `func(scope trbcontext.Context, transaction *orm.TrbOrmTransaction, values []*Category) *orm.DbError`,
		`TrbOrmProductQueryWhere(targetQuery, []string{"category_id"}, []string{"IN"}, []any{arguments})`,
		`trbOrmPreloadCategoryProduct`, `database has_one association returned multiple rows`,
		`TrbOrmAssociationCategory`, `TrbOrmAssociationProducts`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated association query is missing %q:\n%s", expected, output)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", output, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid association Go: %v\n%s", err, output)
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
	}
	context := languageservice.BuildContext(programs, "main")
	complete := func(source string) []languageservice.CompletionItem {
		return languageservice.Complete(languageservice.CompletionRequest{Source: source, Cursor: len(source), Mode: "go", Context: context})
	}
	findCompletion := func(items []languageservice.CompletionItem, name string) (languageservice.CompletionItem, bool) {
		for _, item := range items {
			if item.Label == name {
				return item, true
			}
		}
		return languageservice.CompletionItem{}, false
	}
	association, ok := findCompletion(complete("product := Product.new()\nproduct.cat"), "category")
	if !ok || association.Kind != languageservice.CompletionField || association.InsertText != "category" || association.Detail != "category: DbResult<Category?>" {
		t.Fatalf("unexpected association property completion: %#v", association)
	}
	for _, name := range []string{"load", "loaded?", "reload"} {
		if _, ok := findCompletion(complete("product := Product.new()\nproduct.category."+name[:1]), name); !ok {
			t.Fatalf("missing association member completion %s", name)
		}
	}
	all, ok := findCompletion(complete("Product.al"), "all")
	if !ok || all.Detail != "all(): DbResult<Array<Product>>" {
		t.Fatalf("unexpected ORM effect completion: %#v", all)
	}
	compileInvalidJoin := func(expression string) error {
		invalidSource := []byte(`import { Model, belongs_to } from trb/orm

class Category < Model
end

class Product < Model
	belongs_to(Category)
end

def main()
	query := ` + expression + `
	puts(query.to_sql())
end
`)
		_, err := CompileProject([]SourceUnit{{
			Filename: filepath.Join(root, "src", "invalid.trb"), ModulePath: "invalid", Package: "main", Source: invalidSource,
		}}, Options{
			Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
			PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
		})
		return err
	}
	for _, invalid := range []struct {
		expression string
		want       string
	}{
		{expression: `Product.join(:missing)`, want: `argument 1 to join() must be one of "category"`},
		{expression: `Product.join(:category, Product.where())`, want: `has type ProductQuery, expected CategoryQuery`},
		{expression: `Product.where_exists(:missing)`, want: `argument 1 to where_exists() must be one of "category"`},
		{expression: `Product.where_exists(:category, Product.where())`, want: `has type ProductQuery, expected CategoryQuery`},
		{expression: `Product.preload(:missing)`, want: `argument 1 to preload() must be one of "category"`},
		{expression: `Product.where().preload(:category, Product.where())`, want: `has type ProductQuery, expected CategoryQuery`},
		{expression: `Product.group(:missing)`, want: `argument 1 to group() must be one of`},
		{expression: `Product.group(:category_id).having(:sum, ">", 1)`, want: `argument 1 to having() must be one of "count"`},
		{expression: `Product.group(:category_id).sum(:name)`, want: `argument 1 to sum() must be one of`},
		{expression: `Product.where(category_id: Product.select(:name))`, want: `has type Subquery<String>`},
	} {
		if err := compileInvalidJoin(invalid.expression); err == nil || !strings.Contains(err.Error(), invalid.want) {
			t.Fatalf("expected join diagnostic %q, got %v", invalid.want, err)
		}
	}
}

func TestPortableORMThroughAssociationCompilesToGo(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
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
	source := []byte(`import { DbResult, Model, belongs_to, has_many } from trb/orm

class User < Model
	has_many(Membership)
	has_many(Project, through: :memberships) do |projects|
		projects.where(name: "TypeRB").order(id: :asc)
	end
end

class Project < Model
	has_many(Membership)
end

class Membership < Model
	belongs_to(User)
	belongs_to(Project)
end

def load_projects(): DbResult<Array<Project>>
	users := try User.all()
	joined_users := try User.join(:projects, Project.where(name: "TypeRB")).all()
	puts(joined_users.size())
	return users[0].projects
end

def main()
	puts(load_projects())
end
`)
	artifacts, err := CompileProject([]SourceUnit{{
		Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: source,
	}}, Options{
		Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		`TrbOrmMembershipAssociationPredicate(TrbOrmMembershipQueryWhere(TrbOrmMembershipUsing(`,
		`orm.TrbOrmJoin{Kind: "INNER JOIN", Table: "memberships", SourceColumn: "id", TargetColumn: "project_id"`,
		`TrbOrmProjectJoin(TrbOrmProjectUsing(`,
		`trbArrayIndex(users, 0).TrbOrmTransaction())`,
		`Build: func(arguments *[]any) string`,
		`trbOrmQuoteIdentifier("memberships")`,
		`func(projects TrbOrmProjectQuery) TrbOrmProjectQuery`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated through association query is missing %q:\n%s", expected, output)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", output, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid through association Go: %v\n%s", err, output)
	}
	invalidScope := []byte(strings.Replace(string(source), "do |projects|\n\t\tprojects.where(name: \"TypeRB\").order(id: :asc)", "do |_projects|\n\t\tUser.where()", 1))
	if _, err := CompileProject([]SourceUnit{{
		Filename: filepath.Join(root, "src", "invalid.trb"), ModulePath: "invalid", Package: "main", Source: invalidScope,
	}}, Options{
		Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	}); err == nil || !strings.Contains(err.Error(), "has_many block result has type UserQuery, expected ProjectQuery") {
		t.Fatalf("expected typed association scope diagnostic, got %v", err)
	}
}

func TestPortableORMCompilesModelImportedFromAnotherModule(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename: filepath.Join(root, "src", "models", "product.trb"), ModulePath: "models/product", Package: "models",
			Source: []byte("import { Model } from trb/orm\n\nclass Product < Model\nend\n"),
		},
		{
			Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main",
			Source: []byte("import { DbResult } from trb/orm\nimport { Product } from models/product\n\ndef load_products(): DbResult<Array<Product>>\n\treturn Product.where(name: \"Widget\").all()\nend\n\ndef main()\n\tputs(load_products())\n\tputs(Product.pluck(:name))\n\tputs(Product.pick(:name))\n\tputs(Product.ids())\n\tputs(Product.insert_all([Product.build(name: \"First\"), Product.build(name: \"Second\")]))\nend\n"),
		},
	}, Options{
		Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{}
	for _, artifact := range artifacts {
		outputs[artifact.AST.ModulePath] = string(artifact.Output)
	}
	modelOutput := outputs["models/product"]
	mainOutput := outputs["main"]
	for _, expected := range []string{"type TrbOrmProductQuery struct", "func TrbOrmProductWhere", "func TrbOrmLoadProduct", "func TrbOrmPluckProductName", "func TrbOrmPickProductName", "func (self *Product) TrbOrmColumnName() string"} {
		if !strings.Contains(modelOutput, expected) {
			t.Fatalf("generated model module is missing %q:\n%s", expected, modelOutput)
		}
	}
	for _, expected := range []string{
		"models.TrbOrmProductWhere", "models.TrbOrmLoadProduct", "orm.DbResult[[]*models.Product]",
		"models.TrbOrmPluckProductName", "models.TrbOrmPickProductName", "models.TrbOrmPluckProductId",
		"models.TrbOrmInsertAllProduct([]*models.ProductDraft{models.TrbOrmProductBuildScoped",
	} {
		if !strings.Contains(mainOutput, expected) {
			t.Fatalf("generated main module is missing %q:\n%s", expected, mainOutput)
		}
	}
	for modulePath, output := range outputs {
		if _, err := parser.ParseFile(token.NewFileSet(), modulePath+".go", output, parser.AllErrors); err != nil {
			t.Fatalf("generated invalid Go for %s: %v\n%s", modulePath, err, output)
		}
	}
}

func TestPortableORMResolvesAssociationTargetsAcrossModelModules(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE cross_module_categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE cross_module_products (
			id INTEGER PRIMARY KEY,
			category_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			FOREIGN KEY (category_id) REFERENCES cross_module_categories(id)
		);
		CREATE TABLE cross_module_profiles (
			id INTEGER PRIMARY KEY,
			category_id INTEGER NOT NULL UNIQUE,
			FOREIGN KEY (category_id) REFERENCES cross_module_categories(id)
		);
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	category := SourceUnit{
		Filename: filepath.Join(root, "src", "models", "category.trb"), ModulePath: "models/category", Package: "models",
		Source: []byte(`import { Model, has_many, has_one } from trb/orm

class CrossModuleCategory < Model
	has_many(CrossModuleProduct, foreign_key: :category_id, inverse: :cross_module_category, dependent: :destroy)
	has_one(CrossModuleProfile, foreign_key: :category_id, inverse: :cross_module_category)
end
`),
	}
	product := SourceUnit{
		Filename: filepath.Join(root, "src", "models", "product.trb"), ModulePath: "models/product", Package: "models",
		Source: []byte(`import { Model, belongs_to } from trb/orm

class CrossModuleProduct < Model
	belongs_to(CrossModuleCategory, foreign_key: :category_id, inverse: :cross_module_products)
end
`),
	}
	profile := SourceUnit{
		Filename: filepath.Join(root, "src", "models", "profile.trb"), ModulePath: "models/profile", Package: "models",
		Source: []byte(`import { Model, belongs_to } from trb/orm

record CrossModuleProfileSnapshot
	id: Integer
end

class CrossModuleProfile < Model
	belongs_to(CrossModuleCategory, foreign_key: :category_id, inverse: :cross_module_profile)
end
`),
	}
	main := SourceUnit{
		Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main",
		Source: []byte(`import { CrossModuleProduct } from models/product
import { CrossModuleProfileSnapshot } from models/profile
import { DbResult } from trb/orm

def snapshot_id(snapshot: CrossModuleProfileSnapshot): Integer
	return snapshot.id
end

def load_associations(): DbResult<Integer>
	products := try CrossModuleProduct.preload(:cross_module_category).all()
	product := products[0]
	category := try product.cross_module_category
	if category == nil
		return DbResult<Integer>::Ok(0)
	end
	related := try category.cross_module_products
	return DbResult<Integer>::Ok(related.size())
end

def main()
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{category, product, profile, main}, Options{
				Mode: mode, GoModule: "example.com/orm", RubyLoader: "zeitwerk", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
			})
			if err != nil {
				t.Fatal(err)
			}
			modules := map[string]*Artifact{}
			for _, artifact := range artifacts {
				modules[artifact.AST.ModulePath] = artifact
			}
			for _, modulePath := range []string{"models/category", "models/product", "models/profile"} {
				artifact := modules[modulePath]
				if artifact == nil {
					t.Fatalf("missing artifact for %s", modulePath)
				}
				for _, statement := range artifact.IR.Statements {
					imported, ok := statement.(*ir.Import)
					if ok && imported.Implicit && strings.HasPrefix(imported.Path, "models/") {
						t.Fatalf("model module %s received cyclic runtime import %s", modulePath, imported.Path)
					}
				}
			}
			entrypoint := modules["main"]
			if mode != "go" {
				loaded := map[string]bool{}
				for _, statement := range entrypoint.IR.Statements {
					if imported, ok := statement.(*ir.Import); ok && imported.RuntimeRequired {
						loaded[imported.Path] = true
					}
				}
				for _, modulePath := range []string{"models/category", "models/product", "models/profile"} {
					if !loaded[modulePath] {
						t.Fatalf("entrypoint did not bootstrap ORM model module %s", modulePath)
					}
				}
			}
			switch mode {
			case "go":
				if !strings.Contains(string(entrypoint.Output), `import "example.com/orm/models"`) {
					t.Fatalf("Go entrypoint did not retain its model-group import:\n%s", entrypoint.Output)
				}
			case "ruby":
				if !strings.Contains(string(entrypoint.Output), "models/category") || !strings.Contains(string(entrypoint.Output), "models/product") {
					t.Fatalf("Ruby entrypoint did not retain ORM bootstrap requires:\n%s", entrypoint.Output)
				}
			case "typescript":
				if !strings.Contains(string(entrypoint.Output), "models/category") || !strings.Contains(string(entrypoint.Output), "models/product") {
					t.Fatalf("TypeScript entrypoint did not retain ORM bootstrap imports:\n%s", entrypoint.Output)
				}
				if !strings.Contains(string(entrypoint.Output), `import "./models/profile.ts";`) {
					t.Fatalf("TypeScript type-only model import did not retain its runtime load:\n%s", entrypoint.Output)
				}
			}
		})
	}
}

func TestPortableORMResolvesThroughAssociationsAcrossModelModules(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE cross_file_users (id INTEGER PRIMARY KEY);
		CREATE TABLE cross_file_projects (id INTEGER PRIMARY KEY);
		CREATE TABLE cross_file_memberships (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			project_id INTEGER NOT NULL,
			FOREIGN KEY (user_id) REFERENCES cross_file_users(id),
			FOREIGN KEY (project_id) REFERENCES cross_file_projects(id)
		);
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	sources := []SourceUnit{
		{
			Filename: filepath.Join(root, "src", "models", "user.trb"), ModulePath: "models/user", Package: "models",
			Source: []byte(`import { Model, has_many } from trb/orm

class CrossFileUser < Model
	has_many(CrossFileMembership, foreign_key: :user_id)
	has_many(CrossFileProject, through: :cross_file_memberships, source: :cross_file_project)
end
`),
		},
		{
			Filename: filepath.Join(root, "src", "models", "project.trb"), ModulePath: "models/project", Package: "models",
			Source: []byte(`import { Model, has_many } from trb/orm

class CrossFileProject < Model
	has_many(CrossFileMembership, foreign_key: :project_id)
end
`),
		},
		{
			Filename: filepath.Join(root, "src", "models", "membership.trb"), ModulePath: "models/membership", Package: "models",
			Source: []byte(`import { Model, belongs_to } from trb/orm

class CrossFileMembership < Model
	belongs_to(CrossFileUser, foreign_key: :user_id)
	belongs_to(CrossFileProject, foreign_key: :project_id)
end
`),
		},
		{
			Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main",
			Source: []byte(`import { CrossFileUser } from models/user
import { DbResult } from trb/orm

def project_count(user: CrossFileUser): DbResult<Integer>
	projects := try user.cross_file_projects
	return DbResult<Integer>::Ok(projects.size())
end

def main()
	return
end
`),
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject(sources, Options{
				Mode: mode, GoModule: "example.com/orm", RubyLoader: "require_relative", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPortableORMRejectsModelImportsOfRunnableEntrypoint(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE bootstrap_products (id INTEGER PRIMARY KEY, state TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	main := SourceUnit{
		Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main",
		Source: []byte(`enum BootstrapState
	Active
end

def main()
	return
end
`),
	}
	model := SourceUnit{
		Filename: filepath.Join(root, "src", "models", "product.trb"), ModulePath: "models/product", Package: "models",
		Source: []byte(`import { BootstrapState } from main
import { Model, enum_column } from trb/orm

class BootstrapProduct < Model
	enum_column(:state, BootstrapState)
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, compileErrValue := CompileProject([]SourceUnit{main, model}, Options{
				Mode: mode, GoModule: "example.com/orm", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
			})
			var compileErr *CompileError
			if !errors.As(compileErrValue, &compileErr) || len(compileErr.Diagnostics) != 1 {
				t.Fatalf("expected one ORM bootstrap diagnostic, got %v", compileErrValue)
			}
			item := compileErr.Diagnostics[0]
			if item.Code != diagnostic.ProjectIntegration || item.Path != model.Filename || item.Span.Start.Offset != 0 {
				t.Fatalf("unexpected ORM bootstrap diagnostic: %#v", item)
			}
			if !strings.Contains(item.Message, "main -> models/product -> main") || !strings.Contains(item.Message, "move shared declarations into a separate module") {
				t.Fatalf("ORM bootstrap diagnostic does not explain the cycle: %q", item.Message)
			}
		})
	}
}

func TestPortableORMRejectsAssociationTargetsOutsideModelDirectory(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE cross_directory_categories (id INTEGER PRIMARY KEY);
		CREATE TABLE cross_directory_products (
			id INTEGER PRIMARY KEY,
			category_id INTEGER NOT NULL,
			FOREIGN KEY (category_id) REFERENCES cross_directory_categories(id)
		);
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	sources := []SourceUnit{
		{
			Filename: filepath.Join(root, "src", "models", "category.trb"), ModulePath: "models/category", Package: "models",
			Source: []byte("import { Model, has_many } from trb/orm\n\nclass CrossDirectoryCategory < Model\n\thas_many(CrossDirectoryProduct, foreign_key: :category_id)\nend\n"),
		},
		{
			Filename: filepath.Join(root, "src", "inventory", "product.trb"), ModulePath: "inventory/product", Package: "inventory",
			Source: []byte("import { Model } from trb/orm\n\nclass CrossDirectoryProduct < Model\nend\n"),
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, compileErrValue := CompileProject(sources, Options{
				Mode: mode, GoModule: "example.com/orm", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
			})
			var compileErr *CompileError
			if !errors.As(compileErrValue, &compileErr) || len(compileErr.Diagnostics) != 1 {
				t.Fatalf("expected one structured cross-directory diagnostic, got %v", compileErrValue)
			}
			item := compileErr.Diagnostics[0]
			wantOffset := strings.Index(string(sources[0].Source), "CrossDirectoryProduct")
			if item.Code != diagnostic.ProjectIntegration || item.Path != sources[0].Filename || item.Span.Start.Offset != wantOffset {
				t.Fatalf("unexpected cross-directory diagnostic: %#v", item)
			}
			if len(item.Related) != 1 || item.Related[0].Location.Path != sources[1].Filename {
				t.Fatalf("missing target model location: %#v", item.Related)
			}
			if !strings.Contains(item.Message, `model group "inventory"`) || !strings.Contains(item.Message, `is in "models"`) || !strings.Contains(item.Message, "query through an application repository") {
				t.Fatalf("cross-directory diagnostic does not explain the boundary: %q", item.Message)
			}
		})
	}
}

func TestPortableORMRequiresImportsOutsideAssociationTargets(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE reference_products (id INTEGER PRIMARY KEY)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	sources := []SourceUnit{
		{
			Filename: filepath.Join(root, "src", "models", "product.trb"), ModulePath: "models/product", Package: "models",
			Source: []byte("import { Model } from trb/orm\n\nclass ReferenceProduct < Model\nend\n"),
		},
		{
			Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main",
			Source: []byte("def products()\n\treturn ReferenceProduct.all()\nend\n"),
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject(sources, Options{
				Mode: mode, GoModule: "example.com/orm", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
			})
			if err == nil || !strings.Contains(err.Error(), "type ReferenceProduct is not declared or imported") {
				t.Fatalf("expected an explicit-import diagnostic, got %v", err)
			}
		})
	}
}

func TestPortableORMDeclarationReferencesStayAtClassBody(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE nested_categories (id INTEGER PRIMARY KEY);
		CREATE TABLE nested_products (id INTEGER PRIMARY KEY);
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	sources := []SourceUnit{
		{
			Filename: filepath.Join(root, "src", "models", "category.trb"), ModulePath: "models/category", Package: "models",
			Source: []byte(`import { Model, has_many } from trb/orm

class NestedCategory < Model
	def invalid_association()
		has_many(NestedProduct)
	end
end
`),
		},
		{
			Filename: filepath.Join(root, "src", "models", "product.trb"), ModulePath: "models/product", Package: "models",
			Source: []byte("import { Model } from trb/orm\n\nclass NestedProduct < Model\nend\n"),
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject(sources, Options{
				Mode: mode, GoModule: "example.com/orm", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
			})
			if err == nil || !strings.Contains(err.Error(), "type NestedProduct is not declared or imported") {
				t.Fatalf("expected declaration-only model reference to stay at class body, got %v", err)
			}
		})
	}
}

func TestPortableORMAllowsForeignKeysAcrossModelGroupsWithoutNavigation(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE external_accounts (id INTEGER PRIMARY KEY);
		CREATE TABLE audit_entries (
			id INTEGER PRIMARY KEY,
			external_account_id INTEGER NOT NULL,
			FOREIGN KEY (external_account_id) REFERENCES external_accounts(id)
		);
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	sources := []SourceUnit{
		{Filename: filepath.Join(root, "src", "accounts", "account.trb"), ModulePath: "accounts/account", Package: "accounts", Source: []byte("import { Model } from trb/orm\n\nclass ExternalAccount < Model\nend\n")},
		{Filename: filepath.Join(root, "src", "audit", "entry.trb"), ModulePath: "audit/entry", Package: "audit", Source: []byte("import { Model } from trb/orm\n\nclass AuditEntry < Model\nend\n")},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject(sources, Options{
				Mode: mode, GoModule: "example.com/orm", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTypeScriptORMRetainsImportedModelRegistration(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename: filepath.Join(root, "src", "models", "product.trb"), ModulePath: "models/product", Package: "models",
			Source: []byte("import { Model } from trb/orm\n\nclass Product < Model\nend\n"),
		},
		{
			Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main",
			Source: []byte("import { Product } from models/product\nimport { DbResult } from trb/orm\n\ndef main()\n\tcase Product.all()\n\twhen DbResult::Ok(products)\n\t\tputs(products.size())\n\twhen DbResult::Err(error)\n\t\tputs(error.message)\n\tend\nend\n"),
		},
	}, Options{
		Mode: "typescript", TypeScriptRuntime: "bun", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"` + databasePath + `"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{}
	for _, artifact := range artifacts {
		outputs[artifact.AST.ModulePath] = string(artifact.Output)
	}
	if output := outputs["models/product"]; !strings.Contains(output, `__trbOrm.registerModel(`) {
		t.Fatalf("generated TypeScript model does not register itself:\n%s", output)
	}
	mainOutput := outputs["main"]
	for _, expected := range []string{
		`import { Product } from "./models/product.ts"`,
		`__trbOrm.query(__trbOrm.modelName("Product", Product))`,
		`await __trbOrm.withScope(__trbScope`,
	} {
		if !strings.Contains(mainOutput, expected) {
			t.Fatalf("generated TypeScript consumer is missing %q:\n%s", expected, mainOutput)
		}
	}
}
