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

func TestBuildStandaloneGoExecutableUsesFileRootGraph(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "report.trb")
	helper := filepath.Join(root, "message.trb")
	if err := os.WriteFile(entry, []byte(`import { message } from message

def main()
	puts(message())
	return
end
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte(`def message(): String
	return "standalone-build"
end
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated.trb"), []byte("not valid TypeRB"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "debug", "report")
	t.Setenv("CGO_ENABLED", "0")
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--compile", "--debug", "--outfile", output, entry}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	result, err := exec.Command(output).CombinedOutput()
	if err != nil || string(result) != "standalone-build\n" {
		t.Fatalf("compiled executable failed: err=%v output=%q", err, result)
	}
	binary, err := os.ReadFile(output)
	if err != nil || !bytes.Contains(binary, []byte(entry)) || !bytes.Contains(binary, []byte(helper)) {
		t.Fatalf("debug executable does not retain file-root source paths: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, project.ConfigName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("standalone build wrote project config: %v", err)
	}
}

func TestBuildStandaloneGoExecutableDefaultsToSourceStem(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "report.trb")
	if err := os.WriteFile(entry, []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CGO_ENABLED", "0")
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--compile", entry}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	output := filepath.Join(root, "bin", "report")
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	if info, err := os.Stat(output); err != nil || info.IsDir() {
		t.Fatalf("standalone executable was not created at %s: info=%v err=%v", output, info, err)
	}
}

func TestBuildStandaloneGoExecutableRequiresEntryMain(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "library.trb")
	helper := filepath.Join(root, "helper.trb")
	if err := os.WriteFile(entry, []byte("import { value } from helper\n\ndef answer(): Integer\n\treturn value()\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte("def value(): Integer\n\treturn 42\nend\n\ndef main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--compile", entry}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "standalone file has no top-level main()") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}

func TestBuildStandaloneGoExecutableResolvesRelativeOutputFromEntryRoot(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "report.trb")
	if err := os.WriteFile(entry, []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CGO_ENABLED", "0")
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--compile", "--mode", "go", "--outfile", filepath.Join("dist", "report"), entry}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	output := filepath.Join(root, "dist", "report")
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	if info, err := os.Stat(output); err != nil || info.IsDir() {
		t.Fatalf("standalone executable was not created at %s: info=%v err=%v", output, info, err)
	}
}

func TestBuildCompileKeepsDiscoveredProjectAuthoritative(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/configured"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "report.trb")
	if err := os.WriteFile(entry, []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--compile", entry}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "does not accept source paths") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "bin", "report")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("configured build unexpectedly fell back to a file-root executable: %v", err)
	}
}

func TestBuildStandaloneGoExecutableRejectsSourceOutputFlags(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "report.trb")
	if err := os.WriteFile(entry, []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--stdout", "--out-dir=generated", "--copy=true"} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"build", "--compile", flag, entry}); status != 1 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "cannot be combined") {
				t.Fatalf("unexpected diagnostic: %s", stderr.String())
			}
		})
	}
}
