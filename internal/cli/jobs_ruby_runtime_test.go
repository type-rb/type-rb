package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunRubyJobApplicationPersistsAndPerforms(t *testing.T) {
	requireRubyORMGems(t, "sequel", "sqlite3")
	root := t.TempDir()
	databaseSource := filepath.Join(root, "jobs.sqlite3")
	config := project.New(root, "ruby")
	config.SourceDir = "src"
	config.PackageOptions["trb/jobs"] = json.RawMessage(`{"database_adapter":"sqlite","database":` + quoteJSON(databaseSource) + `}`)
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config.SourcePath(), "jobs"), 0o755); err != nil {
		t.Fatal(err)
	}
	mainSource := `import { Result } from trb/std/result
import { SendReceiptJob } from jobs/send_receipt_job

def main()
	case attempt SendReceiptJob.perform_later(42, "ada@example.test")
	when Result::Ok(reference)
		puts(reference.job_name)
	when Result::Err(error)
		puts(error.message)
	end
	return
end
`
	jobSource := `import { Job } from trb/jobs

class SendReceiptJob < Job
	def perform(order_id: Integer, destination: String)
		puts("performed " + order_id.to_s() + " for " + destination)
		return
	end
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "jobs", "send_receipt_job.trb"), []byte(jobSource), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
		t.Fatalf("run status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "SendReceiptJob\n" {
		t.Fatalf("unexpected enqueue output %q stderr=%s", stdout.String(), stderr.String())
	}
	database, err := sql.Open("sqlite", databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	var jobName, payload, state string
	if err := database.QueryRow(`SELECT job_name, payload, state FROM trb_jobs`).Scan(&jobName, &payload, &state); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if jobName != "SendReceiptJob" || payload != `[42,"ada@example.test"]` || state != "ready" {
		t.Fatalf("unexpected Ruby persisted job name=%q payload=%q state=%q", jobName, payload, state)
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"jobs", "start", "--once", "--config", config.Path}); status != 0 {
		t.Fatalf("worker status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "performed 42 for ada@example.test\n" {
		t.Fatalf("unexpected worker output %q stderr=%s", stdout.String(), stderr.String())
	}
	database, err = sql.Open("sqlite", databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var remaining int
	if err := database.QueryRow(`SELECT COUNT(*) FROM trb_jobs`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("Ruby worker did not acknowledge the job: %d", remaining)
	}
}
