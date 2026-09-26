package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func writeTRBModeFile(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runTRBModeCommand(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	status := command.Run(args)
	return status, stdout.String(), stderr.String()
}

func writeTRBModeProject(t *testing.T, packageModes string) string {
	t.Helper()
	root := t.TempDir()
	writeTRBModeFile(t, filepath.Join(root, "widgets", "trbpackage.json"), `{
  "formatVersion": 1,
  "name": "example.com/local/widgets",
  "version": "0.1.0",
  "sourceDir": "src"`+packageModes+`
}
`)
	writeTRBModeFile(t, filepath.Join(root, "widgets", "src", "index.trb"), "def label(): String\n\treturn \"widget\"\nend\n")
	app := filepath.Join(root, "app")
	writeTRBModeFile(t, filepath.Join(app, project.ConfigName), `{
  "name": "trb-mode",
  "mode": "trb",
  "sourceDir": "src",
  "packages": { "local/widgets": { "path": "../widgets" } }
}
`)
	writeTRBModeFile(t, filepath.Join(app, "src", "main.trb"), "import { label } from local/widgets\n\ndef main()\n\tputs(label())\nend\n")
	writeTRBModeFile(t, filepath.Join(app, "src", "main_test.trb"), "import { label } from local/widgets\nimport { describe, expect, test } from trb/std/test\n\ndescribe(\"Widgets\") do\n\ttest(\"label\") do\n\t\texpect(label()).to_equal(\"widget\")\n\tend\nend\n")
	return app
}

func TestTRBModeSupportsAnalysisCommandsWithoutABackend(t *testing.T) {
	app := writeTRBModeProject(t, "")
	t.Chdir(app)

	if status, stdout, stderr := runTRBModeCommand("install"); status != 0 || !strings.Contains(stdout, "resolved 1 TypeRB package(s)") {
		t.Fatalf("install status=%d stdout=%s stderr=%s", status, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(app, "trb.lock")); err != nil {
		t.Fatalf("install did not write trb.lock: %v", err)
	}
	for _, manifest := range []string{"go.mod", "Gemfile", "package.json"} {
		if _, err := os.Stat(filepath.Join(app, manifest)); !os.IsNotExist(err) {
			t.Fatalf("mode trb wrote host manifest %s: %v", manifest, err)
		}
	}
	if status, stdout, stderr := runTRBModeCommand("check"); status != 0 || !strings.Contains(stdout, "checked 2 file(s) for mode trb") {
		t.Fatalf("check status=%d stdout=%s stderr=%s", status, stdout, stderr)
	}
	for _, args := range [][]string{{"fmt", "--check", "."}, {"lint"}} {
		if status, stdout, stderr := runTRBModeCommand(args...); status != 0 {
			t.Fatalf("%v status=%d stdout=%s stderr=%s", args, status, stdout, stderr)
		}
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"build"}, want: "build is not available for mode trb in this implementation"},
		{args: []string{"run"}, want: "run is not available for mode trb in this implementation"},
		{args: []string{"test"}, want: "test is not available for mode trb in this implementation"},
		{args: []string{"repl"}, want: "repl is not available for mode trb in this implementation"},
		{args: []string{"sync"}, want: "native package management is not available for mode trb in this implementation"},
		{args: []string{"add", "--native", "left-pad", "1.0.0"}, want: "native package management is not available for mode trb in this implementation"},
	} {
		if status, stdout, stderr := runTRBModeCommand(test.args...); status == 0 || !strings.Contains(stderr, test.want) {
			t.Fatalf("%v status=%d stdout=%s stderr=%s", test.args, status, stdout, stderr)
		}
	}
	for _, path := range []string{"build", filepath.Join(".trb", "generated")} {
		if _, err := os.Stat(filepath.Join(app, path)); !os.IsNotExist(err) {
			t.Fatalf("a rejected command produced %s: %v", path, err)
		}
	}

	writeTRBModeFile(t, filepath.Join(app, "src", "main.trb"), "import { label } from local/widgets\n\ndef main()\n\tcount: Integer := label()\n\tputs(count.to_s())\nend\n")
	if status, stdout, stderr := runTRBModeCommand("check"); status == 0 || !strings.Contains(stderr, "error[TRB3000]") {
		t.Fatalf("check must report type errors in mode trb: status=%d stdout=%s stderr=%s", status, stdout, stderr)
	}
}

func TestTRBModeRespectsPackagesRestrictedToOtherModes(t *testing.T) {
	app := writeTRBModeProject(t, `,
  "modes": ["go", "ruby", "typescript"]`)
	t.Chdir(app)
	if status, stdout, stderr := runTRBModeCommand("install"); status == 0 || !strings.Contains(stderr, "does not support mode trb") {
		t.Fatalf("install status=%d stdout=%s stderr=%s", status, stdout, stderr)
	}
}

func TestInitTRBModeWritesOnlyTheConfiguration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "native-app")
	status, stdout, stderr := runTRBModeCommand("init", "--mode", "trb", root)
	if status != 0 {
		t.Fatalf("init status=%d stdout=%s stderr=%s", status, stdout, stderr)
	}
	config, err := project.Load(filepath.Join(root, project.ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	if config.Mode != "trb" || config.Go != nil || config.Ruby != nil || config.TypeScript != nil {
		t.Fatalf("unexpected trb configuration: %#v", config)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != project.ConfigName {
		t.Fatalf("init wrote more than the configuration: %v", entries)
	}
	if strings.TrimSpace(stdout) != filepath.Join(root, project.ConfigName) {
		t.Fatalf("unexpected init output %q", stdout)
	}
}

func TestStandaloneTRBModeSupportsAnalysisOnly(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	writeTRBModeFile(t, filepath.Join(directory, "main.trb"), "def main()\n\tputs(\"ok\")\nend\n")
	for _, args := range [][]string{{"check", "--mode", "trb", "main.trb"}, {"lint", "--mode", "trb", "main.trb"}} {
		if status, stdout, stderr := runTRBModeCommand(args...); status != 0 {
			t.Fatalf("%v status=%d stdout=%s stderr=%s", args, status, stdout, stderr)
		}
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"run", "--mode", "trb", "main.trb"}, want: "run is not available for mode trb in this implementation"},
		{args: []string{"repl", "--mode", "trb"}, want: "repl is not available for mode trb in this implementation"},
	} {
		if status, stdout, stderr := runTRBModeCommand(test.args...); status == 0 || !strings.Contains(stderr, test.want) {
			t.Fatalf("%v status=%d stdout=%s stderr=%s", test.args, status, stdout, stderr)
		}
	}
}
