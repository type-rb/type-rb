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

func TestRunTypeScriptBunORMConformance(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("Bun is required for generated TypeScript ORM runtime conformance")
	}
	requireLive := os.Getenv("TRB_REQUIRE_ORM_CONFORMANCE") == "1"
	for _, adapter := range []string{"sqlite", "postgresql", "mysql"} {
		adapter := adapter
		t.Run(adapter, func(t *testing.T) {
			databaseSource, driver, available := replORMConformanceDatabase(t, adapter)
			if !available {
				if requireLive {
					t.Fatalf("%s Bun ORM conformance requires its live database environment", adapter)
				}
				t.Skipf("set the live %s test database to run Bun ORM conformance", adapter)
			}
			prepareTypeScriptBunORMSchema(t, driver, databaseSource, adapter)

			root := t.TempDir()
			config := project.New(root, "typescript")
			config.SourceDir = "src"
			config.OutDir = "build"
			config.TypeScript.Runtime = project.TypeScriptRuntimeBun
			config.TypeScript.PackageManager = "bun"
			config.PackageOptions["trb/orm"] = json.RawMessage(fmt.Sprintf(`{"adapter":%q,"database":%q}`, adapter, databaseSource))
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(typeScriptBunORMConformanceSource), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("run status=%d stderr=%s", status, stderr.String())
			}
			if stdout.String() != typeScriptBunORMConformanceOutput || stderr.Len() != 0 {
				t.Fatalf("unexpected TypeScript/Bun %s ORM output\nwant:\n%s\ngot:\n%s\nstderr:\n%s", adapter, typeScriptBunORMConformanceOutput, stdout.String(), stderr.String())
			}
		})
	}
}

func prepareTypeScriptBunORMSchema(t *testing.T, driver, databaseSource, adapter string) {
	t.Helper()
	database, err := sql.Open(driver, databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	for _, table := range []string{"trb_bun_profiles", "trb_bun_memberships", "trb_bun_projects", "trb_bun_users", "trb_bun_products", "trb_bun_categories"} {
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
		"CREATE TABLE trb_bun_categories (id " + id + ", name VARCHAR(255) NOT NULL UNIQUE)",
		"CREATE TABLE trb_bun_products (id " + id + ", category_id BIGINT NOT NULL, name VARCHAR(255) NOT NULL UNIQUE, price DOUBLE PRECISION, active BOOLEAN NOT NULL, FOREIGN KEY (category_id) REFERENCES trb_bun_categories(id))",
		"CREATE TABLE trb_bun_users (id " + id + ", name VARCHAR(255) NOT NULL UNIQUE)",
		"CREATE TABLE trb_bun_profiles (id " + id + ", user_id BIGINT NOT NULL UNIQUE, bio VARCHAR(255) NOT NULL, FOREIGN KEY (user_id) REFERENCES trb_bun_users(id))",
		"CREATE TABLE trb_bun_projects (id " + id + ", name VARCHAR(255) NOT NULL UNIQUE)",
		"CREATE TABLE trb_bun_memberships (id " + id + ", user_id BIGINT NOT NULL, project_id BIGINT NOT NULL, FOREIGN KEY (user_id) REFERENCES trb_bun_users(id), FOREIGN KEY (project_id) REFERENCES trb_bun_projects(id))",
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

const typeScriptBunORMConformanceSource = `import { Database, DbError, DbErrorKind, Model, belongs_to, has_many, has_one } from trb/orm
import { Result } from trb/std/result

class TrbBunCategory < Model
	has_many(TrbBunProduct, foreign_key: :category_id, dependent: :delete)
	has_many(TrbBunProduct, name: :active_products, foreign_key: :category_id) do |products|
		products.where(active: true).order(id: :asc)
	end
end

class TrbBunProduct < Model
	belongs_to(TrbBunCategory, name: :category, foreign_key: :category_id)
end

class TrbBunUser < Model
	has_many(TrbBunMembership, foreign_key: :user_id)
	has_many(TrbBunProject, through: :trb_bun_memberships, source: :trb_bun_project) do |projects|
		projects.where(name: "TypeRB").order(id: :asc)
	end
	has_one(TrbBunProfile, name: :profile, foreign_key: :user_id, inverse: :user)
end

class TrbBunProfile < Model
	belongs_to(TrbBunUser, name: :user, foreign_key: :user_id, inverse: :profile)
end

class TrbBunProject < Model
	has_many(TrbBunMembership, foreign_key: :project_id)
end

class TrbBunMembership < Model
	belongs_to(TrbBunUser, foreign_key: :user_id)
	belongs_to(TrbBunProject, foreign_key: :project_id)
end

def exercise(): Integer fails DbError
	category := TrbBunCategory.create(name: "books")
	puts(category.id > 0)
	puts(TrbBunProduct.insert_all([
		TrbBunProduct.build(category_id: category.id, name: "first", price: 1.0, active: true),
		TrbBunProduct.build(category_id: category.id, name: "second", price: 3.0, active: false)
	]))

	loaded := TrbBunCategory.preload(:active_products).first()
	puts(loaded.active_products.size())
	puts(category.active_products.size())
	puts(TrbBunCategory.join(:active_products).count())
	puts(TrbBunCategory.where_exists(:active_products).count())
	puts(TrbBunCategory.where_not_exists(:active_products).count())
	product := TrbBunProduct.order(id: :asc).first()
	puts(product.category.loaded?())
	puts(product.category.reload().name)
	puts(product.category.loaded?())
	puts(TrbBunProduct.find(product.id).name)
	puts(TrbBunProduct.find_by(name: "first").id == product.id)
	puts(TrbBunProduct.exists?(name: "first"))
	puts(TrbBunProduct.not(active: false).count())
	puts(TrbBunProduct.where(active: true).or(TrbBunProduct.where(name: "second")).count())
	puts(TrbBunProduct.distinct().count())
	puts(TrbBunProduct.join(:category).left_join(:category).count())
	puts(TrbBunProduct.where(id: 1..1000000).count())
	puts(TrbBunProduct.order(id: :asc).limit(1).offset(1).all().size())

	user := TrbBunUser.create(name: "Ada")
	primary := TrbBunProject.create(name: "TypeRB")
	secondary := TrbBunProject.create(name: "Other")
	TrbBunMembership.create(user_id: user.id, project_id: primary.id)
	TrbBunMembership.create(user_id: user.id, project_id: secondary.id)
	puts(user.trb_bun_projects.size())
	puts(TrbBunUser.preload(:trb_bun_projects).first().trb_bun_projects.size())
	puts(TrbBunUser.join(:trb_bun_projects).count())
	profile := TrbBunProfile.create(user_id: user.id, bio: "compiler")
	puts(profile.user.reload().name)
	puts(profile.user.profile.bio)

	puts(TrbBunProduct.order(id: :asc).pluck(:name).size())
	puts(TrbBunProduct.order(id: :asc).pick(:name))
	puts(TrbBunProduct.ids().size())
	puts(TrbBunProduct.sum(:price))
	puts(TrbBunProduct.average(:price))
	puts(TrbBunProduct.minimum(:price))
	puts(TrbBunProduct.maximum(:price))
	puts(TrbBunProduct.group(:active).having(:count, ">=", 1).count().size())
	puts(TrbBunProduct.to_sql().size() > 0)
	puts(TrbBunProduct.explain().size() > 0)
	puts(TrbBunProduct.where(category_id: TrbBunCategory.select(:id)).count())
	case attempt TrbBunProduct.lock().all()
	when Result::Ok(_products)
		puts(false)
	when Result::Err(error)
		puts(error.kind == DbErrorKind::InvalidData)
	end

	processed := TrbBunProduct.find_each(batch_size: 1) do |_product|
		_unused_id := _product.id
	end
	puts(processed)
	saved := TrbBunProduct.build(category_id: category.id, name: "saved", price: 2.0, active: true).save()
	puts(saved.id > 0)
	changed := saved.with(price: 5.0).save()
	puts(changed.price)
	updated := changed.update(name: "direct")
	puts(updated.name)
	puts(updated.delete())
	puts(TrbBunProduct.upsert_all([
		TrbBunProduct.build(category_id: category.id, name: "first", price: 4.0, active: true),
		TrbBunProduct.build(category_id: category.id, name: "second", price: 6.0, active: false)
	], unique_by: [:name], update: [:price]))
	puts(TrbBunProduct.insert_if_absent(TrbBunProduct.build(category_id: category.id, name: "first", price: 4.0, active: true), unique_by: [:name]))
	upserted := TrbBunProduct.build(category_id: category.id, name: "first", price: 4.0, active: true).upsert(unique_by: [:name], update: [:price])
	puts(upserted.price)
	puts(TrbBunProduct.update_all(price: 9.0))
	puts(TrbBunProduct.where(active: false).update_all(active: true))
	temporary := TrbBunProduct.create(category_id: category.id, name: "temporary", price: 2.0, active: false)
	puts(temporary.id > 0)
	puts(TrbBunProduct.where(name: "temporary").delete_all())
	puts(TrbBunProduct.delete_all())

	TrbBunProduct.create(category_id: category.id, name: "dependent", price: 2.0, active: true)
	puts(category.destroy())
	puts(TrbBunProduct.count())
	lifecycle := TrbBunCategory.create(name: "lifecycle")
	TrbBunProduct.create(category_id: lifecycle.id, name: "destroy-all", price: 2.0, active: true)
	puts(TrbBunCategory.where(name: "lifecycle").destroy_all())
	puts(TrbBunProduct.count())
	transaction_id := Database.transaction() do |outer|
		nested_id := outer.transaction() do |inner|
			categories := TrbBunCategory.using(inner)
			inside := categories.create(name: "transaction")
			_locked := categories.lock().all()
			_locked_count := _locked.size()
			inside.id
		end
		nested_id
	end
	puts(transaction_id > 0)
	puts(TrbBunCategory.where(id: transaction_id).delete_all())
	return 1
end

def main()
	case attempt exercise()
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.kind)
		puts(error.message)
	end
end
`

const typeScriptBunORMConformanceOutput = `true
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
