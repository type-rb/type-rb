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

func TestRunORMResultTransactionControlFlowAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireORMApplicationRuntime(t, mode, "sqlite")
			config := prepareORMResultTransactionProject(t, mode)

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("run status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			unexpectedStderr := mode != "go" && stderr.Len() != 0
			if got := stdout.String(); got != ormResultTransactionApplicationOutput || unexpectedStderr {
				t.Fatalf("unexpected %s transaction Result output: want %q, got %q, stderr=%q", mode, ormResultTransactionApplicationOutput, got, stderr.String())
			}
		})
	}
}

func TestReplORMResultTransactionControlFlowAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireORMApplicationRuntime(t, mode, "sqlite")
			config := prepareORMResultTransactionProject(t, mode)
			input := "import { exercise } from main\nexercise()\n:quit\n"

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("repl status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			want := strings.TrimSuffix(ormResultTransactionApplicationOutput, "1\n") + "1 : Integer\n"
			if got := stdout.String(); got != want || stderr.Len() != 0 {
				t.Fatalf("unexpected %s transaction Result REPL output: want %q, got %q, stderr=%q", mode, want, got, stderr.String())
			}
		})
	}
}

func prepareORMResultTransactionProject(t *testing.T, mode string) *project.Config {
	t.Helper()
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE result_transaction_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE
	)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	config := project.New(root, mode)
	config.SourceDir = "src"
	config.OutDir = "build"
	if config.Go != nil {
		config.Go.Module = "example.com/type-rb/result-transaction"
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
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(ormResultTransactionApplicationSource), 0o644); err != nil {
		t.Fatal(err)
	}
	return config
}

const ormResultTransactionApplicationSource = `import { Database, DbError, DbResult, Model } from trb/orm
import { Result } from trb/std/result

class ResultTransactionItem < Model
end

def commit_with_catch(): Integer
	value := Database.transaction() do |tx|
		_created := ResultTransactionItem.using(tx).create(name: "catch-committed")
		11
	end catch |_error|
		-1
	end
	return value
end

def commit_with_try(): DbResult<Integer>
	value := try Database.transaction() do |tx|
		_created := ResultTransactionItem.using(tx).create(name: "try-committed")
		22
	end
	return DbResult<Integer>::Ok(value)
end

def rollback_with_inner_try(): DbResult<Integer>
	return Database.transaction() do |outer|
		_outer_item := ResultTransactionItem.using(outer).create(name: "outer-rolled")
		nested_value := try outer.transaction() do |inner|
			_inner_item := ResultTransactionItem.using(inner).create(name: "inner-rolled")
			_duplicate := ResultTransactionItem.using(inner).create(name: "inner-rolled")
			33
		end
		nested_value
	end
end

def catch_observes_rollback(): Integer fails DbError
	observed := Database.transaction() do |tx|
		_created := ResultTransactionItem.using(tx).create(name: "catch-rolled")
		_duplicate := ResultTransactionItem.using(tx).create(name: "catch-rolled")
		44
	end catch |_error|
		return ResultTransactionItem.where(name: "catch-rolled").count()
	end
	return observed
end

def return_catch_observes_rollback(): Integer fails DbError
	return Database.transaction() do |tx|
		_created := ResultTransactionItem.using(tx).create(name: "return-catch-rolled")
		_duplicate := ResultTransactionItem.using(tx).create(name: "return-catch-rolled")
		55
	end catch |_error|
		ResultTransactionItem.where(name: "return-catch-rolled").count()
	end
end

def exercise(): Integer fails DbError
	puts(commit_with_catch())
	try_value := commit_with_try() catch |_error|
		-1
	end
	puts(try_value)
	rollback_value := rollback_with_inner_try() catch |_error|
		0
	end
	puts(rollback_value)
	puts(ResultTransactionItem.where(name: "catch-committed").count())
	puts(ResultTransactionItem.where(name: "try-committed").count())
	puts(ResultTransactionItem.where(name: "outer-rolled").count())
	puts(ResultTransactionItem.where(name: "inner-rolled").count())
	puts(catch_observes_rollback())
	puts(return_catch_observes_rollback())
	return 1
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

const ormResultTransactionApplicationOutput = `11
22
0
1
1
0
0
0
0
1
`
