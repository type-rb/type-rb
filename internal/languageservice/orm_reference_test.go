package languageservice_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
	_ "modernc.org/sqlite"
)

func TestORMModelReferencesCompleteWithoutImports(t *testing.T) {
	project := compileORMReferenceProject(t)
	context := project.contexts["models/category"]

	edited := "# shifts the last-good snapshot offsets\n" + strings.Replace(project.sources["models/category"], "Product", "Pro", 1)
	cursor := strings.Index(edited, "has_many(Pro") + len("has_many(Pro")
	items := languageservice.Complete(languageservice.CompletionRequest{Source: edited, Cursor: cursor, Mode: "go", Context: context})
	product, ok := findCompletion(items, "Product")
	if !ok {
		t.Fatalf("association completion labels=%v, want Product", labels(items))
	}
	if product.Kind != languageservice.CompletionType || product.Detail != "class Product" || len(product.AdditionalEdits) != 0 {
		t.Fatalf("association completion=%#v", product)
	}

	empty := strings.Replace(project.sources["models/category"], "Product", "", 1)
	cursor = strings.Index(empty, "has_many(") + len("has_many(")
	items = languageservice.Complete(languageservice.CompletionRequest{Source: empty, Cursor: cursor, Mode: "go", Context: context})
	if _, found := findCompletion(items, "InventoryProduct"); found {
		t.Fatalf("association completion leaked another model group: %v", labels(items))
	}
	if _, found := findCompletion(items, "Category"); !found {
		t.Fatalf("association completion omitted the self model: %v", labels(items))
	}

	secondArgument := strings.Replace(project.sources["models/category"], "has_many(Product)", "has_many(Product, name: Pro)", 1)
	cursor = strings.Index(secondArgument, "name: Pro") + len("name: Pro")
	items = languageservice.Complete(languageservice.CompletionRequest{Source: secondArgument, Cursor: cursor, Mode: "go", Context: context})
	if item, found := findCompletion(items, "Product"); found && item.Detail == "class Product" {
		t.Fatalf("model reference completion escaped the first argument: %#v", items)
	}

	outside := project.sources["models/category"] + "\nhas_many(Pro)\n"
	cursor = strings.LastIndex(outside, "Pro") + len("Pro")
	items = languageservice.Complete(languageservice.CompletionRequest{Source: outside, Cursor: cursor, Mode: "go", Context: context})
	if item, found := findCompletion(items, "Product"); found && item.Detail == "class Product" {
		t.Fatalf("model reference completion escaped its owning class: %#v", items)
	}

	movedOutside := strings.Replace(project.sources["models/category"], "\thas_many(Product)\nend", "end\nhas_many(Pro)", 1)
	cursor = strings.LastIndex(movedOutside, "Pro") + len("Pro")
	items = languageservice.Complete(languageservice.CompletionRequest{Source: movedOutside, Cursor: cursor, Mode: "go", Context: context})
	if item, found := findCompletion(items, "Product"); found && item.Detail == "class Product" {
		t.Fatalf("last-good model reference range escaped its current owner: %#v", items)
	}

	classStart := strings.Index(project.sources["models/category"], "class Category")
	removedOwner := project.sources["models/category"][:classStart] + "has_many(Pro)\n"
	cursor = strings.LastIndex(removedOwner, "Pro") + len("Pro")
	items = languageservice.Complete(languageservice.CompletionRequest{Source: removedOwner, Cursor: cursor, Mode: "go", Context: context})
	if item, found := findCompletion(items, "Product"); found && item.Detail == "class Product" {
		t.Fatalf("last-good model reference range survived a removed owner: %#v", items)
	}

	nested := strings.Replace(project.sources["models/category"], "\thas_many(Product)\nend", "\thas_many(Product)\n\n\tdef invalid_association()\n\t\thas_many(Pro)\n\tend\nend", 1)
	cursor = strings.LastIndex(nested, "Pro") + len("Pro")
	items = languageservice.Complete(languageservice.CompletionRequest{Source: nested, Cursor: cursor, Mode: "go", Context: context})
	if item, found := findCompletion(items, "Product"); found && item.Detail == "class Product" {
		t.Fatalf("model reference completion escaped into a method: %#v", items)
	}
}

func TestORMModelReferencesSupportHoverDefinitionAndRenameReferences(t *testing.T) {
	project := compileORMReferenceProject(t)
	categorySource := project.sources["models/category"]
	categoryPath := project.paths["models/category"]
	productPath := project.paths["models/product"]
	cursor := strings.Index(categorySource, "Product") + len("Prod")

	hover, ok := languageservice.Hover(languageservice.SemanticRequest{
		Path: categoryPath, Source: categorySource, Cursor: cursor, Mode: "go", Context: project.contexts["models/category"],
	})
	if !ok || hover.Detail != "class Product" {
		t.Fatalf("association hover=(%#v, %v)", hover, ok)
	}
	definition, ok := languageservice.Definition(languageservice.SemanticRequest{
		Path: categoryPath, Source: categorySource, Cursor: cursor, Mode: "go", Context: project.contexts["models/category"],
	})
	if !ok || definition.Path != productPath || definition.Name != "Product" || definition.ID == "" {
		t.Fatalf("association definition=(%#v, %v)", definition, ok)
	}

	documents := make([]languageservice.SemanticDocument, 0, len(project.sources))
	for modulePath, source := range project.sources {
		documents = append(documents, languageservice.SemanticDocument{
			Path: project.paths[modulePath], Source: source, Mode: "go", Context: project.contexts[modulePath],
		})
	}
	references, ok := languageservice.References(languageservice.SemanticRequest{
		Path: categoryPath, Source: categorySource, Cursor: cursor, Mode: "go", Context: project.contexts["models/category"],
	}, documents, true)
	if !ok || len(references) != 3 {
		t.Fatalf("association references=(%#v, %v), want declaration and two targets", references, ok)
	}
	declarations := 0
	for _, reference := range references {
		if reference.ID != definition.ID {
			t.Fatalf("reference resolved to another declaration: %#v", reference)
		}
		if reference.Declaration {
			declarations++
		}
	}
	if declarations != 1 {
		t.Fatalf("association references declarations=%d, want 1", declarations)
	}
	withoutDeclaration, ok := languageservice.References(languageservice.SemanticRequest{
		Path: categoryPath, Source: categorySource, Cursor: cursor, Mode: "go", Context: project.contexts["models/category"],
	}, documents, false)
	if !ok || len(withoutDeclaration) != 2 {
		t.Fatalf("rename references without declaration=(%#v, %v), want two", withoutDeclaration, ok)
	}
}

type ormReferenceProject struct {
	sources  map[string]string
	paths    map[string]string
	contexts map[string]languageservice.Context
}

func compileORMReferenceProject(t *testing.T) ormReferenceProject {
	t.Helper()
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE categories (id INTEGER PRIMARY KEY);
		CREATE TABLE products (id INTEGER PRIMARY KEY, category_id INTEGER NOT NULL, FOREIGN KEY (category_id) REFERENCES categories(id));
		CREATE TABLE reviews (id INTEGER PRIMARY KEY, product_id INTEGER NOT NULL, FOREIGN KEY (product_id) REFERENCES products(id));
		CREATE TABLE inventory_products (id INTEGER PRIMARY KEY);
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{
		"models/category":   "import { Model, has_many } from trb/orm\n\nclass Category < Model\n\thas_many(Product)\nend\n",
		"models/product":    "import { Model, belongs_to, has_many } from trb/orm\n\nclass Product < Model\n\tbelongs_to(Category)\n\thas_many(Review)\nend\n",
		"models/review":     "import { Model, belongs_to } from trb/orm\n\nclass Review < Model\n\tbelongs_to(Product)\nend\n",
		"inventory/product": "import { Model } from trb/orm\n\nclass InventoryProduct < Model\nend\n",
	}
	paths := map[string]string{}
	units := make([]compiler.SourceUnit, 0, len(sources))
	for modulePath, source := range sources {
		path := filepath.Join(root, "src", filepath.FromSlash(modulePath)+".trb")
		paths[modulePath] = path
		units = append(units, compiler.SourceUnit{
			Filename: path, ModulePath: modulePath, Package: filepath.Base(filepath.Dir(path)), Source: []byte(source),
		})
	}
	artifacts, err := compiler.CompileProject(units, compiler.Options{
		Mode: "go", GoModule: "example.com/orm-references", SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
		PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
	}
	return ormReferenceProject{sources: sources, paths: paths, contexts: languageservice.BuildContexts(programs)}
}
