package compiler

import (
	goast "go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	gotypes "go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

const portableMain = `import trb/std/io
import trb/std/strings

def main()
  message := strings.uppercase("Hello, TypeRB")
  puts(1 + 2)
  io.puts(message)
  return
end
`

func TestPortableStandardLibraryLowersAcrossBackends(t *testing.T) {
	goArtifact, err := CompileWithOptions("main.trb", []byte(portableMain), Options{Mode: "go", Package: "main"})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := string(goArtifact.Output)
	for _, want := range []string{`import "fmt"`, `import "strings"`, `strings.ToUpper("Hello, TypeRB")`, `fmt.Println(1 + 2)`, `fmt.Println(message)`} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go does not contain %q:\n%s", want, goOutput)
		}
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "main.go", goArtifact.Output, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", err, goOutput)
	}

	tsArtifact, err := CompileWithOptions("main.trb", []byte(portableMain), Options{Mode: "typescript"})
	if err != nil {
		t.Fatal(err)
	}
	tsOutput := string(tsArtifact.Output)
	for _, want := range []string{`"Hello, TypeRB".toUpperCase()`, `console.log(1 + 2);`, `console.log(message);`, `main();`} {
		if !strings.Contains(tsOutput, want) {
			t.Fatalf("generated TypeScript does not contain %q:\n%s", want, tsOutput)
		}
	}

	rubyArtifact, err := CompileWithOptions("main.trb", []byte(portableMain), Options{Mode: "ruby", RubyLoader: "require_relative"})
	if err != nil {
		t.Fatal(err)
	}
	rubyOutput := string(rubyArtifact.Output)
	for _, want := range []string{`"Hello, TypeRB".upcase`, `$stdout.puts(1 + 2)`, `$stdout.puts(message)`, `main()`} {
		if !strings.Contains(rubyOutput, want) {
			t.Fatalf("generated Ruby does not contain %q:\n%s", want, rubyOutput)
		}
	}

	var resolved *ir.Reference
	for _, statement := range goArtifact.IR.Statements {
		method, ok := statement.(*ir.Method)
		if !ok || method.Name != "main" {
			continue
		}
		variable := method.Body[0].(*ir.Variable)
		call := variable.Value.(*ir.Call)
		resolved = call.Callee.(*ir.Member).Reference
	}
	if resolved == nil || resolved.Package != "trb/std/strings" || resolved.Intrinsic != "trb.std.strings.uppercase" {
		t.Fatalf("standard call was not retained as a resolved IR reference: %#v", resolved)
	}
}

func TestPutsPreludeLowersWithoutImportAcrossBackends(t *testing.T) {
	source := []byte("def main()\n  puts(1 + 2)\n  return\nend\n")
	tests := []struct {
		mode string
		want string
	}{
		{mode: "go", want: "fmt.Println(1 + 2)"},
		{mode: "typescript", want: "console.log(1 + 2);"},
		{mode: "ruby", want: "$stdout.puts(1 + 2)"},
	}
	for _, test := range tests {
		artifact, err := CompileWithOptions("main.trb", source, Options{Mode: test.mode, Package: "main", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s: %v", test.mode, err)
		}
		if output := string(artifact.Output); !strings.Contains(output, test.want) {
			t.Fatalf("%s output does not contain %q:\n%s", test.mode, test.want, output)
		}
	}
}

func TestPortableStringLengthUsesUnicodeCodePoints(t *testing.T) {
	source := []byte("import trb/std/strings\n\ndef count(): Integer\n  return strings.length(\"😀a\")\nend\n")
	tests := []struct {
		mode string
		want string
	}{
		{mode: "go", want: `utf8.RuneCountInString("😀a")`},
		{mode: "typescript", want: `Array.from("😀a").length`},
		{mode: "ruby", want: `"😀a".each_codepoint.count`},
	}
	for _, test := range tests {
		artifact, err := Compile("length.trb", source, test.mode)
		if err != nil {
			t.Fatalf("%s: %v", test.mode, err)
		}
		if !strings.Contains(string(artifact.Output), test.want) {
			t.Fatalf("%s output does not contain %q:\n%s", test.mode, test.want, artifact.Output)
		}
	}
}

func TestPlatformPackageIsModeChecked(t *testing.T) {
	source := []byte("import trb/platform/go/context\n\ndef main()\n  ctx := context.background()\n  return\nend\n")
	if _, err := Compile("main.trb", source, "typescript"); err == nil || !strings.Contains(err.Error(), "does not support mode typescript") {
		t.Fatalf("expected platform mode diagnostic, got %v", err)
	}
	artifact, err := Compile("main.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	if output := string(artifact.Output); !strings.Contains(output, `import "context"`) || !strings.Contains(output, `context.Background()`) {
		t.Fatalf("Go platform binding was not lowered:\n%s", output)
	}
}

func TestStandardPackageSignaturesAndReservedPathsAreChecked(t *testing.T) {
	anyType := []byte("import trb/std/io\n\ndef main()\n  io.puts(1)\n  io.puts([1, 2])\n  return\nend\n")
	if _, err := Compile("main.trb", anyType, "go"); err != nil {
		t.Fatalf("puts should accept any TypeRB value: %v", err)
	}
	wrongArity := []byte("import trb/std/io\n\ndef main()\n  io.puts()\n  return\nend\n")
	if _, err := Compile("main.trb", wrongArity, "go"); err == nil || !strings.Contains(err.Error(), "expects 1..1 arguments") {
		t.Fatalf("expected standard signature diagnostic, got %v", err)
	}
	if _, err := Compile("main.trb", []byte("import trb/std/missing\n"), "ruby"); err == nil || !strings.Contains(err.Error(), "unknown TypeRB package") {
		t.Fatalf("expected unknown standard package diagnostic, got %v", err)
	}
	if _, err := Compile("main.trb", []byte("package main\n"), "go"); err == nil || !strings.Contains(err.Error(), "package is derived from trbconfig.jsonc") {
		t.Fatalf("expected source package diagnostic, got %v", err)
	}
}

func TestRubyNativeSyntaxRequiresExplicitPlatformImport(t *testing.T) {
	source := []byte("class Post < ApplicationRecord\n  belongs_to :author\nend\n")
	if _, err := Compile("post.trb", source, "ruby"); err == nil || !strings.Contains(err.Error(), "requires import trb/platform/ruby") {
		t.Fatalf("expected explicit Ruby platform import diagnostic, got %v", err)
	}
	withImport := append([]byte("import trb/platform/ruby/rails\n\n"), source...)
	if _, err := Compile("post.trb", withImport, "ruby"); err != nil {
		t.Fatal(err)
	}
}

func TestProjectImportResolvesExportsAndBackendPaths(t *testing.T) {
	root := t.TempDir()
	modelPath := filepath.Join(root, "app", "models", "user.trb")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatal(err)
	}
	model := []byte("class User\n  @name: String\n  def initialize(name: String)\n    @name = name\n    return\n  end\nend\n")
	if err := os.WriteFile(modelPath, model, 0o644); err != nil {
		t.Fatal(err)
	}
	main := []byte("import app/models/user\n\ndef build_user(): User\n  return User.new(\"Alice\")\nend\n")

	goArtifact, err := CompileWithOptions("app/services/main.trb", main, Options{
		Mode:       "go",
		Package:    "services",
		ModulePath: "app/services/main",
		GoModule:   "example.com/acme/app",
		SourceRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := string(goArtifact.Output)
	for _, want := range []string{`import "example.com/acme/app/app/models"`, `return models.NewUser("Alice")`} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go does not contain %q:\n%s", want, goOutput)
		}
	}

	tsArtifact, err := CompileWithOptions("app/services/main.trb", main, Options{
		Mode:       "typescript",
		ModulePath: "app/services/main",
		SourceRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if output := string(tsArtifact.Output); !strings.Contains(output, `import { User } from "../models/user.ts";`) {
		t.Fatalf("unexpected TypeScript project import:\n%s", output)
	}

	rubyArtifact, err := CompileWithOptions("app/services/main.trb", main, Options{
		Mode:       "ruby",
		ModulePath: "app/services/main",
		RubyLoader: "zeitwerk",
		SourceRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if output := string(rubyArtifact.Output); strings.Contains(output, "require") {
		t.Fatalf("Zeitwerk project imports must be compile-time only:\n%s", output)
	}
}

func TestProjectCompilerChecksImportedMembersAndSignatures(t *testing.T) {
	model := SourceUnit{
		Filename:   "/project/models/user.trb",
		ModulePath: "models/user",
		Package:    "models",
		Source: []byte(`class User
  @name: String

  def initialize(name: String)
    @name = name
    return
  end

  def rename(name: String): String
    @name = name
    return @name
  end
end
`),
	}
	validMain := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import models/user

def build_user(): String
  user := User.new("Alice")
  return user.rename("Bob")
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{validMain, model}, Options{Mode: "go", GoModule: "example.com/project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected two project artifacts, got %d", len(artifacts))
	}

	wrongArgument := validMain
	wrongArgument.Source = []byte("import models/user\n\ndef build_user(): String\n  user := User.new(\"Alice\")\n  return user.rename(true)\nend\n")
	if _, err := CompileProject([]SourceUnit{wrongArgument, model}, Options{Mode: "go", GoModule: "example.com/project"}); err == nil || !strings.Contains(err.Error(), "expected String") {
		t.Fatalf("expected imported method argument diagnostic, got %v", err)
	}

	missingMember := validMain
	missingMember.Source = []byte("import models/user\n\ndef build_user(): String\n  user := User.new(\"Alice\")\n  return user.missing()\nend\n")
	if _, err := CompileProject([]SourceUnit{missingMember, model}, Options{Mode: "go", GoModule: "example.com/project"}); err == nil || !strings.Contains(err.Error(), "has no member missing") {
		t.Fatalf("expected missing imported member diagnostic, got %v", err)
	}
}

func TestProjectCompilerExportsTopModuleAndClassConstants(t *testing.T) {
	constants := SourceUnit{
		Filename:   "/project/config/constants.trb",
		ModulePath: "config/constants",
		Package:    "config",
		Source: []byte(`APP_NAME := "TypeRB"

module Limits
  MAX_ITEMS := 10
end

class Config
  DEFAULT_LIMIT := 5
end
`),
	}
	main := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { APP_NAME, Config, Limits } from config/constants

def app_name(): String
  return APP_NAME
end

def max_items(): Integer
  return Limits::MAX_ITEMS
end

def default_limit(): Integer
  return Config::DEFAULT_LIMIT
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{constants, main}, Options{Mode: "go", GoModule: "example.com/project"})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		output := string(artifact.Output)
		if artifact.Filename == constants.Filename {
			for _, expected := range []string{"var AppName string", "var LimitsMaxItems int", "var ConfigDefaultLimit int"} {
				if !strings.Contains(output, expected) {
					t.Fatalf("constant module is missing %q:\n%s", expected, output)
				}
			}
		}
		if artifact.Filename == main.Filename {
			for _, expected := range []string{"config.AppName", "config.LimitsMaxItems", "config.ConfigDefaultLimit"} {
				if !strings.Contains(output, expected) {
					t.Fatalf("constant consumer is missing %q:\n%s", expected, output)
				}
			}
		}
	}
}

func TestProjectCompilerExportsEnumsForExhaustiveCase(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/state.trb",
		ModulePath: "contracts/state",
		Package:    "contracts",
		Source: []byte(`enum State
	Open
	Closed
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import contracts/state

def label(value: State): String
	case value
	when State::Open
		return "open"
	when State::Closed
		return "closed"
	end
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{consumer, contract}, Options{Mode: "go", GoModule: "example.com/project"})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		if artifact.Filename == consumer.Filename {
			output := string(artifact.Output)
			for _, want := range []string{"contracts.State", "contracts.StateOpen", "contracts.StateClosed"} {
				if !strings.Contains(output, want) {
					t.Fatalf("enum consumer is missing %q:\n%s", want, output)
				}
			}
		}
	}

	incomplete := consumer
	incomplete.Source = []byte("import contracts/state\n\ndef label(value: State): String\n\tcase value\n\twhen State::Open\n\t\treturn \"open\"\n\tend\nend\n")
	if _, err := CompileProject([]SourceUnit{incomplete, contract}, Options{Mode: "typescript"}); err == nil || !strings.Contains(err.Error(), "missing Closed") {
		t.Fatalf("expected imported enum exhaustiveness diagnostic, got %v", err)
	}

	aliased := consumer
	aliased.Source = []byte("import contracts/state as states\n\ndef label(): String\n\tvalue := states::State::Open\n\tcase value\n\twhen states::State::Open\n\t\treturn \"open\"\n\twhen states::State::Closed\n\t\treturn \"closed\"\n\tend\nend\n")
	aliasedArtifacts, err := CompileProject([]SourceUnit{aliased, contract}, Options{Mode: "go", GoModule: "example.com/project"})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range aliasedArtifacts {
		if artifact.Filename == aliased.Filename && !strings.Contains(string(artifact.Output), "states.StateOpen") {
			t.Fatalf("aliased enum member was not qualified correctly:\n%s", artifact.Output)
		}
	}
}

func TestProjectCompilerRejectsImportCycles(t *testing.T) {
	a := SourceUnit{Filename: "/project/a.trb", ModulePath: "a", Source: []byte("import b\n\nclass A\nend\n")}
	b := SourceUnit{Filename: "/project/b.trb", ModulePath: "b", Source: []byte("import a\n\nclass B\nend\n")}
	if _, err := CompileProject([]SourceUnit{a, b}, Options{Mode: "typescript"}); err == nil || !strings.Contains(err.Error(), "import cycle: a -> b -> a") {
		t.Fatalf("expected deterministic import cycle diagnostic, got %v", err)
	}
}

func TestProjectCompilerRejectsDuplicateMainDefinitions(t *testing.T) {
	a := SourceUnit{Filename: "/project/a.trb", ModulePath: "a", Source: []byte("def main()\n  return\nend\n")}
	b := SourceUnit{Filename: "/project/b.trb", ModulePath: "b", Source: []byte("def main()\n  return\nend\n")}
	if _, err := CompileProject([]SourceUnit{a, b}, Options{Mode: "typescript"}); err == nil || !strings.Contains(err.Error(), "main is already declared") {
		t.Fatalf("expected duplicate main diagnostic, got %v", err)
	}
}

func TestProjectCompilerChecksImportedInterfaces(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/named.trb",
		ModulePath: "contracts/named",
		Source:     []byte("interface Named\n  name(): String\nend\n"),
	}
	valid := SourceUnit{
		Filename:   "/project/models/user.trb",
		ModulePath: "models/user",
		Source:     []byte("import contracts/named\n\nclass User implements Named\n  def name(): String\n    return \"Alice\"\n  end\nend\n"),
	}
	if _, err := CompileProject([]SourceUnit{contract, valid}, Options{Mode: "typescript"}); err != nil {
		t.Fatal(err)
	}

	invalid := valid
	invalid.Source = []byte("import contracts/named\n\nclass User implements Named\n  def name(): Integer\n    return 1\n  end\nend\n")
	if _, err := CompileProject([]SourceUnit{contract, invalid}, Options{Mode: "typescript"}); err == nil || !strings.Contains(err.Error(), "does not match interface Named") {
		t.Fatalf("expected imported interface signature diagnostic, got %v", err)
	}
}

func TestProjectCatalogLinksImportedInheritance(t *testing.T) {
	base := SourceUnit{
		Filename:   "/project/models/base.trb",
		ModulePath: "models/base",
		Source:     []byte("class Base\n  def label(value: String): String\n    return value\n  end\nend\n"),
	}
	child := SourceUnit{
		Filename:   "/project/models/child.trb",
		ModulePath: "models/child",
		Source:     []byte("import models/base\n\nclass Child < Base\nend\n"),
	}
	main := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Source:     []byte("import models/child\n\ndef label(): String\n  child := Child.new()\n  return child.label(true)\nend\n"),
	}
	if _, err := CompileProject([]SourceUnit{base, child, main}, Options{Mode: "typescript"}); err == nil || !strings.Contains(err.Error(), "expected String") {
		t.Fatalf("expected inherited imported member diagnostic, got %v", err)
	}
}

func TestProjectCompilerPreservesImportedClassMemberKindsAndReadonlyFields(t *testing.T) {
	model := SourceUnit{
		Filename:   "/project/models/probe.trb",
		ModulePath: "models/probe",
		Source: []byte(`class Probe
	readonly @id: Integer

	def initialize(id: Integer)
		@id = id
		return
	end

	def value(): Integer
		return @id
	end

	def self.kind(): String
		return "probe"
	end
end
`),
	}
	tests := []struct {
		name string
		body string
		want string
	}{
		{"class method through instance", "mut probe := Probe.new(1)\n\tprobe.kind()", "class Probe has no instance member kind; kind is a class member"},
		{"instance method through class", "Probe.value()", "class Probe has no class member value; value is an instance member"},
		{"readonly assignment", "mut probe := Probe.new(1)\n\tprobe.id = 2", "field id is readonly"},
		{"constructor through instance", "probe := Probe.new(1)\n\tprobe.new(2)", "class Probe has no instance member new; new is a class member"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			consumer := SourceUnit{
				Filename:   "/project/main.trb",
				ModulePath: "main",
				Source:     []byte("import models/probe\n\ndef invalid()\n\t" + test.body + "\n\treturn\nend\n"),
			}
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := CompileProject([]SourceUnit{model, consumer}, Options{Mode: mode, GoModule: "example.com/project"}); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestProjectCatalogRejectsDuplicateExportedTypes(t *testing.T) {
	a := SourceUnit{Filename: "/project/a.trb", ModulePath: "a", Source: []byte("class User\nend\n")}
	b := SourceUnit{Filename: "/project/b.trb", ModulePath: "b", Source: []byte("class User\nend\n")}
	if _, err := CompileProject([]SourceUnit{a, b}, Options{Mode: "typescript"}); err == nil || !strings.Contains(err.Error(), "exported type User is already declared") {
		t.Fatalf("expected duplicate exported type diagnostic, got %v", err)
	}
}
