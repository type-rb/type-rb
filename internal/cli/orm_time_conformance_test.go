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
	"time"

	"github.com/type-rb/type-rb/internal/project"
)

func TestORMPortableTimeAcrossBackendsAndDatabases(t *testing.T) {
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
							t.Fatalf("%s ORM time conformance requires its live database environment", adapter)
						}
						t.Skipf("set the live %s test database to run ORM time conformance", adapter)
					}
					prepareORMTimeSchema(t, driver, databaseSource, adapter)

					root := t.TempDir()
					config := project.New(root, mode)
					config.SourceDir = "src"
					config.OutDir = "build"
					if config.Go != nil {
						config.Go.Module = "example.com/type-rb/orm-time-conformance"
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
					if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(ormTimeConformanceSource), 0o644); err != nil {
						t.Fatal(err)
					}

					var stdout, stderr bytes.Buffer
					command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
					if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
						t.Fatalf("run status=%d stderr=%s", status, stderr.String())
					}
					unexpectedStderr := mode != "go" && stderr.Len() != 0
					if stdout.String() != ormTimeConformanceOutput || unexpectedStderr {
						t.Fatalf("unexpected %s/%s ORM time output\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, adapter, ormTimeConformanceOutput, stdout.String(), stderr.String())
					}
				})
			}
		})
	}
}

func TestTypeScriptMySQLDatabaseDefaultsUseUTCSession(t *testing.T) {
	requireORMApplicationRuntime(t, "typescript", "mysql")
	databaseSource, driver, available := replORMConformanceDatabase(t, "mysql")
	if !available {
		t.Skip("set TRB_TEST_MYSQL_DATABASE to run MySQL database-default time conformance")
	}
	database, err := sql.Open(driver, databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.Exec("CREATE TABLE trb_default_time_events (id BIGINT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255) NOT NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6))"); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	config := project.New(root, "typescript")
	config.SourceDir = "src"
	config.OutDir = "build"
	config.TypeScript.Runtime = project.TypeScriptRuntimeBun
	config.TypeScript.PackageManager = "bun"
	config.PackageOptions["trb/orm"] = json.RawMessage(fmt.Sprintf(`{"adapter":"mysql","database":%q}`, databaseSource))
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(ormMySQLDefaultTimeSource), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TZ", "Asia/Tokyo")
	startedAt := time.Now().UTC().Add(-2 * time.Second)
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
		t.Fatalf("run status=%d stderr=%s", status, stderr.String())
	}
	finishedAt := time.Now().UTC().Add(2 * time.Second)
	createdAt, err := time.Parse("2006-01-02T15:04:05.999999", strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatalf("generated application returned invalid DateTime %q: %v", stdout.String(), err)
	}
	if createdAt.Before(startedAt) || createdAt.After(finishedAt) {
		t.Fatalf("MySQL database default was not evaluated in UTC: created_at=%s expected between %s and %s", createdAt, startedAt, finishedAt)
	}
}

func prepareORMTimeSchema(t *testing.T, driver, databaseSource, adapter string) {
	t.Helper()
	database, err := sql.Open(driver, databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.Exec("DROP TABLE IF EXISTS trb_time_events"); err != nil {
		t.Fatal(err)
	}
	id := "INTEGER PRIMARY KEY AUTOINCREMENT"
	dateType, timeType, dateTimeType, instantType := "DATE", "TIME", "DATETIME", "TIMESTAMPTZ"
	if adapter == "postgresql" {
		id = "BIGSERIAL PRIMARY KEY"
		timeType, dateTimeType, instantType = "TIME(6) WITHOUT TIME ZONE", "TIMESTAMP(6) WITHOUT TIME ZONE", "TIMESTAMP(6) WITH TIME ZONE"
	} else if adapter == "mysql" {
		id = "BIGINT AUTO_INCREMENT PRIMARY KEY"
		timeType, dateTimeType, instantType = "TIME(6)", "DATETIME(6)", "TIMESTAMP(6)"
	}
	statement := "CREATE TABLE trb_time_events (id " + id + ", on_date " + dateType + " NOT NULL, at_time " + timeType + " NOT NULL, local_at " + dateTimeType + " NOT NULL, exact_at " + instantType + " NOT NULL, optional_at " + dateTimeType + ")"
	if _, err := database.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

const ormTimeConformanceSource = `import { DbResult, Model } from trb/orm
import { Date, DateTime, Instant, TimeOfDay } from trb/std/time

class TrbTimeEvent < Model
end

def exercise(): DbResult<Integer>
	on_date := Date.parse("2025-03-08")
	at_time := TimeOfDay.parse("12:34:56.123456")
	local_at := DateTime.parse("2025-03-08T12:34:56.123456")
	exact_at := Instant.parse("2025-03-08T03:34:56.123456Z")
	event := try TrbTimeEvent.create(on_date: on_date, at_time: at_time, local_at: local_at, exact_at: exact_at, optional_at: nil)
	puts(event.id > 0)
	puts(event.on_date.to_s())
	puts(event.at_time.to_s())
	puts(event.local_at.to_s())
	puts(event.exact_at.to_s())

	loaded := try TrbTimeEvent.where(on_date: on_date, exact_at: exact_at).first()
	puts(loaded.on_date.same?(on_date))
	puts(loaded.at_time.same?(at_time))
	puts(loaded.local_at.same?(local_at))
	puts(loaded.exact_at.same?(exact_at))
	puts(try TrbTimeEvent.where("exact_at", ">=", exact_at).count())
	puts(try TrbTimeEvent.where(on_date: [on_date, on_date.add_days(1)]).count())

	changed := try event.update(
		local_at: DateTime.parse("2025-03-09T01:02:03.654321"),
		optional_at: DateTime.parse("2025-03-10T04:05:06.000007")
	)
	puts(changed.local_at.to_s())
	puts(changed.optional_at != nil)
	dates := try TrbTimeEvent.pluck(:on_date)
	puts(dates.size())
	puts(try TrbTimeEvent.minimum(:on_date) != nil)
	puts(try TrbTimeEvent.maximum(:exact_at) != nil)
	return TrbTimeEvent.count()
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

const ormMySQLDefaultTimeSource = `import { Model } from trb/orm

class TrbDefaultTimeEvent < Model
end

def main()
	event := TrbDefaultTimeEvent.create(name: "default") catch |error|
		puts(error.message)
		return
	end
	puts(event.created_at.to_s())
end
`

const ormTimeConformanceOutput = `true
2025-03-08
12:34:56.123456
2025-03-08T12:34:56.123456
2025-03-08T03:34:56.123456Z
true
true
true
true
1
1
2025-03-09T01:02:03.654321
true
1
true
true
1
`
