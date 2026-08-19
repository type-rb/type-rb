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

func TestRunResultFunctionValuesAcrossAvailableBackends(t *testing.T) {
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

def read_number(): Result<Integer, AppError>
	return Result<Integer, AppError>::Ok(7)
end

def read_eight(): Result<Integer, AppError>
	return Result<Integer, AppError>::Ok(8)
end

def invoke<T, E>(callback: () -> Result<T, E>): Result<T, E>
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
	first: () -> Result<Integer, AppError> := fn(): Result<Integer, AppError>
		return read_number()
	end
	second: () -> Result<Integer, AppError> := fn(): Result<Integer, AppError>
		return read_eight()
	end
	print_result(invoke<Integer, AppError>(first))
	print_result(invoke<Integer, AppError>(second))
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

func TestRunCollectionTransformsAcrossAvailableBackends(t *testing.T) {
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
				config.Go.Module = "example.com/type-rb/collection-effect-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}

			source := `def render(value: Integer): String
	return value.to_s()
end

def positive?(value: Integer): Boolean
	return value > 0
end

def add(left: Integer, right: Integer): Integer
	return left + right
end

def render_all(values: Array<Integer>): Array<String>
	return values.map do |value|
		render(value)
	end
end

def exercise_transforms(): Integer
	[1, 2, 3].select do |value|
		positive?(value)
	end
	[1, 2, 3].reduce(0) do |sum, value|
		add(sum, value)
	end
	[1, 2, 3].any? do |value|
		positive?(value)
	end
	[1, 2, 3].all? do |value|
		positive?(value)
	end
	[1, 2, 3].none? do |value|
		positive?(value)
	end
	[1, 2, 3].find do |value|
		positive?(value)
	end
	[1, 2, 3].find_index do |value|
		positive?(value)
	end
	return 1
end

def main()
	values := render_all([1, 2, 3])
	puts(values[0])
	puts(exercise_transforms())
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
			if stdout.String() != "1\n1\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected output %q, stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}
