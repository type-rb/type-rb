package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func configureSQLJobs(t *testing.T, config *project.Config, adapter, database string) {
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
import { SQLAdapter, SQLDatabase } from trb/jobs/sql

def configure_jobs(): JobAdapter
	return SQLAdapter.new(
		database_adapter: SQLDatabase::` + member + `,
		database: ` + strconv.Quote(database) + `,
	)
end
`
	if err := os.WriteFile(filepath.Join(directory, "jobs.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}
