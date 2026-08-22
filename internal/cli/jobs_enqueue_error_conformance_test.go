package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestNegativeJobDelayReturnsPortableErrorAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			requireORMApplicationRuntime(t, mode, "sqlite")
			root := t.TempDir()
			databaseSource := filepath.Join(root, "jobs.sqlite3")
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/jobs-error-conformance"
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
			source := `import { EnqueueErrorKind, Job } from trb/jobs
import { puts } from trb/std/io
import { Result } from trb/std/result
import { Duration } from trb/std/time

class RejectedJob < Job
	def perform(value: Integer)
		return
	end
end

def main()
	case RejectedJob.perform_in(Duration.seconds(-1), 7)
	when Result::Ok(_reference)
		puts("unexpected success")
	when Result::Err(error)
		if error.kind == EnqueueErrorKind::InvalidArgument
			puts("invalid: " + error.message)
		else
			puts("unexpected error")
		end
	end
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "invalid: job delay must not be negative\n" {
				t.Fatalf("unexpected output %q stderr=%s", stdout.String(), stderr.String())
			}
			if _, err := os.Stat(databaseSource); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("negative delay reached the SQL adapter: %v", err)
			}
		})
	}
}

func TestSQLJobAdapterReturnsPortableErrorAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			requireORMApplicationRuntime(t, mode, "sqlite")
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/jobs-adapter-error-conformance"
			}
			if config.TypeScript != nil {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			// A directory is not a valid SQLite database file, so the adapter must
			// convert the native open/schema failure into EnqueueErrorKind::Adapter.
			configureSQLJobs(t, config, "sqlite", root)
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			source := `import { EnqueueErrorKind, Job } from trb/jobs
import { puts } from trb/std/io
import { Result } from trb/std/result

class RejectedJob < Job
	def perform(value: Integer)
		return
	end
end

def main()
	case RejectedJob.perform_later(7)
	when Result::Ok(_reference)
		puts("unexpected success")
	when Result::Err(error)
		if error.kind == EnqueueErrorKind::Adapter
			puts("adapter")
		else
			puts("unexpected error")
		end
	end
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "adapter\n" {
				t.Fatalf("unexpected output %q stderr=%s", stdout.String(), stderr.String())
			}
		})
	}
}
