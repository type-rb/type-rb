package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
	"github.com/type-rb/type-rb/internal/schemalock"
)

func runTestSqldefHelper() {
	arguments := os.Args[1:]
	for _, argument := range arguments {
		if argument == "--version" {
			fmt.Println("v3.11.19")
			return
		}
	}
	for _, argument := range arguments {
		if argument == "--export" {
			fmt.Print("CREATE TABLE exported (id INTEGER PRIMARY KEY);\n")
			return
		}
	}
	desired, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, argument := range arguments {
		if argument == "--apply" {
			database, openErr := sql.Open("sqlite", arguments[len(arguments)-1])
			if openErr != nil {
				fmt.Fprintln(os.Stderr, openErr)
				os.Exit(1)
			}
			defer database.Close()
			if _, execErr := database.Exec(string(desired)); execErr != nil {
				fmt.Fprintln(os.Stderr, execErr)
				os.Exit(1)
			}
			fmt.Println("-- Apply --")
			return
		}
	}
	fmt.Printf("ARGS %s\n%s", strings.Join(arguments, " "), desired)
}

func TestDatabaseLockAndCheckAreModeIndependent(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := databaseTestProject(t, mode)
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			configPath := filepath.Join(root, project.ConfigName)
			if status := command.Run([]string{"db", "lock", "--config", configPath}); status != 0 {
				t.Fatalf("lock status=%d stderr=%s", status, stderr.String())
			}
			lockPath := filepath.Join(root, "db", "schema.lock.json")
			lock, err := os.ReadFile(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"generatedAt", "databaseUrl", "sqldefVersion"} {
				if bytes.Contains(lock, []byte(forbidden)) {
					t.Fatalf("lock contains volatile field %s:\n%s", forbidden, lock)
				}
			}
			stdout.Reset()
			stderr.Reset()
			if status := command.Run([]string{"db", "check", "--config", configPath}); status != 0 {
				t.Fatalf("check status=%d stderr=%s", status, stderr.String())
			}
			if stdout.String() != "Database schema lock is current.\n" {
				t.Fatalf("stdout=%q", stdout.String())
			}
		})
	}
}

func TestDatabaseOfflineLockMatchesSQLiteConstraintMetadata(t *testing.T) {
	schema := `CREATE TABLE sqlitebooks (id INTEGER PRIMARY KEY AUTOINCREMENT);
CREATE TABLE identity (id INTEGER PRIMARY KEY);
CREATE TABLE notes (
  id INTEGER PRIMARY KEY,
  key TEXT,
  identity_id INTEGER REFERENCES identity(id),
  checksum TEXT,
  unique_label TEXT,
  note TEXT DEFAULT 'it''s NOT NULL and UNIQUE and GENERATED here',
  caption TEXT DEFAULT 'example REFERENCES ghosts(id)',
  positive INTEGER CHECK (positive IS NOT NULL OR positive IS NULL),
  owner INTEGER REFERENCES sqlitebooks(id) ON DELETE SET DEFAULT,
  required TEXT/* SQL comment */NOT NULL
);
CREATE UNIQUE INDEX "index ON captions" ON notes(caption);`
	lockAndCheckSQLiteSchema(t, schema)
}

func TestDatabaseLockMatchesSQLitePrimaryKeyGeneration(t *testing.T) {
	const schema = `CREATE TABLE ordinary (id INTEGER PRIMARY KEY);
CREATE TABLE indexed_desc (id INTEGER PRIMARY KEY DESC NOT NULL);
CREATE TABLE table_desc (id INTEGER, PRIMARY KEY(id DESC));
CREATE TABLE composite (id INTEGER NOT NULL, part INTEGER NOT NULL, PRIMARY KEY(id, part));
CREATE TABLE no_rowid (id INTEGER PRIMARY KEY) WITHOUT ROWID;
CREATE TABLE explicit_default (id INTEGER PRIMARY KEY DEFAULT 17) WITHOUT ROWID;`
	lock, database := lockAndCheckSQLiteSchema(t, schema)
	for _, tc := range []struct {
		table                string
		generated, defaulted bool
		id                   int
	}{
		{"ordinary", true, true, 1}, {"indexed_desc", false, false, 0},
		{"table_desc", true, true, 1}, {"composite", false, false, 0},
		{"no_rowid", false, false, 0}, {"explicit_default", false, true, 17},
	} {
		t.Run(tc.table, func(t *testing.T) {
			id := lock.Tables[tc.table].Columns["id"]
			if !id.PrimaryKey || id.Generated != tc.generated || id.HasDefault != tc.defaulted {
				t.Fatalf("unexpected key metadata: %#v", id)
			}
			var assigned int
			err := database.QueryRow("INSERT INTO " + tc.table + " DEFAULT VALUES RETURNING id").Scan(&assigned)
			if tc.defaulted {
				if err != nil || assigned != tc.id {
					t.Fatalf("insert returned %d, %v", assigned, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "NOT NULL") {
				t.Fatalf("expected an explicit key to be required, got %v", err)
			}
		})
	}
}

func lockAndCheckSQLiteSchema(t *testing.T, schema string) (*schemalock.Lock, *sql.DB) {
	t.Helper()
	root := databaseTestProject(t, "go")
	if err := os.WriteFile(filepath.Join(root, "db", "schema.sql"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, project.ConfigName)
	var stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &stderr}
	if code := command.Run([]string{"db", "lock", "--config", config}); code != 0 {
		t.Fatalf("lock failed: %s", stderr.String())
	}
	lock, err := schemalock.Read(filepath.Join(root, "db", "schema.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", filepath.Join(root, "db", "development.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if code := command.Run([]string{"db", "check", "--from-db", "--config", config}); code != 0 {
		t.Fatalf("database and offline schema metadata differ: %s", stderr.String())
	}
	return lock, database
}

func TestDatabaseLockLetsORMCompileWithoutLiveDatabase(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			config.Database = &project.DatabaseConfig{Adapter: "sqlite"}
			config.PackageOptions["trb/orm"] = json.RawMessage(`{
  "adapter": "sqlite",
  "database": {"environment": "TRB_TEST_LOCK_ONLY_DATABASE"}
}`)
			if mode == "go" {
				config.Go.Module = "example.com/lock-only"
			}
			if mode == "typescript" {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "db", "schema.sql"), []byte("CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL, price REAL);\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			source := "import { Model } from trb/orm\n\nclass Product < Model\nend\n\ndef product_name(product: Product): String\n\treturn product.name\nend\n"
			if err := os.WriteFile(filepath.Join(root, "src", "product.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("TRB_TEST_LOCK_ONLY_DATABASE", "")
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			configPath := filepath.Join(root, project.ConfigName)
			if status := command.Run([]string{"db", "lock", "--config", configPath}); status != 0 {
				t.Fatalf("lock status=%d stderr=%s", status, stderr.String())
			}
			stdout.Reset()
			stderr.Reset()
			if status := command.Run([]string{"check", "--config", configPath}); status != 0 {
				t.Fatalf("check status=%d stderr=%s", status, stderr.String())
			}
		})
	}
}

func TestDatabaseCheckReportsSchemaDrift(t *testing.T) {
	root := databaseTestProject(t, "ruby")
	command := &CLI{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard}
	configPath := filepath.Join(root, project.ConfigName)
	if status := command.Run([]string{"db", "lock", "--config", configPath}); status != 0 {
		t.Fatalf("lock status=%d", status)
	}
	if err := os.WriteFile(filepath.Join(root, "db", "schema.sql"), []byte("CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL, price REAL);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if status := command.Run([]string{"db", "check", "--config", configPath}); status != 1 {
		t.Fatalf("check status=%d stderr=%s", status, stderr.String())
	}
	if !strings.Contains(stderr.String(), "run trb db lock") {
		t.Fatalf("unexpected drift diagnostic: %s", stderr.String())
	}
}

func TestDatabasePlanApplyAndExportUsePinnedExternalCommand(t *testing.T) {
	root := databaseTestProject(t, "typescript")
	t.Setenv("TRB_TEST_SQLDEF_HELPER", "1")
	configPath := filepath.Join(root, project.ConfigName)
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"db", "plan", "--config", configPath}); status != 0 {
		t.Fatalf("plan status=%d stderr=%s", status, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--dry-run") || !strings.Contains(stdout.String(), "CREATE TABLE products") {
		t.Fatalf("unexpected plan output: %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"db", "apply", "--config", configPath}); status != 0 {
		t.Fatalf("apply status=%d stderr=%s", status, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "db", "schema.lock.json")); err != nil {
		t.Fatalf("apply did not write lock: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"db", "check", "--config", configPath}); status != 0 {
		t.Fatalf("source check status=%d stderr=%s", status, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"db", "check", "--from-db", "--config", configPath}); status != 0 {
		t.Fatalf("live check status=%d stderr=%s", status, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"db", "export", "--config", configPath}); status != 0 {
		t.Fatalf("export status=%d stderr=%s", status, stderr.String())
	}
	if stdout.String() != "CREATE TABLE exported (id INTEGER PRIMARY KEY);\n" {
		t.Fatalf("unexpected export: %q", stdout.String())
	}
}

func TestDatabasePlanReportsMissingAndMismatchedSqldef(t *testing.T) {
	root := databaseTestProject(t, "ruby")
	configPath := filepath.Join(root, project.ConfigName)
	config, err := project.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config.Database.Sqldef.Command = filepath.Join(root, "missing-sqlite3def")
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &stderr}
	if status := command.Run([]string{"db", "plan", "--config", configPath}); status != 1 {
		t.Fatalf("missing sqldef status=%d stderr=%s", status, stderr.String())
	}
	if !strings.Contains(stderr.String(), "is not installed or is not on PATH") {
		t.Fatalf("unexpected missing-command diagnostic: %s", stderr.String())
	}

	t.Setenv("TRB_TEST_SQLDEF_HELPER", "1")
	config.Database.Sqldef.Command = os.Args[0]
	config.Database.Sqldef.Version = "9.9.9"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if status := command.Run([]string{"db", "plan", "--config", configPath}); status != 1 {
		t.Fatalf("mismatched sqldef status=%d stderr=%s", status, stderr.String())
	}
	if !strings.Contains(stderr.String(), "version 9.9.9 is required, found 3.11.19") {
		t.Fatalf("unexpected version diagnostic: %s", stderr.String())
	}
}

func databaseTestProject(t *testing.T, mode string) string {
	t.Helper()
	root := t.TempDir()
	config := project.New(root, mode)
	if mode == "go" {
		config.Go.Module = "example.com/db-test"
	}
	config.Database = &project.DatabaseConfig{
		Adapter:  "sqlite",
		Database: &project.DatabaseSource{Value: "db/development.sqlite3"},
		Sqldef: &project.DBSqldefConfig{
			Command: os.Args[0], Version: project.DefaultSqldefVersion,
		},
	}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "db", "schema.sql"), []byte("CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
