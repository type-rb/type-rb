package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunORMResultStreamingAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireORMApplicationRuntime(t, mode, "sqlite")
			root := t.TempDir()
			databasePath := filepath.Join(root, "streaming.sqlite3")
			database, err := sql.Open("sqlite", databasePath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec(`
				CREATE TABLE products (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE);
				INSERT INTO products (name) VALUES ('a'), ('b'), ('c');
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
				config.Go.Module = "example.com/type-rb/orm-result-streaming"
			}
			if config.TypeScript != nil {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			config.PackageOptions["trb/orm"] = json.RawMessage(fmt.Sprintf(`{"adapter":"sqlite","database":%q}`, databasePath))
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(ormResultStreamingSource), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("run status=%d stderr=%s", status, stderr.String())
			}
			if stdout.String() != ormResultStreamingOutput || mode != "go" && stderr.Len() != 0 {
				t.Fatalf("unexpected %s Result streaming output\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, ormResultStreamingOutput, stdout.String(), stderr.String())
			}

			database, err = sql.Open("sqlite", databasePath)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			var rolledBack int
			if err := database.QueryRow(`SELECT COUNT(*) FROM products WHERE name = 'rolled'`).Scan(&rolledBack); err != nil {
				t.Fatal(err)
			}
			if rolledBack != 0 {
				t.Fatalf("%s committed the row after a streaming Err", mode)
			}
		})
	}
}

const ormResultStreamingSource = `import { Database, DbErrorKind, DbResult, Model } from trb/orm

class Product < Model
end

def invalid_stream(): DbResult<Integer>
	return Product.find_each(batch_size: 0) do |_product|
	end
end

def block_failure(): DbResult<Integer>
	return Product.find_each(batch_size: 1) do |product|
		if product.id == 2
			puts(try invalid_stream())
		end
		puts(product.name)
	end
end

def transactional_failure(): DbResult<Integer>
	return Database.transaction() do |transaction|
		products := Product.using(transaction)
		created := try products.create(name: "rolled")
		_created_id := created.id
		count := try products.find_each(batch_size: 1) do |product|
			if product.name == "rolled"
				puts(try invalid_stream())
			end
		end
		count
	end
end

def main()
	caught := Product.find_each(batch_size: 0) do |_product|
	end catch |error|
		puts(error.kind == DbErrorKind::InvalidData)
		-1
	end
	puts(caught)

	stopped := Product.find_each(batch_size: 1) do |product|
		if product.id == 1
			next
		end
		puts(product.name)
		if product.id == 2
			break
		end
	end
	case stopped
	when DbResult::Ok(count)
		puts(count)
	when DbResult::Err(error)
		puts(error.message)
	end

	batched := Product.find_in_batches(batch_size: 2) do |products|
		puts(products.size())
		break
	end
	case batched
	when DbResult::Ok(count)
		puts(count)
	when DbResult::Err(error)
		puts(error.message)
	end

	case block_failure()
	when DbResult::Ok(count)
		puts(count)
	when DbResult::Err(error)
		puts(error.message)
	end

	case transactional_failure()
	when DbResult::Ok(count)
		puts(count)
	when DbResult::Err(error)
		puts(error.message)
	end
	return
end
`

const ormResultStreamingOutput = `true
-1
b
2
2
2
a
batch size must be greater than zero
batch size must be greater than zero
`
