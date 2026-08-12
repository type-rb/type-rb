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

func TestJobEnqueueFromProjectREPLAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			requireORMApplicationRuntime(t, mode, "sqlite")
			root := t.TempDir()
			databaseSource := filepath.Join(root, "jobs.sqlite3")
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/jobs-repl-conformance"
			}
			if config.TypeScript != nil {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			configureSQLJobs(t, config, "sqlite", databaseSource)
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			source := `import { Job, priority, queue } from trb/jobs

class ReplJob < Job
	queue("interactive")
	priority(3)

	def perform(value: Integer)
		puts(value)
		return
	end
end

def main()
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			input := "import { ReplJob } from main\nimport { Duration } from trb/std/time\nReplJob.perform_later(7)\nReplJob.perform_later_in(Duration.seconds(60), 8)\n:quit\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			database, err := sql.Open("sqlite", databaseSource)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			var jobName, payload, queueName string
			var priority int
			if err := database.QueryRow(`SELECT job_name, payload, queue_name, priority FROM trb_jobs ORDER BY run_at LIMIT 1`).Scan(&jobName, &payload, &queueName, &priority); err != nil {
				t.Fatalf("REPL did not persist a job: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
			}
			if jobName != "ReplJob" || payload != `[7]` || queueName != "interactive" || priority != 3 {
				t.Fatalf("unexpected REPL job name=%q payload=%q queue=%q priority=%d", jobName, payload, queueName, priority)
			}
			var delayed int
			if err := database.QueryRow(`SELECT COUNT(*) FROM trb_jobs WHERE payload = '[8]' AND run_at > CURRENT_TIMESTAMP`).Scan(&delayed); err != nil {
				t.Fatal(err)
			}
			if delayed != 1 {
				t.Fatal("REPL did not persist the delayed job in the future")
			}
		})
	}
}
