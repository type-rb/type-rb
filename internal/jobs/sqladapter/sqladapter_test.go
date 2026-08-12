package sqladapter

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/parser"
)

func TestParseConfigurationUsesTypedSQLAdapter(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`import { JobAdapter } from trb/jobs
import { SQLAdapter, SQLDatabase } from trb/jobs/sql
import { Duration } from trb/std/time

def configure_jobs(): JobAdapter
	return SQLAdapter.new(
		database_adapter: SQLDatabase::PostgreSQL,
		database: "jobs",
		poll_interval: Duration.milliseconds(250),
		worker_concurrency: 4,
	)
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %v", diagnostics)
	}
	config, err := ParseConfiguration(program)
	if err != nil {
		t.Fatal(err)
	}
	if config.DatabaseAdapter != "postgresql" || config.Database != "jobs" || config.PollIntervalMilliseconds != 250 || config.WorkerConcurrency != 4 {
		t.Fatalf("unexpected SQL job configuration: %#v", config)
	}
}

func TestParseConfigurationRejectsMultipleSQLiteWorkers(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`def configure_jobs(): JobAdapter
	return SQLAdapter.new(worker_concurrency: 2)
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %v", diagnostics)
	}
	_, err := ParseConfiguration(program)
	if err == nil || !strings.Contains(err.Error(), "worker_concurrency: 1") {
		t.Fatalf("expected SQLite concurrency diagnostic, got %v", err)
	}
}
