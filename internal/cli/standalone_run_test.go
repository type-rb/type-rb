package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunStandaloneFileAcrossBackends(t *testing.T) {
	tests := []struct {
		name     string
		required string
		args     func(string) []string
	}{
		{name: "go-default-direct", required: "go", args: func(filename string) []string { return []string{filename} }},
		{name: "ruby", required: "ruby", args: func(filename string) []string { return []string{"run", "--mode", "ruby", filename} }},
		{name: "typescript-node", required: "node", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "node", filename}
		}},
		{name: "typescript-bun", required: "bun", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "bun", filename}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.required); err != nil {
				t.Skipf("%s is unavailable: %v", test.required, err)
			}
			root := t.TempDir()
			filename := filepath.Join(root, "hello.trb")
			source := `import { imported_message } from helper
import { puts } from trb/std/io

def initialize_message(): String
	puts("initializer")
	return "value"
end

puts("standalone-ok")
puts(imported_message())
message := initialize_message()
puts(message)

def main<T>(value: T): T
	return value
end

puts(main<String>("explicit-main"))
`
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "helper.trb"), []byte("def imported_message(): String\n\treturn \"imported\"\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "broken.trb"), []byte("this is not valid TypeRB"), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(test.args(filename)); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "standalone-ok\nimported\ninitializer\nvalue\nexplicit-main\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			for _, path := range []string{filepath.Join(root, project.ConfigName), filepath.Join(root, ".trb")} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("standalone run wrote project state at %s: %v", path, err)
				}
			}
		})
	}
}

func TestRunStandaloneAllowsFilesWithoutMain(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "library.trb")
	if err := os.WriteFile(filename, []byte("def answer(): Integer\n\treturn 42\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", filename}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunStandaloneValidatesOptions(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "library.trb")
	if err := os.WriteFile(filename, []byte("puts(\"ready\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "mode", args: []string{"run", "--mode", "python", filename}, want: "standalone mode must be ruby, go, or typescript"},
		{name: "runtime-mode", args: []string{"run", "--runtime", "bun", filename}, want: "--runtime requires --mode typescript"},
		{name: "runtime-name", args: []string{"run", "--mode", "typescript", "--runtime", "deno", filename}, want: "standalone TypeScript runtime must be node or bun"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(test.args); status != 1 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("diagnostic %q does not contain %q", stderr.String(), test.want)
			}
		})
	}
}

func TestBuildStandaloneDebugExecutable(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	root := t.TempDir()
	filename := filepath.Join(root, "hello.trb")
	if err := os.WriteFile(filename, []byte("puts(\"standalone-debug\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "debug-app")
	t.Setenv("CGO_ENABLED", "0")
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--compile", "--debug", "--outfile", output, filename}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	result, err := exec.Command(output).CombinedOutput()
	if err != nil || string(result) != "standalone-debug\n" {
		t.Fatalf("compiled executable failed: err=%v output=%q", err, result)
	}
	binary, err := os.ReadFile(output)
	if err != nil || !bytes.Contains(binary, []byte(filename)) {
		t.Fatalf("debug executable does not retain the TypeRB source path: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, project.ConfigName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("standalone build wrote project config: %v", err)
	}
}

func TestRunProjectRejectsStandaloneOverrides(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/type-rb/project-precedence"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "main.trb")
	if err := os.WriteFile(filename, []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"run", "--mode", "ruby", filename},
		{"run", "--runtime", "bun", filename},
	} {
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run(args); status != 1 {
			t.Fatalf("args=%v status=%d stdout=%s stderr=%s", args, status, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), "available only when trbconfig.jsonc is unavailable") {
			t.Fatalf("args=%v unexpected diagnostic: %s", args, stderr.String())
		}
	}
}

func TestRunWithoutProjectOrFileExplainsStandaloneRequirement(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "run requires FILE.trb when trbconfig.jsonc is unavailable") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}
