package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestReplResultTopLevelBoundariesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/repl-result-boundary-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}

			input := "import { Result } from trb/std/result\n" +
				"def source(success: Boolean): Result<Integer, String>; if success; return Result<Integer, String>::Ok(7); end; return Result<Integer, String>::Err(\"missing\"); end\n" +
				"source(false)\n" +
				"source(false) catch |_error|\n41\nend\n" +
				"try source(true)\n" +
				"1 + 1\n" +
				":quit\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
			}

			want := "Result::Err(error: \"missing\") : Result<Integer, String>\n41 : Integer\n2 : Integer\n"
			if stdout.String() != want {
				t.Fatalf("unexpected %s Result boundary REPL output\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "try is only valid inside a function or method that returns Result<T, E>") {
				t.Fatalf("%s REPL did not diagnose top-level try before continuing:\n%s", mode, stderr.String())
			}
		})
	}
}
