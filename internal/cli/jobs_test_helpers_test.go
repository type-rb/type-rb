package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func configureSQLJobs(t *testing.T, config *project.Config, adapter, database string) {
	configureSQLJobsSource(t, config, adapter, "source: "+strconv.Quote(database))
}

func configureSQLJobsFromEnvironment(t *testing.T, config *project.Config, adapter, environment string) {
	configureSQLJobsSource(t, config, adapter, "source_environment: "+strconv.Quote(environment))
}

func configureSQLJobsSource(t *testing.T, config *project.Config, adapter, sourceArgument string) {
	t.Helper()
	member := map[string]string{"sqlite": "SQLite", "postgresql": "PostgreSQL", "mysql": "MySQL"}[adapter]
	if member == "" {
		t.Fatalf("unsupported jobs test adapter %q", adapter)
	}
	config.Jobs = &project.JobsConfig{Configuration: "config/jobs"}
	directory := filepath.Join(config.SourcePath(), "config")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { JobAdapter } from trb/jobs
import { SQLAdapter, SQLDialect } from trb/jobs/sql

def configure_jobs(): JobAdapter
	return SQLAdapter.new(
		dialect: SQLDialect::` + member + `,
		` + sourceArgument + `,
	)
end
`
	if err := os.WriteFile(filepath.Join(directory, "jobs.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}
