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

func TestORMEnumColumnsAcrossBackendsAndDatabases(t *testing.T) {
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
							t.Fatalf("%s ORM enum conformance requires its live database environment", adapter)
						}
						t.Skipf("set the live %s test database to run ORM enum conformance", adapter)
					}
					prepareORMEnumConformanceTable(t, driver, databaseSource, adapter)

					root := t.TempDir()
					config := project.New(root, mode)
					config.SourceDir = "src"
					config.OutDir = "build"
					if config.Go != nil {
						config.Go.Module = "example.com/type-rb/orm-enum-conformance"
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
					if err := os.MkdirAll(filepath.Join(config.SourcePath(), "domain"), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(config.SourcePath(), "domain", "statuses.trb"), []byte(ormEnumConformanceEnumsSource), 0o644); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(ormEnumConformanceSource), 0o644); err != nil {
						t.Fatal(err)
					}

					var stdout, stderr bytes.Buffer
					command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
					if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
						t.Fatalf("run status=%d stderr=%s", status, stderr.String())
					}
					if stdout.String() != ormEnumConformanceOutput || mode != "go" && stderr.Len() != 0 {
						t.Fatalf("unexpected %s/%s ORM enum output\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, adapter, ormEnumConformanceOutput, stdout.String(), stderr.String())
					}

					stdout.Reset()
					stderr.Reset()
					input := "import { TrbEnumProduct } from main\n" +
						"import { TrbOrderStatus } from domain/statuses\n" +
						"TrbEnumProduct.where(status: TrbOrderStatus::Pending).count()\n" +
						"attempt TrbEnumProduct.find(999)\n" +
						":quit\n"
					command = &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
					if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
						t.Fatalf("REPL status=%d stderr=%s", status, stderr.String())
					}
					if !strings.Contains(stdout.String(), "1 : Integer") || !strings.Contains(stdout.String(), "DbErrorKind::InvalidData") || stderr.Len() != 0 {
						t.Fatalf("unexpected %s/%s ORM enum REPL result: stdout=%q stderr=%q", mode, adapter, stdout.String(), stderr.String())
					}
				})
			}
		})
	}
}

func prepareORMEnumConformanceTable(t *testing.T, driver, databaseSource, adapter string) {
	t.Helper()
	database, err := sql.Open(driver, databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec("DROP TABLE IF EXISTS trb_enum_products")
		database.Close()
	})
	if _, err := database.Exec("DROP TABLE IF EXISTS trb_enum_products"); err != nil {
		t.Fatal(err)
	}
	id := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if adapter == "postgresql" {
		id = "BIGSERIAL PRIMARY KEY"
	} else if adapter == "mysql" {
		id = "BIGINT PRIMARY KEY AUTO_INCREMENT"
	}
	statement := "CREATE TABLE trb_enum_products (id " + id + ", status VARCHAR(255) NOT NULL, priority BIGINT, phase VARCHAR(255) NOT NULL)"
	if _, err := database.Exec(statement); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO trb_enum_products (id, status, priority, phase) VALUES (999, 'UNKNOWN', NULL, 'pending_review')"); err != nil {
		t.Fatal(err)
	}
}

const ormEnumConformanceEnumsSource = `enum TrbOrderStatus
	Pending = "PENDING"
	Completed = "COMPLETED"
end

enum TrbPriority
	Low = -1
	High = 2
end

enum TrbPhase
	PendingReview
	ReadyToShip
end
`

const ormEnumConformanceSource = `import { TrbOrderStatus, TrbPhase, TrbPriority } from domain/statuses
import { Result } from trb/std/result
import { DbError, DbErrorKind, Model, enum_column } from trb/orm

class TrbEnumProduct < Model
	enum_column(:status, TrbOrderStatus)
	enum_column(:priority, TrbPriority)
	enum_column(:phase, TrbPhase)
end

def exercise(): Boolean fails DbError
	product := TrbEnumProduct.create(status: TrbOrderStatus::Pending, priority: TrbPriority::High, phase: TrbPhase::PendingReview)
	puts(product.status == TrbOrderStatus::Pending)
	puts(product.priority != nil)
	puts(TrbEnumProduct.where(priority: TrbPriority::High).count() == 1)
	puts(product.phase == TrbPhase::PendingReview)
	puts(TrbEnumProduct.where(status: [TrbOrderStatus::Pending, TrbOrderStatus::Completed]).count() == 1)
	statuses := TrbEnumProduct.where(status: TrbOrderStatus::Pending).pluck(:status)
	puts(statuses[0] == TrbOrderStatus::Pending)
	counts := TrbEnumProduct.where(status: TrbOrderStatus::Pending).group(:status).count()
	puts(counts[TrbOrderStatus::Pending] == 1)
	updated := product.update(status: TrbOrderStatus::Completed, priority: TrbPriority::Low, phase: TrbPhase::ReadyToShip)
	puts(updated.status == TrbOrderStatus::Completed)
	puts(updated.priority != nil)
	puts(TrbEnumProduct.where(priority: TrbPriority::Low).count() == 1)
	puts(updated.phase == TrbPhase::ReadyToShip)
	TrbEnumProduct.where(id: product.id).update_all(status: TrbOrderStatus::Pending)
	puts(TrbEnumProduct.find(product.id).status == TrbOrderStatus::Pending)
	case attempt TrbEnumProduct.find(999)
	when Result::Ok(_value)
		puts(false)
	when Result::Err(error)
		puts(error.kind == DbErrorKind::InvalidData)
	end
	return true
end


def main()
	case attempt exercise()
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.kind)
		puts(error.message)
	end
	return
end
`

const ormEnumConformanceOutput = `true
true
true
true
true
true
true
true
true
true
true
true
true
true
`
