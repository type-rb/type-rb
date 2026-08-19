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

func TestJobResultControlsAcknowledgementAndRetryAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			requireORMApplicationRuntime(t, mode, "sqlite")
			root := t.TempDir()
			databaseSource := filepath.Join(root, "jobs.sqlite3")
			database, err := sql.Open("sqlite", databaseSource)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec(`CREATE TABLE job_transaction_items (
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
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/jobs-result-runtime"
			}
			if config.TypeScript != nil {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			config.PackageOptions["trb/orm"] = json.RawMessage(fmt.Sprintf(`{"adapter":"sqlite","database":%q}`, databaseSource))
			configureSQLJobs(t, config, "sqlite", databaseSource)
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(config.SourcePath(), "jobs"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(jobResultMainSource), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "jobs", "transaction_result_job.trb"), []byte(jobResultJobSource), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("enqueue status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 || mode != "go" && stderr.Len() != 0 {
				t.Fatalf("unexpected enqueue output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}

			database, err = sql.Open("sqlite", databaseSource)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if _, err := database.Exec(`UPDATE trb_jobs SET run_at = CASE payload WHEN '["failed"]' THEN '2000-01-01 00:00:00' ELSE '2001-01-01 00:00:00' END`); err != nil {
				t.Fatal(err)
			}

			runWorker := func(stage string) {
				t.Helper()
				stdout.Reset()
				stderr.Reset()
				if status := command.Run([]string{"jobs", "start", "--once", "--config", config.Path}); status != 0 {
					t.Fatalf("%s worker status=%d stdout=%s stderr=%s", stage, status, stdout.String(), stderr.String())
				}
				if stdout.Len() != 0 || mode != "go" && stderr.Len() != 0 {
					t.Fatalf("%s worker output stdout=%q stderr=%q", stage, stdout.String(), stderr.String())
				}
			}

			runWorker("first failure")
			var state, lastError string
			var attempts int
			if err := database.QueryRow(`SELECT state, attempts, last_error FROM trb_jobs WHERE payload = '["failed"]'`).Scan(&state, &attempts, &lastError); err != nil {
				t.Fatal(err)
			}
			if state != "ready" || attempts != 1 || lastError != "transaction rejected" {
				t.Fatalf("first Result Err state=%q attempts=%d last_error=%q", state, attempts, lastError)
			}
			assertJobTransactionItemCount(t, database, "failed", 0)

			runWorker("successful result")
			var successfulJobs int
			if err := database.QueryRow(`SELECT COUNT(*) FROM trb_jobs WHERE payload = '["succeeded"]'`).Scan(&successfulJobs); err != nil {
				t.Fatal(err)
			}
			if successfulJobs != 0 {
				t.Fatalf("Result Ok job was not acknowledged: remaining=%d", successfulJobs)
			}
			assertJobTransactionItemCount(t, database, "succeeded", 1)

			if _, err := database.Exec(`UPDATE trb_jobs SET run_at = CURRENT_TIMESTAMP WHERE payload = '["failed"]'`); err != nil {
				t.Fatal(err)
			}
			runWorker("final failure")
			if err := database.QueryRow(`SELECT state, attempts, last_error FROM trb_jobs WHERE payload = '["failed"]'`).Scan(&state, &attempts, &lastError); err != nil {
				t.Fatal(err)
			}
			if state != "failed" || attempts != 2 || lastError != "transaction rejected" {
				t.Fatalf("final Result Err state=%q attempts=%d last_error=%q", state, attempts, lastError)
			}
			assertJobTransactionItemCount(t, database, "failed", 0)
		})
	}
}

func assertJobTransactionItemCount(t *testing.T, database *sql.DB, name string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT COUNT(*) FROM job_transaction_items WHERE name = ?`, name).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("job_transaction_items name=%q count=%d, want %d", name, got, want)
	}
}

const jobResultMainSource = `import { TransactionResultJob } from jobs/transaction_result_job

def main()
	_failed := TransactionResultJob.perform_later("failed") catch |error|
		puts(error.message)
		return
	end
	_succeeded := TransactionResultJob.perform_later("succeeded") catch |error|
		puts(error.message)
		return
	end
	return
end
`

const jobResultJobSource = `import { Job, JobError, JobResult, maximum_attempts } from trb/jobs
import { Database, Model } from trb/orm
import { Unit } from trb/std/unit

class JobTransactionItem < Model
end

class TransactionResultJob < Job
	maximum_attempts(2)

	def perform(name: String): JobResult
		completed := Database.transaction() do |tx|
			_created := JobTransactionItem.using(tx).create(name: name)
			if name == "failed"
				_duplicate := JobTransactionItem.using(tx).create(name: name)
			end
			Unit.new()
		end catch |_error|
			return JobResult::Err(JobError.new(message: "transaction rejected"))
		end
		return JobResult::Ok(completed)
	end
end
`
