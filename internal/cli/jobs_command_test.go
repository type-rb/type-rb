package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestJobsCommandsRequireADispatchableEntrypoint(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			if config.Go != nil {
				config.Go.Module = "example.com/jobs-entrypoint"
			}
			if config.TypeScript != nil {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte("def main()\n  puts(\"application main ran\")\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, setup := range []string{"ordinary project", "adapter without jobs"} {
				t.Run(setup, func(t *testing.T) {
					database := filepath.Join(root, "jobs.sqlite3")
					if setup == "adapter without jobs" {
						configureSQLJobs(t, config, "sqlite", database)
					}
					if err := config.Save(); err != nil {
						t.Fatal(err)
					}
					for _, action := range [][]string{{"list"}, {"retry", "missing"}, {"discard", "missing"}, {"start", "--once"}} {
						var stdout, stderr bytes.Buffer
						command := &CLI{Stdout: &stdout, Stderr: &stderr}
						args := append([]string{"jobs"}, action...)
						status := command.Run(append(args, "--config", config.Path))
						if status == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "jobs "+action[0]+" requires an application entrypoint") {
							t.Errorf("%s: status=%d stdout=%q stderr=%q", action[0], status, stdout.String(), stderr.String())
						}
					}
					if _, err := os.Stat(database); !os.IsNotExist(err) {
						t.Fatalf("unexpected Jobs database: %v", err)
					}
				})
			}
		})
	}
}
