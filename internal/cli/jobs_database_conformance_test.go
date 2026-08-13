package cli

import (
	"bytes"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/type-rb/type-rb/internal/project"
)

func TestJobApplicationConformanceAcrossServerDatabases(t *testing.T) {
	requireLive := os.Getenv("TRB_REQUIRE_JOBS_CONFORMANCE") == "1"
	for _, adapter := range []string{"postgresql", "mysql"} {
		adapter := adapter
		t.Run(adapter, func(t *testing.T) {
			configSource, inspectionSource, driver, available := jobsConformanceDatabase(t, adapter)
			if !available {
				if requireLive {
					t.Fatalf("%s jobs conformance requires its live database environment", adapter)
				}
				t.Skipf("set the live %s test database to run jobs conformance", adapter)
			}
			for _, mode := range []string{"go", "ruby", "typescript"} {
				mode := mode
				t.Run(mode, func(t *testing.T) {
					requireORMApplicationRuntime(t, mode, adapter)
					root := t.TempDir()
					config := project.New(root, mode)
					config.SourceDir = "src"
					if config.Go != nil {
						config.Go.Module = "example.com/type-rb/jobs-server-conformance"
					}
					if config.TypeScript != nil {
						config.TypeScript.Runtime = project.TypeScriptRuntimeBun
						config.TypeScript.PackageManager = "bun"
					}
					environment := "TRB_JOBS_CONFORMANCE_DATABASE"
					t.Setenv(environment, configSource)
					configureSQLJobsFromEnvironment(t, config, adapter, environment)
					if err := config.Save(); err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(filepath.Join(config.SourcePath(), "jobs"), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(jobsServerMainSource), 0o644); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(config.SourcePath(), "jobs", "server_job.trb"), []byte(jobsServerJobSource), 0o644); err != nil {
						t.Fatal(err)
					}
					var stdout, stderr bytes.Buffer
					command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
					if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
						t.Fatalf("enqueue status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
					}
					stdout.Reset()
					stderr.Reset()
					if status := command.Run([]string{"jobs", "start", "--once", "--config", config.Path}); status != 0 {
						t.Fatalf("worker status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
					}
					if stdout.String() != "performed server job 42\n" {
						t.Fatalf("unexpected %s/%s output %q stderr=%s", mode, adapter, stdout.String(), stderr.String())
					}
					database, err := sql.Open(driver, inspectionSource)
					if err != nil {
						t.Fatal(err)
					}
					defer database.Close()
					var remaining int
					if err := database.QueryRow(`SELECT COUNT(*) FROM trb_jobs`).Scan(&remaining); err != nil {
						t.Fatal(err)
					}
					if remaining != 0 {
						t.Fatalf("%s/%s worker did not acknowledge the job: %d", mode, adapter, remaining)
					}
				})
			}
		})
	}
}

func jobsConformanceDatabase(t *testing.T, adapter string) (string, string, string, bool) {
	t.Helper()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	switch adapter {
	case "postgresql":
		value := strings.TrimSpace(os.Getenv("TRB_TEST_POSTGRESQL_DATABASE"))
		if value == "" {
			return "", "", "pgx", false
		}
		parsed, err := url.Parse(value)
		if err != nil {
			t.Fatal(err)
		}
		schema := "trb_jobs_conformance_" + suffix
		admin, err := sql.Open("pgx", value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
			admin.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`); admin.Close() })
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		return parsed.String(), parsed.String(), "pgx", true
	case "mysql":
		value := strings.TrimSpace(os.Getenv("TRB_TEST_MYSQL_DATABASE"))
		if value == "" {
			return "", "", "mysql", false
		}
		base, err := mysqldriver.ParseDSN(value)
		if err != nil {
			t.Fatal(err)
		}
		name := "trb_jobs_conformance_" + suffix
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
		t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE `" + name + "`"); admin.Close() })
		testConfig := *base
		testConfig.DBName = name
		user := url.UserPassword(testConfig.User, testConfig.Passwd).String()
		standardURL := "mysql://" + user + "@" + testConfig.Addr + "/" + name
		return standardURL, testConfig.FormatDSN(), "mysql", true
	default:
		t.Fatalf("unsupported jobs conformance adapter %q", adapter)
		return "", "", "", false
	}
}

const jobsServerMainSource = `import { Result } from trb/std/result
import { ServerJob } from jobs/server_job

def main()
	case attempt ServerJob.perform_later(42)
	when Result::Ok(_reference)
		return
	when Result::Err(error)
		puts(error.message)
	end
end
`

const jobsServerJobSource = `import { Job } from trb/jobs

class ServerJob < Job
	def perform(value: Integer)
		puts("performed server job " + value.to_s())
		return
	end
end
`
