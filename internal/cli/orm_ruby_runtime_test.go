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

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/type-rb/type-rb/internal/project"
)

func TestRubyORMNativeDependenciesFollowTheSelectedAdapter(t *testing.T) {
	for _, test := range []struct {
		adapter      string
		database     string
		dependencies map[string]string
	}{
		{adapter: "sqlite", database: "application.sqlite3", dependencies: map[string]string{"sequel": "latest", "sqlite3": "latest"}},
		{adapter: "postgresql", database: "postgres://localhost/app", dependencies: map[string]string{"pg": "latest", "sequel": "latest"}},
		{adapter: "mysql", database: "root@tcp(localhost)/app", dependencies: map[string]string{"mysql2": "latest", "sequel": "latest"}},
	} {
		t.Run(test.adapter, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, "ruby")
			config.SourceDir = "src"
			config.PackageOptions["trb/orm"] = json.RawMessage(fmt.Sprintf(`{"adapter":%q,"database":%q}`, test.adapter, test.database))
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(config.SourcePath(), "main.trb")
			if err := os.WriteFile(sourcePath, []byte("import { Model } from trb/orm\nclass Product < Model\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			dependencies, err := projectPackageDependencies(config, []string{sourcePath})
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(dependencies) != fmt.Sprint(test.dependencies) {
				t.Fatalf("unexpected %s Ruby dependencies: want %#v, got %#v", test.adapter, test.dependencies, dependencies)
			}
		})
	}
}

func TestRunRubyORMApplicationWithSQLite(t *testing.T) {
	requireRubyORMGems(t, "sequel", "sqlite3")
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	prepareRubyORMRuntimeSchema(t, database, "sqlite")
	runRubyORMApplication(t, root, "sqlite", databasePath)
}

func TestRunRubyORMApplicationWithLiveDatabases(t *testing.T) {
	for _, test := range []struct {
		name, adapter, environment, driver, gem string
	}{
		{name: "postgresql", adapter: "postgresql", environment: "TRB_TEST_POSTGRESQL_DATABASE", driver: "pgx", gem: "pg"},
		{name: "mysql", adapter: "mysql", environment: "TRB_TEST_MYSQL_DATABASE", driver: "mysql", gem: "mysql2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			databaseSource := os.Getenv(test.environment)
			if strings.TrimSpace(databaseSource) == "" {
				t.Skipf("set %s to run the live Ruby ORM application test", test.environment)
			}
			requireRubyORMGems(t, "sequel", test.gem)
			database, err := sql.Open(test.driver, databaseSource)
			if err != nil {
				t.Fatal(err)
			}
			prepareRubyORMRuntimeSchema(t, database, test.adapter)
			root := t.TempDir()
			runRubyORMApplication(t, root, test.adapter, databaseSource)
		})
	}
}

func TestRubyProjectREPLUsesPortableORMSchema(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	prepareRubyORMRuntimeSchema(t, database, "sqlite")
	config := rubyORMRuntimeConfig(t, root, "sqlite", databasePath)
	input := "import { exercise } from main\nexercise()\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	const want = "Tools\nCompiler 2\n2\ntrue\nfalse\n3\n3\n3\n3\n1\n0\nPrimary\n2\n3\ntrue\n7\n4\n4\n1\n0 : Integer\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected Ruby project ORM REPL result: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func runRubyORMApplication(t *testing.T, root, adapter, databaseSource string) {
	t.Helper()
	config := rubyORMRuntimeConfig(t, root, adapter, databaseSource)
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("%s build status=%d stderr=%s", adapter, status, stderr.String())
	}
	generated, err := os.ReadFile(filepath.Join(config.OutputPath(), "main.rb"))
	if err != nil || !strings.Contains(string(generated), `name: "trb_ruby_test_products"`) ||
		!strings.Contains(string(generated), `scope: ->(products) { products.where([["active", "=", true]]) }`) {
		t.Fatalf("%s generated Ruby omitted typed association scopes: err=%v\n%s", adapter, err, generated)
	}
	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
		t.Fatalf("%s status=%d stdout=%s stderr=%s", adapter, status, stdout.String(), stderr.String())
	}
	const want = "Tools\nCompiler 2\n2\ntrue\nfalse\n3\n3\n3\n3\n1\n0\nPrimary\n2\n3\ntrue\n7\n4\n4\n1\n0\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected %s Ruby ORM application result: want %q, got %q, stderr=%q", adapter, want, stdout.String(), stderr.String())
	}
}

func rubyORMRuntimeConfig(t *testing.T, root, adapter, databaseSource string) *project.Config {
	t.Helper()
	config := project.New(root, "ruby")
	config.SourceDir = "src"
	config.PackageOptions["trb/orm"] = json.RawMessage(fmt.Sprintf(`{"adapter":%q,"database":%q}`, adapter, databaseSource))
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(rubyORMRuntimeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	return config
}

func prepareRubyORMRuntimeSchema(t *testing.T, database *sql.DB, adapter string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = database.Exec(`DROP TABLE IF EXISTS ruby_memberships`)
		_, _ = database.Exec(`DROP TABLE IF EXISTS ruby_projects`)
		_, _ = database.Exec(`DROP TABLE IF EXISTS ruby_users`)
		_, _ = database.Exec(`DROP TABLE IF EXISTS trb_ruby_test_products`)
		_, _ = database.Exec(`DROP TABLE IF EXISTS trb_ruby_test_profiles`)
		_, _ = database.Exec(`DROP TABLE IF EXISTS trb_ruby_test_categories`)
		database.Close()
	})
	for _, statement := range rubyORMRuntimeSchema(adapter) {
		if _, err := database.Exec(statement); err != nil {
			t.Fatalf("prepare %s Ruby ORM schema: %v\n%s", adapter, err, statement)
		}
	}
}

func rubyORMRuntimeSchema(adapter string) []string {
	id := "INTEGER PRIMARY KEY AUTOINCREMENT"
	boolean := "BOOLEAN"
	if adapter == "postgresql" {
		id = "BIGSERIAL PRIMARY KEY"
	} else if adapter == "mysql" {
		id = "BIGINT PRIMARY KEY AUTO_INCREMENT"
		boolean = "BOOLEAN"
	}
	return []string{
		`DROP TABLE IF EXISTS ruby_memberships`,
		`DROP TABLE IF EXISTS ruby_projects`,
		`DROP TABLE IF EXISTS ruby_users`,
		`DROP TABLE IF EXISTS trb_ruby_test_products`,
		`DROP TABLE IF EXISTS trb_ruby_test_profiles`,
		`DROP TABLE IF EXISTS trb_ruby_test_categories`,
		fmt.Sprintf(`CREATE TABLE trb_ruby_test_categories (id %s, name VARCHAR(255) NOT NULL UNIQUE)`, id),
		fmt.Sprintf(`CREATE TABLE trb_ruby_test_products (id %s, trb_ruby_test_category_id BIGINT NOT NULL, name VARCHAR(255) NOT NULL UNIQUE, price DOUBLE PRECISION NOT NULL, active %s NOT NULL, FOREIGN KEY (trb_ruby_test_category_id) REFERENCES trb_ruby_test_categories(id))`, id, boolean),
		fmt.Sprintf(`CREATE TABLE trb_ruby_test_profiles (id %s, trb_ruby_test_category_id BIGINT NOT NULL UNIQUE, label VARCHAR(255) NOT NULL, active %s NOT NULL, FOREIGN KEY (trb_ruby_test_category_id) REFERENCES trb_ruby_test_categories(id))`, id, boolean),
		fmt.Sprintf(`CREATE TABLE ruby_users (id %s, name VARCHAR(255) NOT NULL UNIQUE)`, id),
		fmt.Sprintf(`CREATE TABLE ruby_projects (id %s, name VARCHAR(255) NOT NULL UNIQUE, active %s NOT NULL)`, id, boolean),
		fmt.Sprintf(`CREATE TABLE ruby_memberships (id %s, ruby_user_id BIGINT NOT NULL, ruby_project_id BIGINT NOT NULL, FOREIGN KEY (ruby_user_id) REFERENCES ruby_users(id), FOREIGN KEY (ruby_project_id) REFERENCES ruby_projects(id))`, id),
	}
}

func requireRubyORMGems(t *testing.T, gems ...string) {
	t.Helper()
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby is not installed")
	}
	arguments := []string{"-e", strings.Repeat("require ARGV.shift;", len(gems))}
	arguments = append(arguments, gems...)
	if output, err := exec.Command("ruby", arguments...).CombinedOutput(); err != nil {
		t.Skipf("Ruby ORM gems are not installed: %v: %s", err, output)
	}
}

const rubyORMRuntimeSource = `import { Database, DbError, Model, belongs_to, has_many, has_one } from trb/orm
import { Result } from trb/std/result

class TrbRubyTestCategory < Model
	has_many(TrbRubyTestProduct, dependent: :destroy) do |products|
		products.where(active: true)
	end
	has_one(TrbRubyTestProfile, dependent: :delete) do |profiles|
		profiles.where(active: true)
	end
end

class TrbRubyTestProduct < Model
	belongs_to(TrbRubyTestCategory)
end

class TrbRubyTestProfile < Model
	belongs_to(TrbRubyTestCategory)
end

class RubyUser < Model
	has_many(RubyMembership)
	has_many(RubyProject, through: :ruby_memberships, source: :ruby_project) do |projects|
		projects.where(active: true)
	end
end

class RubyProject < Model
	has_many(RubyMembership)
end

class RubyMembership < Model
	belongs_to(RubyUser)
	belongs_to(RubyProject)
end

def exercise(): Integer fails DbError
	category := TrbRubyTestCategory.create(name: "Tools")
	TrbRubyTestProfile.create(trb_ruby_test_category_id: category.id, label: "Primary", active: true)
	first := TrbRubyTestProduct.create(trb_ruby_test_category_id: category.id, name: "Compiler", price: 10.0, active: true)
	updated := first.with(name: "Compiler 2").save()
	inserted := TrbRubyTestProduct.insert_all([
		TrbRubyTestProduct.build(trb_ruby_test_category_id: category.id, name: "REPL", price: 20.0, active: true),
		TrbRubyTestProduct.build(trb_ruby_test_category_id: category.id, name: "Formatter", price: 30.0, active: false)
	])
	absent := TrbRubyTestProduct.build(trb_ruby_test_category_id: category.id, name: "Absent", price: 40.0, active: true)
	puts(category.name)
	puts(updated.name)
	puts(inserted)
	puts(TrbRubyTestProduct.insert_if_absent(absent, unique_by: [:name]))
	puts(TrbRubyTestProduct.insert_if_absent(absent, unique_by: [:name]))
	puts(TrbRubyTestProduct.where(active: true).not(name: "Missing").count())
	active_count := category.trb_ruby_test_products.size()
	preloaded_active_count := TrbRubyTestCategory.preload(:trb_ruby_test_products).first().trb_ruby_test_products.size()
	joined_active_count := TrbRubyTestCategory.join(:trb_ruby_test_products).count()
	existing_active_count := TrbRubyTestCategory.where_exists(:trb_ruby_test_products).count()
	puts(active_count)
	puts(preloaded_active_count)
	puts(joined_active_count)
	puts(existing_active_count)
	missing_join := TrbRubyTestCategory.join(:trb_ruby_test_products, TrbRubyTestProduct.where(name: "Missing")).count()
	missing_exists := TrbRubyTestCategory.where_exists(:trb_ruby_test_products, TrbRubyTestProduct.where(name: "Missing")).count()
	puts(missing_join + missing_exists)
	puts(category.trb_ruby_test_profile.label)
	user := RubyUser.create(name: "Ada")
	active_project := RubyProject.create(name: "TypeRB", active: true)
	inactive_project := RubyProject.create(name: "Archived", active: false)
	RubyMembership.create(ruby_user_id: user.id, ruby_project_id: active_project.id)
	RubyMembership.create(ruby_user_id: user.id, ruby_project_id: inactive_project.id)
	through_lazy := user.ruby_projects.size()
	through_preload := RubyUser.preload(:ruby_projects).first().ruby_projects.size()
	puts(through_lazy + through_preload)
	puts(TrbRubyTestCategory.preload(:trb_ruby_test_products).first().trb_ruby_test_products.size())
	transaction_count := Database.transaction() do |tx|
		inside := TrbRubyTestProduct.using(tx).count()
		locked := TrbRubyTestProduct.using(tx).lock().first()
		puts(locked.id > 0)
		nested_count := try tx.transaction() do |nested|
			TrbRubyTestProduct.using(nested).where(active: true).count()
		end
		inside + nested_count
	end catch |_error|
		-1
	end
	puts(transaction_count)
	batch_count := TrbRubyTestProduct.find_each(batch_size: 2) do |_product|
	end
	puts(batch_count)
	puts(TrbRubyTestProduct.update_all(active: true))
	destroyed := category.destroy()
	if destroyed
		later := TrbRubyTestCategory.create(name: "Later")
		TrbRubyTestProduct.create(trb_ruby_test_category_id: later.id, name: "Later product", price: 50.0, active: true)
		puts(TrbRubyTestProduct.delete_all())
		return TrbRubyTestProduct.count()
	end
	return -1
end

def main()
	case attempt exercise()
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
	return
end
`
