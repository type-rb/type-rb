package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/type-rb/type-rb/internal/project"
	_ "modernc.org/sqlite"
)

func TestReplORMConformanceAcrossModesAndDatabases(t *testing.T) {
	requireLive := os.Getenv("TRB_REQUIRE_ORM_CONFORMANCE") == "1"
	for _, adapter := range []string{"sqlite", "postgresql", "mysql"} {
		adapter := adapter
		t.Run(adapter, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				mode := mode
				t.Run(mode, func(t *testing.T) {
					databaseSource, driver, available := replORMConformanceDatabase(t, adapter)
					if !available {
						if requireLive {
							t.Fatalf("%s ORM conformance requires its live database environment", adapter)
						}
						t.Skipf("set the live %s test database to run ORM conformance", adapter)
					}
					prepareReplORMConformanceTable(t, driver, databaseSource, adapter)

					root := t.TempDir()
					config := project.New(root, mode)
					config.SourceDir = "src"
					if config.Go != nil {
						config.Go.Module = "example.com/type-rb/repl-orm-conformance"
					}
					config.PackageOptions["trb/orm"] = json.RawMessage(fmt.Sprintf(`{"adapter":%q,"database":%q}`, adapter, databaseSource))
					if err := config.Save(); err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
						t.Fatal(err)
					}
					source := "import { Model } from trb/orm\n\nclass TrbReplConformanceProduct < Model\nend\n"
					if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
						t.Fatal(err)
					}

					input := strings.Join([]string{
						"import { TrbReplConformanceProduct } from main",
						"TrbReplConformanceProduct.count()",
						`TrbReplConformanceProduct.update_all(name: "Updated")`,
						`TrbReplConformanceProduct.where(name: "Updated").count()`,
						"TrbReplConformanceProduct.delete_all()",
						"TrbReplConformanceProduct.count()",
						":quit",
					}, "\n") + "\n"
					var stdout, stderr bytes.Buffer
					command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
					if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
						t.Fatalf("status=%d stderr=%s", status, stderr.String())
					}
					const want = "1 : Integer\n1 : Integer\n1 : Integer\n1 : Integer\n0 : Integer\n"
					if stdout.String() != want || stderr.Len() != 0 {
						t.Fatalf("unexpected %s/%s ORM REPL output: stdout=%q stderr=%q", mode, adapter, stdout.String(), stderr.String())
					}
				})
			}
		})
	}
}

func replORMConformanceDatabase(t *testing.T, adapter string) (string, string, bool) {
	t.Helper()
	switch adapter {
	case "sqlite":
		return filepath.Join(t.TempDir(), "application.sqlite3"), "sqlite", true
	case "postgresql":
		value := strings.TrimSpace(os.Getenv("TRB_TEST_POSTGRESQL_DATABASE"))
		if value == "" {
			return "", "pgx", false
		}
		parsed, err := url.Parse(value)
		if err != nil {
			t.Fatal(err)
		}
		schema := "trb_repl_conformance_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		admin, err := sql.Open("pgx", value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
			admin.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
			admin.Close()
		})
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		return parsed.String(), "pgx", true
	case "mysql":
		value := strings.TrimSpace(os.Getenv("TRB_TEST_MYSQL_DATABASE"))
		if value == "" {
			return "", "mysql", false
		}
		base, err := mysqldriver.ParseDSN(value)
		if err != nil {
			t.Fatal(err)
		}
		name := "trb_repl_conformance_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		adminConfig := *base
		adminConfig.DBName = ""
		admin, err := sql.Open("mysql", adminConfig.FormatDSN())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec("CREATE DATABASE `" + name + "`"); err != nil {
			admin.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = admin.Exec("DROP DATABASE `" + name + "`")
			admin.Close()
		})
		testConfig := *base
		testConfig.DBName = name
		return testConfig.FormatDSN(), "mysql", true
	default:
		t.Fatalf("unsupported ORM conformance adapter %q", adapter)
		return "", "", false
	}
}

func prepareReplORMConformanceTable(t *testing.T, driver, databaseSource, adapter string) {
	t.Helper()
	database, err := sql.Open(driver, databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec("DROP TABLE IF EXISTS trb_repl_conformance_products")
		database.Close()
	})
	if _, err := database.Exec("DROP TABLE IF EXISTS trb_repl_conformance_products"); err != nil {
		t.Fatal(err)
	}
	id := "INTEGER PRIMARY KEY"
	if adapter == "postgresql" {
		id = "BIGINT PRIMARY KEY"
	} else if adapter == "mysql" {
		id = "BIGINT PRIMARY KEY"
	}
	if _, err := database.Exec("CREATE TABLE trb_repl_conformance_products (id " + id + ", name VARCHAR(255) NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO trb_repl_conformance_products (id, name) VALUES (1, 'Portable')"); err != nil {
		t.Fatal(err)
	}
}
