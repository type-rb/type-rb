package compiler

import (
	goast "go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	gotypes "go/types"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func TestCompileRubyRailsKeepsNativeDSLAndLowersTypedCore(t *testing.T) {
	source := []byte(`import trb/platform/ruby/rails

# A normal Active Record model.
class Post < ApplicationRecord
  belongs_to :user
  validates :title, presence: true
  scope :published, -> { where.not(published_at: nil) }

  @summary_cache: String?

  def initialize(attributes: Hash)
    super(attributes)
    @summary_cache = nil
    return
  end

  def summary(limit: Integer = 80): String
    return body.to_s().truncate(limit)
  end

  def self.search(term: String): Any
    where("title LIKE ?", "%#{term}%").order(created_at: :desc)
  end
end
`)
	artifact, err := Compile("post.trb", source, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{
		"class Post < ApplicationRecord",
		"belongs_to :user",
		"validates :title, presence: true",
		"scope :published, -> { where.not(published_at: nil) }",
		"def summary(limit = 80)",
		"def self.search(term)",
		"# A normal Active Record model.",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in generated Ruby:\n%s", expected, output)
		}
	}
	for _, forbidden := range []string{"mode: ruby", "limit: Integer", "): String", "@summary_cache: String"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("generated Ruby retained %q:\n%s", forbidden, output)
		}
	}
	if len(artifact.AST.Statements) == 0 || len(artifact.IR.Statements) == 0 {
		t.Fatal("AST and IR must both be populated")
	}
}

func TestCompileGoProducesValidGoFromTypedIR(t *testing.T) {
	source := []byte(`import trb/std/io

class User
  @name: String

  def initialize(name: String)
    @name = name
    return
  end

  def name(): String
    return @name
  end
end

def main()
  user := User.new("Alice")
  io.puts(user.name())
  return
end
`)
	artifact, err := Compile("main.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "main.go", artifact.Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, artifact.Output)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", err, artifact.Output)
	}
	output := string(artifact.Output)
	for _, expected := range []string{"type User struct", "func NewUser(name string) *User", "func (self *User) Name() string", `user := NewUser("Alice")`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in generated Go:\n%s", expected, output)
		}
	}

	var classAST *ast.ClassStatement
	for _, statement := range artifact.AST.Statements {
		if candidate, ok := statement.(*ast.ClassStatement); ok {
			classAST = candidate
			break
		}
	}
	if classAST == nil || classAST.Name != "User" {
		t.Fatalf("expected class AST, got %#v", artifact.AST.Statements)
	}
	var classIR *ir.Class
	var mainIR *ir.Method
	for _, statement := range artifact.IR.Statements {
		if candidate, ok := statement.(*ir.Class); ok {
			classIR = candidate
		}
		if candidate, ok := statement.(*ir.Method); ok && candidate.Name == "main" {
			mainIR = candidate
		}
	}
	if classIR == nil || classIR.Name != "User" {
		t.Fatalf("expected class IR, got %#v", artifact.IR.Statements)
	}
	if mainIR == nil {
		t.Fatalf("expected main IR, got %#v", artifact.IR.Statements)
	}
	variable := mainIR.Body[0].(*ir.Variable)
	if variable.Type.Kind != types.Named || variable.Type.Name != "User" {
		t.Fatalf("constructor inference was not retained in IR: %#v", variable.Type)
	}
}

func TestCompileTypeScript(t *testing.T) {
	source := []byte(`import trb/std/io

class Greeter
  readonly @message: String := "Hello"

  def greet(name: String): String
    return @message + ", " + name
  end
end

def main()
  greeter := Greeter.new()
  io.puts(greeter.greet("TypeRB"))
  return
end
`)
	artifact, err := Compile("greeter.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{
		"export class Greeter {",
		`readonly __trb_message: string = "Hello";`,
		"greet(name: string): string {",
		"const greeter: Greeter = new Greeter();",
		`console.log(greeter.greet("TypeRB"));`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in generated TypeScript:\n%s", expected, output)
		}
	}
}

func TestPortableIterationAndRangesLowerAcrossBackends(t *testing.T) {
	source := []byte(`def total(): Integer
  mut result := 0
  [1, 2, 3].each { |value| result += value }
  [1].each do |unused|
    result += 1
  end
  (0...3).each.with_index do |value, index|
    result += value + index
  end
  [1, 2, 3, 4, 5].each_slice(2).with_index do |slice, index|
    result += slice[0] + index
  end
  [1, 2].each_slice(1) do |_|
    result += 1
  end
  return result
end

def sum(values: Iterable<Integer>): Integer
  mut result := 0
  values.each do |value|
    result += value
  end
  return result
end

def sum_range(): Integer
  return sum(0...3)
end
`)

	goArtifact, err := Compile("iteration.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "iteration.go", goArtifact.Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, goArtifact.Output)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", err, goArtifact.Output)
	}
	goOutput := string(goArtifact.Output)
	for _, expected := range []string{"for _, value := range []int{1, 2, 3}", "for index, value := range func() []int", "__trbItems1 := []int{1, 2, 3, 4, 5}", "slice := __trbItems1[", "func Sum(values []int) int", "return Sum(func() []int"} {
		if !strings.Contains(goOutput, expected) {
			t.Fatalf("missing %q in generated Go:\n%s", expected, goOutput)
		}
	}

	tsArtifact, err := Compile("iteration.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	tsOutput := string(tsArtifact.Output)
	for _, expected := range []string{"for (let value of [1, 2, 3])", ".entries()) {", "const __trbItems1 = [1, 2, 3, 4, 5];", "let slice = __trbItems1.slice(", "function sum(values: Array<number>): number", "return sum(((start: number"} {
		if !strings.Contains(tsOutput, expected) {
			t.Fatalf("missing %q in generated TypeScript:\n%s", expected, tsOutput)
		}
	}

	rubyArtifact, err := Compile("iteration.trb", source, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	rubyOutput := string(rubyArtifact.Output)
	for _, expected := range []string{"[1, 2, 3].each do |value|", "(0...3).each.with_index do |value, index|", "[1, 2, 3, 4, 5].each_slice(2).with_index do |slice, index|"} {
		if !strings.Contains(rubyOutput, expected) {
			t.Fatalf("missing %q in generated Ruby:\n%s", expected, rubyOutput)
		}
	}

	method := goArtifact.IR.Statements[0].(*ir.Method)
	iteration, ok := method.Body[1].(*ir.Iterate)
	if !ok || iteration.ItemType.Kind != types.Int {
		t.Fatalf("iterator item type was not retained in IR: %#v", method.Body[1])
	}
	rangeIteration := method.Body[3].(*ir.Iterate)
	if sourceRange, ok := rangeIteration.Source.(*ir.Range); !ok || !sourceRange.Exclusive {
		t.Fatalf("exclusive range was not retained in IR: %#v", rangeIteration.Source)
	}
}

func TestPortableIterationChecksSourceParametersAndSliceSize(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"def bad()\n  1.each do |value|\n    puts(value)\n  end\n  return\nend\n", "Integer is not iterable"},
		{"def bad()\n  [1].each do |value, index|\n    puts(value)\n  end\n  return\nend\n", "each block expects 1 parameter"},
		{"def bad()\n  [1].each_slice(0) do |slice|\n    puts(slice)\n  end\n  return\nend\n", "each_slice size must be greater than zero"},
		{"def bad()\n  (\"a\"..\"z\").each do |value|\n    puts(value)\n  end\n  return\nend\n", "range endpoints must be Integer"},
	}
	for _, test := range tests {
		if _, err := Compile("bad.trb", []byte(test.source), "go"); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected %q diagnostic, got %v", test.want, err)
		}
	}
}

func TestRubyNativeEnumerableCanUsePortableIterationBlock(t *testing.T) {
	source := []byte(`import trb/platform/ruby/native

def print_all(values: Any)
  values.each do |value|
    puts(value)
  end
  return
end
`)
	artifact, err := Compile("enumerable.trb", source, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	if output := string(artifact.Output); !strings.Contains(output, "values.each do |value|") {
		t.Fatalf("Ruby Enumerable block was not lowered:\n%s", output)
	}
}

func TestRecordIsAFirstClassASTAndIRNodeAcrossPortableBackends(t *testing.T) {
	source := []byte(`record Todo
  id: Integer @gorm("primaryKey")
  title: String
  completed: Boolean
end

def sample(): Todo
  return Todo.new(id: 1, title: "Ship TypeRB", completed: false)
end
`)
	goArtifact, err := Compile("todo.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := goArtifact.AST.Statements[0].(*ast.RecordStatement); !ok {
		t.Fatalf("expected record AST, got %#v", goArtifact.AST.Statements[0])
	}
	if _, ok := goArtifact.IR.Statements[0].(*ir.Record); !ok {
		t.Fatalf("expected record IR, got %#v", goArtifact.IR.Statements[0])
	}
	goOutput := string(goArtifact.Output)
	for _, expected := range []string{
		"type Todo struct {",
		"`json:\"id\" gorm:\"primaryKey\"`",
		`return Todo{Id: 1, Title: "Ship TypeRB", Completed: false}`,
	} {
		if !strings.Contains(goOutput, expected) {
			t.Fatalf("missing %q in generated Go:\n%s", expected, goOutput)
		}
	}

	tsSource := []byte(`record Todo
  id: Integer
  title: String
  completed: Boolean
end

def sample(): Todo
  return Todo.new(id: 1, title: "Ship TypeRB", completed: false)
end
`)
	tsArtifact, err := Compile("todo.trb", tsSource, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	tsOutput := string(tsArtifact.Output)
	for _, expected := range []string{
		"export interface Todo {",
		"id: number;",
		`return ({id: 1, title: "Ship TypeRB", completed: false} satisfies Todo);`,
	} {
		if !strings.Contains(tsOutput, expected) {
			t.Fatalf("missing %q in generated TypeScript:\n%s", expected, tsOutput)
		}
	}
}

func TestRecordConstructionChecksKeywordFields(t *testing.T) {
	source := []byte("record Todo\n  id: Integer\n  title: String\nend\n\ndef bad(): Todo\n  return Todo.new(id: 1)\nend\n")
	if _, err := Compile("todo.trb", source, "go"); err == nil || !strings.Contains(err.Error(), "missing record field title") {
		t.Fatalf("expected missing record field diagnostic, got %v", err)
	}
}

func TestPortableBackendRejectsRailsNativeNodes(t *testing.T) {
	_, err := Compile("bad.trb", []byte("class User\n  belongs_to :account\nend\n"), "go")
	if err == nil || !strings.Contains(err.Error(), "Ruby-native statement") {
		t.Fatalf("expected native-node diagnostic, got %v", err)
	}
}

func TestRubyCompatibilityForNamespacesKeywordsSettersAndSingletonClass(t *testing.T) {
	source := []byte(`import trb/platform/ruby/native

class Admin::Post < ActiveRecord::Base
  def configure(cache:: Boolean = false, required:, raw: nil): Boolean
    return cache
  end

  def title=(value)
    @title = value
    return
  end

  def visible? = true

  class << self
    def recent()
      return order(created_at: :desc)
    end
  end
end
`)
	artifact, err := Compile("admin_post.trb", source, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{
		"class Admin::Post < ActiveRecord::Base",
		"def configure(cache: false, required:, raw: nil)",
		"def title=(value)",
		"def visible? = true",
		"class << self",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q:\n%s", expected, output)
		}
	}
}

func TestRubyCompatibilityForConstantsNativeBlocksSplatAndQuotedSymbols(t *testing.T) {
	source := []byte(`import trb/platform/ruby/rails

DEFAULTS = { timeout: 5 }

class PostsController < ApplicationController
  rescue_from ActiveRecord::RecordNotFound, with: :not_found

  def index(page: 1, filters:, **options)
    respond_to do |format|
      format.html { render(:index) }
      format.json { render(**options) }
    end
    return
  end

  def predicate(): Any
    return public_send(:"published?")
  end
end
`)
	artifact, err := Compile("posts_controller.trb", source, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{
		"DEFAULTS = {:timeout => 5}",
		"rescue_from ActiveRecord::RecordNotFound, with: :not_found",
		"def index(page: 1, filters:, **options)",
		"respond_to do |format|",
		"render(**options)",
		`public_send(:"published?")`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q:\n%s", expected, output)
		}
	}
}

func TestInterfaceAndWhileReachGoBackend(t *testing.T) {
	source := []byte(`interface Counter
  value(): Integer
end

class Box implements Counter
  @value: Integer := 0

  def value(): Integer
    return @value
  end
end

def main()
  mut count := 0
  while count < 2
    count += 1
  end
  return
end
`)
	artifact, err := Compile("counter.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "counter.go", artifact.Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, artifact.Output)
	}
	if _, err := (&gotypes.Config{}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", err, artifact.Output)
	}
	output := string(artifact.Output)
	for _, expected := range []string{"type Counter interface", "Value() int", "for count < 2 {", "count += 1"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q:\n%s", expected, output)
		}
	}
}

func TestInterpolatedStringIsTypedAndLoweredPerTarget(t *testing.T) {
	goArtifact, err := CompileWithOptions("greet.trb", []byte(`import trb/std/io

def greet(name: String): String
  return "Hello, #{name}!"
end
def main()
  io.puts(greet("World"))
  return
end
`), Options{Mode: "go", Package: "greet"})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := string(goArtifact.Output)
	if !strings.Contains(goOutput, `import "fmt"`) || !strings.Contains(goOutput, `fmt.Sprintf("Hello, %v!", name)`) {
		t.Fatalf("unexpected Go interpolation:\n%s", goOutput)
	}
	if !strings.Contains(goOutput, `fmt.Println(Greet("World"))`) {
		t.Fatalf("top-level call was not resolved:\n%s", goOutput)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "greet.go", goArtifact.Output, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("greet", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", err, goOutput)
	}

	tsArtifact, err := Compile("greet.trb", []byte(`def greet(name: String): String
  return "Hello, #{name}!"
end
`), "typescript")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tsArtifact.Output), "return `Hello, ${name}!`;") {
		t.Fatalf("unexpected TypeScript interpolation:\n%s", tsArtifact.Output)
	}
}

func TestVoidReturnTypeMustBeOmitted(t *testing.T) {
	valid := []byte("def save()\n  return\nend\n")
	if _, err := Compile("valid.trb", valid, "go"); err != nil {
		t.Fatalf("omitted no-value return annotation should compile: %v", err)
	}

	tests := []string{
		"def save(): Void\n  return\nend\n",
		"interface Saver\n  save(): Void\nend\n",
	}
	for _, source := range tests {
		if _, err := Compile("invalid.trb", []byte(source), "go"); err == nil || !strings.Contains(err.Error(), "Void return type must be omitted") {
			t.Fatalf("expected explicit Void return type diagnostic, got %v", err)
		}
	}
}

func TestImmutableBindingsRequireMutForReassignmentAndArrayUpdates(t *testing.T) {
	invalid := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "reassignment",
			source: "def example()\n  value := 1\n  value = 2\n  return\nend\n",
			want:   "value is immutable; declare it with mut to use assignment",
		},
		{
			name:   "portable push",
			source: "import trb/std/arrays\n\ndef example()\n  values := [1]\n  arrays.push(values, 2)\n  return\nend\n",
			want:   "values is immutable; declare it with mut to use push()",
		},
		{
			name:   "member push",
			source: "def example()\n  values := [1]\n  values.push(2)\n  return\nend\n",
			want:   "values is immutable; declare it with mut to use push()",
		},
		{
			name:   "readonly alias",
			source: "def example()\n  values := [1]\n  mut alias := values\n  return\nend\n",
			want:   "cannot initialize mutable alias from an immutable value",
		},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Compile("immutable.trb", []byte(test.source), "go"); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q diagnostic, got %v", test.want, err)
			}
		})
	}

	source := []byte(`def build(): Array<Integer>
  mut values := [1]
  values.push(2)
  mut index := 0
  index += 1
  return values
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("mutable.trb", source, mode)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		output := string(artifact.Output)
		if mode == "go" && !strings.Contains(output, "values = append(values, 2)") {
			t.Fatalf("Go member push was not lowered portably:\n%s", output)
		}
	}
}

func TestConstantsAreRuntimeInitializedImmutableScopedBindings(t *testing.T) {
	invalid := []struct {
		source string
		want   string
	}{
		{"def bad()\n  INNER := 1\n  return\nend\n", "constant INNER may only be declared at top level or directly inside a module or class"},
		{"mut MAX_ITEMS := 1\n", "constant MAX_ITEMS cannot be declared with mut"},
		{"MAX_ITEMS := 1\nMAX_ITEMS = 2\n", "MAX_ITEMS is immutable"},
		{"DEFAULT_TAGS := [\"work\"]\nDEFAULT_TAGS.push(\"home\")\n", "DEFAULT_TAGS is immutable"},
	}
	for _, test := range invalid {
		if _, err := Compile("constant.trb", []byte(test.source), "go"); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected %q diagnostic, got %v", test.want, err)
		}
	}

	source := []byte(`import trb/std/strings

APP_NAME := strings.uppercase("typerb")

module Limits
  MAX_ITEMS := 10

  def self.current(): Integer
    return MAX_ITEMS
  end
end

class Config
  DEFAULT_NAME := "TypeRB"

  def name(): String
    return DEFAULT_NAME
  end
end
`)

	goArtifact, err := Compile("constants.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"var AppName string", "var LimitsMaxItems int", "return LimitsMaxItems", "var ConfigDefaultName string", "return ConfigDefaultName"} {
		if !strings.Contains(string(goArtifact.Output), expected) {
			t.Fatalf("generated Go is missing %q:\n%s", expected, goArtifact.Output)
		}
	}

	tsArtifact, err := Compile("constants.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"export const APP_NAME: string", "export const MAX_ITEMS: number", "return Limits.MAX_ITEMS", "static readonly DEFAULT_NAME: string", "return Config.DEFAULT_NAME"} {
		if !strings.Contains(string(tsArtifact.Output), expected) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", expected, tsArtifact.Output)
		}
	}

	rubyArtifact, err := Compile("constants.trb", source, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"APP_NAME =", "MAX_ITEMS = 10", "DEFAULT_NAME = \"TypeRB\""} {
		if !strings.Contains(string(rubyArtifact.Output), expected) {
			t.Fatalf("generated Ruby is missing %q:\n%s", expected, rubyArtifact.Output)
		}
	}
}
