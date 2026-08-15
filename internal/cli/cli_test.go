package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/project"
)

func TestVersionCommandsPrintBuildVersion(t *testing.T) {
	previous := Version
	Version = "9.8.7-test"
	t.Cleanup(func() { Version = previous })

	for _, argument := range []string{"version", "--version", "-v"} {
		t.Run(argument, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{argument}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if stdout.String() != "9.8.7-test\n" {
				t.Fatalf("unexpected version output %q", stdout.String())
			}
		})
	}
}

func TestTestRunsPortableSuiteAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			required := map[string]string{"go": "go", "ruby": "ruby", "typescript": "bun"}[mode]
			if _, err := exec.LookPath(required); err != nil {
				t.Skipf("%s is unavailable: %v", required, err)
			}
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if mode == "go" {
				config.Go.Module = "example.com/type-rb/test-suite"
			}
			if mode == "typescript" {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if mode == "go" {
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/type-rb/test-suite\n\ngo 1.26\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			applicationSource := "def add(left: Integer, right: Integer): Integer\n\treturn left + right\nend\n\ndef main()\n\tputs(\"APPLICATION MAIN RAN\")\n\treturn\nend\n"
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "calculator.trb"), []byte(applicationSource), 0o644); err != nil {
				t.Fatal(err)
			}
			testSource := `import { add } from calculator
import { describe, expect, test } from trb/std/test

record Point
	x: Integer
	y: Integer
end

describe("Calculator") do
	test("adds numbers") do
		expect(add(1, 2)).to_equal(3)
		expect([1, 2]).to_equal([1, 2])
		expect({ a: 1, b: 2 }).to_equal({ b: 2, a: 1 })
		expect(Point.new(x: 1, y: 2)).to_equal(Point.new(x: 1, y: 2))
		expect(true).to_be_true()
		expect(false).to_be_false()
		expect(nil).to_be_nil()
	end
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "calculator_test.trb"), []byte(testSource), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"test", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "PASS Calculator / adds numbers") || !strings.Contains(stdout.String(), "1 test(s), 0 failure(s)") {
				t.Fatalf("unexpected test output:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String(), "APPLICATION MAIN RAN") {
				t.Fatalf("trb test executed the application main():\n%s", stdout.String())
			}
		})
	}
}

func TestTestReturnsFailureAndJSONEvents(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go is unavailable: %v", err)
	}
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/test-failure"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/type-rb/test-failure\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `import { describe, expect, test } from trb/std/test

describe("Failure") do
	test("reports its source") do
		expect(1).to_equal(2)
	end
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "failure_test.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"test", "--config", config.Path, "--reporter", "json"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"type":"test_failed"`) || !strings.Contains(stdout.String(), `"name":"Failure / reports its source"`) || !strings.Contains(stdout.String(), `"test_file":`) || !strings.Contains(stdout.String(), `expected 1 to equal 2`) {
		t.Fatalf("unexpected JSON event output:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `{"failed":1,"total":1,"type":"test_summary"}`) {
		t.Fatalf("JSON summary does not expose total and failed counts:\n%s", stdout.String())
	}
}

func TestTestCompileCreatesDebuggableGoExecutable(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go is unavailable: %v", err)
	}
	root := t.TempDir()
	config := project.New(root, "go")
	config.Name = "debug-tests"
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/debug-tests"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/type-rb/debug-tests\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testPath := filepath.Join(config.SourcePath(), "calculator_test.trb")
	source := "import { describe, expect, test } from trb/std/test\n\ndescribe(\"Calculator\") do\n\ttest(\"adds numbers\") do\n\t\texpect(1 + 2).to_equal(3)\n\tend\nend\n"
	if err := os.WriteFile(testPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, ".trb", "debug", "tests")
	actualOutput := output
	if runtime.GOOS == "windows" {
		actualOutput += ".exe"
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"test", "--config", config.Path, "--compile", "--debug", "--outfile", output}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if want := "test executable -> " + actualOutput + "\n"; stdout.String() != want {
		t.Fatalf("unexpected output: want %q, got %q", want, stdout.String())
	}
	binary, err := os.ReadFile(actualOutput)
	if err != nil || !bytes.Contains(binary, []byte(testPath)) {
		t.Fatalf("debug test executable does not retain the TypeRB source path: err=%v", err)
	}
	result, err := exec.Command(actualOutput).CombinedOutput()
	if err != nil || !strings.Contains(string(result), "PASS Calculator / adds numbers") {
		t.Fatalf("compiled tests failed: err=%v output=%q", err, result)
	}
}

func TestTestFileFilterDisambiguatesDuplicateNames(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go is unavailable: %v", err)
	}
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/test-file-filter"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/type-rb/test-file-filter\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := "import { describe, expect, test } from trb/std/test\n\ndescribe(\"Duplicate\") do\n\ttest(\"same name\") do\n\t\texpect(1).to_equal(1)\n\tend\nend\n"
	selected := filepath.Join(config.SourcePath(), "first_test.trb")
	for _, filename := range []string{selected, filepath.Join(config.SourcePath(), "second_test.trb")} {
		if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"test", "--config", config.Path, "--file", selected, "--reporter", "json"}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if strings.Count(stdout.String(), `"type":"test_started"`) != 1 || !strings.Contains(stdout.String(), `"test_file":`) {
		t.Fatalf("file filter did not select one declaration:\n%s", stdout.String())
	}
}

func TestCheckEmitsVersionedJSONDiagnosticsAcrossFiles(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/check"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"alpha.trb": "def alpha(): Integer\n\treturn missing_alpha()\nend\n",
		"beta.trb":  "def beta(): Integer\n\treturn missing_beta()\nend\n",
	} {
		if err := os.WriteFile(filepath.Join(config.SourcePath(), name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"check", "--config", config.Path, "--diagnostic-format", "json"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON diagnostics wrote to stderr: %s", stderr.String())
	}
	var report diagnostic.JSONReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON diagnostics: %v\n%s", err, stdout.String())
	}
	if report.SchemaVersion != diagnostic.JSONSchemaVersion || report.Summary.Errors != 2 || len(report.Diagnostics) != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
	for _, item := range report.Diagnostics {
		if item.Code != diagnostic.TypeError || item.Location == nil || item.Location.Span.Start.Line != 2 {
			t.Fatalf("unexpected diagnostic: %#v", item)
		}
	}
}

func TestCheckEmitsEmptyJSONReportForValidProject(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/type-rb/check-valid"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.trb"), []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"check", "--config", config.Path, "--diagnostic-format", "json"}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	var report diagnostic.JSONReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != diagnostic.JSONSchemaVersion || report.Summary.Errors != 0 || len(report.Diagnostics) != 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestInteractiveNoArgumentCommandStartsREPL(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	command := &CLI{
		Stdin:  strings.NewReader("1 + 2\n:quit\n"),
		Stdout: &stdout,
		Stderr: &stderr,
		terminal: func(io.Reader, io.Writer) bool {
			return true
		},
	}
	if status := command.Run(nil); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if stdout.String() != "3 : Integer\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestNonInteractiveNoArgumentCommandPrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run(nil); status != 2 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:\n  trb\n") || stderr.Len() != 0 {
		t.Fatalf("unexpected usage output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestOfficialPackageImportsContributeNativeDependencies(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/orm-app"
	config.PackageOptions["trb/orm"] = json.RawMessage(`{"adapter":"sqlite","database":"application.sqlite3"}`)
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(config.SourcePath(), "main.trb")
	if err := os.WriteFile(sourcePath, []byte("import { Model } from trb/orm\nclass Product < Model\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dependencies, err := projectPackageDependencies(config, []string{sourcePath})
	if err != nil {
		t.Fatal(err)
	}
	if dependencies["modernc.org/sqlite"] != "v1.53.0" {
		t.Fatalf("unexpected trb/orm dependencies: %#v", dependencies)
	}
}

func TestOfficialPackageOptionsSelectNativeDependencies(t *testing.T) {
	for _, test := range []struct {
		adapter, database, dependency, version string
	}{
		{adapter: "postgresql", database: "postgres://localhost/app", dependency: "github.com/jackc/pgx/v5", version: "v5.10.0"},
		{adapter: "mysql", database: "root@tcp(localhost)/app", dependency: "github.com/go-sql-driver/mysql", version: "v1.10.0"},
	} {
		t.Run(test.adapter, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, "go")
			config.SourceDir = "src"
			config.Go.Module = "example.com/orm-app"
			config.PackageOptions["trb/orm"] = json.RawMessage(`{"adapter":"` + test.adapter + `","database":"` + test.database + `"}`)
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(config.SourcePath(), "main.trb")
			if err := os.WriteFile(sourcePath, []byte("import { Model } from trb/orm\nclass Product < Model\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			dependencies, err := projectPackageDependencies(config, []string{sourcePath})
			if err != nil {
				t.Fatal(err)
			}
			if dependencies[test.dependency] != test.version || len(dependencies) != 1 {
				t.Fatalf("unexpected %s dependencies: %#v", test.adapter, dependencies)
			}
		})
	}
}

func TestTypedSQLJobsConfigurationSelectsNativeDependencies(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/jobs-app"
	configureSQLJobs(t, config, "postgresql", "postgres://localhost/jobs")
	jobPath := filepath.Join(config.SourcePath(), "send_receipt_job.trb")
	if err := os.WriteFile(jobPath, []byte("import { Job } from trb/jobs\n\nclass SendReceiptJob < Job\n\tdef perform()\n\t\treturn\n\tend\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(config.SourcePath(), "config", "jobs.trb")
	dependencies, err := projectPackageDependencies(config, []string{jobPath, configurationPath})
	if err != nil {
		t.Fatal(err)
	}
	if dependencies["github.com/jackc/pgx/v5"] != "v5.10.0" || dependencies["modernc.org/sqlite"] != "" || len(dependencies) != 1 {
		t.Fatalf("unexpected typed SQL jobs dependencies: %#v", dependencies)
	}
}

func TestInitWebTemplateBuildsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "web-app")
			arguments := []string{"init", "--mode", mode, "--template", "web"}
			if mode == "go" {
				arguments = append(arguments, "--module", "example.com/type-rb/web-template")
			}
			arguments = append(arguments, root)

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(arguments); status != 0 {
				t.Fatalf("init status=%d stderr=%s", status, stderr.String())
			}
			config, err := project.Load(filepath.Join(root, project.ConfigName))
			if err != nil {
				t.Fatal(err)
			}
			if config.SourceDir != "src" {
				t.Fatalf("sourceDir=%q, want src", config.SourceDir)
			}
			for _, relative := range []string{"main.trb", "routes/index.trb", "routes/_middleware.trb"} {
				path := filepath.Join(config.SourcePath(), filepath.FromSlash(relative))
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("generated template is missing %s: %v", relative, err)
				}
				if !strings.Contains(stdout.String(), path) {
					t.Fatalf("init output does not list %s:\n%s", path, stdout.String())
				}
			}

			stdout.Reset()
			stderr.Reset()
			if status := command.Run([]string{"fmt", "--check", config.SourcePath()}); status != 0 {
				t.Fatalf("fmt status=%d stderr=%s", status, stderr.String())
			}
			if status := command.Run([]string{"check", "--config", config.Path}); status != 0 {
				t.Fatalf("check status=%d stderr=%s", status, stderr.String())
			}
		})
	}
}

func TestInitTypeScriptBunRuntime(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bun-api")
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"init", "--mode", "typescript", "--runtime", "bun", root}); status != 0 {
		t.Fatalf("init status=%d stderr=%s", status, stderr.String())
	}
	config, err := project.Load(filepath.Join(root, project.ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	if config.TypeScript.Runtime != project.TypeScriptRuntimeBun || config.TypeScript.PackageManager != "bun" {
		t.Fatalf("unexpected Bun config: %#v", config.TypeScript)
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"packageManager": "bun"`) {
		t.Fatalf("package.json does not select Bun:\n%s", data)
	}
}

func TestInitRejectsInvalidTypeScriptRuntimeSelection(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown", args: []string{"init", "--mode", "typescript", "--runtime", "deno"}, want: "typescript.runtime must be browser, bun, or node"},
		{name: "other-mode", args: []string{"init", "--mode", "ruby", "--runtime", "bun"}, want: "--runtime is supported only for mode typescript"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "app")
			args := append(append([]string(nil), test.args...), root)
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(args); status != 1 || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
		})
	}
}

func TestTypeScriptRunCommandUsesConfiguredRuntime(t *testing.T) {
	target := "/project/main.ts"
	tests := []struct {
		runtime string
		want    []string
	}{
		{runtime: project.TypeScriptRuntimeNode, want: []string{"node", "--experimental-strip-types", target, "one"}},
		{runtime: project.TypeScriptRuntimeBun, want: []string{"bun", "run", target, "one"}},
	}
	for _, test := range tests {
		t.Run(test.runtime, func(t *testing.T) {
			command, err := typeScriptRunCommand(test.runtime, target, []string{"one"})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(command.Args, "\x00") != strings.Join(test.want, "\x00") {
				t.Fatalf("unexpected command: %#v", command.Args)
			}
		})
	}
	if _, err := typeScriptRunCommand(project.TypeScriptRuntimeBrowser, target, nil); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unexpected browser runtime result: %v", err)
	}
}

func TestRubyRunCommandUsesOneInterpreterForBundler(t *testing.T) {
	target := "/project/main.rb"
	command := rubyRunCommand(target, []string{"one"})
	want := []string{"ruby", "-rbundler/setup", target, "one"}
	if strings.Join(command.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected Ruby run command: want %#v, got %#v", want, command.Args)
	}
}

func TestRunTypeScriptProjectWithBunRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test executable uses a POSIX shell")
	}
	bin := t.TempDir()
	bun := filepath.Join(bin, "bun")
	if err := os.WriteFile(bun, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	config := project.New(root, "typescript")
	config.TypeScript.Runtime = project.TypeScriptRuntimeBun
	config.TypeScript.PackageManager = "bun"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	source := "def main()\n\tputs(\"ignored by fake Bun\")\n\treturn\nend\n"
	if err := os.WriteFile(filepath.Join(root, "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path, "--", "argument"}); status != 0 {
		t.Fatalf("run status=%d stderr=%s", status, stderr.String())
	}
	output := stdout.String()
	if !strings.HasPrefix(output, "run\n") || !strings.Contains(output, ".ts\nargument\n") {
		t.Fatalf("unexpected Bun invocation:\n%s", output)
	}
}

func TestInitWebTemplateDoesNotOverwriteSourceFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "src", "routes", "index.trb")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	status := command.Run([]string{"init", "--mode", "go", "--module", "example.com/existing", "--template", "web", root})
	if status != 1 || !strings.Contains(stderr.String(), "project template would overwrite") {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, project.ConfigName)); !os.IsNotExist(err) {
		t.Fatalf("init wrote config before validating template targets: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "existing\n" {
		t.Fatalf("existing source changed: %q, %v", data, err)
	}
}

func TestPlaygroundModeSelection(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		selected, err := playgroundMode(mode)
		if err != nil || selected != mode {
			t.Fatalf("playgroundMode(%q) = %q, %v", mode, selected, err)
		}
	}
	if _, err := playgroundMode("python"); err == nil || !strings.Contains(err.Error(), "--mode must be") {
		t.Fatalf("unexpected invalid-mode result: %v", err)
	}
}

func TestTourDoesNotExposeCheckFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"tour", "--check"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -check") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestReplUsesProjectModeKeepsStateAndLoadsProjectImports(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(root, "src", "models", "user.trb")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modelPath, []byte("record User\n  name: String\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		"import trb/std/strings",
		"import { User } from models/user",
		`name := "Ada"`,
		"strings.uppercase(name)",
		"user := User.new(name: name)",
		"user.name",
		":type user",
		"name = 1",
		"name",
		":quit",
	}, "\n") + "\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := strings.Join([]string{
		`"Ada" : String`,
		`"ADA" : String`,
		`User(name: "Ada") : User`,
		`"Ada" : String`,
		`User`,
		`"Ada" : String`,
		"",
	}, "\n")
	if stdout.String() != want {
		t.Fatalf("unexpected REPL output\nwant:\n%s\ngot:\n%s", want, stdout.String())
	}
	if !strings.Contains(stderr.String(), "cannot assign Integer to String") {
		t.Fatalf("REPL did not report and recover from the type error:\n%s", stderr.String())
	}
}

func TestReplAutomaticallyImportsUniqueProjectExportsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/repl-auto-import"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			modelPath := filepath.Join(root, "src", "models", "user.trb")
			if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(modelPath, []byte("record User\n\tname: String\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			input := "user := User.new(name: \"Ada\")\nuser.name\n:quit\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if got, want := stdout.String(), "User(name: \"Ada\") : User\n\"Ada\" : String\n"; got != want {
				t.Fatalf("stdout=%q, want %q; stderr=%s", got, want, stderr.String())
			}
		})
	}
}

func TestReplEvaluatesGenericInterfaceValuesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/repl-generic-interface"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			contractPath := filepath.Join(root, "src", "stores", "string_store.trb")
			if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
				t.Fatal(err)
			}
			source := `interface Store<T>
	get(): T
end

class MemoryStore<T> implements Store<T>
	@value: T

	def initialize(value: T)
		@value = value
		return
	end

	def get(): T
		return @value
	end
end
`
			if err := os.WriteFile(contractPath, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			input := "store: Store<String> := MemoryStore<String>.new(\"generic\")\nstore.get()\n:quit\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if !strings.HasSuffix(stdout.String(), "\"generic\" : String\n") || stderr.Len() != 0 {
				t.Fatalf("unexpected %s generic interface REPL output: stdout=%q stderr=%s", mode, stdout.String(), stderr.String())
			}
		})
	}
}

func TestReplAutomaticallyImportsPortableStandardTypesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/repl-standard-auto-import"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}

			input := "Date.parse(\"2026-08-11\").to_s()\n:quit\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if got, want := stdout.String(), "\"2026-08-11\" : String\n"; got != want {
				t.Fatalf("stdout=%q, want %q; stderr=%s", got, want, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", stderr.String())
			}
		})
	}
}

func TestReplPreludeDoesNotDuplicateExplicitImports(t *testing.T) {
	imports := []replImport{{path: "models/user", symbols: []string{"Profile", "User"}}}
	if got, want := replPrelude(imports, "import { User } from models/user\n"), "import { Profile } from models/user\n"; got != want {
		t.Fatalf("named import prelude=%q, want %q", got, want)
	}
	if got := replPrelude(imports, "import models/user\n"); got != "" {
		t.Fatalf("whole-module import prelude=%q, want empty", got)
	}
	if got, want := replPrelude(imports, "record User\nend\n"), "import { Profile } from models/user\n"; got != want {
		t.Fatalf("session declaration prelude=%q, want %q", got, want)
	}
}

func TestReplPreludeLazilyActivatesPortableStandardTypes(t *testing.T) {
	imports := []replImport{{path: "trb/std/time", symbols: []string{"Date", "DateTime"}, standard: true}}
	if got := replPrelude(imports, "1 + 2\n"); got != "" {
		t.Fatalf("unused standard prelude=%q, want empty", got)
	}
	if got, want := replPrelude(imports, "Date.parse(\"2026-08-11\")\n"), "import { Date } from trb/std/time\n"; got != want {
		t.Fatalf("referenced standard prelude=%q, want %q", got, want)
	}
}

func TestReplStandardCandidatesIncludeDateAndClassMembers(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-standard-candidates"
	imports := uniqueReplImports(nil, "__trb_repl__", "go")
	candidates, err := replStandardCandidates(config, imports, config.Go.RootPackage)
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range candidates.Symbols {
		if symbol.Name != "Date" {
			continue
		}
		for _, member := range symbol.Members {
			if member.Name == "parse" {
				return
			}
		}
		t.Fatal("Date completion candidate is missing parse()")
	}
	t.Fatal("Date completion candidate is missing")
}

func TestUniqueReplImportsOmitProjectAndStandardNameConflicts(t *testing.T) {
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{
		{Filename: "models/date.trb", ModulePath: "models/date", Package: "main", Source: []byte("record Date\nend\n")},
		{Filename: ".trb-repl.trb", ModulePath: "__trb_repl__", Package: "main", Source: []byte("")},
	}, compiler.Options{Mode: "go", Package: "main", ModulePath: "__trb_repl__", AllowUnusedImports: true, InteractiveModule: "__trb_repl__"})
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range uniqueReplImports(artifacts, "__trb_repl__", "go") {
		for _, symbol := range imported.symbols {
			if symbol == "Date" {
				t.Fatalf("ambiguous Date was auto-imported from %s", imported.path)
			}
		}
	}
}

func TestReplAutoImportsDoNotShiftUserDiagnostics(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-auto-import-diagnostic"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(root, "src", "models", "user.trb")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modelPath, []byte("record User\n\tname: String\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader("missing()\n:quit\n"), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if !strings.Contains(stderr.String(), ".trb-repl.trb:1:1: error[TRB3000]:") {
		t.Fatalf("hidden imports shifted the user diagnostic: %s", stderr.String())
	}
}

func TestReplExecutesPortableORMReads(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE products (id INTEGER PRIMARY KEY, category_id INTEGER NOT NULL, name TEXT NOT NULL, price REAL, active BOOLEAN NOT NULL, FOREIGN KEY (category_id) REFERENCES categories(id));
		CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE projects (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE memberships (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, project_id INTEGER NOT NULL, FOREIGN KEY (user_id) REFERENCES users(id), FOREIGN KEY (project_id) REFERENCES projects(id));
		INSERT INTO categories (id, name) VALUES (1, 'Featured'), (2, 'Archived');
		INSERT INTO products (category_id, name, price, active) VALUES (1, 'Priority', 10.5, TRUE), (2, 'Archive', NULL, FALSE);
		INSERT INTO users (id, name) VALUES (1, 'Ada');
		INSERT INTO projects (id, name) VALUES (1, 'TypeRB'), (2, 'Other');
		INSERT INTO memberships (id, user_id, project_id) VALUES (1, 1, 1), (2, 1, 2);
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-orm-test"
	config.PackageOptions["trb/orm"] = json.RawMessage(`{"adapter":"sqlite","database":"application.sqlite3"}`)
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte("import { Model, belongs_to, has_many } from trb/orm\n\nclass Category < Model\n\thas_many(Product)\nend\n\nclass Product < Model\n\tbelongs_to(Category)\nend\n\nclass User < Model\n\thas_many(Membership)\n\thas_many(Project, through: :memberships) do |projects|\n\t\tprojects.where(name: \"TypeRB\").order(id: :asc)\n\tend\nend\n\nclass Project < Model\n\thas_many(Membership)\nend\n\nclass Membership < Model\n\tbelongs_to(User)\n\tbelongs_to(Project)\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		"import { Category, Product, Project, User } from main",
		"import { Database, DbError } from trb/orm",
		"Product.where(id: [1, 2]).to_sql()",
		`Product.join(:category, Category.where(name: "Featured")).to_sql()`,
		`Product.join(:category, Category.where(name: "Featured")).count()`,
		`Product.left_join(:category, Category.where(name: "Missing")).count()`,
		`Product.where(category_id: Category.where(name: "Featured").select(:id)).count()`,
		`Product.where("category_id", "!=", Category.select(:id)).count()`,
		`Category.select(:id)`,
		`Product.where_exists(:category, Category.where(name: "Featured")).to_sql()`,
		`Product.where_exists(:category, Category.where(name: "Featured")).count()`,
		`Product.where_not_exists(:category, Category.where(name: "Featured")).count()`,
		"def grouped_product_count(): Integer fails DbError\n\tcounts := Product.group(:category_id).count()\n\tfiltered := Product.group(:category_id).having(:count, \">=\", 1).count()\n\tsums := Product.group(:category_id).sum(:id)\n\tlarge := Product.group(:category_id).having(:sum, :id, \">=\", 1).sum(:id)\n\taverages := Product.group(:category_id).average(:id)\n\tminimums := Product.group(:category_id).minimum(:name)\n\tmaximums := Product.group(:category_id).maximum(:price)\n\tpaged := Product.order(category_id: :desc).limit(1).group(:category_id).count()\n\toffset := Product.order(category_id: :asc).offset(1).group(:category_id).count()\n\treturn counts.size() + filtered.size() + sums.size() + large.size() + averages.size() + minimums.size() + maximums.size() + paged.size() + offset.size()\nend",
		"grouped_product_count()",
		"def nested_preload_count(): Integer fails DbError\n\tcategories := Category.preload(:products, Product.where(active: true).preload(:category)).all()\n\tproduct := categories[0].products[0]\n\tputs(product.category.name)\n\treturn categories[0].products.size()\nend",
		"nested_preload_count()",
		"def user_project_count(): Integer fails DbError\n\tusers := User.all()\n\treturn users[0].projects_query().count()\nend",
		"user_project_count()",
		"def preloaded_user_project_count(): Integer fails DbError\n\tusers := User.preload(:projects, Project.where(name: \"TypeRB\")).all()\n\treturn users[0].projects.size()\nend",
		"preloaded_user_project_count()",
		`User.join(:projects, Project.where(name: "TypeRB")).count()`,
		"attempt Category.preload(:products, Product.limit(1)).all()",
		`Product.exists?(name: "Priority")`,
		"Product.order(id: :asc).pluck(:name)",
		"Product.where(active: true).first()",
		"Product.count()",
		"Product.sum(:id)",
		"Product.order(id: :desc).limit(1).sum(:id)",
		"Product.sum(:price)",
		"Product.average(:id)",
		"Product.minimum(:name)",
		"Product.maximum(:price)",
		"Product.where(id: 999).sum(:price)",
		"Product.where(id: 999).minimum(:price)",
		"def first_product_name(): String fails DbError\n\tproducts := Product.where(id: 1).all()\n\treturn products[0].name\nend",
		"first_product_name()",
		"def locked_product_count(): Integer fails DbError\n\treturn Database.transaction() do |tx|\n\t\tproducts := Product.using(tx)\n\t\tlocked := products.lock().all()\n\t\tlocked.size()\n\tend\nend",
		"locked_product_count()",
		"def nested_product_count(): Integer fails DbError\n\treturn Database.transaction() do |tx|\n\t\tnested_result := tx.transaction() do |nested|\n\t\t\tproducts := Product.using(nested)\n\t\t\tloaded := products.all()\n\t\t\tloaded.size()\n\t\tend\n\t\tnested_result\n\tend\nend",
		"nested_product_count()",
		"def scoped_subquery_count(): Integer fails DbError\n\treturn Database.transaction() do |tx|\n\t\tcategory_ids := Category.using(tx).select(:id)\n\t\tcount := Product.using(tx).where(category_id: category_ids).count()\n\t\tcount\n\tend\nend",
		"scoped_subquery_count()",
		"attempt Product.lock().all()",
		":quit",
	}, "\n") + "\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := strings.Join([]string{
		`"SELECT \"id\", \"category_id\", \"name\", \"price\", \"active\" FROM \"products\" WHERE \"id\" IN (?, ?)" : String`,
		`"SELECT \"id\", \"category_id\", \"name\", \"price\", \"active\" FROM \"products\" INNER JOIN (SELECT \"id\" AS \"__trb_join_key\" FROM \"categories\" WHERE \"name\" = ?) AS \"__trb_join_0\" ON \"category_id\" = \"__trb_join_0\".\"__trb_join_key\"" : String`,
		`1 : Integer`,
		`2 : Integer`,
		`1 : Integer`,
		`0 : Integer`,
		`#<Subquery<Integer>> : Subquery<Integer>`,
		`"SELECT \"id\", \"category_id\", \"name\", \"price\", \"active\" FROM \"products\" WHERE EXISTS (SELECT 1 FROM \"categories\" WHERE \"categories\".\"id\" = \"products\".\"category_id\" AND (\"name\" = ?))" : String`,
		`1 : Integer`,
		`1 : Integer`,
		`16 : Integer`,
		`Featured`,
		`1 : Integer`,
		`1 : Integer`,
		`1 : Integer`,
		`1 : Integer`,
		`Result::Err(error: DbError(kind: DbErrorKind::InvalidData, message: "ORM preload query does not accept limit, offset, or lock")) : Result<Array<Category>, DbError>`,
		`true : Boolean`,
		`["Priority", "Archive"] : Array<String>`,
		`#<Product active: true, category_id: 1, id: 1, name: "Priority", price: 10.5> : Product?`,
		`2 : Integer`,
		`3 : Integer`,
		`2 : Integer`,
		`10.5 : Float`,
		`1.5 : Float?`,
		`"Archive" : String?`,
		`10.5 : Float?`,
		`0 : Float`,
		`nil : Float?`,
		`"Priority" : String`,
		`2 : Integer`,
		`2 : Integer`,
		`2 : Integer`,
		`Result::Err(error: DbError(kind: DbErrorKind::InvalidData, message: "database lock requires an explicit transaction scope")) : Result<Array<Product>, DbError>`,
		"",
	}, "\n")
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected ORM REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplExecutesORMThroughSharedHostRuntimeInEveryMode(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec(`
				CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
				INSERT INTO products (id, name) VALUES (1, 'Portable');
			`); err != nil {
				database.Close()
				t.Fatal(err)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}

			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/repl-orm-portable-test"
			}
			config.PackageOptions["trb/orm"] = json.RawMessage(`{"adapter":"sqlite","database":"application.sqlite3"}`)
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte("import { Model } from trb/orm\n\nclass Product < Model\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{
				Stdin: strings.NewReader(strings.Join([]string{
					"import { Product } from main",
					"Product.count()",
					`Product.update_all(name: "Updated")`,
					`Product.where(name: "Updated").count()`,
					"Product.delete_all()",
					"Product.count()",
					":quit",
				}, "\n") + "\n"),
				Stdout: &stdout, Stderr: &stderr,
			}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if stdout.String() != "1 : Integer\n1 : Integer\n1 : Integer\n1 : Integer\n0 : Integer\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected %s ORM REPL output: stdout=%q stderr=%q", mode, stdout.String(), stderr.String())
			}
		})
	}
}

func TestReplLoadsPortableORMAssociationProperties(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE products (id INTEGER PRIMARY KEY, category_id INTEGER NOT NULL, name TEXT NOT NULL UNIQUE, FOREIGN KEY (category_id) REFERENCES categories(id));
		INSERT INTO categories (id, name) VALUES (1, 'Featured');
		INSERT INTO products (id, category_id, name) VALUES (1, 1, 'First'), (2, 1, 'Second');
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-orm-association-test"
	config.PackageOptions["trb/orm"] = json.RawMessage(`{"adapter":"sqlite","database":"application.sqlite3"}`)
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { DbError, Model, belongs_to, has_many } from trb/orm
import { Result } from trb/std/result

class Category < Model
	has_many(Product)
end

class Product < Model
	belongs_to(Category)
end

def association_count(): Integer fails DbError
	product := Product.find(1)
	puts(product.category.loaded?())
	puts(product.category.name)
	puts(product.category.loaded?())
	category := Category.find(1)
	puts(category.products.size())
	return category.products.size()
end

def batch_count(): Integer fails DbError
	each_count := Product.find_each(batch_size: 1) do |product|
		puts(product.name)
	end
	batch_count := Product.find_in_batches(batch_size: 1) do |products|
		puts(products.size())
	end
	return each_count + batch_count
end

def write_products(): Integer fails DbError
	draft := Product.build(category_id: 1, name: "Draft")
	created := draft.save()
	puts(created.name)
	saved := created.with(name: "Saved").save()
	puts(saved.name)
	direct := Product.create(category_id: 1, name: "Direct")
	updated := direct.update(name: "Updated")
	puts(updated.name)
	updated_count := Product.where(id: saved.id).update_all(name: "Bulk")
	puts(updated_count)
	puts(saved.delete())
	puts(Product.where(id: updated.id).delete_all())
	return Product.count()
end

def write_conflicts(): Integer fails DbError
	bulk_count := Product.insert_all([
		Product.build(category_id: 1, name: "Bulk A"),
		Product.build(category_id: 1, name: "Bulk B")
	])
	puts(bulk_count)
	absent := Product.build(category_id: 1, name: "Absent")
	puts(Product.insert_if_absent(absent, unique_by: [:name]))
	puts(Product.insert_if_absent(absent, unique_by: [:name]))
	upserted := Product.build(category_id: 1, name: "Upsert").upsert(unique_by: [:name], update: [:category_id])
	puts(upserted.name)
	upsert_count := Product.upsert_all([
		Product.build(category_id: 1, name: "Upsert A"),
		Product.build(category_id: 1, name: "Upsert B")
	], unique_by: [:name], update: [:category_id])
	puts(upsert_count)
	puts(Product.where(name: ["Bulk A", "Bulk B", "Absent", "Upsert", "Upsert A", "Upsert B"]).delete_all())
	return Product.count()
end

def main()
	case attempt association_count()
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
	case attempt batch_count()
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"import { Category, Product, batch_count, write_conflicts, write_products } from main",
		"product := Product.find(1)",
		"product.category.loaded?()",
		"product.category",
		"product.category.loaded?()",
		"product.category.load()",
		"product.category.reload()",
		"category := Category.find(1)",
		"category.products.loaded?()",
		"category.products",
		"category.products.loaded?()",
		"batch_count()",
		"write_products()",
		"write_conflicts()",
		":quit",
	}, "\n") + "\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	for _, expected := range []string{
		`false : Boolean`,
		`#<Category id: 1, name: "Featured"> : Category?`,
		`true : Boolean`,
		`[#<Product category_id: 1, id: 1, name: "First">, #<Product category_id: 1, id: 2, name: "Second">] : Array<Product>`,
		`4 : Integer`,
		`Draft`,
		`Saved`,
		`Updated`,
		`2 : Integer`,
		`true`,
		`false`,
		`Upsert`,
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("ORM association REPL output is missing %q:\n%s\nstderr:\n%s", expected, stdout.String(), stderr.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("ORM association REPL reported errors:\n%s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	command = &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
		t.Fatalf("association run status=%d stderr=%s", status, stderr.String())
	}
	if want := "false\nFeatured\ntrue\n2\n2\nFirst\nSecond\n1\n1\n4\n"; stdout.String() != want {
		t.Fatalf("unexpected generated ORM association result: want %q, got %q, stderr=%s", want, stdout.String(), stderr.String())
	}
}

func TestRunPortableORMDestroyLifecycle(t *testing.T) {
	root := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE products (id INTEGER PRIMARY KEY, category_id INTEGER NOT NULL, name TEXT NOT NULL, FOREIGN KEY (category_id) REFERENCES categories(id));
		CREATE TABLE teams (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE members (id INTEGER PRIMARY KEY, team_id INTEGER NOT NULL, name TEXT NOT NULL, FOREIGN KEY (team_id) REFERENCES teams(id));
		CREATE TABLE authors (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE articles (id INTEGER PRIMARY KEY, author_id INTEGER, title TEXT NOT NULL, FOREIGN KEY (author_id) REFERENCES authors(id));
		CREATE TABLE folders (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE files (id INTEGER PRIMARY KEY, folder_id INTEGER NOT NULL, name TEXT NOT NULL, FOREIGN KEY (folder_id) REFERENCES folders(id));
		INSERT INTO categories (id, name) VALUES (1, 'Featured'), (2, 'Archived');
		INSERT INTO products (id, category_id, name) VALUES (1, 1, 'TypeRB'), (2, 2, 'Old TypeRB');
		INSERT INTO teams (id, name) VALUES (1, 'Compiler');
		INSERT INTO members (id, team_id, name) VALUES (1, 1, 'Ada');
		INSERT INTO authors (id, name) VALUES (1, 'Matz');
		INSERT INTO articles (id, author_id, title) VALUES (1, 1, 'Language design');
		INSERT INTO folders (id, name) VALUES (1, 'Temporary');
		INSERT INTO files (id, folder_id, name) VALUES (1, 1, 'draft.txt');
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/orm-destroy-test"
	config.PackageOptions["trb/orm"] = json.RawMessage(`{"adapter":"sqlite","database":"application.sqlite3"}`)
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { DbError, DbErrorKind, Model, belongs_to, has_many } from trb/orm
import { Result } from trb/std/result

class Category < Model
	has_many(Product, dependent: :destroy)
end

class Product < Model
	belongs_to(Category)
end

class Team < Model
	has_many(Member, dependent: :restrict)
end

class Member < Model
	belongs_to(Team)
end

class Author < Model
	has_many(Article, dependent: :nullify)
end

class Article < Model
	belongs_to(Author)
end

class Folder < Model
	has_many(File, dependent: :delete)
end

class File < Model
	belongs_to(Folder)
end

def destroy_category(id: Integer): Integer fails DbError
	category := Category.find(id)
	destroyed := category.destroy()
	if destroyed
		return Product.count()
	end
	return -1
end

def destroy_remaining_categories(): Integer fails DbError
	destroyed := Category.destroy_all()
	if destroyed >= 0
		return Product.count()
	end
	return -1
end

def restrict_team(): Boolean fails DbError
	team := Team.find(1)
	case attempt team.destroy()
	when Result::Ok(_)
		return false
	when Result::Err(error)
		return error.kind == DbErrorKind::Constraint
	end
end

def nullify_articles(): Integer fails DbError
	author := Author.find(1)
	destroyed := author.destroy()
	if destroyed
		return Article.where(author_id: nil).count()
	end
	return -1
end

def delete_files(): Integer fails DbError
	folder := Folder.find(1)
	destroyed := folder.destroy()
	if destroyed
		return File.count()
	end
	return -1
end

def print_integer(result: Result<Integer, DbError>)
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
end

def print_boolean(result: Result<Boolean, DbError>)
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
end

def main()
	print_integer(attempt destroy_category(1))
	print_integer(attempt destroy_remaining_categories())
	print_boolean(attempt restrict_team())
	print_integer(attempt nullify_articles())
	print_integer(attempt delete_files())
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if want := "1\n0\ntrue\n1\n0\n"; stdout.String() != want {
		t.Fatalf("unexpected ORM destroy result: want %q, got %q, stderr=%s", want, stdout.String(), stderr.String())
	}
	database, err = sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		DELETE FROM products; DELETE FROM categories; DELETE FROM members; DELETE FROM teams;
		DELETE FROM articles; DELETE FROM authors; DELETE FROM files; DELETE FROM folders;
		INSERT INTO categories (id, name) VALUES (1, 'Featured'), (2, 'Archived');
		INSERT INTO products (id, category_id, name) VALUES (1, 1, 'TypeRB'), (2, 2, 'Old TypeRB');
		INSERT INTO teams (id, name) VALUES (1, 'Compiler');
		INSERT INTO members (id, team_id, name) VALUES (1, 1, 'Ada');
		INSERT INTO authors (id, name) VALUES (1, 'Matz');
		INSERT INTO articles (id, author_id, title) VALUES (1, 1, 'Language design');
		INSERT INTO folders (id, name) VALUES (1, 'Temporary');
		INSERT INTO files (id, folder_id, name) VALUES (1, 1, 'draft.txt');
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"import { destroy_category, destroy_remaining_categories, restrict_team, nullify_articles, delete_files } from main",
		"destroy_category(1)",
		"destroy_remaining_categories()",
		"restrict_team()",
		"nullify_articles()",
		"delete_files()",
		":quit",
	}, "\n") + "\n"
	stdout.Reset()
	stderr.Reset()
	command = &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("REPL status=%d stderr=%s", status, stderr.String())
	}
	want := strings.Join([]string{
		"1 : Integer",
		"0 : Integer",
		"true : Boolean",
		"1 : Integer",
		"0 : Integer",
		"",
	}, "\n")
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected ORM destroy REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplExecutesORMDistinct(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO products (id, name) VALUES (1, 'Same'), (2, 'Same'), (3, 'Other');
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-orm-distinct-test"
	config.PackageOptions["trb/orm"] = json.RawMessage(`{"adapter":"sqlite","database":"application.sqlite3"}`)
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte("import { Model } from trb/orm\n\nclass Product < Model\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		"import { Product } from main",
		"Product.distinct().to_sql()",
		"Product.order(name: :asc).distinct().pluck(:name)",
		"Product.distinct().count()",
		":quit",
	}, "\n") + "\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := strings.Join([]string{
		`"SELECT DISTINCT \"id\", \"name\" FROM \"products\"" : String`,
		`["Other", "Same"] : Array<String>`,
		`3 : Integer`,
		"",
	}, "\n")
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected distinct REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplRejectsDuplicateHasOnePreload(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "application.sqlite3")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE products (
			id INTEGER PRIMARY KEY,
			category_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			FOREIGN KEY (category_id) REFERENCES categories(id)
		);
		INSERT INTO categories (id, name) VALUES (1, 'Featured');
		INSERT INTO products (category_id, name) VALUES (1, 'First'), (1, 'Second');
	`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-orm-has-one-test"
	config.PackageOptions["trb/orm"] = json.RawMessage(`{"adapter":"sqlite","database":"application.sqlite3"}`)
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { Model, belongs_to, has_one } from trb/orm

class Category < Model
	has_one(Product)
end

class Product < Model
	belongs_to(Category)
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	input := "import { Category } from main\nattempt Category.preload(:product).all()\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "Result::Err(error: DbError(kind: DbErrorKind::InvalidData, message: \"database has_one association returned multiple rows\")) : Result<Array<Category>, DbError>\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected has_one REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplRejectsValueReturningFunctionThatFallsThrough(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-return-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := `def hello2(): String
	puts("hello!")
end
def hello2(): String
	return "hello!"
end
hello2()
:quit
`
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if want := "\"hello!\" : String\n"; stdout.String() != want {
		t.Fatalf("unexpected REPL output\nwant:\n%s\ngot:\n%s", want, stdout.String())
	}
	if !strings.Contains(stderr.String(), "hello2() must return String on every path") {
		t.Fatalf("REPL did not reject the incomplete function:\n%s", stderr.String())
	}
	if strings.Contains(stdout.String(), "nil : String") {
		t.Fatalf("REPL evaluated an incomplete function as a String:\n%s", stdout.String())
	}
}

func TestReplSupportsPreludeAndNamespacedPutsForAnyValue(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-puts-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "puts(1 + 2)\nimport trb/std/io\nio.puts([1, 2])\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "3\n[1, 2]\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesPortableReceiverMethodsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-receiver-method-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := `import trb/std/math
123.to_s()
0.25.to_s()
(-0.0).to_s()
2.to_f()
(-2.75).to_i()
(-4).abs()
0.zero?()
1.positive?()
(-1).negative?()
2.even?()
3.odd?()
(-0.25).abs()
0.25.finite?()
true.to_s()
false.to_s()
0.25 * 100
1 == 1.0
"123".to_i()
"123".try_to_i()
"12x".try_to_i()
"9007199254740992".try_to_i()
"12.5".to_f()
"+.5e1".try_to_f()
"1.2x".try_to_f()
"1e9999".try_to_f()
5.min(3)
5.max(7)
12.clamp(0, 10)
(-2.75).floor()
(-2.75).ceil()
2.5.round()
(-2.5).round()
2.75.truncate()
math.sqrt(9)
math.exp(0)
math.log(1)
math.log2(8)
math.log10(100)
math.sqrt(-1).nan?()
math.log(0).infinite?()
"a😀".size()
"A😀"[1]
"A😀".try_fetch(2)
bounds := 1...3
"A😀BC".slice(bounds)
"A😀BC".try_slice(4...4)
"A😀BC".index("😀")
"A😀B😀".rindex("😀")
[10, 20, 30, 40].slice(bounds)
[10, 20].try_slice(1...3)
"A😀".chars()
"A😀".reverse()
:quit
`
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := `"123" : String
"0.25" : String
"0.0" : String
2 : Float
-2 : Integer
4 : Integer
true : Boolean
true : Boolean
true : Boolean
true : Boolean
true : Boolean
0.25 : Float
true : Boolean
"true" : String
"false" : String
25 : Float
true : Boolean
123 : Integer
Result::Ok(value: 123) : Result<Integer, NumberParseError>
Result::Err(error: NumberParseError(kind: NumberParseErrorKind::InvalidFormat, input: "12x", message: "invalid Integer")) : Result<Integer, NumberParseError>
Result::Err(error: NumberParseError(kind: NumberParseErrorKind::OutOfRange, input: "9007199254740992", message: "Integer is outside the portable range")) : Result<Integer, NumberParseError>
12.5 : Float
Result::Ok(value: 5) : Result<Float, NumberParseError>
Result::Err(error: NumberParseError(kind: NumberParseErrorKind::InvalidFormat, input: "1.2x", message: "invalid Float")) : Result<Float, NumberParseError>
Result::Err(error: NumberParseError(kind: NumberParseErrorKind::OutOfRange, input: "1e9999", message: "Float is outside the portable range")) : Result<Float, NumberParseError>
3 : Integer
7 : Integer
10 : Integer
-3 : Integer
-2 : Integer
3 : Integer
-3 : Integer
2 : Integer
3 : Float
1 : Float
0 : Float
3 : Float
2 : Float
true : Boolean
true : Boolean
2 : Integer
"😀" : String
Result::Err(error: IndexLookupError(index: 2, size: 2, message: "String index is out of bounds")) : Result<String, IndexLookupError>
1...3 : Range<Integer>
"😀B" : String
Result::Ok(value: "") : Result<String, SliceRangeError>
1 : Integer?
3 : Integer?
[20, 30] : Array<Integer>
Result::Err(error: SliceRangeError(start: 1, finish: 3, exclusive: true, size: 2, message: "Array slice range is out of bounds")) : Result<Array<Integer>, SliceRangeError>
["A", "😀"] : Array<String>
"😀A" : String
`
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s receiver-method REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableArraySortingAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-array-sorting-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := `[3, 1, 2].sort()
[3, 1, 2].sort_descending()
[[2, 0], [1, 1], [2, 2]].sort_by do |value|
	value[0]
end
[[2, 0], [1, 1], [2, 2]].sort_by_descending do |value|
	value[0]
end
:quit
`
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[1, 2, 3] : Array<Integer>\n[3, 2, 1] : Array<Integer>\n[[1, 1], [2, 0], [2, 2]] : Array<Array<Integer>>\n[[2, 0], [2, 2], [1, 1]] : Array<Array<Integer>>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("%s unexpected sorting REPL result\nstdout:\n%s\nstderr:\n%s", mode, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableArrayUniqAndConcatAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-array-uniq-concat-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := `[3, 1, 3, 2, 1].uniq()
[1, 2].concat([3, 4])
:quit
`
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[3, 1, 2] : Array<Integer>\n[1, 2, 3, 4] : Array<Integer>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("%s unexpected uniq/concat REPL result\nstdout:\n%s\nstderr:\n%s", mode, stdout.String(), stderr.String())
		}
	}
}

func TestReplRetainsPredicateAndBangFunctionNamesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-suffixed-name-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "def ready?(): Boolean; return true; end\n" +
			"def save!(): String; return \"saved\"; end\n" +
			"ready?()\n" +
			"save!()\n" +
			":quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "true : Boolean\n\"saved\" : String\n"; stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s suffixed-name REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesFallibleFunctionValuesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-function-effect-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "record AppError; message: String; end\n" +
			"def read_number(): Integer fails AppError; return 7; end\n" +
			"callback := fn(): Integer fails AppError; return read_number(); end\n" +
			"attempt callback()\n" +
			":quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Result::Ok(value: 7) : Result<Integer, AppError>") || stderr.Len() != 0 {
			t.Fatalf("unexpected %s fallible function REPL result\nstdout:\n%s\nstderr:\n%s", mode, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableStringTrimmingAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-string-trimming-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "\"\\t\\u00a0\\u3000 TypeRB \\u0085\\n\".strip()\n" +
			"\"\\t\\u3000TypeRB\".lstrip()\n" +
			"\"TypeRB\\u00a0\\u3000\".rstrip()\n" +
			"\" \\ufeffTypeRB\\ufeff \".strip() == \"\\ufeffTypeRB\\ufeff\"\n" +
			":quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "\"TypeRB\" : String\n\"TypeRB\" : String\n\"TypeRB\" : String\ntrue : Boolean\n"; stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s String trimming REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableStringReplacementAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-string-replacement-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "\"a😀a\".replace_all(\"a\", \"$&\")\n" +
			"\"aaaa\".replace_all(\"aa\", \"b\")\n" +
			":quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "\"$&😀$&\" : String\n\"bb\" : String\n"; stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s String replacement REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableBytesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-bytes-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "value := \"A😀\".to_bytes()\nvalue\nvalue.size()\nvalue.at(1)\nvalue.to_s()\nvalue.valid_utf8()\nvalue.concat(\"!\".to_bytes()).to_s()\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Bytes[65, 240, 159, 152, 128] : Bytes\nBytes[65, 240, 159, 152, 128] : Bytes\n5 : Integer\n240 : Integer\n\"A😀\" : String\ntrue : Boolean\n\"A😀!\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s Bytes REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableHexAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-hex-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/encoding/hex\nhex.encode(\"A😀\".to_bytes())\nhex.decode(\"41F09F9880\")\nhex.decode(\"0g\")\nhex.decode(\"abc\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"41f09f9880\" : String\n" +
			"Result::Ok(value: Bytes[65, 240, 159, 152, 128]) : Result<Bytes, HexDecodeError>\n" +
			"Result::Err(error: HexDecodeError(kind: HexDecodeErrorKind::InvalidCharacter, input: \"0g\", index: 1, message: \"invalid hexadecimal character\")) : Result<Bytes, HexDecodeError>\n" +
			"Result::Err(error: HexDecodeError(kind: HexDecodeErrorKind::OddLength, input: \"abc\", index: 3, message: \"hex input has odd length\")) : Result<Bytes, HexDecodeError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s hex REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableBase64AcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-base64-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/encoding/base64\nbase64.encode(\"A😀\".to_bytes())\nbase64.url_encode(\"???\".to_bytes())\nbase64.decode(\"QfCfmIA=\")\nbase64.url_decode(\"Pz8_\")\nbase64.decode(\"AAA\")\nbase64.decode(\"AA=A\")\nbase64.decode(\"AA$=\")\nbase64.decode(\"AB==\")\nbase64.url_decode(\"A\")\nbase64.url_decode(\"AA==\")\nbase64.url_decode(\"AA$\")\nbase64.url_decode(\"AB\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"QfCfmIA=\" : String\n" +
			"\"Pz8_\" : String\n" +
			"Result::Ok(value: Bytes[65, 240, 159, 152, 128]) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Ok(value: Bytes[63, 63, 63]) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidLength, input: \"AAA\", index: 3, message: \"base64 input length must be a multiple of 4\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidPadding, input: \"AA=A\", index: 3, message: \"invalid base64 padding\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidCharacter, input: \"AA$=\", index: 2, message: \"invalid base64 character\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::NonCanonical, input: \"AB==\", index: 1, message: \"non-canonical base64 encoding\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidLength, input: \"A\", index: 1, message: \"base64url input has invalid length\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidPadding, input: \"AA==\", index: 2, message: \"base64url input must not contain padding\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidCharacter, input: \"AA$\", index: 2, message: \"invalid base64url character\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::NonCanonical, input: \"AB\", index: 1, message: \"non-canonical base64url encoding\")) : Result<Bytes, Base64DecodeError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s base64 REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableHashAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-hash-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/hash\nimport trb/std/encoding/hex\nhex.encode(hash.md5(\"\".to_bytes()))\nhex.encode(hash.md5(\"abc\".to_bytes()))\nhex.encode(hash.sha1(\"\".to_bytes()))\nhex.encode(hash.sha1(\"abc\".to_bytes()))\nhex.encode(hash.sha256(\"\".to_bytes()))\nhex.encode(hash.sha256(\"abc\".to_bytes()))\nhex.encode(hash.sha512(\"\".to_bytes()))\nhex.encode(hash.sha512(\"abc\".to_bytes()))\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"d41d8cd98f00b204e9800998ecf8427e\" : String\n" +
			"\"900150983cd24fb0d6963f7d28e17f72\" : String\n" +
			"\"da39a3ee5e6b4b0d3255bfef95601890afd80709\" : String\n" +
			"\"a9993e364706816aba3e25717850c26c9cd0d89d\" : String\n" +
			"\"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\" : String\n" +
			"\"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\" : String\n" +
			"\"cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e\" : String\n" +
			"\"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s hash REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableHMACAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-hmac-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/hmac\nimport trb/std/secure_compare\nimport trb/std/encoding/hex\nkey := \"Jefe\".to_bytes()\nmessage := \"what do ya want for nothing?\".to_bytes()\ntag := hmac.sha256(key, message)\nhex.encode(tag)\nhex.encode(hmac.sha512(key, message))\nhmac.equal(tag, tag)\nhmac.equal(tag, hmac.sha256(key, \"other\".to_bytes()))\nhmac.equal(tag, \"short\".to_bytes())\nsecure_compare.equal(tag, tag)\nsecure_compare.equal(tag, hmac.sha256(key, \"other\".to_bytes()))\nsecure_compare.equal(tag, \"short\".to_bytes())\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Bytes[74, 101, 102, 101] : Bytes\n" +
			"Bytes[119, 104, 97, 116, 32, 100, 111, 32, 121, 97, 32, 119, 97, 110, 116, 32, 102, 111, 114, 32, 110, 111, 116, 104, 105, 110, 103, 63] : Bytes\n" +
			"Bytes[91, 220, 193, 70, 191, 96, 117, 78, 106, 4, 36, 38, 8, 149, 117, 199, 90, 0, 63, 8, 157, 39, 57, 131, 157, 236, 88, 185, 100, 236, 56, 67] : Bytes\n" +
			"\"5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843\" : String\n" +
			"\"164b7a7bfcf819e2e395fbe73b56e0a387bd64222e831fd610270cd7ea2505549758bf75c05a994a6d034f65f8f0e6fdcaeab1a34d4a6b4b636e070a38bce737\" : String\n" +
			"true : Boolean\nfalse : Boolean\nfalse : Boolean\n" +
			"true : Boolean\nfalse : Boolean\nfalse : Boolean\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s hmac REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableRandomAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-random-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/random\nimport trb/std/secure_random\nrandom.float() >= 0.0\nrandom.float() < 1.0\nrandom.integer(10) >= 0\nrandom.integer(10) < 10\nsecure_random.bytes(0).size()\nsecure_random.bytes(32).size()\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "true : Boolean\ntrue : Boolean\ntrue : Boolean\ntrue : Boolean\n0 : Integer\n32 : Integer\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s random REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplRejectsInvalidPortableRandomBounds(t *testing.T) {
	input := "import trb/std/random\nimport trb/std/secure_random\nrandom.integer(0)\nsecure_random.bytes(65537)\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--mode", "go"}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	for _, want := range []string{
		"random.integer upper bound must be greater than zero",
		"secure_random.bytes length must be between 0 and 65536",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("random REPL error is missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestReplEvaluatesPortableStringBuilderAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-string-builder-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/string_builder\nmut builder := string_builder.from_string(\"A\")\nbuilder.empty?()\nbuilder.append(\"😀\")\nbuilder.append_codepoint(33)\nbuilder\nbuilder.size()\nsnapshot := builder.to_s()\nbuilder.clear()\nbuilder.empty?()\nbuilder.to_s()\nsnapshot\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "StringBuilder(\"A\") : StringBuilder\nfalse : Boolean\nStringBuilder(\"A😀!\") : StringBuilder\n3 : Integer\n\"A😀!\" : String\ntrue : Boolean\n\"\" : String\n\"A😀!\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s StringBuilder REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableArrayAndHashOperationsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-collections-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/arrays\nimport trb/std/hashes\nmut numbers := [1, 2]\nnumbers.first()\nnumbers.last()\nnumbers[1]\nnumbers.try_fetch(1)\nmissing := numbers.try_fetch(9)\nnumbers.empty?()\nnumbers.dup()\narrays.push(numbers, 3)\nnumbers\nnumbers.shift()\nnumbers.unshift(0)\narrays.reverse(numbers)\nnumbers\narrays.shift(numbers)\narrays.unshift(numbers, 1)\nnumbers.reverse()\nnumbers\nnumbers.include?(2)\nnumbers.count(2)\narrays.contains(numbers, 9)\narrays.count(numbers, 1)\nlabels: Hash<Integer, String> := {1 => \"one\", 2 => \"two\"}\nlabels.fetch(2)\nlabels.try_fetch(2)\nlabels.try_fetch(9)\nlabels.key?(3)\nlabels.keys()\nlabels.values()\nhashes.copy(labels)\nlabels.merge({2 => \"TWO\", 3 => \"three\"})\nlabels\nmut editable := labels.dup()\neditable.update({2 => \"TWO\", 3 => \"three\"})\nhashes.update(editable, {4 => \"four\"})\neditable.delete(1)\nhashes.delete(editable, 2)\neditable\n\"a/b/\".split(\"/\")\n\"TypeRB\".start_with?(\"Type\")\n\"TypeRB\".end_with?(\"RB\")\nmut words := [\"root\", \"leaf\"]\nwords.pop()\nwords.join(\"/\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[1, 2] : Array<Integer>\n1 : Integer\n2 : Integer\n2 : Integer\nResult::Ok(value: 2) : Result<Integer, IndexLookupError>\nResult::Err(error: IndexLookupError(index: 9, size: 2, message: \"Array index is out of bounds\")) : Result<Integer, IndexLookupError>\nfalse : Boolean\n[1, 2] : Array<Integer>\n[1, 2, 3] : Array<Integer>\n1 : Integer\n[3, 2, 0] : Array<Integer>\n[0, 2, 3] : Array<Integer>\n0 : Integer\n[3, 2, 1] : Array<Integer>\n[1, 2, 3] : Array<Integer>\ntrue : Boolean\n1 : Integer\nfalse : Boolean\n1 : Integer\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n\"two\" : String\nResult::Ok(value: \"two\") : Result<String, KeyLookupError>\nResult::Err(error: KeyLookupError(key: 9, message: \"Hash key is missing\")) : Result<String, KeyLookupError>\nfalse : Boolean\n[1, 2] : Array<Integer>\n[\"one\", \"two\"] : Array<String>\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n{1: \"one\", 2: \"TWO\", 3: \"three\"} : Hash<Integer, String>\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n\"one\" : String\n\"TWO\" : String\n{3: \"three\", 4: \"four\"} : Hash<Integer, String>\n[\"a\", \"b\", \"\"] : Array<String>\ntrue : Boolean\ntrue : Boolean\n[\"root\", \"leaf\"] : Array<String>\n\"leaf\" : String\n\"root\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s Array/Hash REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableCollectionTransformationsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-collection-transformation-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "[1, 2, 3].map do |value|\ndoubled := value * 2\ndoubled\nend\n[1, 2, 3].select.with_index { |value, index| value > 1 and index < 2 }\n[1, 2, 3].reduce(10) { |sum, value| sum + value }\n[1, 2, 3].any? { |value| value > 2 }\n[1, 2, 3].all?() { |value| value > 0 }\n[1, 2, 3].none? { |value| value < 0 }\n[1, 2, 3].find { |value| value > 1 }\n[1, 2, 3].find_index() { |value| value == 3 }\n[1, 2, 3].find { |value| value > 9 }\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[2, 4, 6] : Array<Integer>\n[2] : Array<Integer>\n16 : Integer\ntrue : Boolean\ntrue : Boolean\ntrue : Boolean\n2 : Integer?\n2 : Integer?\nnil : Integer?\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s collection-transformation REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortablePathAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-path-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/path\npath.separator()\npath.clean(\"a/./b/../c\")\npath.clean(\"/../../srv//app\")\npath.join(\"/srv/app\", \"../data\")\npath.absolute(\"/srv/app\")\npath.components(\"/srv/app/main.trb\")\npath.base(\"/srv/app/main.trb\")\npath.directory(\"/srv/app/main.trb\")\npath.join(\"\", \"\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"/\" : String\n\"a/c\" : String\n\"/srv/app\" : String\n\"/srv/data\" : String\ntrue : Boolean\n[\"srv\", \"app\", \"main.trb\"] : Array<String>\n\"main.trb\" : String\n\"/srv/app\" : String\n\".\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s path REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableFilesystemAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-filesystem-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		directory := filepath.Join(root, "data")
		textPath := filepath.Join(directory, "note.txt")
		bytesPath := filepath.Join(directory, "value.bin")
		missingPath := filepath.Join(directory, "missing.txt")
		input := strings.Join([]string{
			"import { FileError, create_directory, exists, list, read_bytes, read_text, write_bytes, write_text } from trb/std/filesystem",
			"import { Result } from trb/std/result",
			"def describe(value: Result<String, FileError>): String; case value; when Result::Ok(text); return text; when Result::Err(error); return error.operation; end; end",
			"create_directory(" + strconv.Quote(directory) + ")",
			"write_text(" + strconv.Quote(textPath) + ", \"A😀\")",
			"read_text(" + strconv.Quote(textPath) + ")",
			"exists(" + strconv.Quote(textPath) + ")",
			"exists(" + strconv.Quote(missingPath) + ")",
			"list(" + strconv.Quote(directory) + ")",
			"write_bytes(" + strconv.Quote(bytesPath) + ", \"B\".to_bytes())",
			"read_bytes(" + strconv.Quote(bytesPath) + ")",
			"describe(read_text(" + strconv.Quote(missingPath) + "))",
			":quit",
		}, "\n") + "\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Result::Ok(value: Unit()) : Result<Unit, FileError>\n" +
			"Result::Ok(value: Unit()) : Result<Unit, FileError>\n" +
			"Result::Ok(value: \"A😀\") : Result<String, FileError>\n" +
			"Result::Ok(value: true) : Result<Boolean, FileError>\n" +
			"Result::Ok(value: false) : Result<Boolean, FileError>\n" +
			"Result::Ok(value: [\"note.txt\"]) : Result<Array<String>, FileError>\n" +
			"Result::Ok(value: Unit()) : Result<Unit, FileError>\n" +
			"Result::Ok(value: Bytes[66]) : Result<Bytes, FileError>\n" +
			"\"read_text\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s filesystem REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableProcessAcrossModes(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-process-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("TRB_PROCESS_REPL_TEST", "available")
		input := "import trb/std/process\n" +
			"import { Result } from trb/std/result\n" +
			"def describe(value: Result<ProcessResult, ProcessError>): String; case value; when Result::Ok(result); return result.status.to_s() + \":\" + result.stdout + \":\" + result.stderr; when Result::Err(error); return error.operation; end; end\n" +
			"def operation(value: Result<ProcessResult, ProcessError>): String; case value; when Result::Ok(result); return result.stdout; when Result::Err(error); return error.operation; end; end\n" +
			"process.argv()\n" +
			"process.environment(\"TRB_PROCESS_REPL_TEST\")\n" +
			"describe(process.run(\"/bin/sh\", [\"-c\", \"printf out; printf err >&2; exit 3\"]))\n" +
			"empty_args: Array<String> := []\n" +
			"operation(process.run(\"/type-rb-command-that-does-not-exist\", empty_args))\n" +
			":quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[] : Array<String>\n\"available\" : String?\n\"3:out:err\" : String\n[] : Array<String>\n\"run\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s process REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableJSONAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-json-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := strings.Join([]string{
			"import { JsonError, JsonValue, as_string, parse, stringify } from trb/std/json",
			"import trb/std/jsonc",
			"import { Result } from trb/std/result",
			`parse("1")`,
			`parse("1.5")`,
			`parse("{\"name\":\"Ada\",\"enabled\":true}")`,
			`jsonc.parse("{\n  // comment\n  \"name\": \"Ada\"\n}")`,
			`stringify(JsonValue::Object({"name" => JsonValue::String("Ada")}))`,
			`as_string(JsonValue::Integer(1))`,
			":quit",
		}, "\n") + "\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Result::Ok(value: JsonValue::Integer(value: 1)) : Result<JsonValue, JsonError>\n" +
			"Result::Ok(value: JsonValue::Float(value: 1.5)) : Result<JsonValue, JsonError>\n" +
			"Result::Ok(value: JsonValue::Object(value: {\"enabled\": JsonValue::Boolean(value: true), \"name\": JsonValue::String(value: \"Ada\")})) : Result<JsonValue, JsonError>\n" +
			"Result::Ok(value: JsonValue::Object(value: {\"name\": JsonValue::String(value: \"Ada\")})) : Result<JsonValue, JsonError>\n" +
			"Result::Ok(value: \"{\\\"name\\\":\\\"Ada\\\"}\") : Result<String, JsonError>\n" +
			"Result::Err(error: JsonError(kind: JsonErrorKind::Decode, message: \"JSON value is not String\", path: \"\", line: nil, column: nil)) : Result<String, JsonError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s JSON REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesTypedJSONRecordCodecsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-json-codec-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := strings.Join([]string{
			"import { JsonError, decode, encode } from trb/std/json",
			"import { Result } from trb/std/result",
			`record User; id: Integer @json("user_id"); name: String; end`,
			`decode<User>("{\"user_id\":1,\"name\":\"Ada\"}")`,
			`encode(User.new(id: 2, name: "Lin"))`,
			`decode<User>("{\"user_id\":1}")`,
			":quit",
		}, "\n") + "\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Result::Ok(value: User(id: 1, name: \"Ada\")) : Result<User, JsonError>\n" +
			"Result::Ok(value: \"{\\\"name\\\":\\\"Lin\\\",\\\"user_id\\\":2}\") : Result<String, JsonError>\n" +
			"Result::Err(error: JsonError(kind: JsonErrorKind::Decode, message: \"missing field name\", path: \"/name\", line: nil, column: nil)) : Result<User, JsonError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s typed JSON codec REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesTypedWebQueryBindingAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-web-query-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := strings.Join([]string{
			"import { Body, Header, Headers, HttpMethod } from trb/http",
			"import { ParameterError, Request } from trb/web",
			"import { Result } from trb/std/result",
			`record Query; page: Integer; tag: Array<String>; end`,
			`Request.new(method: HttpMethod.get(), path: "/", query_string: "page=2&tag=a&tag=b", headers: Headers.new(), body: Body.empty()).query<Query>()`,
			`Request.new(method: HttpMethod.get(), path: "/", query_string: "tag=a", headers: Headers.new(), body: Body.empty()).query<Query>()`,
			`Request.new(method: HttpMethod.get(), path: "/", query_string: "page=%ZZ", headers: Headers.new(), body: Body.empty()).query<Query>()`,
			`record Payload; title: String; end`,
			`Request.new(method: HttpMethod.post(), path: "/", query_string: "", headers: Headers.new([Header.new(name: "content-type", value: "application/json")]), body: Body.new("{\"title\":\"ship\"}".to_bytes())).json<Payload>()`,
			":quit",
		}, "\n") + "\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Result::Ok(value: Query(page: 2, tag: [\"a\", \"b\"])) : Result<Query, ParameterError>\n" +
			"Result::Err(error: ParameterError::Missing(source: ParameterSource::Query, name: \"page\")) : Result<Query, ParameterError>\n" +
			"Result::Err(error: ParameterError::MalformedQuery(error: PercentDecodeError(kind: PercentDecodeErrorKind::InvalidEscape, input: \"%ZZ\", message: \"invalid percent escape in URL query component\"))) : Result<Query, ParameterError>\n" +
			"Result::Ok(value: Payload(title: \"ship\")) : Result<Payload, RequestError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s typed web query REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesTypedWebContextKeysAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-web-context-key-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := strings.Join([]string{
			"import { Body, Headers, HttpMethod } from trb/http",
			"import { Context, ContextKey, ContextValueError, Request } from trb/web",
			"import { Result } from trb/std/result",
			`record User; name: String; end`,
			`def empty_context(): Context; request := Request.new(method: HttpMethod.get(), path: "/", query_string: "", headers: Headers.new(), body: Body.empty()); return Context.new(request: request, path_parameters: {}); end`,
			`def missing(): Result<User, ContextValueError>; key := ContextKey<User>.new("current_user"); return empty_context().fetch(key); end`,
			`def present(): Result<User, ContextValueError>; key := ContextKey<User>.new("current_user"); return empty_context().with(key, User.new(name: "Ada")).fetch(key); end`,
			`def replaced(): Result<User, ContextValueError>; key := ContextKey<User>.new("current_user"); return empty_context().with(key, User.new(name: "Ada")).with(key, User.new(name: "Lin")).fetch(key); end`,
			`def distinct(): Result<User, ContextValueError>; first := ContextKey<User>.new("current_user"); second := ContextKey<User>.new("current_user"); return empty_context().with(first, User.new(name: "Ada")).fetch(second); end`,
			`missing()`,
			`present()`,
			`replaced()`,
			`distinct()`,
			":quit",
		}, "\n") + "\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Result::Err(error: ContextValueError(key: \"current_user\")) : Result<User, ContextValueError>\n" +
			"Result::Ok(value: User(name: \"Ada\")) : Result<User, ContextValueError>\n" +
			"Result::Ok(value: User(name: \"Lin\")) : Result<User, ContextValueError>\n" +
			"Result::Err(error: ContextValueError(key: \"current_user\")) : Result<User, ContextValueError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s typed web context key REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesWebEndpointInputBindingAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-web-endpoint-input-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := strings.Join([]string{
			"import { Body, Header, Headers, HttpMethod } from trb/http",
			"import { Context, EndpointInputError, Request } from trb/web",
			"import { Result } from trb/std/result",
			`record Params; id: Integer; end`,
			`record Query; page: Integer; end`,
			`record Payload; title: String; end`,
			`record Input; params: Params; query: Query; body: Payload; end`,
			`def context(path_id: String, query: String, body: String): Context; request := Request.new(method: HttpMethod.post(), path: "/", query_string: query, headers: Headers.new([Header.new(name: "content-type", value: "application/json")]), body: Body.new(body.to_bytes())); return Context.new(request: request, path_parameters: {"id" => path_id}); end`,
			`context("7", "page=2", "{\"title\":\"ship\"}").bind<Input>()`,
			`context("bad", "page=2", "{\"title\":\"ship\"}").bind<Input>()`,
			`context("7", "", "{\"title\":\"ship\"}").bind<Input>()`,
			`context("7", "page=2", "{}").bind<Input>()`,
			":quit",
		}, "\n") + "\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Result::Ok(value: Input(params: Params(id: 7), query: Query(page: 2), body: Payload(title: \"ship\"))) : Result<Input, EndpointInputError>\n" +
			"Result::Err(error: EndpointInputError::Params(error: ParameterError::Invalid(source: ParameterSource::Path, name: \"id\", value: \"bad\", expected: \"Integer\"))) : Result<Input, EndpointInputError>\n" +
			"Result::Err(error: EndpointInputError::Query(error: ParameterError::Missing(source: ParameterSource::Query, name: \"page\"))) : Result<Input, EndpointInputError>\n" +
			"Result::Err(error: EndpointInputError::Body(error: RequestError::InvalidJson(error: JsonError(kind: JsonErrorKind::Decode, message: \"missing field title\", path: \"/title\", line: nil, column: nil)))) : Result<Input, EndpointInputError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s endpoint input REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestRunOfficialWebTypedContextKeysAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-web-context-key-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}

			mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Context, ContextKey, ContextValueError, Request } from trb/web
import { Result } from trb/std/result

record User
	name: String
end

CURRENT_USER := ContextKey<User>.new("current_user")
SAME_NAME := ContextKey<User>.new("current_user")

def render(result: Result<User, ContextValueError>): String
	case result
	when Result::Ok(user)
		return "user:" + user.name
	when Result::Err(error)
		return "missing:" + error.key
	end
end

def main()
	request := Request.new(
		method: HttpMethod.get(),
		path: "/",
		query_string: "",
		headers: Headers.new(),
		body: Body.empty(),
	)
	context := Context.new(request: request, path_parameters: {})
	puts(render(context.fetch(CURRENT_USER)))
	updated := context.with(CURRENT_USER, User.new(name: "Ada"))
	puts(render(updated.fetch(CURRENT_USER)))
	puts(render(context.fetch(CURRENT_USER)))
	puts(render(updated.with(CURRENT_USER, User.new(name: "Lin")).fetch(CURRENT_USER)))
	puts(render(updated.with_request(request).fetch(CURRENT_USER)))
	puts(render(updated.fetch(SAME_NAME)))
	return
end
`
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
			}
			want := "missing:current_user\nuser:Ada\nmissing:current_user\nuser:Lin\nuser:Ada\nmissing:current_user\n"
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("unexpected %s typed context key output: want %q, got %q, stderr=%s", mode, want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestReplEvaluatesCompilerOwnedUnicodeAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-unicode-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/unicode\nunicode.version()\nunicode.letter(12354)\nunicode.digit(1632)\nunicode.identifier_start(64)\nunicode.valid_scalar(55296)\nunicode.from_codepoint(128512)\n\"A😀\".codepoints()\n\"\".empty?()\n\"TypeRB\".include?(\"RB\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"15.0.0\" : String\ntrue : Boolean\ntrue : Boolean\ntrue : Boolean\nfalse : Boolean\n\"😀\" : String\n[65, 128512] : Array<Integer>\ntrue : Boolean\ntrue : Boolean\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s Unicode REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableArithmeticSemantics(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-operators-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "-5 / 2\n-5 % 2\n2 ** 3\n(1 + 2) * 3\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "-2 : Integer\n-1 : Integer\n8 : Integer\n9 : Integer\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesBreakAndNext(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-loop-control-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := `mut total := 0
[1, 2, 3, 4].each do |value|
  if value == 2
    next
  end
  if value == 4
    break
  end
  total += value
end
total
:quit
`
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "0 : Integer\n4 : Integer\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesTypedHashAndReportsMissingKeys(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-hash-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := `mut scores: Hash<String, Integer> := {"one" => 1}
scores["one"]
scores["two"] = 2
scores["two"]
scores["missing"]
scores["one"]
:quit
`
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "{\"one\": 1} : Hash<String, Integer>\n1 : Integer\n2 : Integer\n2 : Integer\n1 : Integer\n"
	if stdout.String() != want {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Hash key is missing") {
		t.Fatalf("REPL did not report and recover from missing Hash key:\n%s", stderr.String())
	}
}

func TestReplInfersCommonNumericCollectionTypesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-common-collection-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "numbers := [1, 2.5]\nnumbers\nvalues := { integer: 1, float: 2.5 }\nvalues\nvalues[:integer]\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[1, 2.5] : Array<Float>\n[1, 2.5] : Array<Float>\n{\"integer\": 1, \"float\": 2.5} : Hash<String, Float>\n{\"integer\": 1, \"float\": 2.5} : Hash<String, Float>\n1 : Float\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s common collection REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplInfersAndNarrowsUnionTypesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-union-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := `def describe(value: Integer | String): String
	case value
	when Integer(number)
		return number.to_s()
	when String(text)
		return text
	end
end
describe(3)
describe("Ada")
values := [1, "two"]
values
fields := { count: 1, name: "Ada" }
fields
fields[:count]
:quit
`
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"3\" : String\n\"Ada\" : String\n[1, \"two\"] : Array<Integer | String>\n[1, \"two\"] : Array<Integer | String>\n{\"count\": 1, \"name\": \"Ada\"} : Hash<String, Integer | String>\n{\"count\": 1, \"name\": \"Ada\"} : Hash<String, Integer | String>\n1 : Integer | String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s union REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplNarrowsNullableBindingsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-nullable-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := `def maybe_name(present: Boolean): String?
	if present
		return "Ada"
	else
		return nil
	end
end
def label(value: String?): String
	if value == nil
		return "missing"
	end
	return value + "!"
end
label(maybe_name(true))
label(maybe_name(false))
:quit
`
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"Ada!\" : String\n\"missing\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s nullable REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestRunUnionTypesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby union run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript union run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-union-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `def describe(value: Integer | String): String
	case value
	when Integer(number)
		return number.to_s()
	when String(text)
		return text
	end
end

def widen(value: Integer | String): Float | String
	return value
end

def describe_wide(value: Float | String): String
	case value
	when Float(number)
		return number.to_s()
	when String(text)
		return text
	end
end

def main()
	values := [1, "Ada"]
	values.each do |value|
		puts(describe(value))
	end
	fields := { count: 2, name: "Grace" }
	puts(describe(fields[:count]))
	puts(describe(fields[:name]))
	puts(describe_wide(widen(1)))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "1\nAda\n2\nGrace\n1.0\n"; stdout.String() != want {
			t.Fatalf("unexpected %s union output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestDiscriminatedUnionNarrowingAcrossAvailableBackendsAndREPL(t *testing.T) {
	definitions := `record CreatedResponse
	status: 201
	body: String
end

record InvalidResponse
	status: 422
	body: Array<String>
end

type CreateResponse = CreatedResponse | InvalidResponse

def render(response: CreateResponse): String
	case response.status
	when 201
		return response.body
	when 422
		return response.body[0]
	end
end

def classify(status: Integer): String
	case status
	when 200
		return "ok"
	else
		return "other"
	end
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby discriminated union run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript discriminated union run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/discriminated-union-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := definitions + `render(CreatedResponse.new(status: 201, body: "created"))
render(InvalidResponse.new(status: 422, body: ["invalid"]))
classify(200)
classify(500)
:quit
`
		var replStdout, replStderr bytes.Buffer
		replCommand := &CLI{Stdin: strings.NewReader(input), Stdout: &replStdout, Stderr: &replStderr}
		if status := replCommand.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s REPL status=%d stderr=%s", mode, status, replStderr.String())
		}
		if want := "\"created\" : String\n\"invalid\" : String\n\"ok\" : String\n\"other\" : String\n"; replStdout.String() != want || replStderr.Len() != 0 {
			t.Fatalf("unexpected %s discriminated union REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, replStdout.String(), replStderr.String())
		}

		source := definitions + `
def main()
	puts(render(CreatedResponse.new(status: 201, body: "created")))
	puts(render(InvalidResponse.new(status: 422, body: ["invalid"])))
	puts(classify(200))
	puts(classify(500))
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "created\ninvalid\nok\nother\n"; stdout.String() != want {
			t.Fatalf("unexpected %s discriminated union output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestDivergingControlFlowExpressionsAcrossAvailableBackendsAndREPL(t *testing.T) {
	definitions := `enum Outcome
	Found(value: String)
	Missing
end

def describe(outcome: Outcome): String
	value := case outcome
	when Outcome::Found(found)
		found
	when Outcome::Missing
		return "missing"
	end
	return "found: " + value
end

def stop_before_two(): String
	mut result := ""
	[1, 2, 3].each do |number|
		value := if number == 2
			break
		else
			number.to_s()
		end
		result += value
	end
	return result
end

def skip_two(): String
	mut result := ""
	[1, 2, 3].each do |number|
		value := if number == 2
			next
		else
			number.to_s()
		end
		result += value
	end
	return result
end

def nested_choice(primary: Boolean, secondary: Boolean): String
	value := if primary
		if secondary
			return "first"
		else
			return "second"
		end
	else
		"fallback"
	end
	return value
end

def stop_on_string(values: Array<Integer | String>): String
	mut result := ""
	values.each do |value|
		text := case value
		when Integer(number)
			number.to_s()
		when String(_text)
			break
		end
		result += text
	end
	return result
end
`

	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/diverging-control-flow-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		replInput := definitions + `describe(Outcome::Found("Ada"))
describe(Outcome::Missing)
stop_before_two()
skip_two()
nested_choice(true, false)
nested_choice(false, false)
stop_on_string([1, "stop", 2])
:quit
`
		var replStdout, replStderr bytes.Buffer
		replCommand := &CLI{Stdin: strings.NewReader(replInput), Stdout: &replStdout, Stderr: &replStderr}
		if status := replCommand.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s REPL status=%d stderr=%s", mode, status, replStderr.String())
		}
		replWant := "\"found: Ada\" : String\n\"missing\" : String\n\"1\" : String\n\"13\" : String\n\"second\" : String\n\"fallback\" : String\n\"1\" : String\n"
		if replStdout.String() != replWant || replStderr.Len() != 0 {
			t.Fatalf("unexpected %s diverging REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, replWant, replStdout.String(), replStderr.String())
		}

		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby diverging run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript diverging run")
				continue
			}
		}
		source := definitions + `
def main()
	puts(describe(Outcome::Found("Ada")))
	puts(describe(Outcome::Missing))
	puts(stop_before_two())
	puts(skip_two())
	puts(nested_choice(true, false))
	puts(nested_choice(false, false))
	puts(stop_on_string([1, "stop", 2]))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "found: Ada\nmissing\n1\n13\nsecond\nfallback\n1\n"; stdout.String() != want {
			t.Fatalf("unexpected %s diverging output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestReplRejectsPlatformPackageForConfiguredModeAndContinues(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-mode-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{
		Stdin:  strings.NewReader("import trb/platform/typescript/node\n1 + 1\n:quit\n"),
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if stdout.String() != "2 : Integer\n" {
		t.Fatalf("unexpected output %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "does not support mode go") {
		t.Fatalf("configured mode was not enforced:\n%s", stderr.String())
	}
}

func TestReplEvaluatesMultilineClassThroughTypedIR(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-class-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	input := "class Box\n" +
		"  @value: Integer\n\n" +
		"  def initialize(value: Integer)\n" +
		"    @value = value\n" +
		"    return\n" +
		"  end\n\n" +
		"  def value(): Integer\n" +
		"    return @value\n" +
		"  end\n" +
		"end\n" +
		"box := Box.new(4)\n" +
		"box.value() + 1\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "#<Box value: 4> : Box\n5 : Integer\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestReplResolvesClassConstantsInsideInstanceAndClassMethods(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-class-constant-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	classSource := "class Config\n" +
		"  DEFAULT_NAME := \"TypeRB\"\n" +
		"  def name(): String\n" +
		"    return DEFAULT_NAME\n" +
		"  end\n" +
		"  def self.default_name(): String\n" +
		"    return DEFAULT_NAME\n" +
		"  end\n" +
		"end\n"
	if err := os.WriteFile(filepath.Join(root, "src", "config.trb"), []byte(classSource), 0o644); err != nil {
		t.Fatal(err)
	}
	input := "import { Config } from config\n" +
		"config := Config.new()\n" +
		"config.name()\n" +
		"Config.default_name()\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "#<Config > : Config\n\"TypeRB\" : String\n\"TypeRB\" : String\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesSemicolonSeparatedDeclarations(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-separator-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "enum State; Open; Closed; end\n" +
		"def label(state: State): String; case state; when State::Open; return \"open\"; when State::Closed; return \"closed\"; end; end\n" +
		"label(State::Closed)\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if want := "\"closed\" : String\n"; stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesPayloadEnumPatternBindings(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-payload-enum-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "enum Token; Text(value: String); EOF; def text?(): Boolean; case self; when Token::Text(_); return true; when Token::EOF; return false; end; end; end\n" +
		"def render(token: Token): String; case token; when Token::Text(value); return value; when Token::EOF; return \"eof\"; end; end\n" +
		"render(Token::Text(\"Ada\"))\n" +
		"Token::Text(\"Ada\")\n" +
		"token := Token::Text(\"Ada\")\n" +
		"token.text?()\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "\"Ada\" : String\nToken::Text(value: \"Ada\") : Token\nToken::Text(value: \"Ada\") : Token\ntrue : Boolean\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesRawValueEnumsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/repl-raw-enum-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}

			input := "import { decode, encode } from trb/std/json\n" +
				"enum OrderStatus; Pending = \"PENDING\"; Completed = \"COMPLETED\"; def terminal?(): Boolean; return self == OrderStatus::Completed; end; end\n" +
				"status := OrderStatus::Completed\n" +
				"status.raw_value()\n" +
				"status.terminal?()\n" +
				"encode(status)\n" +
				"decode<OrderStatus>(\"\\\"PENDING\\\"\")\n" +
				"OrderStatus.from_raw(\"PENDING\")\n" +
				"OrderStatus.from_raw(\"UNKNOWN\")\n" +
				":quit\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			want := "OrderStatus::Completed : OrderStatus\n" +
				"\"COMPLETED\" : String\n" +
				"true : Boolean\n" +
				"Result::Ok(value: \"\\\"COMPLETED\\\"\") : Result<String, JsonError>\n" +
				"Result::Ok(value: OrderStatus::Pending) : Result<OrderStatus, JsonError>\n" +
				"Result::Ok(value: OrderStatus::Pending) : Result<OrderStatus, EnumValueError>\n" +
				"Result::Err(error: EnumValueError(value: \"UNKNOWN\", message: \"unknown raw value for OrderStatus\")) : Result<OrderStatus, EnumValueError>\n"
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("unexpected %s raw enum REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestReplEvaluatesExplicitUserGenerics(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-generics-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "enum Result<T, E>; Ok(value: T); Err(error: E); end\n" +
		"def identity<T>(value: T): T; return value; end\n" +
		"def unwrap(value: Result<Integer, String>): Integer; case value; when Result::Ok(number); return number; when Result::Err(_); return 0; end; end\n" +
		"unwrap(Result<Integer, String>::Ok(identity<Integer>(7)))\n" +
		"identity<String>(\"Ada\")\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "7 : Integer\n\"Ada\" : String\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected generic REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesGenericClassesAndMethods(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-generic-objects-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "class Box<T>; @value: T; def initialize(value: T); @value = value; return; end; def value(): T; return @value; end; def echo<U>(value: U): U; return value; end; end\n" +
		"box := Box<Integer>.new(7)\n" +
		"box.value()\n" +
		"box.echo<String>(\"Ada\")\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "#<Box value: 7> : Box<Integer>\n7 : Integer\n\"Ada\" : String\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected generic object REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesStandardResult(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-result-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "import { Result } from trb/std/result\n" +
		"def unwrap(value: Result<Integer, String>): Integer; case value; when Result::Ok(number); return number; when Result::Err(_); return 0; end; end\n" +
		"unwrap(Result<Integer, String>::Ok(7))\n" +
		"Result<Integer, String>::Err(\"missing\")\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "7 : Integer\nResult::Err(error: \"missing\") : Result<Integer, String>\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected standard Result REPL output\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplDefaultsToGoWithoutProjectConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "broken.trb"), []byte("if\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader("import trb/platform/go/context\n1 + 2\n:quit\n"), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl"}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "3 : Integer\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected configless REPL output\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestReplModeFlagWorksWithoutProjectConfiguration(t *testing.T) {
	for _, test := range []struct {
		mode        string
		packagePath string
	}{
		{mode: "go", packagePath: "trb/platform/go/context"},
		{mode: "ruby", packagePath: "trb/platform/ruby/rails"},
		{mode: "typescript", packagePath: "trb/platform/typescript/node"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			root := t.TempDir()
			previous, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(root); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Chdir(previous) }()

			input := "import " + test.packagePath + "\n1\n:quit\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--mode", test.mode}); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "1 : Integer\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected %s REPL output\nstdout=%s\nstderr=%s", test.mode, stdout.String(), stderr.String())
			}
		})
	}
}

func TestReplModeFlagOverridesProjectMode(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/type-rb/repl-mode-override"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	input := "import trb/platform/ruby/rails\n1\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path, "--mode", "ruby"}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "1 : Integer\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected overridden REPL output\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestReplRejectsInvalidMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(":quit\n"), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--mode", "python"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "repl --mode must be ruby, go, or typescript") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestReplDoesNotIgnoreMissingExplicitConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(":quit\n"), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", "missing.jsonc", "--mode", "ruby"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "missing.jsonc") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestBuildCopiesRailsProjectAndTranspilesTRBTree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := project.New(root, "ruby")
	config.Dependencies["rails"] = "~> 8.0"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	routes := "import trb/platform/ruby/rails\n\nRails.application.routes.draw do\n  resources :posts\nend\n"
	if err := os.WriteFile(filepath.Join(root, "config", "routes.trb"), []byte(routes), 0o644); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build"}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "build", "Gemfile")); err != nil {
		t.Fatalf("Gemfile was not copied: %v", err)
	}
	gemfile, err := os.ReadFile(filepath.Join(root, "Gemfile"))
	if err != nil || !strings.Contains(string(gemfile), `gem "rails", "~> 8.0"`) {
		t.Fatalf("Gemfile was not managed from config: err=%v\n%s", err, gemfile)
	}
	generated, err := os.ReadFile(filepath.Join(root, "build", "config", "routes.rb"))
	if err != nil {
		t.Fatalf("routes were not generated: %v", err)
	}
	if strings.Contains(string(generated), "mode: ruby") || !strings.Contains(string(generated), "resources :posts") {
		t.Fatalf("unexpected generated routes:\n%s", generated)
	}
}

func TestBuildRemovesOutputsForDeletedSources(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.SourceDir = "src"
	config.OutDir = "build"
	copyFiles := false
	config.CopyFiles = &copyFiles
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	oldSource := filepath.Join(config.SourcePath(), "old.trb")
	if err := os.WriteFile(oldSource, []byte("def old_value(): Integer\n\treturn 1\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("initial build status=%d stderr=%s", status, stderr.String())
	}
	oldOutput := filepath.Join(config.OutputPath(), "old.rb")
	if _, err := os.Stat(oldOutput); err != nil {
		t.Fatalf("initial output was not generated: %v", err)
	}
	if err := os.Remove(oldSource); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "current.trb"), []byte("def current_value(): Integer\n\treturn 2\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("second build status=%d stderr=%s", status, stderr.String())
	}
	if _, err := os.Stat(oldOutput); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted source left stale generated output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.OutputPath(), "current.rb")); err != nil {
		t.Fatalf("current output was not generated: %v", err)
	}
}

func TestBuildCompileCreatesRunnableGoExecutable(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	for _, test := range []struct {
		name     string
		outfile  string
		relative string
		debug    bool
	}{
		{name: "default", relative: filepath.Join("bin", "hello-default")},
		{name: "outfile", outfile: filepath.Join("dist", "hello"), relative: filepath.Join("dist", "hello")},
		{name: "debug", relative: filepath.Join("bin", "hello-debug"), debug: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, "go")
			config.Name = "hello-" + test.name
			config.SourceDir = "src"
			config.Go.Module = "example.com/type-rb/" + config.Name
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			source := "def main()\n  puts(\"Hello compiled\")\n  return\nend\n"
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CGO_ENABLED", "0")

			args := []string{"build", "--config", config.Path, "--compile"}
			if test.debug {
				args = append(args, "--debug")
			}
			if test.outfile != "" {
				args = append(args, "--outfile", test.outfile)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(args); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			output := filepath.Join(root, test.relative)
			if runtime.GOOS == "windows" {
				output += ".exe"
			}
			info, err := os.Stat(output)
			if err != nil || info.IsDir() {
				t.Fatalf("executable was not created at %s: info=%v err=%v", output, info, err)
			}
			if want := "executable -> " + output + "\n"; stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("unexpected build output\nwant: %q\nstdout: %q\nstderr: %q", want, stdout.String(), stderr.String())
			}
			result, err := exec.Command(output).CombinedOutput()
			if err != nil || string(result) != "Hello compiled\n" {
				t.Fatalf("compiled executable failed: err=%v output=%q", err, result)
			}
			if _, err := os.Stat(filepath.Join(root, "build", "main.go")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("--compile retained generated source: %v", err)
			}
			if test.debug {
				binary, err := os.ReadFile(output)
				if err != nil || !bytes.Contains(binary, []byte(filepath.Join(root, "src", "main.trb"))) {
					t.Fatalf("debug executable does not retain the TypeRB source path: err=%v", err)
				}
			}
		})
	}
}

func TestBuildCompileValidatesModeAndFlags(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
		args []string
		want string
	}{
		{name: "ruby", mode: "ruby", args: []string{"--compile"}, want: "--compile is supported only for mode go"},
		{name: "typescript", mode: "typescript", args: []string{"--compile"}, want: "--compile is supported only for mode go"},
		{name: "outfile", mode: "go", args: []string{"--outfile", "bin/app"}, want: "--outfile requires --compile"},
		{name: "debug", mode: "go", args: []string{"--debug"}, want: "--debug requires --compile"},
		{name: "path", mode: "go", args: []string{"--compile", "."}, want: "--compile builds the configured project"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, test.mode)
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/compile-flags"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"build", "--config", config.Path}, test.args...)
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(args); status != 1 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("unexpected diagnostic: %s", stderr.String())
			}
		})
	}
}

func TestBuildCompileRequiresMain(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/library"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "library.trb"), []byte("def value(): Integer\n  return 1\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path, "--compile"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "project has no top-level main()") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}

func TestRunCompilesProjectImportClosure(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/acme/import-run"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(root, "src", "models", "user.trb")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatal(err)
	}
	model := "class User\n  @name: String\n\n  def initialize(name: String)\n    @name = name\n    return\n  end\n\n  def name(): String\n    return @name\n  end\nend\n"
	if err := os.WriteFile(modelPath, []byte(model), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "src", "main.trb")
	main := "import trb/std/io\nimport models/user\n\ndef main()\n  user := User.new(\"Imported\")\n  io.puts(user.name())\n  return\nend\n"
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	for _, args := range [][]string{{"run"}, {"run", mainPath}} {
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run(args); status != 0 {
			t.Fatalf("args=%v status=%d stderr=%s", args, status, stderr.String())
		}
		if stdout.String() != "Imported\n" {
			t.Fatalf("args=%v unexpected program output %q", args, stdout.String())
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, "trb-run-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("run directory leaked: matches=%v err=%v", matches, err)
	}
}

func TestRunPredicateAndBangNamesAcrossAvailableBackends(t *testing.T) {
	files := map[string]string{
		"contracts/capability.trb": "interface Capability\n\tready?(): Boolean\n\tsave!(): String\nend\n",
		"helpers/functions.trb": "def imported_ready?(): Boolean\n\treturn true\nend\n\n" +
			"def imported_save!(): String\n\treturn \"imported\"\nend\n\n" +
			"def imported_label?(): String\n\treturn \"question\"\nend\n",
		"models/base.trb": "import { Capability } from contracts/capability\n\n" +
			"class Base implements Capability\n" +
			"\tdef ready?(): Boolean\n\t\treturn true\n\tend\n\n" +
			"\tdef save!(): String\n\t\treturn \"base\"\n\tend\n\n" +
			"\tdef self.available?(): Boolean\n\t\treturn true\n\tend\n" +
			"end\n\n" +
			"def base_available?(): Boolean\n\treturn Base.available?()\nend\n",
		"models/child.trb": "import { Base, base_available? } from models/base\n\n" +
			"class Child < Base\n" +
			"\tdef ready?(): Boolean\n\t\treturn true\n\tend\n\n" +
			"\tdef child_ready?(): Boolean\n\t\treturn self.ready?()\n\tend\n" +
			"\n\tdef inherited_available?(): Boolean\n\t\treturn base_available?()\n\tend\n" +
			"end\n",
		"main.trb": "import { imported_ready?, imported_save!, imported_label? } from helpers/functions\n" +
			"import { base_available? } from models/base\n" +
			"import { Child } from models/child\n\n" +
			"def local_ready?(): Boolean\n\treturn true\nend\n\n" +
			"def local_save!(): String\n\treturn \"local\"\nend\n\n" +
			"def main()\n" +
			"\tchild := Child.new()\n" +
			"\tputs(local_ready?())\n" +
			"\tputs(local_save!())\n" +
			"\tputs(imported_ready?())\n" +
			"\tputs(imported_save!())\n" +
			"\tputs(imported_label?())\n" +
			"\tputs(base_available?())\n" +
			"\tputs(child.ready?())\n" +
			"\tputs(child.save!())\n" +
			"\tputs(child.child_ready?())\n" +
			"\tputs(child.inherited_available?())\n" +
			"\treturn\n" +
			"end\n",
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby suffixed-name run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript suffixed-name run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/suffixed-name-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		for name, source := range files {
			filename := filepath.Join(root, "src", filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "true\nlocal\ntrue\nimported\nquestion\ntrue\ntrue\nbase\ntrue\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s suffixed-name output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunInterfaceValuesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby interface-value run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript interface-value run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/interface-value-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `interface Named
	name(): String
end

class Person implements Named
	@value: String

	def initialize(name: String)
		@value = name
		return
	end

	def name(): String
		return @value
	end
end

interface Box<T>
	value(): T
end

class ValueBox<T> implements Box<T>
	@value: T

	def initialize(value: T)
		@value = value
		return
	end

	def value(): T
		return @value
	end
end

def display(value: Named): String
	return value.name()
end

def main()
	values: Array<Named> := [Person.new("Ada"), Person.new("Grace")]
	values.each do |value|
		puts(display(value))
	end
	box: Box<String> := ValueBox<String>.new("generic")
	puts(box.value())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if stdout.String() != "Ada\nGrace\ngeneric\n" {
			t.Fatalf("unexpected %s interface-value output %q", mode, stdout.String())
		}
	}
}

func TestRunPortableStringTrimmingAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby String trimming run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript String trimming run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/string-trimming-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "def main()\n" +
			"\tputs(\"\\t\\u00a0\\u3000 TypeRB \\u0085\\n\".strip())\n" +
			"\tputs(\"\\t\\u3000TypeRB\".lstrip())\n" +
			"\tputs(\"TypeRB\\u00a0\\u3000\".rstrip())\n" +
			"\tputs(\" \\ufeffTypeRB\\ufeff \".strip() == \"\\ufeffTypeRB\\ufeff\")\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "TypeRB\nTypeRB\nTypeRB\ntrue\n"; stdout.String() != want {
			t.Fatalf("unexpected %s String trimming output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableStringReplacementAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby String replacement run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript String replacement run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/string-replacement-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "import trb/std/strings\n\n" +
			"def main()\n" +
			"\tputs(\"a😀a\".replace_all(\"a\", \"$&\"))\n" +
			"\tputs(strings.replace_all(\"aaaa\", \"aa\", \"$1\"))\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "$&😀$&\n$1$1\n"; stdout.String() != want {
			t.Fatalf("unexpected %s String replacement output: want %q, got %q", mode, want, stdout.String())
		}

		source = "def main()\n" +
			"\tputs(\"value\".replace_all(\"\", \"x\"))\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		stdout.Reset()
		stderr.Reset()
		if status := command.Run([]string{"run", "--config", config.Path}); status != 1 {
			t.Fatalf("%s empty-pattern status=%d stdout=%s stderr=%s", mode, status, stdout.String(), stderr.String())
		}
		if want := "String replacement pattern is empty"; !strings.Contains(stderr.String(), want) {
			t.Fatalf("unexpected %s empty-pattern diagnostic: want %q in %q", mode, want, stderr.String())
		}
	}
}

func TestRunPortableMathAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby math run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript math run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-math-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import trb/std/math

def main()
	puts(5.min(3))
	puts(5.max(7))
	puts(12.clamp(0, 10))
	puts((-2.75).floor())
	puts((-2.75).ceil())
	puts(2.5.round())
	puts((-2.5).round())
	puts(2.75.truncate())
	puts(math.sqrt(9) == 3.0)
	puts(math.exp(0) == 1.0)
	puts(math.log(1) == 0.0)
	puts(math.log2(8) == 3.0)
	puts(math.log10(100) == 2.0)
	puts(math.sqrt(-1).nan?())
	puts(math.log(0).infinite?())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "3\n7\n10\n-3\n-2\n3\n-3\n2\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable math output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableHexAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby hex run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript hex run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-hex-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { Result } from trb/std/result
import { decode, encode } from trb/std/encoding/hex

def decoded_text(input: String): String
	case decode(input)
	when Result::Ok(value)
		return value.to_s()
	when Result::Err(error)
		return error.message + ":" + error.index.to_s()
	end
end

def main()
	puts(encode("A😀".to_bytes()))
	puts(decoded_text("41F09F9880"))
	puts(decoded_text("0g"))
	puts(decoded_text("abc"))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "41f09f9880\nA😀\ninvalid hexadecimal character:1\nhex input has odd length:3\n"; stdout.String() != want {
			t.Fatalf("unexpected %s portable hex output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunCompilerOwnedNamespaceImportsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping namespace import run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping namespace import run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/namespace-import-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import trb/std/encoding/hex
import trb/std/process
import { Result } from trb/std/result

def decoded_text(input: String): String
	case hex.decode(input)
	when Result::Ok(value)
		return value.to_s()
	when Result::Err(error)
		return error.message
	end
end

def main()
	puts(hex.encode("A".to_bytes()))
	puts(decoded_text("41"))
	puts(process.argv().size())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "41\nA\n0\n"; stdout.String() != want {
			t.Fatalf("unexpected %s namespace import output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableBase64AcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby base64 run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript base64 run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-base64-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { Result } from trb/std/result
import { decode, encode, url_decode, url_encode } from trb/std/encoding/base64

def decoded_text(input: String): String
	case decode(input)
	when Result::Ok(value)
		return value.to_s()
	when Result::Err(error)
		return error.message + ":" + error.index.to_s()
	end
end

def url_decoded_text(input: String): String
	case url_decode(input)
	when Result::Ok(value)
		return value.to_s()
	when Result::Err(error)
		return error.message + ":" + error.index.to_s()
	end
end

def main()
	puts(encode("A😀".to_bytes()))
	puts(url_encode("???".to_bytes()))
	puts(decoded_text("QfCfmIA="))
	puts(url_decoded_text("Pz8_"))
	puts(decoded_text("AAA"))
	puts(decoded_text("AA=A"))
	puts(decoded_text("AA$="))
	puts(decoded_text("AB=="))
	puts(url_decoded_text("A"))
	puts(url_decoded_text("AA=="))
	puts(url_decoded_text("AA$"))
	puts(url_decoded_text("AB"))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "QfCfmIA=\nPz8_\nA😀\n???\n" +
			"base64 input length must be a multiple of 4:3\n" +
			"invalid base64 padding:3\n" +
			"invalid base64 character:2\n" +
			"non-canonical base64 encoding:1\n" +
			"base64url input has invalid length:1\n" +
			"base64url input must not contain padding:2\n" +
			"invalid base64url character:2\n" +
			"non-canonical base64url encoding:1\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable base64 output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableHashAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby hash run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript hash run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-hash-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { md5, sha1, sha256, sha512 } from trb/std/hash
import { decode, encode } from trb/std/encoding/hex

def main()
	a8 := "aaaaaaaa"
	a56 := a8 + a8 + a8 + a8 + a8 + a8 + a8
	a112 := a56 + a56
	_decoded := decode("00")
	puts(encode(md5("".to_bytes())))
	puts(encode(md5("abc".to_bytes())))
	puts(encode(md5(a56.to_bytes())))
	puts(encode(sha1("".to_bytes())))
	puts(encode(sha1("abc".to_bytes())))
	puts(encode(sha1(a56.to_bytes())))
	puts(encode(sha256("".to_bytes())))
	puts(encode(sha256("abc".to_bytes())))
	puts(encode(sha256(a56.to_bytes())))
	puts(encode(sha512("".to_bytes())))
	puts(encode(sha512("abc".to_bytes())))
	puts(encode(sha512(a112.to_bytes())))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "d41d8cd98f00b204e9800998ecf8427e\n" +
			"900150983cd24fb0d6963f7d28e17f72\n" +
			"3b0c8ac703f828b04c6c197006d17218\n" +
			"da39a3ee5e6b4b0d3255bfef95601890afd80709\n" +
			"a9993e364706816aba3e25717850c26c9cd0d89d\n" +
			"c2db330f6083854c99d4b5bfb6e8f29f201be699\n" +
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n" +
			"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\n" +
			"b35439a4ac6f0948b6d6f9e3c6af0f5f590ce20f1bde7090ef7970686ec6738a\n" +
			"cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e\n" +
			"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f\n" +
			"c01d080efd492776a1c43bd23dd99d0a2e626d481e16782e75d54c2503b5dc32bd05f0f1ba33e568b88fd2d970929b719ecbb152f58f130a407c8830604b70ca\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable hash output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableHMACAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby hmac run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript hmac run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-hmac-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { equal, sha256, sha512 } from trb/std/hmac
import trb/std/secure_compare
import { decode, encode } from trb/std/encoding/hex

def main()
	a8 := "aaaaaaaa"
	a64 := a8 + a8 + a8 + a8 + a8 + a8 + a8 + a8
	key80 := a64 + a8 + a8
	key136 := a64 + a64 + a8
	key := "Jefe".to_bytes()
	message := "what do ya want for nothing?".to_bytes()
	tag := sha256(key, message)
	_decoded := decode("00")
	puts(encode(tag))
	puts(encode(sha512(key, message)))
	puts(encode(sha256(key80.to_bytes(), "message".to_bytes())))
	puts(encode(sha512(key136.to_bytes(), "message".to_bytes())))
	puts(equal(tag, tag))
	puts(equal(tag, sha256(key, "other".to_bytes())))
	puts(equal(tag, "short".to_bytes()))
	puts(secure_compare.equal(tag, tag))
	puts(secure_compare.equal(tag, sha256(key, "other".to_bytes())))
	puts(secure_compare.equal(tag, "short".to_bytes()))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843\n" +
			"164b7a7bfcf819e2e395fbe73b56e0a387bd64222e831fd610270cd7ea2505549758bf75c05a994a6d034f65f8f0e6fdcaeab1a34d4a6b4b636e070a38bce737\n" +
			"d0c62b445e5d504c9809dcaa12bfedd969deb591591984b81c68b352cec257ee\n" +
			"435bf6bbcffb2d5301b470b17314c3571666de1cd1f96776dfd9e59ce07f32338bfca69d7be3f6d33c3eee5def6ebec48e8181d86ea9ebeeb639fa3ce6da44d7\n" +
			"true\nfalse\nfalse\ntrue\nfalse\nfalse\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable hmac output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableRandomAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby random run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript random run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-random-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import trb/std/random
import trb/std/secure_random

def main()
	fraction := random.float()
	index := random.integer(10)
	puts(fraction >= 0.0 && fraction < 1.0)
	puts(index >= 0 && index < 10)
	puts(secure_random.bytes(0).size())
	puts(secure_random.bytes(32).size())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "true\ntrue\n0\n32\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable random output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableURLComponentsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby URL component run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript URL component run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-url-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { Result } from trb/std/result
import { PercentDecodeError, PercentDecodeErrorKind, decode_component, encode_component } from trb/std/url

def decode(value: String): Result<String, PercentDecodeError>
	return decode_component(value)
end

def decoded(value: String): String
	case decode(value)
	when Result::Ok(text)
		return text
	when Result::Err(error)
		case error.kind
		when PercentDecodeErrorKind::InvalidEscape
			return "invalid escape"
		when PercentDecodeErrorKind::InvalidUtf8
			return "invalid utf8"
		end
	end
end

def main()
	puts(encode_component("a b/😀+~"))
	puts(decoded("a%20b%2F%F0%9F%98%80%2B~"))
	puts(decoded("a+b"))
	puts(decoded("%"))
	puts(decoded("%FF"))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "a%20b%2F%F0%9F%98%80%2B~\na b/😀+~\na+b\ninvalid escape\ninvalid utf8\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable URL component output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableURLQueryAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby URL query run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript URL query run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-url-query-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { Result } from trb/std/result
import { QueryParameter, build_query, parse_query } from trb/std/url

def print_query(source: String)
	case parse_query(source)
	when Result::Ok(parameters)
		parameters.each do |parameter|
			puts(parameter.name + ":" + parameter.value)
		end
	when Result::Err(error)
		puts(error.input + ":" + error.message)
	end
	return
end

def main()
	query := build_query([
		QueryParameter.new(name: "tag", value: "type rb"),
		QueryParameter.new(name: "tag", value: "go"),
		QueryParameter.new(name: "symbol", value: "+&="),
		QueryParameter.new(name: "tilde", value: "~"),
		QueryParameter.new(name: "star", value: "*"),
	])
	puts(query)
	print_query("tag=go&&tag=type+rb&empty&symbol=%2B&text=%E6%97%A5%E6%9C%AC%E8%AA%9E&")
	print_query("name=%")
	print_query("%FF=value")
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "tag=type+rb&tag=go&symbol=%2B%26%3D&tilde=%7E&star=*\n" +
			"tag:go\ntag:type rb\nempty:\nsymbol:+\ntext:日本語\n" +
			"%:invalid percent escape in URL query component\n" +
			"%FF:decoded URL query component is not valid UTF-8\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable URL query output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestREPLPortableURLQueryUsesCompilerOwnedSource(t *testing.T) {
	input := "import { QueryParameter, build_query, parse_query } from trb/std/url\n" +
		"build_query([QueryParameter.new(name: \"tag\", value: \"type rb\"), QueryParameter.new(name: \"tag\", value: \"go\")])\n" +
		"parse_query(\"tag=type+rb&tag=go\")\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--mode", "go"}); status != 0 {
		t.Fatalf("REPL status=%d stderr=%s", status, stderr.String())
	}
	want := "\"tag=type+rb&tag=go\" : String\n" +
		"Result::Ok(value: [QueryParameter(name: \"tag\", value: \"type rb\"), QueryParameter(name: \"tag\", value: \"go\")]) : Result<Array<QueryParameter>, PercentDecodeError>\n"
	if stdout.String() != want {
		t.Fatalf("unexpected URL query REPL output: want %q, got %q; stderr=%s", want, stdout.String(), stderr.String())
	}
}

func TestRunCompilerOwnedUnicodeAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby Unicode run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript Unicode run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-unicode-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "import trb/std/unicode\n\ndef main()\n\tputs(unicode.version())\n\tputs(unicode.letter(12354))\n\tputs(unicode.from_codepoint(128512))\n\treturn\nend\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "15.0.0\ntrue\n😀\n"; stdout.String() != want {
			t.Fatalf("unexpected %s Unicode program output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunSafePortableConversionAndLookupAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby safe-operation run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript safe-operation run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-safe-operation-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "import { Result } from trb/std/result\n" +
			"import { IndexLookupError, KeyLookupError, NumberParseError } from trb/std/errors\n" +
			"import trb/std/string_builder\n" +
			"import trb/std/numbers\n" +
			"import trb/std/booleans\n" +
			"import trb/std/strings\n\n" +
			"def parse_result(value: Result<Integer, NumberParseError>): String; case value; when Result::Ok(number); return \"ok:\" + number.to_s(); when Result::Err(error); return \"err:\" + error.message; end; end\n" +
			"def float_result(value: Result<Float, NumberParseError>): String; case value; when Result::Ok(number); return \"ok:\" + number.to_s(); when Result::Err(error); return \"err:\" + error.message; end; end\n" +
			"def string_index_result(value: Result<String, IndexLookupError>): String; case value; when Result::Ok(text); return \"ok:\" + text; when Result::Err(error); return \"err:\" + error.index.to_s() + \"/\" + error.size.to_s() + \" \" + error.message; end; end\n" +
			"def index_result(value: Result<Integer, IndexLookupError>): String; case value; when Result::Ok(number); return \"ok:\" + number.to_s(); when Result::Err(error); return \"err:\" + error.message; end; end\n" +
			"def key_result(value: Result<String, KeyLookupError>): String; case value; when Result::Ok(text); return \"ok:\" + text; when Result::Err(error); return \"err:\" + error.message; end; end\n" +
			"def scalar_check(value: Float): Boolean; return (-4).abs() == numbers.absolute(-4) && 0.zero?() && 1.positive?() && (-1).negative?() && 2.even?() && 3.odd?() && (-0.25).abs() == 0.25 && (value.finite?() || value.infinite?() || value.nan?()) && true.to_s() == booleans.to_string(true); end\n\n" +
			"def main()\n" +
			"\tputs(scalar_check(0.25))\n" +
			"\tputs(parse_result(\"12\".try_to_i()))\n" +
			"\tputs(parse_result(\"12x\".try_to_i()))\n" +
			"\tputs(parse_result(\"9007199254740992\".try_to_i()))\n" +
			"\tputs(float_result(\"12.5\".try_to_f()))\n" +
			"\tputs(float_result(\"+.5e1\".try_to_f()))\n" +
			"\tputs(float_result(\"1.2x\".try_to_f()))\n" +
			"\tputs(float_result(\"1e9999\".try_to_f()))\n" +
			"\tputs(float_result(\"1e-9999\".try_to_f()))\n" +
			"\tputs(\"A😀\"[1])\n" +
			"\tputs(string_index_result(\"A😀\".try_fetch(0)))\n" +
			"\tputs(string_index_result(\"A😀\".try_fetch(2)))\n" +
			"\tputs(string_index_result(\"A😀\".try_fetch(-1)))\n" +
			"\tputs(\"A😀\".chars().join(\"|\"))\n" +
			"\tputs(\"A😀\".reverse())\n" +
			"\tputs(strings.reverse(\"TypeRB\"))\n" +
			"\tvalues := [7]\n" +
			"\tputs(index_result(values.try_fetch(0)))\n" +
			"\tputs(index_result(values.try_fetch(1)))\n" +
			"\tlabels: Hash<String, String> := {\"name\" => \"Ada\"}\n" +
			"\tputs(key_result(labels.try_fetch(\"name\")))\n" +
			"\tputs(key_result(labels.try_fetch(\"missing\")))\n" +
			"\tbuilder := string_builder.new()\n" +
			"\tputs(builder.empty?())\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "true\nok:12\nerr:invalid Integer\nerr:Integer is outside the portable range\nok:12.5\nok:5.0\nerr:invalid Float\nerr:Float is outside the portable range\nok:0.0\n😀\nok:A\nerr:2/2 String index is out of bounds\nerr:-1/2 String index is out of bounds\nA|😀\n😀A\nBRepyT\nok:7\nerr:Array index is out of bounds\nok:Ada\nerr:Hash key is missing\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s safe-operation output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableCollectionTransformationsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby collection-transformation run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript collection-transformation run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-collection-transformation-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import trb/std/math

enum UniqueState
	Ready
	Done
end

class SortKey
	@calls: Integer

	def initialize()
		@calls = 0
		return
	end

	def rank(value: Array<Integer>): Integer
		@calls += 1
		return value[0]
	end

	def calls(): Integer
		return @calls
	end
end

class Box
	@value: Integer

	def initialize(value: Integer)
		@value = value
		return
	end

	def value(): Integer
		return @value
	end
end

def empty_integers(): Array<Integer>
	return []
end

def main()
	mapped := [1, 2, 3].map do |value|
		doubled := value * 2
		doubled
	end
	selected := mapped.select.with_index do |value, index|
		large_enough := value > 2
		large_enough and index < 2
	end
	total := selected.reduce(10) do |sum, value|
		next_sum := sum + value
		next_sum
	end
	any_large := mapped.any? do |value|
		minimum := 5
		value > minimum
	end
	all_positive := selected.all?() do |value|
		value > 0
	end
	none_negative := mapped.none? do |value|
		value < 0
	end
	empty := empty_integers()
	empty_any := empty.any? do |value|
		value > 0
	end
	empty_all := empty.all? do |value|
		value > 0
	end
	empty_none := empty.none? do |value|
		value > 0
	end
	short_any := [1, 0].any? do |value|
		1 / value > 0
	end
	short_all := [10, 0].all? do |value|
		1 / value > 0
	end
	short_none := [1, 0].none? do |value|
		1 / value > 0
	end
	found := mapped.find do |value|
		value > 2
	end
	missing := mapped.find do |value|
		value > 99
	end
	found_index := mapped.find_index do |value|
		value == 6
	end
	short_find := [1, 0].find do |value|
		1 / value > 0
	end
	found_box := [Box.new(7)].find do |box|
		box.value() == 7
	end
	mut original := [3, 1, 2]
	ascending := original.sort()
	descending := original.sort_descending()
	key := SortKey.new()
	stable := [[2, 0], [1, 1], [2, 2]].sort_by do |value|
		rank := key.rank(value)
		rank
	end
	stable_descending := [[2, 0], [1, 1], [2, 2]].sort_by_descending do |value|
		value[0]
	end
	ordered_strings := ["😀", ""].sort()
	ordered_strings_descending := ["😀", ""].sort_descending()
	floats := [math.sqrt(-1), 1.0, -1.0]
	floats_ascending := floats.sort()
	floats_descending := floats.sort_descending()
	mut repeated := [3, 1, 3, 2, 1]
	unique_values := repeated.uniq()
	concatenated := repeated.concat([4, 5])
	unique_states := [UniqueState::Ready, UniqueState::Ready, UniqueState::Done].uniq()
	puts(mapped[2])
	puts(selected.size())
	puts(total)
	puts(any_large)
	puts(all_positive)
	puts(none_negative)
	puts(empty_any)
	puts(empty_all)
	puts(empty_none)
	puts(short_any)
	puts(short_all)
	puts(short_none)
	if found == nil
		puts(0)
	else
		puts(found + 1)
	end
	puts(missing == nil)
	if found_index != nil
		puts(found_index)
	end
	puts(short_find)
	if found_box != nil
		puts(found_box.value())
	end
	puts(original[0])
	puts(ascending[0])
	puts(descending[0])
	puts(stable[1][1])
	puts(stable[2][1])
	puts(stable_descending[0][1])
	puts(stable_descending[1][1])
	puts(key.calls())
	puts(ordered_strings[0])
	puts(ordered_strings_descending[0])
	puts(floats_ascending[0])
	puts(floats_ascending[2].nan?())
	puts(floats_descending[0])
	puts(floats_descending[2].nan?())
	puts(repeated.size())
	puts(unique_values.size())
	puts(unique_values[0])
	puts(unique_values[1])
	puts(unique_values[2])
	puts(concatenated.size())
	puts(concatenated[6])
	puts(unique_states.size())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "6\n1\n14\ntrue\ntrue\ntrue\nfalse\ntrue\ntrue\ntrue\nfalse\nfalse\n5\ntrue\n2\n1\n7\n3\n1\n3\n0\n2\n0\n2\n3\n\n😀\n-1.0\ntrue\n1.0\ntrue\n5\n3\n3\n1\n2\n7\n5\n2\n"; stdout.String() != want {
			t.Fatalf("unexpected %s collection-transformation output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableSlicingAndStringSearchAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby slice run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript slice run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-slice-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { Result } from trb/std/result
import { SliceRangeError } from trb/std/errors

def numbers_text(items: Array<Integer>): String
	texts := items.map do |item|
		item.to_s()
	end
	return texts.join(",")
end

def array_slice(value: Result<Array<Integer>, SliceRangeError>): String
	case value
	when Result::Ok(items)
		return numbers_text(items)
	when Result::Err(error)
		return "error:" + error.start.to_s() + ":" + error.finish.to_s() + ":" + error.size.to_s()
	end
end

def string_slice(value: Result<String, SliceRangeError>): String
	case value
	when Result::Ok(text)
		return text
	when Result::Err(error)
		return "error:" + error.message
	end
end

def main()
	bounds := 1...3
	puts(numbers_text([10, 20, 30, 40].slice(bounds)))
	puts(numbers_text([10, 20, 30, 40].slice(1..2)))
	puts([10, 20, 30, 40].slice(4...4).size())
	puts(array_slice([10, 20].try_slice(1...3)))
	puts("A😀BC"[1])
	puts("A😀BC".slice(bounds))
	puts("A😀BC".slice(1..2))
	puts("A😀BC".slice(4...4).size())
	puts(string_slice("A😀".try_slice(-1...1)))
	puts("A😀B😀".index("😀"))
	puts("A😀B😀".rindex("😀"))
	puts("A😀B😀".index("missing") == nil)
	puts("A😀B😀".index(""))
	puts("A😀B😀".rindex(""))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "20,30\n20,30\n0\nerror:1:3:2\n😀\n😀B\n😀B\n0\nerror:String slice range is out of bounds\n1\n3\ntrue\n0\n4\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s slice output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunNullableNarrowingAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby nullable-narrowing run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript nullable-narrowing run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-nullable-narrowing-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `def maybe_name(present: Boolean): String?
	if present
		return "Ada"
	else
		return nil
	end
end

def label(value: String?): String
	if value == nil
		return "missing"
	end
	return value + "!"
end

def has_name(value: String?): Boolean
	return value != nil and value.size() > 0
end

def missing_or_empty(value: String?): Boolean
	return value == nil or value.size() == 0
end

def main()
	present := maybe_name(true)
	missing := maybe_name(false)
	puts(label(present))
	puts(label(missing))
	puts(has_name(present))
	puts(has_name(missing))
	puts(missing_or_empty(present))
	puts(missing_or_empty(missing))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "Ada!\nmissing\ntrue\nfalse\nfalse\ntrue\n"; stdout.String() != want {
			t.Fatalf("unexpected %s nullable-narrowing output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunCompilerOwnedFilesystemAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby filesystem run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript filesystem run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-filesystem-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(root, "data")
		textPath := filepath.Join(directory, "note.txt")
		missingPath := filepath.Join(directory, "missing.txt")
		bmpPath := filepath.Join(directory, "\uE000")
		astralPath := filepath.Join(directory, "\U00010000")
		source := "import { FileError, create_directory, exists, list, read_text, write_text } from trb/std/filesystem\n" +
			"import { Result } from trb/std/result\n\n" +
			"def text_or_operation(value: Result<String, FileError>): String; case value; when Result::Ok(text); return text; when Result::Err(error); return error.operation; end; end\n" +
			"def names_or_error(value: Result<Array<String>, FileError>): Array<String>; case value; when Result::Ok(names); return names; when Result::Err(error); return [error.operation]; end; end\n" +
			"def boolean_or_false(value: Result<Boolean, FileError>): Boolean; case value; when Result::Ok(found); return found; when Result::Err(error); return error.operation.empty?(); end; end\n\n" +
			"def main()\n" +
			"\tcreate_directory(" + strconv.Quote(directory) + ")\n" +
			"\twrite_text(" + strconv.Quote(textPath) + ", \"A😀\")\n" +
			"\twrite_text(" + strconv.Quote(astralPath) + ", \"\")\n" +
			"\twrite_text(" + strconv.Quote(bmpPath) + ", \"\")\n" +
			"\tputs(text_or_operation(read_text(" + strconv.Quote(textPath) + ")))\n" +
			"\tputs(text_or_operation(read_text(" + strconv.Quote(missingPath) + ")))\n" +
			"\tputs(names_or_error(list(" + strconv.Quote(directory) + ")).join(\",\"))\n" +
			"\tputs(boolean_or_false(exists(" + strconv.Quote(textPath) + ")))\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "A😀\nread_text\nnote.txt,\uE000,\U00010000\ntrue\n"; stdout.String() != want {
			t.Fatalf("unexpected %s filesystem program output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunCompilerOwnedProcessAcrossAvailableBackends(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby process run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript process run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-process-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { ProcessError, ProcessResult, argv, run, working_directory } from trb/std/process
import { Result } from trb/std/result

def describe(value: Result<ProcessResult, ProcessError>): String
	case value
	when Result::Ok(result)
		return result.status.to_s() + ":" + result.stdout + ":" + result.stderr
	when Result::Err(error)
		return "error:" + error.operation
	end
end

def succeeded(value: Result<ProcessResult, ProcessError>): Boolean
	case value
	when Result::Ok(result)
		return result.success
	when Result::Err(error)
		return error.message.empty?()
	end
end

def operation(value: Result<ProcessResult, ProcessError>): String
	case value
	when Result::Ok(result)
		return result.stdout
	when Result::Err(error)
		return error.operation
	end
end

def directory_available(value: Result<String, ProcessError>): Boolean
	case value
	when Result::Ok(directory)
		return !directory.empty?()
	when Result::Err(error)
		return error.message.empty?()
	end
end

def main()
	result := run("/bin/sh", ["-c", "printf out; printf err >&2; exit 7"])
	puts(describe(result))
	puts(succeeded(result))
	empty_arguments: Array<String> := []
	puts(operation(run("/type-rb-command-that-does-not-exist", empty_arguments)))
	puts(directory_available(working_directory()))
	puts(argv().size())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "7:out:err\nfalse\nrun\ntrue\n0\n"; stdout.String() != want {
			t.Fatalf("unexpected %s process output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunCompilerOwnedJSONAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby JSON run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript JSON run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-json-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "import { JsonError, JsonValue, parse, stringify } from trb/std/json\n" +
			"import trb/std/jsonc\n" +
			"import { Result } from trb/std/result\n\n" +
			"def render(value: Result<JsonValue, JsonError>): String; case value; when Result::Ok(item); case stringify(item); when Result::Ok(source); return source; when Result::Err(error); return error.path; end; when Result::Err(error); return error.path; end; end\n" +
			"def error_path(value: Result<JsonValue, JsonError>): String; case value; when Result::Ok(item); return render(Result<JsonValue, JsonError>::Ok(item)); when Result::Err(error); return error.path; end; end\n\n" +
			"def valid(value: Result<JsonValue, JsonError>): Boolean; case value; when Result::Ok(item); return render(Result<JsonValue, JsonError>::Ok(item)).empty?(); when Result::Err(error); return error.message.empty?() or !error.message.empty?(); end; end\n\n" +
			"def main()\n" +
			"\tputs(render(jsonc.parse(\"{\\n  // comment\\n  \\\"items\\\": [1, 1.5, true, null]\\n}\")))\n" +
			"\tputs(error_path(parse(\"{\\\"items\\\":[9007199254740992]}\")))\n" +
			"\tputs(valid(jsonc.parse(\"{\\\"value\\\":1,}\")))\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "{\"items\":[1,1.5,true,null]}\n/items/0\ntrue\n"; stdout.String() != want {
			t.Fatalf("unexpected %s JSON program output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunTypedJSONRecordCodecsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby typed JSON codec run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript typed JSON codec run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-json-codec-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		contracts := filepath.Join(root, "src", "contracts", "user.trb")
		if err := os.MkdirAll(filepath.Dir(contracts), 0o755); err != nil {
			t.Fatal(err)
		}
		contractSource := "record Address\n\tcity: String\nend\n\n" +
			"record User\n\tid: Integer @json(\"user_id\")\n\tname: String\n\tnickname: String?\n\tscores: Array<Float>\n\tmetadata: Hash<String, Integer>\n\taddress: Address\nend\n"
		if err := os.WriteFile(contracts, []byte(contractSource), 0o644); err != nil {
			t.Fatal(err)
		}
		mainSource := "import { Address, User } from contracts/user\n" +
			"import { JsonError, decode, encode } from trb/std/json\n" +
			"import { Result } from trb/std/result\n\n" +
			"def round_trip(source: String): String; case decode<User>(source); when Result::Ok(user); case encode(user); when Result::Ok(encoded); case decode<User>(encoded); when Result::Ok(copy); return copy.name + \":\" + copy.address.city; when Result::Err(error); return error.path; end; when Result::Err(error); return error.path; end; when Result::Err(error); return error.path; end; end\n" +
			"def main()\n" +
			"\tputs(round_trip(\"{\\\"user_id\\\":7,\\\"name\\\":\\\"Ada\\\",\\\"scores\\\":[1,1.5],\\\"metadata\\\":{\\\"active\\\":1},\\\"address\\\":{\\\"city\\\":\\\"Tokyo\\\"}}\"))\n" +
			"\tputs(round_trip(\"{\\\"user_id\\\":7,\\\"scores\\\":[],\\\"metadata\\\":{},\\\"address\\\":{\\\"city\\\":\\\"Tokyo\\\"}}\"))\n" +
			"\tputs(round_trip(\"{\\\"user_id\\\":7,\\\"name\\\":\\\"Ada\\\",\\\"scores\\\":[],\\\"metadata\\\":{},\\\"address\\\":{\\\"city\\\":1}}\"))\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "Ada:Tokyo\n/name\n/address/city\n"; stdout.String() != want {
			t.Fatalf("unexpected %s typed JSON codec output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableDefaultArgumentsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby default argument run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript default argument run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-default-argument-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `class Greeter
	@prefix: String

	def initialize(prefix: String = "Hello")
		@prefix = prefix
		return
	end

	def greet(name: String, suffix: String = "!"): String
		return @prefix + ", " + name + suffix
	end
end

def count_label(count: Integer = 2): String
	return count.to_s()
end

def fallback(value: String, replacement: String = value): String
	return replacement
end

def missing?(value: String? = nil): Boolean
	return value == nil
end

def main()
	puts(Greeter.new().greet("Ada"))
	puts(Greeter.new("Hi").greet("Lin", "."))
	puts(count_label())
	puts(count_label(3))
	puts(fallback("same"))
	puts(missing?())
	puts(missing?("value"))
	puts(missing?(nil))
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Hello, Ada!\nHi, Lin.\n2\n3\nsame\ntrue\nfalse\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s default argument output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialHTTPValuesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/http run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/http run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-http-values-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Header, HeaderValueError, Headers, HttpMethod } from trb/http
import { Result } from trb/std/result

def render(result: Result<String, HeaderValueError>): String
	case result
	when Result::Ok(value)
		return "ok:" + value
	when Result::Err(error)
		case error
		when HeaderValueError::Missing(name)
			return "missing:" + name
		when HeaderValueError::Duplicate(name)
			return "duplicate:" + name
		end
	end
end

def main()
	headers := Headers.new([
		Header.new(name: "Content-Type", value: "application/json"),
		Header.new(name: "set-cookie", value: "one"),
		Header.new(name: "Set-Cookie", value: "two"),
	])
	mut copy := headers.entries()
	copy.push(Header.new(name: "x-copy", value: "only"))
	puts(headers.size())
	puts(headers.values("content-type")[0])
	puts(headers.values("SET-COOKIE").join("|"))
	puts(render(headers.value("content-type")))
	puts(render(headers.value("set-cookie")))
	puts(render(headers.value("missing")))
	puts(headers.key?("x-copy"))
	puts(HttpMethod.post().to_s())
	puts(HttpMethod.new("PROPFIND").to_s())
	body := Body.new("hello".to_bytes())
	puts(body.size())
	puts(body.bytes().to_s())
	puts(Body.empty().empty?())
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "3\napplication/json\none|two\nok:application/json\nduplicate:set-cookie\nmissing:missing\nfalse\nPOST\nPROPFIND\n5\nhello\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/http output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebRequestHeadersAndCookiesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web cookie run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web cookie run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-cookie-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Header, Headers, HeaderValueError, HttpMethod } from trb/http
import {
	CookieValueError,
	Request,
} from trb/web
import { Result } from trb/std/result

def render_header_value(result: Result<String, HeaderValueError>): String
	case result
	when Result::Ok(value)
		return "ok:" + value
	when Result::Err(error)
		case error
		when HeaderValueError::Missing(name)
			return "missing:" + name
		when HeaderValueError::Duplicate(name)
			return "duplicate:" + name
		end
	end
end

def render_cookie_value(result: Result<String, CookieValueError>): String
	case result
	when Result::Ok(value)
		return "ok:" + value
	when Result::Err(error)
		case error
		when CookieValueError::Missing(name)
			return "missing:" + name
		when CookieValueError::Duplicate(name)
			return "duplicate:" + name
		end
	end
end

def main()
	request := Request.new(
		method: HttpMethod.get(),
		path: "/",
		query_string: "",
		headers: Headers.new([
			Header.new(name: "Cookie", value: "session=abc; theme=dark"),
			Header.new(name: "Cookie", value: "tag=first; broken; =empty; tag=second; token=a=b"),
			Header.new(name: "X-Request-ID", value: "req-1"),
		]),
		body: Body.new("body".to_bytes()),
	)
	replaced := request.with_header("cookie", "fresh=one")
	added := replaced.add_header("COOKIE", "fresh=two")
	removed := added.without_header("x-request-id")
	puts(request.header_values("cookie").size())
	puts(added.header_values("cookie").size())
	puts(added.header_values("cookie")[0])
	puts(added.header_values("cookie")[1])
	puts(removed.header_values("x-request-id").size())
	puts(request.header_values("COOKIE").size())
	puts(render_header_value(request.header_value("x-request-id")))
	puts(render_header_value(request.header_value("cookie")))
	puts(render_header_value(request.header_value("missing")))
	parsed := request.cookies()
	puts(parsed.size())
	parsed.each do |value|
		puts(value.name + "=" + value.value)
	end
	puts(request.cookie_values("tag").size())
	puts(render_cookie_value(request.cookie_value("session")))
	puts(render_cookie_value(request.cookie_value("tag")))
	puts(render_cookie_value(request.cookie_value("missing")))
	puts(request.cookie_values("tag").empty?())
	puts(request.cookie_values("missing").empty?())
	puts(request.bytes().size())
	case request.text()
	when Result::Ok(value)
		puts(value)
	when Result::Err(_error)
		puts("invalid")
	end
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "2\n2\nfresh=one\nfresh=two\n0\n2\nok:req-1\nduplicate:cookie\nmissing:missing\n5\nsession=abc\ntheme=dark\ntag=first\ntag=second\ntoken=a=b\n2\nok:abc\nduplicate:tag\nmissing:missing\nfalse\ntrue\n4\nbody\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web cookie output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebQueryHelpersAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web query helper run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web query helper run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-query-helper-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { QueryValueError, Request } from trb/web
import { PercentDecodeError } from trb/std/url
import { Result } from trb/std/result

def request(query_string: String): Request
	return Request.new(method: HttpMethod.get(), path: "/", query_string: query_string, headers: Headers.new(), body: Body.empty())
end

def render_value(result: Result<String, QueryValueError>): String
	case result
	when Result::Ok(value)
		return "ok:" + value
	when Result::Err(error)
		case error
		when QueryValueError::Malformed(decode_error)
			return "malformed:" + decode_error.input
		when QueryValueError::Missing(name)
			return "missing:" + name
		when QueryValueError::Duplicate(name)
			return "duplicate:" + name
		end
	end
end

def print_values(result: Result<Array<String>, PercentDecodeError>)
	case result
	when Result::Ok(values)
		puts(values.size())
		values.each do |value|
			puts(value)
		end
	when Result::Err(error)
		puts(error.message)
	end
	return
end

def main()
	parsed := request("tag=go&tag=web&page=2&empty=")
	print_values(parsed.query_values("tag"))
	print_values(parsed.query_values("missing"))
	puts(render_value(parsed.query_value("page")))
	puts(render_value(parsed.query_value("missing")))
	puts(render_value(parsed.query_value("tag")))
	puts(render_value(request("value=%ZZ").query_value("value")))
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "2\ngo\nweb\n0\nok:2\nmissing:missing\nduplicate:tag\nmalformed:%ZZ\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web query helper output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebTypedParameterBindingAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-web-typed-parameter-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def show(path: String, query: String)
	response := dispatch(Request.new(method: HttpMethod.get(), path: path, query_string: query, headers: Headers.new(), body: Body.empty()))
	puts(response.status)
	puts(response.body.to_s())
	return
end

def main()
	show("/todos/7", "page=2&tag=go&tag=web&published=true&rating=4.5&date=2026-08-13&visibility=PUBLIC")
	show("/todos/7", "published=false")
	show("/todos/7", "page=1&page=2&published=true")
	show("/todos/nope", "published=true")
	show("/todos/7", "published=%ZZ")
	show("/todos/7", "published=true&visibility=UNKNOWN")
	return
end
`
			routeSource := `import {
	Context,
	ParameterError,
	ParameterSource,
	Response,
	text,
} from trb/web
import { Result } from trb/std/result
import { Date } from trb/std/time

enum Visibility
	Public = "PUBLIC"
	Private = "PRIVATE"
end

record TodoParams
	id: Integer
end

record TodoQuery
	page: Integer?
	tag: Array<String>
	published: Boolean
	rating: Float?
	date: Date?
	visibility: Visibility?
end

def parameter_error(error: ParameterError): Response
	case error
	when ParameterError::MalformedQuery(decode_error)
		return text("malformed:" + decode_error.input, 400)
	when ParameterError::Missing(source, name)
		return text("missing:" + source_name(source) + ":" + name, 400)
	when ParameterError::Duplicate(source, name)
		return text("duplicate:" + source_name(source) + ":" + name, 400)
	when ParameterError::Invalid(source, name, value, expected)
		return text("invalid:" + source_name(source) + ":" + name + ":" + value + ":" + expected, 400)
	end
end

def source_name(source: ParameterSource): String
	case source
	when ParameterSource::Path
		return "path"
	when ParameterSource::Query
		return "query"
	end
end

def get(context: Context): Response
	case context.params<TodoParams>()
	when Result::Err(error)
		return parameter_error(error)
	when Result::Ok(params)
		case context.request.query<TodoQuery>()
		when Result::Err(error)
			return parameter_error(error)
		when Result::Ok(query)
			page := if query.page == nil
				"nil"
			else
				"set"
			end
			extra := query.rating != nil && query.date != nil && query.visibility != nil
			return text(params.id.to_s() + "|" + page + "|" + query.tag.size().to_s() + "|" + query.published.to_s() + "|" + extra.to_s())
		end
	end
end
`
			if err := os.MkdirAll(filepath.Join(root, "src", "routes", "todos"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "routes", "todos", "[id].trb"), []byte(routeSource), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			want := "200\n7|set|2|true|true\n200\n7|nil|0|false|false\n400\nduplicate:query:page\n400\ninvalid:path:id:nope:Integer\n400\nmalformed:%ZZ\n400\ninvalid:query:visibility:UNKNOWN:Visibility\n"
			if stdout.String() != want {
				t.Fatalf("unexpected %s typed web parameter output: want %q, got %q", mode, want, stdout.String())
			}
		})
	}
}

func TestRunOfficialWebEndpointInputBindingAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-web-endpoint-input-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			mainSource := `import { Body, Header, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def show(path: String, query: String, body: String)
	request := Request.new(
		method: HttpMethod.post(),
		path: path,
		query_string: query,
		headers: Headers.new([Header.new(name: "content-type", value: "application/json")]),
		body: Body.new(body.to_bytes()),
	)
	response := dispatch(request)
	puts(response.status)
	puts(response.body.to_s())
	return
end

def main()
	show("/todos/7", "page=2", "{\"title\":\"ship\"}")
	show("/todos/bad", "page=2", "{\"title\":\"ship\"}")
	show("/todos/7", "", "{\"title\":\"ship\"}")
	show("/todos/7", "page=2", "{}")
	return
end
`
			routeSource := `import { Context, EndpointInputError, Response, text } from trb/web
import { Result } from trb/std/result

record TodoParams
	id: Integer
end

record TodoQuery
	page: Integer
end

record TodoBody
	title: String
end

record TodoInput
	params: TodoParams
	query: TodoQuery
	body: TodoBody
end

def get_error(error: EndpointInputError): Response
	case error
	when EndpointInputError::Params(_error)
		return text("params", 400)
	when EndpointInputError::Query(_error)
		return text("query", 400)
	when EndpointInputError::Body(_error)
		return text("body", 400)
	end
end

def post(context: Context): Response
	case context.bind<TodoInput>()
	when Result::Ok(input)
		return text(input.params.id.to_s() + "|" + input.query.page.to_s() + "|" + input.body.title)
	when Result::Err(error)
		return get_error(error)
	end
end
`
			if err := os.MkdirAll(filepath.Join(root, "src", "routes", "todos"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "routes", "todos", "[id].trb"), []byte(routeSource), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
			}
			want := "200\n7|2|ship\n400\nparams\n400\nquery\n400\nbody\n"
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("unexpected %s endpoint input output: want %q, got %q, stderr=%s", mode, want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunOfficialWebResponseHeadersAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web response header run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web response header run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-response-header-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Header, Headers, HeaderValueError } from trb/http
import {
	Response,
	redirect,
} from trb/web
import { Result } from trb/std/result

def render_header_value(result: Result<String, HeaderValueError>): String
	case result
	when Result::Ok(value)
		return "ok:" + value
	when Result::Err(error)
		case error
		when HeaderValueError::Missing(name)
			return "missing:" + name
		when HeaderValueError::Duplicate(name)
			return "duplicate:" + name
		end
	end
end

def main()
	base := Response.new(
		status: 204,
		headers: Headers.new([
			Header.new(name: "X-Trace", value: "one"),
			Header.new(name: "x-keep", value: "yes"),
			Header.new(name: "Vary", value: "Accept, Accept-Encoding"),
		]),
		body: Body.new("body".to_bytes()),
	)
	replaced := base.with_header("x-TRACE", "two")
	added := replaced.add_header("X-Trace", "three")
	removed := added.without_header("X-Keep")
	created := removed.with_status(201)
	varied := created.vary("accept").vary("Origin").vary("origin")
	found := varied.header_values("VARY")
	default_redirect := redirect("/login")
	temporary_redirect := redirect("/next", 307)
	puts(base.headers.values("X-Trace").size())
	puts(base.headers.values("X-Trace")[0])
	puts(added.headers.values("x-trace").size())
	puts(added.headers.values("x-trace")[0])
	puts(added.headers.values("x-trace")[1])
	puts(added.headers.values("x-keep")[0])
	puts(removed.headers.key?("x-keep"))
	puts(varied.status)
	puts(varied.body.to_s())
	puts(found.size())
	puts(found.join("|"))
	puts(render_header_value(base.header_value("x-keep")))
	puts(render_header_value(added.header_value("x-trace")))
	puts(render_header_value(base.header_value("missing")))
	puts(default_redirect.status)
	puts(default_redirect.headers.values("location")[0])
	puts(default_redirect.body.size())
	puts(temporary_redirect.status)
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "1\none\n2\ntwo\nthree\nyes\nfalse\n201\nbody\n2\nAccept, Accept-Encoding|Origin\nok:yes\nduplicate:x-trace\nmissing:missing\n302\n/login\n0\n307\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web response headers: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebResponseBuildersAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web response builder run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web response builder run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-response-builder-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { bytes, empty, text } from trb/web

def main()
	plain := text("hello")
	not_found := text("missing", 404)
	binary := bytes("raw".to_bytes())
	no_content := empty()
	reset := empty(205)
	puts(plain.status)
	puts(plain.headers.values("content-type")[0])
	puts(plain.body.to_s())
	puts(not_found.status)
	puts(binary.status)
	puts(binary.headers.values("content-type")[0])
	puts(binary.body.to_s())
	puts(no_content.status)
	puts(no_content.headers.size())
	puts(no_content.body.size())
	puts(reset.status)
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "200\ntext/plain; charset=utf-8\nhello\n404\n200\napplication/octet-stream\nraw\n204\n0\n0\n205\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web response builder output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebResponseCookiesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web response cookie run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web response cookie run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-response-cookie-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Headers } from trb/http
import {
	CookieSameSite,
	Response,
	ResponseCookie,
	ResponseCookieAttribute,
	new_response_cookie,
} from trb/web

def main()
	base := Response.new(status: 204, headers: Headers.new(), body: Body.empty())
	simple := base.set_cookie(new_response_cookie("theme", "dark"))
	session_cookie := ResponseCookie.new(
		name: "session",
		value: "abc",
		attributes: [
			ResponseCookieAttribute::Domain("example.com"),
			ResponseCookieAttribute::Path("/"),
			ResponseCookieAttribute::MaxAge(3600),
			ResponseCookieAttribute::Secure,
			ResponseCookieAttribute::HttpOnly,
			ResponseCookieAttribute::SameSite(CookieSameSite::Lax),
		],
	)
	complete := simple.set_cookie(session_cookie)
	puts(base.headers.size())
	puts(complete.headers.values("set-cookie").size())
	puts(complete.headers.values("set-cookie")[0])
	puts(complete.headers.values("set-cookie")[1])
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "0\n2\ntheme=dark\nsession=abc; Domain=example.com; Path=/; Max-Age=3600; Secure; HttpOnly; SameSite=Lax\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web response cookie output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebRejectsInvalidResponseCookiesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-web-response-cookie-validation-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	[
		"name",
		"value",
		"domain",
		"path",
		"max-age",
		"attribute",
		"same-site",
		"secure-prefix",
		"host-prefix",
		"duplicate",
		"valid",
	].each do |kind|
		response := dispatch(Request.new(
			method: HttpMethod.get(),
			path: "/cookies/" + kind,
			query_string: "",
			headers: Headers.new(),
			body: Body.empty(),
		))
		puts(response.status)
		values := response.header_values("set-cookie")
		puts(values.size())
		if !values.empty?()
			puts(values[0])
		end
	end
	return
end
`
			routeSource := `import { Body, Headers } from trb/http
import {
	Context,
	CookieSameSite,
	Response,
	ResponseCookie,
	ResponseCookieAttribute,
	new_response_cookie,
} from trb/web

def get(context: Context): Response
	kind := context.path_value("kind")
	base := Response.new(status: 204, headers: Headers.new(), body: Body.empty())
	if kind == "name"
		return base.set_cookie(new_response_cookie("bad name", "value"))
	end
	if kind == "value"
		return base.set_cookie(new_response_cookie("session", "non ascii"))
	end
	if kind == "domain"
		return base.set_cookie(ResponseCookie.new(
			name: "session",
			value: "value",
			attributes: [ResponseCookieAttribute::Domain("-example.com")],
		))
	end
	if kind == "path"
		return base.set_cookie(ResponseCookie.new(
			name: "session",
			value: "value",
			attributes: [ResponseCookieAttribute::Path("relative")],
		))
	end
	if kind == "max-age"
		return base.set_cookie(ResponseCookie.new(
			name: "session",
			value: "value",
			attributes: [ResponseCookieAttribute::MaxAge(-1)],
		))
	end
	if kind == "attribute"
		return base.set_cookie(ResponseCookie.new(
			name: "session",
			value: "value",
			attributes: [ResponseCookieAttribute::Secure, ResponseCookieAttribute::Secure],
		))
	end
	if kind == "same-site"
		return base.set_cookie(ResponseCookie.new(
			name: "session",
			value: "value",
			attributes: [ResponseCookieAttribute::SameSite(CookieSameSite::None)],
		))
	end
	if kind == "secure-prefix"
		return base.set_cookie(new_response_cookie("__Secure-session", "value"))
	end
	if kind == "host-prefix"
		return base.set_cookie(ResponseCookie.new(
			name: "__Host-session",
			value: "value",
			attributes: [ResponseCookieAttribute::Secure, ResponseCookieAttribute::Path("/app")],
		))
	end
	if kind == "duplicate"
		first := base.set_cookie(new_response_cookie("session", "first"))
		return first.set_cookie(new_response_cookie("session", "second"))
	end
	return base.set_cookie(ResponseCookie.new(
		name: "__Host-session",
		value: "value",
		attributes: [
			ResponseCookieAttribute::Secure,
			ResponseCookieAttribute::Path("/"),
			ResponseCookieAttribute::SameSite(CookieSameSite::None),
		],
	))
end
`
			if err := os.MkdirAll(filepath.Join(root, "src", "routes", "cookies"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "routes", "cookies", "[kind].trb"), []byte(routeSource), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
			}
			want := strings.Repeat("500\n0\n", 10) + "204\n1\n__Host-session=value; Secure; Path=/; SameSite=None\n"
			if stdout.String() != want {
				t.Fatalf("unexpected %s response-cookie validation output: want %q, got %q", mode, want, stdout.String())
			}
		})
	}
}

func TestRunOfficialWebJSONAPIsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web JSON run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web JSON run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-json-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Header, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	request := Request.new(
		method: HttpMethod.post(),
		path: "/todos/7",
		query_string: "tag=type+rb&tag=go",
		headers: Headers.new([Header.new(name: "content-type", value: "application/json")]),
		body: Body.new("{\"title\":\"ship\"}".to_bytes()),
	)
	response := dispatch(request)
	puts(response.status)
	puts(response.body.to_s())
	method_not_allowed := dispatch(Request.new(method: HttpMethod.get(), path: "/todos/7", query_string: "", headers: Headers.new(), body: Body.empty()))
	puts(method_not_allowed.status)
	puts(method_not_allowed.headers.values("allow")[0])
	puts(method_not_allowed.body.to_s())
	mut oversized_body := "a".to_bytes()
	(0...21).each do |_index|
		oversized_body = oversized_body.concat(oversized_body)
	end
	payload_too_large := dispatch(Request.new(method: HttpMethod.post(), path: "/todos/7", query_string: "", headers: Headers.new(), body: Body.new(oversized_body)))
	puts(payload_too_large.status)
	puts(payload_too_large.body.to_s())
	return
end
`
		routeSource := `import { Context, Response, json } from trb/web
import { Result } from trb/std/result

record TodoRequest
	title: String
end

record TodoResponse
	id: String
	title: String
end

def post(context: Context): Response
	id := context.path_value("id")
	case context.request.json<TodoRequest>()
	when Result::Ok(payload)
		case context.request.query_parameters()
		when Result::Ok(parameters)
			return json(TodoResponse.new(id: id, title: payload.title + ":" + parameters[0].value + ":" + parameters[1].value), 201)
		when Result::Err(_error)
			return json(TodoResponse.new(id: id, title: "invalid query"), 400)
		end
	when Result::Err(_error)
		return json(TodoResponse.new(id: id, title: "invalid"), 400)
	end
end
`
		rootMiddlewareSource := `import { Context, Next, Response } from trb/web

def call(context: Context, next_handler: Next): Response
	puts("root:before")
	response := next_handler.call(context)
	puts("root:after")
	return response
end
`
		nestedMiddlewareSource := `import { Context, Next, Response } from trb/web

def call(context: Context, next_handler: Next): Response
	puts("todos:before")
	response := next_handler.call(context)
	puts("todos:after")
	return response
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		routePath := filepath.Join(root, "src", "routes", "todos", "[id].trb")
		if err := os.MkdirAll(filepath.Dir(routePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(routePath, []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(rootMiddlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "todos", "_middleware.trb"), []byte(nestedMiddlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "root:before\ntodos:before\ntodos:after\nroot:after\n201\n{\"id\":\"7\",\"title\":\"ship:type rb:go\"}\nroot:before\nroot:after\n405\nOPTIONS, POST\n{\"error\":\"method_not_allowed\"}\nroot:before\nroot:after\n413\n{\"error\":\"payload_too_large\"}\n"; stdout.String() != want {
			t.Fatalf("unexpected %s trb/web JSON output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebCatchAllRoutesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-web-catch-all-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def show(method: String, path: String)
	response := dispatch(Request.new(method: HttpMethod.new(method), path: path, query_string: "", headers: Headers.new(), body: Body.empty()))
	puts(response.status)
	puts(response.body.to_s())
	allow := response.header_values("allow")
	if allow.empty?()
		puts("-")
	else
		puts(allow[0])
	end
	return
end

def main()
	show("GET", "/files/guides/getting%20started")
	show("GET", "/files/readme")
	show("GET", "/files")
	show("GET", "/files/%2Fsecret")
	show("POST", "/files/a/b")
	show("OPTIONS", "/files/a/b")
	return
end
`
			routeSource := `import { Context, Response, text } from trb/web

def get(context: Context): Response
	return text(context.path_value("path"))
end
`
			if err := os.MkdirAll(filepath.Join(root, "src", "routes", "files"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "routes", "files", "[...path].trb"), []byte(routeSource), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			want := "200\nguides/getting started\n-\n200\nreadme\n-\n404\n{\"error\":\"not_found\"}\n-\n400\n{\"error\":\"bad_request\"}\n-\n405\n{\"error\":\"method_not_allowed\"}\nGET, HEAD, OPTIONS\n204\n\nGET, HEAD, OPTIONS\n"
			if stdout.String() != want {
				t.Fatalf("unexpected %s catch-all output: want %q, got %q", mode, want, stdout.String())
			}
		})
	}
}

func TestRunOfficialWebRequestErrorsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web request error run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web request error run")
				continue
			}
		}

		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-request-error-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}

		mainSource := `import { Body, Header, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing
import { decode } from trb/std/encoding/hex
import { Result } from trb/std/result

record RequestInput
	headers: Headers
	body: Body
end

def print_response(input: RequestInput)
	response := dispatch(Request.new(
		method: HttpMethod.post(),
		path: "/payload",
		query_string: "",
		headers: input.headers,
		body: input.body,
	))
	puts(response.status)
	puts(response.body.to_s())
	return
end

def main()
	valid_body := Body.new("{\"title\":\"ship\"}".to_bytes())
	print_response(RequestInput.new(headers: Headers.new(), body: valid_body))
	print_response(RequestInput.new(headers: Headers.new([Header.new(name: "Content-Type", value: "application/json"), Header.new(name: "content-type", value: "application/json")]), body: valid_body))
	print_response(RequestInput.new(headers: Headers.new([Header.new(name: "content-type", value: "text/plain")]), body: valid_body))
	print_response(RequestInput.new(headers: Headers.new([Header.new(name: "content-type", value: "Application/JSON; Charset=UTF-8")]), body: valid_body))
	print_response(RequestInput.new(headers: Headers.new([Header.new(name: "content-type", value: "application/vnd.example+json")]), body: valid_body))
	case decode("FF")
	when Result::Ok(invalid_utf8)
		print_response(RequestInput.new(headers: Headers.new([Header.new(name: "content-type", value: "application/json")]), body: Body.new(invalid_utf8)))
	when Result::Err(_error)
		return
	end
	print_response(RequestInput.new(headers: Headers.new([Header.new(name: "content-type", value: "application/json")]), body: Body.new("{".to_bytes())))
	return
end
`
		routeSource := `import { Context, RequestError, Response, text } from trb/web
import { Result } from trb/std/result

record Payload
	title: String
end

def post(context: Context): Response
	case context.request.json<Payload>()
	when Result::Ok(payload)
		return text("ok:" + payload.title)
	when Result::Err(error)
		case error
		when RequestError::MissingContentType
			return text("missing_content_type", 400)
		when RequestError::DuplicateContentType
			return text("duplicate_content_type", 400)
		when RequestError::UnsupportedContentType(value)
			return text("unsupported_content_type:" + value, 400)
		when RequestError::InvalidUtf8
			return text("invalid_utf8", 400)
		when RequestError::InvalidJson(_json_error)
			return text("invalid_json", 400)
		end
	end
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "payload.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "400\nmissing_content_type\n" +
			"400\nduplicate_content_type\n" +
			"400\nunsupported_content_type:text/plain\n" +
			"200\nok:ship\n" +
			"200\nok:ship\n" +
			"400\ninvalid_utf8\n" +
			"400\ninvalid_json\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web request error output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebMethodSemanticsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web method semantics run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web method semantics run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-head-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Header, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def request(method: String, path: String): Request
	return Request.new(
		method: HttpMethod.new(method),
		path: path,
		query_string: "",
		headers: Headers.new([Header.new(name: "X-Trace", value: "value")]),
		body: Body.empty(),
	)
end

def main()
	fallback := dispatch(request("head", "/fallback"))
	puts(fallback.status)
	puts(fallback.body.size())
	puts(fallback.headers.values("x-handler")[0])
	puts(fallback.headers.values("x-method")[0])
	puts(fallback.headers.values("x-trace")[0])
	explicit := dispatch(request("HEAD", "/explicit"))
	puts(explicit.status)
	puts(explicit.body.size())
	puts(explicit.headers.values("x-handler")[0])
	automatic_options := dispatch(request("OPTIONS", "/fallback"))
	puts(automatic_options.status)
	puts(automatic_options.headers.values("allow")[0])
	puts(automatic_options.body.size())
	explicit_options := dispatch(request("OPTIONS", "/explicit"))
	puts(explicit_options.status)
	puts(explicit_options.headers.values("x-handler")[0])
	puts(explicit_options.body.to_s())
	unsupported := dispatch(request("POST", "/fallback"))
	puts(unsupported.status)
	puts(unsupported.headers.values("allow")[0])
	puts(unsupported.body.to_s())
	missing := dispatch(request("HEAD", "/missing"))
	puts(missing.status)
	puts(missing.body.size())
	return
end
`
		fallbackRouteSource := `import { Body, Header, Headers } from trb/http
import { Context, Response } from trb/web

def get(context: Context): Response
	puts("fallback:get")
	return Response.new(
		status: 200,
		headers: Headers.new([
			Header.new(name: "x-handler", value: "get"),
			Header.new(name: "x-method", value: context.request.method.to_s()),
			Header.new(name: "x-trace", value: context.request.headers.values("x-trace")[0]),
		]),
		body: Body.new("fallback".to_bytes()),
	)
end
`
		explicitRouteSource := `import { Body, Header, Headers } from trb/http
import { Context, Response } from trb/web

def get(_context: Context): Response
	puts("explicit:get")
	return Response.new(status: 200, headers: Headers.new([Header.new(name: "x-handler", value: "get")]), body: Body.new("get".to_bytes()))
end

def head(_context: Context): Response
	puts("explicit:head")
	return Response.new(status: 202, headers: Headers.new([Header.new(name: "x-handler", value: "head")]), body: Body.new("head".to_bytes()))
end

def options(_context: Context): Response
	puts("explicit:options")
	return Response.new(status: 203, headers: Headers.new([Header.new(name: "x-handler", value: "options")]), body: Body.new("explicit-options".to_bytes()))
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web

def call(context: Context, next_handler: Next): Response
	puts("middleware:before")
	response := next_handler.call(context)
	puts("middleware:after")
	return response
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "fallback.trb"), []byte(fallbackRouteSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "explicit.trb"), []byte(explicitRouteSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "middleware:before\nfallback:get\nmiddleware:after\n200\n0\nget\nHEAD\nvalue\nmiddleware:before\nexplicit:head\nmiddleware:after\n202\n0\nhead\nmiddleware:before\nmiddleware:after\n204\nGET, HEAD, OPTIONS\n0\nmiddleware:before\nexplicit:options\nmiddleware:after\n203\noptions\nexplicit-options\nmiddleware:before\nmiddleware:after\n405\nGET, HEAD, OPTIONS\n{\"error\":\"method_not_allowed\"}\nmiddleware:before\nmiddleware:after\n404\n0\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web method semantics output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebRecoveryAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web recovery run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web recovery run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-recovery-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	request := Request.new(method: HttpMethod.get(), path: "/failure", query_string: "", headers: Headers.new(), body: Body.empty())
	response := dispatch(request)
	puts(response.status)
	puts(response.body.to_s())
	return
end
`
		routeSource := `import { Context, Response, json } from trb/web

record FailureResponse
	value: Integer
end

def get(_context: Context): Response
	value := "not-an-integer".to_i()
	return json(FailureResponse.new(value: value))
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		routePath := filepath.Join(root, "src", "routes", "failure.trb")
		if err := os.MkdirAll(filepath.Dir(routePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(routePath, []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "500\n{\"error\":\"internal_server_error\"}\n"; stdout.String() != want {
			t.Fatalf("unexpected %s trb/web recovery output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebResponseValidationAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web response validation run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web response validation run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-response-validation-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request, Response } from trb/web
import { dispatch } from trb/web/testing

def print_response(response: Response)
	puts(response.status)
	puts(response.body.to_s())
	puts(response.headers.key?("x-injected"))
	return
end

def main()
	["invalid-name", "invalid-value", "invalid-status", "valid"].each do |path|
		print_response(dispatch(Request.new(
			method: HttpMethod.get(),
			path: "/" + path,
			query_string: "",
			headers: Headers.new(),
			body: Body.empty(),
		)))
	end
	return
end
`
		routes := map[string]string{
			"invalid-name.trb": `import { Body, Header, Headers } from trb/http
import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 200, headers: Headers.new([Header.new(name: "bad name", value: "value")]), body: Body.new("unsafe".to_bytes()))
end
`,
			"invalid-value.trb": `import { Body, Header, Headers } from trb/http
import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 200, headers: Headers.new([Header.new(name: "x-safe", value: "safe\r\nx-injected: yes")]), body: Body.new("unsafe".to_bytes()))
end
`,
			"invalid-status.trb": `import { Body, Headers } from trb/http
import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 42, headers: Headers.new(), body: Body.new("unsafe".to_bytes()))
end
`,
			"valid.trb": `import { Body, Header, Headers } from trb/http
import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 218, headers: Headers.new([Header.new(name: "x-valid_token", value: "value")]), body: Body.new("valid".to_bytes()))
end
`,
		}
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		for filename, source := range routes {
			if err := os.WriteFile(filepath.Join(root, "src", "routes", filename), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "500\n{\"error\":\"internal_server_error\"}\nfalse\n500\n{\"error\":\"internal_server_error\"}\nfalse\n500\n{\"error\":\"internal_server_error\"}\nfalse\n218\nvalid\nfalse\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web response validation output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebJSONLLoggerAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web logger run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web logger run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-logger-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	_logged_response := dispatch(Request.new(method: HttpMethod.get(), path: "/logged", query_string: "token=must-not-be-logged", headers: Headers.new(), body: Body.empty()))
	_excluded_response := dispatch(Request.new(method: HttpMethod.get(), path: "/health", query_string: "", headers: Headers.new(), body: Body.empty()))
	_not_found_response := dispatch(Request.new(method: HttpMethod.get(), path: "/missing", query_string: "", headers: Headers.new(), body: Body.empty()))
	_method_not_allowed_response := dispatch(Request.new(method: HttpMethod.post(), path: "/logged", query_string: "", headers: Headers.new(), body: Body.empty()))
	_failure_response := dispatch(Request.new(method: HttpMethod.get(), path: "/failure", query_string: "", headers: Headers.new(), body: Body.empty()))
	return
end
`
		routeSource := `import { Body, Headers } from trb/http
import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 204, headers: Headers.new(), body: Body.empty())
end
`
		failureRouteSource := `import { Context, Response, json } from trb/web

record FailureResponse
	value: Integer
end

def get(_context: Context): Response
	value := "not-an-integer".to_i()
	return json(FailureResponse.new(value: value))
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/logger
import { LoggerOptions } from trb/web/middleware/logger

LOGGER_OPTIONS := LoggerOptions.new(stderr: false, exclude_paths: ["/health"])

def call(context: Context, next_handler: Next): Response
	return logger.call(context, next_handler, LOGGER_OPTIONS)
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "logged.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "health.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "failure.trb"), []byte(failureRouteSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		if len(lines) != 4 {
			t.Fatalf("%s logger emitted %d lines, want 4: %q", mode, len(lines), stdout.String())
		}
		wantEntries := []struct {
			method string
			path   string
			status float64
			level  string
		}{
			{method: "GET", path: "/logged", status: 204, level: "info"},
			{method: "GET", path: "/missing", status: 404, level: "info"},
			{method: "POST", path: "/logged", status: 405, level: "info"},
			{method: "GET", path: "/failure", status: 500, level: "error"},
		}
		for index, line := range lines {
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("%s logger did not emit JSONL: %v: %q", mode, err, line)
			}
			want := wantEntries[index]
			if entry["event"] != "http_request" || entry["level"] != want.level || entry["method"] != want.method || entry["path"] != want.path || entry["status"] != want.status {
				t.Fatalf("unexpected %s logger entry %d: %#v", mode, index, entry)
			}
			if timestamp, ok := entry["timestamp"].(string); !ok || timestamp == "" {
				t.Fatalf("%s logger timestamp is missing: %#v", mode, entry)
			}
			if duration, ok := entry["duration_ms"].(float64); !ok || duration < 0 {
				t.Fatalf("%s logger duration is invalid: %#v", mode, entry)
			}
		}
	}
}

func TestRunOfficialWebMiddlewareCompositionAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby middleware composition run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript middleware composition run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-middleware-composition-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	ordered := dispatch(Request.new(method: HttpMethod.get(), path: "/ordered", query_string: "", headers: Headers.new(), body: Body.empty()))
	puts(ordered.status)
	puts(ordered.headers.values("x-content-type-options")[0])
	rejected := dispatch(Request.new(method: HttpMethod.get(), path: "/twice", query_string: "", headers: Headers.new(), body: Body.empty()))
	puts(rejected.status)
	puts(rejected.body.to_s())
	return
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import { Middleware, compose } from trb/web/middleware
import trb/web/middleware/secure_headers

class TraceMiddleware implements Middleware
	@label: String

	def initialize(label: String)
		@label = label
		return
	end

	def call(context: Context, next_handler: Next): Response
		puts(@label + ":before")
		response := next_handler.call(context)
		puts(@label + ":after")
		return response
	end
end

class DoubleCallMiddleware implements Middleware
	def call(context: Context, next_handler: Next): Response
		_first := next_handler.call(context)
		return next_handler.call(context)
	end
end

ORDERED: Array<Middleware> := [
	TraceMiddleware.new("first"),
	secure_headers.middleware(),
	TraceMiddleware.new("second"),
]
DOUBLE_CALL: Array<Middleware> := [DoubleCallMiddleware.new()]

def call(context: Context, next_handler: Next): Response
	if context.request.path == "/twice"
		return compose(context, next_handler, DOUBLE_CALL)
	end
	return compose(context, next_handler, ORDERED)
end
`
		routeSource := `import { Context, Response, text } from trb/web

def get(context: Context): Response
	puts("route:" + context.request.path)
	return text("ok")
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "ordered.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "twice.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "first:before\nsecond:before\nroute:/ordered\nsecond:after\nfirst:after\n200\nnosniff\nroute:/twice\n500\n{\"error\":\"internal_server_error\"}\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s middleware composition output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebSecureHeadersAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web secure headers run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web secure headers run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-secure-headers-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	default_response := dispatch(Request.new(method: HttpMethod.get(), path: "/default", query_string: "", headers: Headers.new(), body: Body.empty()))
	puts(default_response.headers.values("x-content-type-options")[0])
	puts(default_response.headers.values("x-frame-options")[0])
	puts(default_response.headers.values("referrer-policy")[0])
	puts(default_response.headers.values("x-xss-protection")[0])
	custom_response := dispatch(Request.new(method: HttpMethod.get(), path: "/custom", query_string: "", headers: Headers.new(), body: Body.empty()))
	puts(custom_response.headers.values("x-custom-security")[0])
	puts(custom_response.headers.key?("x-content-type-options"))
	return
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/secure_headers
import { SecureHeadersOptions } from trb/web/middleware/secure_headers

CUSTOM_OPTIONS := SecureHeadersOptions.new(headers: {"x-custom-security" => "enabled"})

def call(context: Context, next_handler: Next): Response
	if context.request.path == "/custom"
		return secure_headers.call(context, next_handler, CUSTOM_OPTIONS)
	end
	return secure_headers.call(context, next_handler)
end
`
		routeSource := `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("ok").with_header("X-Frame-Options", "DENY")
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "default.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "custom.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "nosniff\nSAMEORIGIN\nno-referrer\n0\nenabled\nfalse\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web secure headers output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebRequestIDAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web request ID run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web request ID run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-request-id-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Header, Headers, HttpMethod } from trb/http
import { Request, Response } from trb/web
import { dispatch } from trb/web/testing

def send(headers: Headers): Response
	return dispatch(Request.new(
		method: HttpMethod.get(),
		path: "/id",
		query_string: "",
		headers: headers,
		body: Body.empty(),
	))
end

def print_generated(response: Response)
	value := response.body.to_s()
	puts(value.size())
	puts(value == response.headers.values("x-request-id")[0])
	return
end

def main()
	preserved := send(Headers.new([Header.new(name: "x-request-id", value: "upstream-123")]))
	puts(preserved.body.to_s())
	puts(preserved.body.to_s() == preserved.headers.values("x-request-id")[0])

	invalid := send(Headers.new([Header.new(name: "x-request-id", value: "bad id")]))
	print_generated(invalid)
	puts(invalid.body.to_s() != "bad id")

	duplicate := send(Headers.new([Header.new(name: "x-request-id", value: "first"), Header.new(name: "x-request-id", value: "second")]))
	print_generated(duplicate)

	missing := send(Headers.new())
	print_generated(missing)
	return
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/request_id

def call(context: Context, next_handler: Next): Response
	return request_id.call(context, next_handler)
end
`
		routeSource := `import { Context, Response, text } from trb/web
import { Result } from trb/std/result

def get(context: Context): Response
	case context.request.header_value("x-request-id")
	when Result::Ok(value)
		return text(value)
	when Result::Err(_error)
		return text("missing", 500)
	end
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "id.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "upstream-123\ntrue\n32\ntrue\ntrue\n32\ntrue\n32\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web request ID output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebCORSAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web CORS run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web CORS run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-cors-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Body, Header, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	allowed := dispatch(Request.new(
		method: HttpMethod.get(),
		path: "/allowed",
		query_string: "",
		headers: Headers.new([Header.new(name: "origin", value: "https://app.example")]),
		body: Body.empty(),
	))
	puts(allowed.headers.values("access-control-allow-origin")[0])
	puts(allowed.headers.values("access-control-allow-credentials")[0])
	puts(allowed.headers.values("access-control-expose-headers")[0])
	puts(allowed.headers.values("vary").join("|"))

	disallowed := dispatch(Request.new(
		method: HttpMethod.get(),
		path: "/disallowed",
		query_string: "",
		headers: Headers.new([Header.new(name: "origin", value: "https://other.example")]),
		body: Body.empty(),
	))
	puts(disallowed.headers.key?("access-control-allow-origin"))
	puts(disallowed.headers.values("vary").join("|"))

	preflight := dispatch(Request.new(
		method: HttpMethod.options(),
		path: "/allowed",
		query_string: "",
		headers: Headers.new([
			Header.new(name: "origin", value: "https://app.example"),
			Header.new(name: "access-control-request-method", value: "POST"),
			Header.new(name: "access-control-request-headers", value: "content-type, x-trace"),
		]),
		body: Body.empty(),
	))
	puts(preflight.status)
	puts(preflight.headers.values("access-control-allow-methods")[0])
	puts(preflight.headers.values("access-control-allow-headers")[0])
	puts(preflight.headers.values("access-control-max-age")[0])
	puts(preflight.headers.values("vary").join("|"))
	puts(preflight.headers.key?("x-handler"))

	wildcard := dispatch(Request.new(
		method: HttpMethod.get(),
		path: "/wildcard",
		query_string: "",
		headers: Headers.new([Header.new(name: "origin", value: "https://any.example")]),
		body: Body.empty(),
	))
	puts(wildcard.headers.values("access-control-allow-origin")[0])
	puts(wildcard.headers.values("vary").join("|"))
	return
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/cors
import { CORSOptions, PreflightMaxAge } from trb/web/middleware/cors

CORS_OPTIONS := CORSOptions.new(
	allow_origins: ["https://app.example"],
	allow_methods: ["GET", "POST", "OPTIONS"],
	allow_headers: [],
	expose_headers: ["x-trace-id"],
	credentials: true,
	max_age: PreflightMaxAge::Seconds(600),
)

def call(context: Context, next_handler: Next): Response
	if context.request.path == "/wildcard"
		return cors.call(context, next_handler)
	end
	return cors.call(context, next_handler, CORS_OPTIONS)
end
`
		routeSource := `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("ok").with_header("Vary", "Accept").with_header("X-Handler", "route")
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"allowed.trb", "disallowed.trb", "wildcard.trb"} {
			if err := os.WriteFile(filepath.Join(root, "src", "routes", name), []byte(routeSource), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "https://app.example\ntrue\nx-trace-id\nAccept|Origin\nfalse\nAccept|Origin\n204\nGET, POST, OPTIONS\ncontent-type, x-trace\n600\nOrigin|Access-Control-Request-Headers\nfalse\n*\nAccept\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web CORS output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunWithoutSourceRequiresMain(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/acme/no-main"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "library.trb"), []byte("def value(): Integer\n  return 1\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "project has no top-level main()") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}

func TestBuildCanEmbedInExistingRailsProjectWithoutManagingGemfile(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.SourceDir = "type_rb"
	config.OutDir = "type_rb_build"
	config.PackageManagement = project.ExternalPackages
	copyFiles := false
	config.CopyFiles = &copyFiles
	config.Ruby.Loader = "zeitwerk"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	gemfile := filepath.Join(root, "Gemfile")
	const originalGemfile = "source 'https://example.invalid'\n"
	if err := os.WriteFile(gemfile, []byte(originalGemfile), 0o644); err != nil {
		t.Fatal(err)
	}
	controller := filepath.Join(root, "type_rb", "app", "controllers", "api", "v1", "internal", "insurers_controller.trb")
	if err := os.MkdirAll(filepath.Dir(controller), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "import trb/platform/ruby/rails\n\nmodule Api\n  module V1\n    module Internal\n      class InsurersController < Api::ApplicationController\n        include PaginationHelper\n\n        def index()\n          page := paginate_with_headers(Insurer.all())\n          insurers := page[0]\n          render(json: insurers)\n          return\n        end\n\n        def show()\n          insurer := Insurer.find_by!(code: params[:code])\n          render(json: insurer.as_json())\n          return\n        end\n      end\n    end\n  end\nend\n"
	if err := os.WriteFile(controller, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	actualGemfile, err := os.ReadFile(gemfile)
	if err != nil || string(actualGemfile) != originalGemfile {
		t.Fatalf("host Gemfile was modified: err=%v\n%s", err, actualGemfile)
	}
	generated := filepath.Join(root, "type_rb_build", "app", "controllers", "api", "v1", "internal", "insurers_controller.rb")
	output, err := os.ReadFile(generated)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "Insurer.find_by!(code: params[:code])") {
		t.Fatalf("unexpected generated controller:\n%s", output)
	}
	if !strings.Contains(string(output), "page = paginate_with_headers(Insurer.all())") {
		t.Fatalf("generated controller omitted index pagination:\n%s", output)
	}
	if strings.Contains(stdout.String(), "packages ->") {
		t.Fatalf("external build reported a managed manifest:\n%s", stdout.String())
	}
}

func TestBuildEmitsCompilerOwnedResultRuntimeWhenSourceDirIsProjectRoot(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.SourceDir = "."
	config.OutDir = "build"
	copyFiles := false
	config.CopyFiles = &copyFiles
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	source := `import { Result } from trb/std/result

def successful(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end
`
	if err := os.WriteFile(filepath.Join(root, "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	runtime, err := os.ReadFile(filepath.Join(root, "build", "trb", "std", "result", "index.rb"))
	if err != nil || !strings.Contains(string(runtime), "module Result") {
		t.Fatalf("compiler-owned Result runtime was not emitted: err=%v\n%s", err, runtime)
	}
	consumer, err := os.ReadFile(filepath.Join(root, "build", "main.rb"))
	if err != nil || !strings.Contains(string(consumer), `require_relative "./trb/std/result/index"`) {
		t.Fatalf("Result consumer did not require its runtime: err=%v\n%s", err, consumer)
	}
}

func TestBuildEmitsOfficialPackageOutsideProjectSourceTree(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.SourceDir = "."
	config.OutDir = "build"
	copyFiles := false
	config.CopyFiles = &copyFiles
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	source := `import { Body, Headers } from trb/http
import { Response } from trb/web

def response(): Response
	return Response.new(status: 204, headers: Headers.new(), body: Body.empty())
end
`
	if err := os.WriteFile(filepath.Join(root, "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	packageOutput, err := os.ReadFile(filepath.Join(root, "build", "trb", "web", "index.rb"))
	if err != nil || !strings.Contains(string(packageOutput), "class Response") {
		t.Fatalf("official package was not emitted: err=%v\n%s", err, packageOutput)
	}
	consumer, err := os.ReadFile(filepath.Join(root, "build", "main.rb"))
	if err != nil || !strings.Contains(string(consumer), `require_relative "./trb/web/index"`) {
		t.Fatalf("official package consumer did not require its runtime: err=%v\n%s", err, consumer)
	}
}

func TestBuildCompilesLocalRecordPackageIntoGoTargetTree(t *testing.T) {
	workspace := t.TempDir()
	appRoot := filepath.Join(workspace, "api")
	contractRoot := filepath.Join(workspace, "contracts")
	if err := os.MkdirAll(filepath.Join(appRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(contractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	config := project.New(appRoot, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/local-package"
	config.LocalPackages["acme/contracts"] = "../contracts"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contractRoot, "index.trb"), []byte("record Message\n  text: String\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appRoot, "src", "main.trb")
	main := "import { Message } from acme/contracts\n\ndef main()\n  message := Message.new(text: \"shared\")\n  puts(message.text)\n  return\nend\n"
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	contractOutput, err := os.ReadFile(filepath.Join(appRoot, "build", "acme", "contracts", "index.go"))
	if err != nil || !strings.Contains(string(contractOutput), "type Message struct") {
		t.Fatalf("local contract was not generated: err=%v\n%s", err, contractOutput)
	}
	mainOutput, err := os.ReadFile(filepath.Join(appRoot, "build", "main.go"))
	if err != nil || !strings.Contains(string(mainOutput), `contracts.Message{Text: "shared"}`) {
		t.Fatalf("application did not consume local record: err=%v\n%s", err, mainOutput)
	}
}

func TestRunOfficialWebPathNormalizationAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web path normalization run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web path normalization run")
				continue
			}
		}

		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-path-normalization-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}

		mainSource := `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def print_response(path: String)
	response := dispatch(Request.new(
		method: HttpMethod.get(),
		path: path,
		query_string: "",
		headers: Headers.new(),
		body: Body.empty(),
	))
	puts(response.status)
	puts(response.body.to_s())
	return
end

def main()
	print_response("/files/hello%20world")
	print_response("/files/%E3%81%82")
	print_response("/files/a+b")
	[
		"/files/%",
		"/files/%FF",
		"/files/%2F",
		"/files/%5c",
		"/files/\\",
		"/files/.",
		"/files/%2e",
		"/files/..",
		"/files/%2E%2e",
		"files/value",
	].each do |path|
		print_response(path)
	end
	print_response("/files//value")
	print_response("/files/value/")
	return
end
`
		routeSource := `import { Body, Headers } from trb/http
import { Context, Response } from trb/web

def get(context: Context): Response
	value := context.request.path + "|" + context.path_value("name")
	return Response.new(status: 200, headers: Headers.new(), body: Body.new(value.to_bytes()))
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes", "files"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "files", "[name].trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}

		var want strings.Builder
		want.WriteString("200\n/files/hello world|hello world\n")
		want.WriteString("200\n/files/あ|あ\n")
		want.WriteString("200\n/files/a+b|a+b\n")
		for range 10 {
			want.WriteString("400\n{\"error\":\"bad_request\"}\n")
		}
		for range 2 {
			want.WriteString("404\n{\"error\":\"not_found\"}\n")
		}
		if stdout.String() != want.String() {
			t.Fatalf("unexpected %s trb/web path output: want %q, got %q", mode, want.String(), stdout.String())
		}
	}
}
