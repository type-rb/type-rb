package sqladapter

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/parser"
)

func TestParseConfigurationUsesTypedSQLAdapter(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`import { JobAdapter } from trb/jobs
import { SQLAdapter, SQLDialect } from trb/jobs/sql
import { Duration } from trb/std/time

def configure_jobs(): JobAdapter
	return SQLAdapter.new(
		dialect: SQLDialect::PostgreSQL,
		source: "jobs",
		poll_interval: Duration.milliseconds(250),
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
	if config.Dialect != "postgresql" || config.Source != "jobs" || config.PollIntervalMilliseconds != 250 {
		t.Fatalf("unexpected SQL job configuration: %#v", config)
	}
}

func TestParseConfigurationRejectsUnimplementedRuntimeTuning(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`def configure_jobs(): JobAdapter
	return SQLAdapter.new(worker_concurrency: 2)
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %v", diagnostics)
	}
	_, err := ParseConfiguration(program)
	if err == nil || !strings.Contains(err.Error(), "has no option worker_concurrency") {
		t.Fatalf("expected unsupported runtime tuning diagnostic, got %v", err)
	}
}

func TestParseConfigurationTreatsSourceEnvironmentAsRequired(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`def configure_jobs(): JobAdapter
	return SQLAdapter.new(source_environment: "JOBS_DATABASE_URL")
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %v", diagnostics)
	}
	config, err := ParseConfiguration(program)
	if err != nil {
		t.Fatal(err)
	}
	if config.SourceEnvironment != "JOBS_DATABASE_URL" {
		t.Fatalf("source environment = %q", config.SourceEnvironment)
	}
}
