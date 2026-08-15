package cli

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunGoJobApplicationPersistsEnqueue(t *testing.T) {
	root := t.TempDir()
	databaseSource := filepath.Join(root, "jobs.sqlite3")
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/jobs-conformance"
	configureSQLJobs(t, config, "sqlite", databaseSource)
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config.SourcePath(), "jobs"), 0o755); err != nil {
		t.Fatal(err)
	}
	mainSource := `import { Result } from trb/std/result
import { Duration, Instant } from trb/std/time
import { SendReceiptJob } from jobs/send_receipt_job

def main()
	case attempt SendReceiptJob.perform_later(42, "ada@example.test")
	when Result::Ok(reference)
		puts(reference.job_name)
	when Result::Err(error)
		puts(error.message)
	end
	case attempt SendReceiptJob.perform_in(Duration.seconds(60), 43, "later@example.test")
	when Result::Ok(reference)
		puts(reference.job_name)
	when Result::Err(error)
		puts(error.message)
	end
	case attempt SendReceiptJob.perform_at(Instant.now().add(Duration.seconds(120)), 44, "scheduled@example.test")
	when Result::Ok(reference)
		puts(reference.job_name)
	when Result::Err(error)
		puts(error.message)
	end
	return
end
`
	jobSource := `import { Job, priority, queue } from trb/jobs
import { puts } from trb/std/io

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
		t.Fatalf("unexpected output %q stderr=%s", stdout.String(), stderr.String())
	}
	database, err := sql.Open("sqlite", databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	var jobName, payload, queueName, state string
	var payloadVersion, priority int
	if err := database.QueryRow(`SELECT job_name, payload, payload_version, queue_name, priority, state FROM trb_jobs ORDER BY run_at LIMIT 1`).Scan(&jobName, &payload, &payloadVersion, &queueName, &priority, &state); err != nil {
		t.Fatal(err)
	}
	if jobName != "SendReceiptJob" || payload != `[42,"ada@example.test"]` || payloadVersion != 1 || queueName != "mail" || priority != 10 || state != "ready" {
		t.Fatalf("unexpected persisted job: name=%q payload=%q version=%d queue=%q priority=%d state=%q", jobName, payload, payloadVersion, queueName, priority, state)
	}
	if _, err := database.Exec(`INSERT INTO trb_jobs (id, queue_name, job_name, payload, payload_version, priority, run_at, state, attempts, maximum_attempts, created_at, updated_at) VALUES ('other-queue', 'default', 'RemovedJob', '[]', 1, 0, CURRENT_TIMESTAMP, 'ready', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
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
	if remaining != 3 {
		t.Fatalf("queue-filtered worker changed an unexpected number of jobs: %d", remaining)
	}
	if _, err := database.Exec(`DELETE FROM trb_jobs`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO trb_jobs (id, queue_name, job_name, payload, payload_version, priority, run_at, state, attempts, maximum_attempts, created_at, updated_at) VALUES ('unknown-1', 'default', 'RemovedJob', '[]', 1, 0, CURRENT_TIMESTAMP, 'ready', 0, 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(1100 * time.Millisecond)
		}
		stdout.Reset()
		stderr.Reset()
		if status := command.Run([]string{"jobs", "start", "--once", "--config", config.Path}); status != 0 {
			t.Fatalf("failing worker attempt %d status=%d stdout=%s stderr=%s", attempt+1, status, stdout.String(), stderr.String())
		}
	}
	database, err = sql.Open("sqlite", databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	var failedState string
	var failedAttempts int
	if err := database.QueryRow(`SELECT state, attempts FROM trb_jobs WHERE id = 'unknown-1'`).Scan(&failedState, &failedAttempts); err != nil {
		t.Fatal(err)
	}
	if failedState != "failed" || failedAttempts != 2 {
		t.Fatalf("unexpected failed job state=%q attempts=%d", failedState, failedAttempts)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"jobs", "list", "--config", config.Path}); status != 0 {
		t.Fatalf("list status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "unknown-1\tfailed\tRemovedJob\t2/2\tunknown job RemovedJob") {
		t.Fatalf("failed job not shown by list: %q stderr=%s", stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"jobs", "retry", "unknown-1", "--config", config.Path}); status != 0 || stderr.Len() != 0 {
		t.Fatalf("retry status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	database, err = sql.Open("sqlite", databaseSource)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT state, attempts FROM trb_jobs WHERE id = 'unknown-1'`).Scan(&failedState, &failedAttempts); err != nil {
		t.Fatal(err)
	}
	if failedState != "ready" || failedAttempts != 0 {
		t.Fatalf("retry did not reset job state=%q attempts=%d", failedState, failedAttempts)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"jobs", "discard", "unknown-1", "--config", config.Path}); status != 0 || stderr.Len() != 0 {
		t.Fatalf("discard status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"jobs", "retry", "missing-job", "--config", config.Path}); status == 0 || !strings.Contains(stderr.String(), "failed job not found") {
		t.Fatalf("missing retry status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
}

func TestGoJobWorkerBuildsWithWebAndImportedAliasPayload(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/jobs-web-integration"
	configureSQLJobs(t, config, "sqlite", filepath.Join(root, "jobs.sqlite3"))
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"domain", "jobs", "routes"} {
		if err := os.MkdirAll(filepath.Join(config.SourcePath(), directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sources := map[string]string{
		"domain/item_id.trb": "type ItemId = Integer\n",
		"jobs/sync_item_job.trb": `import { ItemId } from domain/item_id
import { Job } from trb/jobs

class SyncItemJob < Job
	def perform(item_id: ItemId)
		puts(item_id)
		return
	end
end
`,
		"routes/index.trb": `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("ok")
end
`,
		"main.trb": `import { configure_server, serve } from trb/web

def main()
	serve(configure_server(host: "127.0.0.1", port: 0))
end
`,
	}
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(config.SourcePath(), name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"jobs", "start", "--once", "--config", config.Path}); status != 0 {
		t.Fatalf("worker status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
}
