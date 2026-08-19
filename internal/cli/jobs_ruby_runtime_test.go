package cli

import (
	"bytes"
	"database/sql"
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
	configureSQLJobs(t, config, "sqlite", databaseSource)
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config.SourcePath(), "jobs"), 0o755); err != nil {
		t.Fatal(err)
	}
	mainSource := `import { Duration, Instant } from trb/std/time
import { SendReceiptJob } from jobs/send_receipt_job

def main()
	reference := SendReceiptJob.perform_later(42, "ada@example.test") catch |error|
		puts(error.message)
		return
	end
	puts(reference.job_name)
	later_reference := SendReceiptJob.perform_in(Duration.seconds(60), 43, "later@example.test") catch |error|
		puts(error.message)
		return
	end
	puts(later_reference.job_name)
	scheduled_reference := SendReceiptJob.perform_at(Instant.now().add(Duration.seconds(120)), 44, "scheduled@example.test") catch |error|
		puts(error.message)
		return
	end
	puts(scheduled_reference.job_name)
	return
end
`
	jobSource := `import { Job, priority, queue } from trb/jobs

class SendReceiptJob < Job
	queue("mail")
	priority(10)

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
	if stdout.String() != "SendReceiptJob\nSendReceiptJob\nSendReceiptJob\n" {
		t.Fatalf("unexpected enqueue output %q stderr=%s", stdout.String(), stderr.String())
	}
	database, err := sql.Open("sqlite", databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	var jobName, payload, queueName, state string
	var priority int
	if err := database.QueryRow(`SELECT job_name, payload, queue_name, priority, state FROM trb_jobs ORDER BY run_at LIMIT 1`).Scan(&jobName, &payload, &queueName, &priority, &state); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if jobName != "SendReceiptJob" || payload != `[42,"ada@example.test"]` || queueName != "mail" || priority != 10 || state != "ready" {
		t.Fatalf("unexpected Ruby persisted job name=%q payload=%q queue=%q priority=%d state=%q", jobName, payload, queueName, priority, state)
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"jobs", "start", "--once", "--queue", "mail", "--config", config.Path}); status != 0 {
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
	if remaining != 2 {
		t.Fatalf("Ruby worker did not leave the future jobs: %d", remaining)
	}
}
