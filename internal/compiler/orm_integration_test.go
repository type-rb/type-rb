package compiler

import (
	"database/sql"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

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

type ProductList = Array<Product>

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
	for _, name := range []string{"where", "not", "find", "build", "create", "insert_all", "insert_if_absent", "upsert_all", "find_each", "find_in_batches"} {
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
	for _, name := range []string{"where", "not", "or", "order", "limit", "offset", "all", "first", "count", "to_sql", "explain", "find_each", "find_in_batches"} {
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
			Source: []byte("import { Model } from trb/orm\nclass Product < Model\nend\ndef main()\n\t" + call + "\nend\n"),
		}}, Options{
			Mode: "go", GoModule: "example.com/orm", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
			PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
		})
	}
	artifacts, err := compile(`Product.create(name: "Widget")`)
	if err != nil {
		t.Fatal(err)
	}
	if output := string(artifacts[0].Output); !strings.Contains(output, `TrbOrmCreateProduct([]string{"name"}, []any{"Widget"})`) {
		t.Fatalf("generated create call does not preserve schema keywords:\n%s", output)
	}
	artifacts, err = compile(`Product.build(name: "Widget").save()`)
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmSaveProductDraft(TrbOrmBuildProduct([]string{"name"}, []any{"Widget"}))`) {
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
	if !strings.Contains(output, `TrbOrmInsertAllProduct([]*ProductDraft{TrbOrmBuildProduct`) {
		t.Fatalf("generated strict bulk insert does not use typed drafts:\n%s", output)
	}
	artifacts, err = compile(`puts(Product.insert_if_absent(Product.build(name: "Unique"), unique_by: [:name]))`)
	if err != nil {
		t.Fatal(err)
	}
	output = string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmInsertProductIfAbsent(TrbOrmBuildProduct([]string{"name"}, []any{"Unique"}), []string{"name"})`) {
		t.Fatalf("generated insert_if_absent does not preserve unique_by:\n%s", output)
	}
	artifacts, err = compile(`puts(Product.build(name: "Unique", price: 1.0).upsert(unique_by: [:name], update: [:price]))`)
	if err != nil {
		t.Fatal(err)
	}
	output = string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmUpsertProduct(TrbOrmBuildProduct`) || !strings.Contains(output, `[]string{"name"}, []string{"price"}`) {
		t.Fatalf("generated upsert does not preserve conflict and update columns:\n%s", output)
	}
	artifacts, err = compile(`puts(Product.upsert_all([Product.build(name: "First", price: 1.0), Product.build(price: 2.0, name: "Second")], unique_by: [:name], update: [:price]))`)
	if err != nil {
		t.Fatal(err)
	}
	output = string(artifacts[0].Output)
	if !strings.Contains(output, `TrbOrmUpsertAllProduct([]*ProductDraft{TrbOrmBuildProduct`) || !strings.Contains(output, `[]string{"name"}, []string{"price"}`) {
		t.Fatalf("generated upsert_all does not use typed drafts and literal columns:\n%s", output)
	}
	for _, test := range []struct {
		call string
		want string
	}{
		{call: `Product.create(price: 10.0)`, want: "create() is missing required argument name"},
		{call: `Product.create(name: "Widget", price: "wrong")`, want: "has type String, expected Float?"},
		{call: `Product.create(name: "Widget", missing: true)`, want: "create() has no keyword argument missing"},
		{call: `Product.build(price: 10.0)`, want: "build() is missing required argument name"},
		{call: `Product.build(name: "Widget", price: "wrong")`, want: "has type String, expected Float?"},
		{call: `Product.build(name: "Widget", missing: true)`, want: "build() has no keyword argument missing"},
		{call: `Product.new().with(id: 2)`, want: "with() has no keyword argument id"},
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
	Product.where("price", ">=", 10).where(id: ids).not(id: 3...5).where(discount: nil).not(discount: nil).all()
end
`
	artifacts, err := compile(valid)
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		`[]string{"price"}`, `[]string{">="}`, `[]string{"id"}`, `[]string{"IN"}`,
		`[]string{"RANGE_EXCLUSIVE"}`, `trbOrmRange{start: 3, end: 5}`,
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
		return compileSource("import { Model } from trb/orm\nclass Product < Model\nend\ndef main()\n" + body + "\nend\n")
	}
	artifacts, err := compile("\tquery := Product.where(\"price\", \">=\", 10).not(name: \"Deleted\").or(Product.where(name: \"Widget\")).order(price: :desc).limit(5).offset(1)\n\tputs(Product.not(name: \"Deleted\").to_sql())\n\tputs(query.to_sql())\n\tputs(query.explain())\n\tputs(query.count())\n\tputs(query.first())\n\tputs(query.all())")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		"TrbOrmProductQueryWhere", "TrbOrmProductNot", "TrbOrmProductQueryNot", "TrbOrmProductQueryOr",
		"trbOrmProductPredicateSQL", "TrbOrmProductOrder", "TrbOrmProductLimit", "TrbOrmProductOffset",
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
	for _, expected := range []string{"func ProcessProducts() orm.DbResult[int]", "return orm.NewDbResultErr[int]", "return orm.NewDbResultOk[int]"} {
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
	}
	for _, test := range invalid {
		if _, err := compile(test.body); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected query composition diagnostic %q, got %v", test.want, err)
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
		source := "import { Model } from trb/orm\nclass Product < Model\nend\ndef main()\n" + body + "\nend\n"
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
		`"batch queries do not accept order, limit, or offset"`, "reassigned = orm.NewDbResultOk[int]",
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
		{body: "\tProduct.find_each(batch_size: 1.5) do |product|\n\t\tputs(product.name)\n\tend", want: "has type Float, expected Integer"},
		{body: "\tProduct.find_each() do |product|\n\t\tputs(1)\n\tend", want: "block parameter product is not used"},
		{body: "\tProduct.find_in_batches() do |left, right|\n\t\tputs(left)\n\t\tputs(right)\n\tend", want: "find_in_batches block expects 1 parameter(s), got 2"},
		{body: "\tProduct.find_each() do |product|\n\t\tputs(product.name)\n\tend", want: "result of find_each() must be assigned or returned"},
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
	source := []byte(`import { DbResult, Model, belongs_to, has_many } from trb/orm

class Category < Model
	has_many(Product)
end

class Product < Model
	belongs_to(Category)
end

def main()
	case Product.where().preload(:category).all()
	when DbResult::Ok(products)
		products.each do |product|
			puts(product.category())
			puts(product.category_query().count())
		end
	when DbResult::Err(error)
		puts(error.message)
	end
	case Category.where().preload(:products).all()
	when DbResult::Ok(categories)
		categories.each do |category|
			puts(category.products().size())
			puts(category.products_query().count())
		end
	when DbResult::Err(error)
		puts(error.message)
	end
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
		`TrbOrmCategoryWhere([]string{"id"}, []string{"="}, []any{product.TrbOrmColumnCategoryId()})`,
		`TrbOrmProductWhere([]string{"category_id"}, []string{"="}, []any{category.TrbOrmColumnId()})`,
		`TrbOrmProductPreload`, `trbOrmPreloadProductCategory`, `trbOrmPreloadCategoryProducts`,
		`TrbOrmAssociationCategory`, `TrbOrmAssociationProducts`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated association query is missing %q:\n%s", expected, output)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", output, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid association Go: %v\n%s", err, output)
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
			Source: []byte("import { DbResult } from trb/orm\nimport { Product } from models/product\n\ndef load_products(): DbResult<Array<Product>>\n\treturn Product.where(name: \"Widget\").all()\nend\n\ndef main()\n\tputs(load_products())\n\tputs(Product.insert_all([Product.build(name: \"First\"), Product.build(name: \"Second\")]))\nend\n"),
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
	for _, expected := range []string{"type TrbOrmProductQuery struct", "func TrbOrmProductWhere", "func TrbOrmLoadProduct", "func (self *Product) TrbOrmColumnName() string"} {
		if !strings.Contains(modelOutput, expected) {
			t.Fatalf("generated model module is missing %q:\n%s", expected, modelOutput)
		}
	}
	for _, expected := range []string{
		"models.TrbOrmProductWhere", "models.TrbOrmLoadProduct", "orm.DbResult[[]*models.Product]",
		"models.TrbOrmInsertAllProduct([]*models.ProductDraft{models.TrbOrmBuildProduct",
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
