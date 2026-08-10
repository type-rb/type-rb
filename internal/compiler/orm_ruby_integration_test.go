package compiler

import (
	"database/sql"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestPortableORMCompilesRubyRuntimeAndTypedIntrinsics(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE)`,
		`CREATE TABLE products (id INTEGER PRIMARY KEY, category_id INTEGER, name TEXT NOT NULL UNIQUE, price REAL, active BOOLEAN NOT NULL, FOREIGN KEY(category_id) REFERENCES categories(id))`,
	} {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	source := []byte(`import { Database, DbError, Model, belongs_to, has_many } from trb/orm

class Category < Model
	has_many(Product)
end

class Product < Model
	belongs_to(Category)
end

def query_products(): Integer fails DbError
	base := Product.where(active: true).not(name: "hidden")
	combined := base.or(Product.where(id: 10..20))
	loaded := combined.order(name: :asc).limit(10).preload(:category).all()
	return loaded.size()
end

def write_product(): Boolean fails DbError
	draft := Product.build(name: "created", active: true)
	created := draft.save()
	updated := created.update(name: "updated")
	return updated.delete()
end

def mutate_products(): Integer fails DbError
	updated := Product.update_all(active: false)
	deleted := Product.delete_all()
	return updated + deleted
end

def transaction_count(): Integer fails DbError
	return Database.transaction() do |tx|
		Product.using(tx).lock().count()
	end
end

def batch_count(): Integer fails DbError
	return Product.find_each(batch_size: 10) do |product|
		puts(product.name)
	end
end

def main()
	puts(attempt query_products())
	puts(attempt write_product())
	puts(attempt mutate_products())
	puts(attempt transaction_count())
	puts(attempt batch_count())
end
`)
	artifacts, err := CompileProject([]SourceUnit{{
		Filename: filepath.Join(root, "src", "main.trb"), Source: source, ModulePath: "main", Package: "main",
	}}, Options{
		Mode: "ruby", RubyLoader: "require_relative", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	var mainOutput, ormOutput string
	for _, artifact := range artifacts {
		source := string(artifact.Output)
		assertCompiledRubySyntax(t, source)
		switch artifact.IR.ModulePath {
		case "main":
			mainOutput = source
		case "trb/orm/index":
			ormOutput = source
		}
	}
	for _, expected := range []string{
		`TrbOrmRuntime.query(Product).where([["active", "=", true]])`,
		`.not([["name", "=", "hidden"]])`, ".or_query(", ".order_by(", ".preload_association(", ".all_result",
		"TrbOrmRuntime.build(", "TrbOrmRuntime.save_draft_result(", "TrbOrmRuntime.update_model_result(", "TrbOrmRuntime.delete_model_result(",
		"TrbOrmRuntime.query(Product).update_all_result({\"active\" => false})", "TrbOrmRuntime.query(Product).delete_all_result",
		"TrbOrmRuntime.transaction_result(nil)", ".lock_rows.count_result", ".each_batch(10)",
	} {
		if !strings.Contains(mainOutput, expected) {
			t.Fatalf("generated Ruby ORM application is missing %q:\n%s", expected, mainOutput)
		}
	}
	for _, expected := range []string{
		`require "sequel"`, `TrbOrmRuntime.configure(adapter: "sqlite"`, "class Query", "def destroy_model_result",
	} {
		if !strings.Contains(ormOutput, expected) {
			t.Fatalf("generated Ruby ORM package is missing %q:\n%s", expected, ormOutput)
		}
	}
}

func assertCompiledRubySyntax(t *testing.T, source string) {
	t.Helper()
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby is not installed")
	}
	command := exec.Command("ruby", "-c")
	command.Stdin = strings.NewReader(source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated invalid Ruby: %v\n%s\n%s", err, output, source)
	}
}
