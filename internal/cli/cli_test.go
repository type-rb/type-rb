package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestVersionCommandUsesBuildVersion(t *testing.T) {
	previous := Version
	Version = "9.8.7-test"
	t.Cleanup(func() { Version = previous })

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"version"}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if stdout.String() != "trb 9.8.7-test\n" {
		t.Fatalf("unexpected version output %q", stdout.String())
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

	input := "enum Token; Text(value: String); EOF; end\n" +
		"def render(token: Token): String; case token; when Token::Text(value); return value; when Token::EOF; return \"eof\"; end; end\n" +
		"render(Token::Text(\"Ada\"))\n" +
		"Token::Text(\"Ada\")\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "\"Ada\" : String\nToken::Text(value: \"Ada\") : Token\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
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
		"def unwrap(value: Result<Integer, String>): Integer; case value; when Result::Ok(number); return number; when Result::Err(error); return 0; end; end\n" +
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

func TestReplRequiresProjectConfiguration(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(":quit\n"), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "repl requires a trbconfig.jsonc") {
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
	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
	t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))

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
	main := "import { Message } from acme/contracts\n\ndef main()\n  message := Message.new(text: \"shared\")\n  return\nend\n"
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
