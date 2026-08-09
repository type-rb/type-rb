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
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL, price REAL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	source := []byte(`import { Model } from trb/orm

class Product < Model
end

def main()
	products := Product.where(name: "Widget").all()
	products.each do |product|
		puts(product.name)
	end
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
	var output string
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
		if artifact.AST.ModulePath == "src/main" {
			output = string(artifact.Output)
			break
		}
	}
	if output == "" {
		t.Fatal("main artifact was not generated")
	}
	for _, expected := range []string{"type Product struct", "type trbOrmProductQuery struct", `SELECT \"id\", \"name\", \"price\" FROM \"products\"`, "modernc.org/sqlite", "trbOrmLoadProduct"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated Go is missing %q:\n%s", expected, output)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", output, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid Go: %v\n%s", err, output)
	}
	context := languageservice.BuildContext(programs, "src/main")
	assertORMCompletionContext(t, context)
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
	foundWhere := false
	for _, member := range product.Members {
		foundWhere = foundWhere || member.Name == "where"
	}
	if !foundWhere {
		t.Fatalf("Product.where is missing from completion context: %#v", product.Members)
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
	if err == nil || !strings.Contains(err.Error(), "expected Integer") {
		t.Fatalf("expected schema type error, got %v", err)
	}
}
