package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunFallibleFunctionValuesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ruby" {
				if _, err := exec.LookPath("ruby"); err != nil {
					t.Skip("ruby is not installed")
				}
			}
			if mode == "typescript" {
				if _, err := exec.LookPath("node"); err != nil {
					t.Skip("node is not installed")
				}
			}

			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/function-effect-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}

			source := `import { Result } from trb/std/result

record AppError
	message: String
end

def read_number(): Integer fails AppError
	return 7
end

def invoke<T, E>(callback: () -> T fails E): T fails E
	return callback()
end

def print_result(result: Result<Integer, AppError>)
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
	return
end

def main()
	fallible: () -> Integer fails AppError := fn(): Integer fails AppError
		return read_number()
	end
	pure: () -> Integer fails AppError := fn(): Integer
		return 8
	end
	print_result(attempt invoke<Integer, AppError>(fallible))
	print_result(attempt invoke<Integer, AppError>(pure))
	return
end
`
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if stdout.String() != "7\n8\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected output %q, stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}
