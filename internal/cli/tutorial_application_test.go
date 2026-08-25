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

func TestWebORMJobsTutorialAcrossBackends(t *testing.T) {
	fixtureRoot := filepath.Join("..", "..", "examples", "tutorials", "web-orm-jobs")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			requireORMApplicationRuntime(t, mode, "sqlite")
			root := t.TempDir()
			if err := os.CopyFS(root, os.DirFS(fixtureRoot)); err != nil {
				t.Fatal(err)
			}
			configureTutorialMode(t, root, mode)

			applicationDatabase := filepath.Join(root, "application.sqlite3")
			jobsDatabase := filepath.Join(root, "jobs.sqlite3")
			initializeTutorialDatabase(t, root, applicationDatabase)
			t.Setenv("DATABASE_URL", applicationDatabase)
			t.Setenv("JOBS_DATABASE_URL", jobsDatabase)

			configPath := filepath.Join(root, project.ConfigName)
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"test", "--config", configPath}); status != 0 {
				t.Fatalf("test status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "PASS Report API / accepts a report for background processing") {
				t.Fatalf("tutorial request test did not run: stdout=%s stderr=%s", stdout.String(), stderr.String())
			}
			assertTutorialReportStatus(t, applicationDatabase, "pending")
			assertTutorialQueuedJob(t, jobsDatabase)

			stdout.Reset()
			stderr.Reset()
			if status := command.Run([]string{"jobs", "start", "--once", "--config", configPath}); status != 0 {
				t.Fatalf("worker status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			assertTutorialReportStatus(t, applicationDatabase, "ready")
			assertTutorialQueueEmpty(t, jobsDatabase)
		})
	}
}

func configureTutorialMode(t *testing.T, root, mode string) {
	t.Helper()
	config, err := project.Load(filepath.Join(root, project.ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	config.Mode = mode
	config.Go = nil
	config.Ruby = nil
	config.TypeScript = nil
	switch mode {
	case "go":
		config.Go = &project.GoConfig{Module: "example.com/report-api", Version: project.DefaultGoVersion, RootPackage: "main"}
	case "ruby":
		config.Ruby = &project.RubyConfig{Source: "https://rubygems.org", Version: project.DefaultRubyVersion, Loader: "require_relative"}
	case "typescript":
		config.TypeScript = &project.TypeScriptConfig{PackageManager: "bun", ModuleType: "module", Runtime: project.TypeScriptRuntimeBun}
		if config.DevDependencies == nil {
			config.DevDependencies = map[string]string{}
		}
		config.DevDependencies["typescript"] = project.DefaultTypeScriptVersion
	default:
		t.Fatalf("unsupported tutorial mode %q", mode)
	}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
}

func initializeTutorialDatabase(t *testing.T, root, databasePath string) {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join(root, "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
}

func assertTutorialReportStatus(t *testing.T, databasePath, want string) {
	t.Helper()
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var title, status string
	if err := database.QueryRow("SELECT title, status FROM reports ORDER BY id LIMIT 1").Scan(&title, &status); err != nil {
		t.Fatal(err)
	}
	if title != "August report" || status != want {
		t.Fatalf("tutorial report title=%q status=%q, want title=%q status=%q", title, status, "August report", want)
	}
}

func assertTutorialQueuedJob(t *testing.T, databasePath string) {
	t.Helper()
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var jobName, queue, state string
	if err := database.QueryRow("SELECT job_name, queue_name, state FROM trb_jobs LIMIT 1").Scan(&jobName, &queue, &state); err != nil {
		t.Fatal(err)
	}
	if jobName != "GenerateReportJob" || queue != "reports" || state != "ready" {
		t.Fatalf("tutorial job name=%q queue=%q state=%q", jobName, queue, state)
	}
}

func assertTutorialQueueEmpty(t *testing.T, databasePath string) {
	t.Helper()
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM trb_jobs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tutorial worker left %d queued job(s)", count)
	}
}
