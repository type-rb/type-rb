package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunORMApplicationConformanceAcrossBackendsAndDatabases(t *testing.T) {
	requireLive := os.Getenv("TRB_REQUIRE_ORM_CONFORMANCE") == "1"
	for _, adapter := range []string{"sqlite", "postgresql", "mysql"} {
		adapter := adapter
		t.Run(adapter, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				mode := mode
				t.Run(mode, func(t *testing.T) {
					requireORMApplicationRuntime(t, mode, adapter)
					databaseSource, driver, available := replORMConformanceDatabase(t, adapter)
					if !available {
						if requireLive {
							t.Fatalf("%s ORM conformance requires its live database environment", adapter)
						}
						t.Skipf("set the live %s test database to run ORM conformance", adapter)
					}
					prepareORMApplicationConformanceSchema(t, driver, databaseSource, adapter)

					root := t.TempDir()
					config := project.New(root, mode)
					config.SourceDir = "src"
					config.OutDir = "build"
					if config.Go != nil {
						config.Go.Module = "example.com/type-rb/orm-application-conformance"
					}
					if config.TypeScript != nil {
						config.TypeScript.Runtime = project.TypeScriptRuntimeBun
						config.TypeScript.PackageManager = "bun"
					}
					config.PackageOptions["trb/orm"] = json.RawMessage(fmt.Sprintf(`{"adapter":%q,"database":%q}`, adapter, databaseSource))
					if err := config.Save(); err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(ormApplicationConformanceSource), 0o644); err != nil {
						t.Fatal(err)
					}

					var stdout, stderr bytes.Buffer
					command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
					if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
						t.Fatalf("run status=%d stderr=%s", status, stderr.String())
					}
					unexpectedStderr := mode != "go" && stderr.Len() != 0
					if stdout.String() != ormApplicationConformanceOutput || unexpectedStderr {
						t.Fatalf("unexpected %s/%s ORM output\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, adapter, ormApplicationConformanceOutput, stdout.String(), stderr.String())
					}
				})
			}
		})
	}
}

func TestRunORMBackedWebJSONAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			requireORMApplicationRuntime(t, mode, "sqlite")
			root := t.TempDir()
			databaseSource := filepath.Join(root, "web.sqlite3")
			database, err := sql.Open("sqlite", databaseSource)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec(`
				CREATE TABLE web_conformance_categories (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, state TEXT NOT NULL);
				CREATE TABLE web_conformance_products (id INTEGER PRIMARY KEY AUTOINCREMENT, category_id INTEGER NOT NULL, name TEXT NOT NULL UNIQUE, FOREIGN KEY (category_id) REFERENCES web_conformance_categories(id));
				INSERT INTO web_conformance_categories (name, state) VALUES ('books', 'active');
				INSERT INTO web_conformance_products (category_id, name) VALUES (1, 'first'), (1, 'second');
			`); err != nil {
				database.Close()
				t.Fatal(err)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}

			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/orm-web-conformance"
			}
			if config.TypeScript != nil {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			config.PackageOptions["trb/orm"] = json.RawMessage(fmt.Sprintf(`{"adapter":"sqlite","database":%q}`, databaseSource))
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}

			files := map[string]string{
				"main.trb": `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	response := dispatch(Request.new(method: HttpMethod.get(), path: "/products", query_string: "", headers: Headers.new(), body: Body.empty()))
	puts(response.status)
	puts(response.body.to_s())
	return
end
`,
				"models/category.trb": `import { WebConformanceState } from models/product
import { Model, enum_column, has_many } from trb/orm

class WebConformanceCategory < Model
	enum_column(:state, WebConformanceState)
	has_many(WebConformanceProduct, foreign_key: :category_id)
end
`,
				"models/product.trb": `import { Model, belongs_to } from trb/orm

enum WebConformanceState
	Active
end

class WebConformanceProduct < Model
	belongs_to(WebConformanceCategory, name: :category, foreign_key: :category_id)
end
`,
				"routes/products.trb": `import { WebConformanceProduct } from models/product
import { Context, Response } from trb/web
import { DbResult } from trb/orm

record WebProductResponse
	category: String
	id: Integer
	name: String
end

record WebDatabaseError
	message: String
end

def web_product_response(product: WebConformanceProduct): DbResult<WebProductResponse>
	category := try product.category
	return DbResult<WebProductResponse>::Ok(WebProductResponse.new(id: product.id, name: product.name, category: category.name))
end

def load_web_product_responses(): DbResult<Array<WebProductResponse>>
	products := try WebConformanceProduct.preload(:category).order(id: :asc).all()
	mut responses: Array<WebProductResponse> := []
	products.each do |product|
		responses.push(try web_product_response(product))
	end
	return DbResult<Array<WebProductResponse>>::Ok(responses)
end

def get(_context: Context): Response
	case load_web_product_responses()
	when DbResult::Ok(products)
		return Response.json(products)
	when DbResult::Err(error)
		return Response.json(WebDatabaseError.new(message: error.message), 500)
	end
end
`,
			}
			for relative, source := range files {
				path := filepath.Join(config.SourcePath(), relative)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("run status=%d stderr=%s", status, stderr.String())
			}
			const want = "200\n[{\"category\":\"books\",\"id\":1,\"name\":\"first\"},{\"category\":\"books\",\"id\":2,\"name\":\"second\"}]\n"
			unexpectedStderr := mode != "go" && stderr.Len() != 0
			if stdout.String() != want || unexpectedStderr {
				t.Fatalf("unexpected %s ORM-backed web output: want %q, got %q, stderr=%q", mode, want, stdout.String(), stderr.String())
			}
		})
	}
}

func requireORMApplicationRuntime(t *testing.T, mode, adapter string) {
	t.Helper()
	switch mode {
	case "ruby":
		driverGem := map[string]string{"sqlite": "sqlite3", "postgresql": "pg", "mysql": "mysql2"}[adapter]
		requireRubyORMGems(t, "sequel", driverGem)
	case "typescript":
		if _, err := exec.LookPath("bun"); err != nil {
			t.Skip("Bun is required for generated TypeScript ORM runtime conformance")
		}
	}
}

func prepareORMApplicationConformanceSchema(t *testing.T, driver, databaseSource, adapter string) {
	t.Helper()
	database, err := sql.Open(driver, databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	for _, table := range []string{"trb_conformance_profiles", "trb_conformance_memberships", "trb_conformance_projects", "trb_conformance_users", "trb_conformance_products", "trb_conformance_categories"} {
		if _, err := database.Exec("DROP TABLE IF EXISTS " + table); err != nil {
			t.Fatal(err)
		}
	}
	id := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if adapter == "postgresql" {
		id = "BIGSERIAL PRIMARY KEY"
	} else if adapter == "mysql" {
		id = "BIGINT AUTO_INCREMENT PRIMARY KEY"
	}
	statements := []string{
		"CREATE TABLE trb_conformance_categories (id " + id + ", name VARCHAR(255) NOT NULL UNIQUE)",
		"CREATE TABLE trb_conformance_products (id " + id + ", category_id BIGINT NOT NULL, name VARCHAR(255) NOT NULL UNIQUE, price DOUBLE PRECISION, active BOOLEAN NOT NULL, FOREIGN KEY (category_id) REFERENCES trb_conformance_categories(id))",
		"CREATE TABLE trb_conformance_users (id " + id + ", name VARCHAR(255) NOT NULL UNIQUE)",
		"CREATE TABLE trb_conformance_profiles (id " + id + ", user_id BIGINT NOT NULL UNIQUE, bio VARCHAR(255) NOT NULL, FOREIGN KEY (user_id) REFERENCES trb_conformance_users(id))",
		"CREATE TABLE trb_conformance_projects (id " + id + ", name VARCHAR(255) NOT NULL UNIQUE)",
		"CREATE TABLE trb_conformance_memberships (id " + id + ", user_id BIGINT NOT NULL, project_id BIGINT NOT NULL, FOREIGN KEY (user_id) REFERENCES trb_conformance_users(id), FOREIGN KEY (project_id) REFERENCES trb_conformance_projects(id))",
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

const ormApplicationConformanceSource = `import { Database, DbErrorKind, DbResult, Model, belongs_to, has_many, has_one } from trb/orm

class TrbConformanceCategory < Model
	has_many(TrbConformanceProduct, foreign_key: :category_id, dependent: :delete)
	has_many(TrbConformanceProduct, name: :active_products, foreign_key: :category_id) do |products|
		products.where(active: true).order(id: :asc)
	end
end

class TrbConformanceProduct < Model
	belongs_to(TrbConformanceCategory, name: :category, foreign_key: :category_id)
end

class TrbConformanceUser < Model
	has_many(TrbConformanceMembership, foreign_key: :user_id)
	has_many(TrbConformanceProject, through: :trb_conformance_memberships, source: :trb_conformance_project) do |projects|
		projects.where(name: "TypeRB").order(id: :asc)
	end
	has_one(TrbConformanceProfile, name: :profile, foreign_key: :user_id, inverse: :user)
end

class TrbConformanceProfile < Model
	belongs_to(TrbConformanceUser, name: :user, foreign_key: :user_id, inverse: :profile)
end

class TrbConformanceProject < Model
	has_many(TrbConformanceMembership, foreign_key: :project_id)
end

class TrbConformanceMembership < Model
	belongs_to(TrbConformanceUser, foreign_key: :user_id)
	belongs_to(TrbConformanceProject, foreign_key: :project_id)
end

def exercise(): DbResult<Integer>
	category := try TrbConformanceCategory.create(name: "books")
	puts(category.id > 0)
	puts(try TrbConformanceProduct.insert_all([
		TrbConformanceProduct.build(category_id: category.id, name: "first", price: 1.0, active: true),
		TrbConformanceProduct.build(category_id: category.id, name: "second", price: 3.0, active: false)
	]))

	loaded := try TrbConformanceCategory.preload(:active_products).first()
	loaded_active_products := try loaded.active_products
	puts(loaded_active_products.size())
	category_active_products := try category.active_products
	puts(category_active_products.size())
	puts(try TrbConformanceCategory.join(:active_products).count())
	puts(try TrbConformanceCategory.where_exists(:active_products).count())
	puts(try TrbConformanceCategory.where_not_exists(:active_products).count())
	product := try TrbConformanceProduct.order(id: :asc).first()
	puts(product.category.loaded?())
	reloaded_category := try product.category.reload()
	puts(reloaded_category.name)
	puts(product.category.loaded?())
	found_product := try TrbConformanceProduct.find(product.id)
	puts(found_product.name)
	found_by_product := try TrbConformanceProduct.find_by(name: "first")
	puts(found_by_product.id == product.id)
	puts(try TrbConformanceProduct.exists?(name: "first"))
	puts(try TrbConformanceProduct.not(active: false).count())
	puts(try TrbConformanceProduct.where(active: true).or(TrbConformanceProduct.where(name: "second")).count())
	puts(try TrbConformanceProduct.distinct().count())
	puts(try TrbConformanceProduct.join(:category).left_join(:category).count())
	puts(try TrbConformanceProduct.where(id: 1..1000000).count())
	puts(try TrbConformanceProduct.where("name", "LIKE", "%irs%").count())
	paged_products := try TrbConformanceProduct.order(id: :asc).limit(1).offset(1).all()
	puts(paged_products.size())

	user := try TrbConformanceUser.create(name: "Ada")
	primary := try TrbConformanceProject.create(name: "TypeRB")
	secondary := try TrbConformanceProject.create(name: "Other")
	_primary_membership := try TrbConformanceMembership.create(user_id: user.id, project_id: primary.id)
	_secondary_membership := try TrbConformanceMembership.create(user_id: user.id, project_id: secondary.id)
	user_projects := try user.trb_conformance_projects
	puts(user_projects.size())
	preloaded_user := try TrbConformanceUser.preload(:trb_conformance_projects).first()
	preloaded_projects := try preloaded_user.trb_conformance_projects
	puts(preloaded_projects.size())
	puts(try TrbConformanceUser.join(:trb_conformance_projects).count())
	profile := try TrbConformanceProfile.create(user_id: user.id, bio: "compiler")
	profile_user := try profile.user.reload()
	puts(profile_user.name)
	profile_from_user := try profile_user.profile
	puts(profile_from_user.bio)

	names := try TrbConformanceProduct.order(id: :asc).pluck(:name)
	puts(names.size())
	puts(try TrbConformanceProduct.order(id: :asc).pick(:name))
	ids := try TrbConformanceProduct.ids()
	puts(ids.size())
	puts(try TrbConformanceProduct.sum(:price))
	puts(try TrbConformanceProduct.average(:price))
	puts(try TrbConformanceProduct.minimum(:price))
	puts(try TrbConformanceProduct.maximum(:price))
	group_counts := try TrbConformanceProduct.group(:active).having(:count, ">=", 1).count()
	puts(group_counts.size())
	puts(TrbConformanceProduct.to_sql().size() > 0)
	explanation := try TrbConformanceProduct.explain()
	puts(explanation.size() > 0)
	puts(try TrbConformanceProduct.where(category_id: TrbConformanceCategory.select(:id)).count())
	case TrbConformanceProduct.lock().all()
	when DbResult::Ok(_products)
		puts(false)
	when DbResult::Err(error)
		puts(error.kind == DbErrorKind::InvalidData)
	end

	processed := TrbConformanceProduct.find_each(batch_size: 1) do |_product|
		_unused_id := _product.id
	end catch |_error|
		-1
	end
	puts(processed)
	saved := try TrbConformanceProduct.build(category_id: category.id, name: "saved", price: 2.0, active: true).save()
	puts(saved.id > 0)
	changed := try saved.with(price: 5.0).save()
	puts(changed.price)
	updated := try changed.update(name: "direct")
	puts(updated.name)
	puts(try updated.delete())
	puts(try TrbConformanceProduct.upsert_all([
		TrbConformanceProduct.build(category_id: category.id, name: "first", price: 4.0, active: true),
		TrbConformanceProduct.build(category_id: category.id, name: "second", price: 6.0, active: false)
	], unique_by: [:name], update: [:price]))
	puts(try TrbConformanceProduct.insert_if_absent(TrbConformanceProduct.build(category_id: category.id, name: "first", price: 4.0, active: true), unique_by: [:name]))
	upserted := try TrbConformanceProduct.build(category_id: category.id, name: "first", price: 4.0, active: true).upsert(unique_by: [:name], update: [:price])
	puts(upserted.price)
	puts(try TrbConformanceProduct.update_all(price: 9.0))
	puts(try TrbConformanceProduct.where(active: false).update_all(active: true))
	temporary := try TrbConformanceProduct.create(category_id: category.id, name: "temporary", price: 2.0, active: false)
	puts(temporary.id > 0)
	puts(try TrbConformanceProduct.where(name: "temporary").delete_all())
	puts(try TrbConformanceProduct.delete_all())

	_dependent := try TrbConformanceProduct.create(category_id: category.id, name: "dependent", price: 2.0, active: true)
	puts(try category.destroy())
	puts(try TrbConformanceProduct.count())
	lifecycle := try TrbConformanceCategory.create(name: "lifecycle")
	_destroy_all_product := try TrbConformanceProduct.create(category_id: lifecycle.id, name: "destroy-all", price: 2.0, active: true)
	puts(try TrbConformanceCategory.where(name: "lifecycle").destroy_all())
	puts(try TrbConformanceProduct.count())
	transaction_id := Database.transaction() do |outer|
		nested_id := try outer.transaction() do |inner|
			categories := TrbConformanceCategory.using(inner)
			inside := try categories.create(name: "transaction")
			_locked := try categories.lock().all()
			_locked_count := _locked.size()
			inside.id
		end
		nested_id
	end catch |_error|
		0
	end
	puts(transaction_id > 0)
	puts(try TrbConformanceCategory.where(id: transaction_id).delete_all())
	return DbResult<Integer>::Ok(1)
end

def main()
	case exercise()
	when DbResult::Ok(value)
		puts(value)
	when DbResult::Err(error)
		puts(error.kind)
		puts(error.message)
	end
end
`

const ormApplicationConformanceOutput = `true
2
1
1
1
1
0
false
books
true
first
true
true
1
2
2
2
2
1
1
1
1
1
Ada
compiler
2
first
2
4.0
2.0
1.0
3.0
2
true
true
2
true
2
true
5.0
direct
true
2
false
4.0
2
1
true
1
2
true
0
1
0
true
1
1
`
