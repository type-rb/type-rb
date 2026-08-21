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
	"github.com/type-rb/type-rb/internal/sourcemap"
	"github.com/type-rb/type-rb/internal/types"
)

func TestCompileCarriesTypeScriptRuntimeToTypedIR(t *testing.T) {
	artifact, err := CompileWithOptions("main.trb", []byte("def main()\n\treturn\nend\n"), Options{
		Mode:              "typescript",
		TypeScriptRuntime: "bun",
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.IR.TypeScriptRuntime != "bun" {
		t.Fatalf("TypeScript runtime was not carried to typed IR: %#v", artifact.IR)
	}
}

func TestCompileMapsGeneratedStatementsBackToTypeRBSource(t *testing.T) {
	source := []byte(`def main()
	message := "mapped"
	puts(message)
	return
end
`)
	targets := map[string]string{
		"go":         "fmt.Println(message)",
		"ruby":       "puts(message)",
		"typescript": "console.log(message);",
	}
	for mode, target := range targets {
		artifact, err := Compile("src/main.trb", source, mode)
		if err != nil {
			t.Fatalf("%s compilation failed: %v", mode, err)
		}
		output := string(artifact.Output)
		if strings.Contains(output, "__trb_source_") {
			t.Fatalf("%s source marker leaked into generated output:\n%s", mode, output)
		}
		offset := strings.Index(output, target)
		if offset < 0 {
			t.Fatalf("%s generated output is missing %q:\n%s", mode, target, output)
		}
		location, found := artifact.SourceMap.SourceAt(sourcemap.PositionAt(output, offset))
		if !found || location.Path != "src/main.trb" || location.Span.Start.Line != 3 {
			t.Fatalf("%s generated statement mapped to %#v, found=%t", mode, location, found)
		}
		if artifact.SourceMap.Version != sourcemap.Version {
			t.Fatalf("%s source map version=%d, want %d", mode, artifact.SourceMap.Version, sourcemap.Version)
		}
	}
}

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

func TestClassAndInstanceMembersAreCheckedAcrossModes(t *testing.T) {
	valid := []byte(`class Probe
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

def inspect_probe(): String
	probe := Probe.new(1)
	probe.value()
	return Probe.kind()
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("class_members.trb", valid, mode); err != nil {
			t.Fatalf("%s rejected valid class/instance access: %v", mode, err)
		}
	}

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "class method through instance",
			body: "mut probe := Probe.new(1)\n\tprobe.kind()",
			want: "class Probe has no instance member kind; kind is a class member",
		},
		{
			name: "instance method through class",
			body: "Probe.value()",
			want: "class Probe has no class member value; value is an instance member",
		},
		{
			name: "readonly field assignment",
			body: "mut probe := Probe.new(1)\n\tprobe.id = 2",
			want: "field id is readonly",
		},
		{
			name: "constructor through instance",
			body: "probe := Probe.new(1)\n\tprobe.new(2)",
			want: "class Probe has no instance member new; new is a class member",
		},
	}
	declaration := `class Probe
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
`
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(declaration + "\ndef invalid()\n\t" + test.body + "\n\treturn\nend\n")
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_class_member.trb", source, mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestGoClassFieldAccessUsesClassStorage(t *testing.T) {
	source := []byte(`class Probe
	@count: Integer
	readonly @label: String

	def initialize(label: String)
		@count = 1
		@label = label
		return
	end

	def increment(): Integer
		@count += 1
		return @count
	end
end

def main()
	mut probe := Probe.new("items")
	probe.count += 1
	puts(probe.label)
	puts(probe.count)
	puts(probe.increment())
	return
end
`)
	artifact, err := Compile("class_fields.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{"probe.TrbFieldCount += 1", "fmt.Println(probe.TrbFieldLabel)", "fmt.Println(probe.TrbFieldCount)", "fmt.Println(probe.Increment())"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated Go is missing %q:\n%s", expected, output)
		}
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "class_fields.go", artifact.Output, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated class field Go did not type-check: %v\n%s", err, output)
	}

	ruby, err := Compile("class_fields.trb", source, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	rubyOutput := string(ruby.Output)
	for _, expected := range []string{
		"def __trb_field_label; @label; end",
		"def __trb_field_count=(value); @count = value; end",
		"probe.__trb_field_count += 1",
		"$stdout.puts(probe.__trb_field_label)",
		"$stdout.puts(probe.increment())",
	} {
		if !strings.Contains(rubyOutput, expected) {
			t.Fatalf("generated Ruby is missing %q:\n%s", expected, rubyOutput)
		}
	}
	if strings.Contains(rubyOutput, "def __trb_field_label=(value)") {
		t.Fatalf("generated Ruby exposed a writer for a readonly field:\n%s", rubyOutput)
	}

	typeScript, err := Compile("class_fields.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	typeScriptOutput := string(typeScript.Output)
	for _, expected := range []string{
		"probe.__trb_count += 1;",
		"console.log(probe.__trb_label);",
		"console.log(probe.__trb_count);",
		"console.log(probe.increment());",
	} {
		if !strings.Contains(typeScriptOutput, expected) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", expected, typeScriptOutput)
		}
	}
}

func TestPortableIterationAndRangesLowerAcrossBackends(t *testing.T) {
	source := []byte(`def total(): Integer
  mut result := 0
  [1, 2, 3].each { |value| result += value }
  [1].each do |_|
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
	for _, expected := range []string{"for _, value := range []int{1, 2, 3}", "for index, value := range func(bounds [3]int) []int", "__trbItems1 := []int{1, 2, 3, 4, 5}", "slice := __trbItems1[", "func Sum(values []int) int", "return Sum(func(bounds [3]int) []int"} {
		if !strings.Contains(goOutput, expected) {
			t.Fatalf("missing %q in generated Go:\n%s", expected, goOutput)
		}
	}

	tsArtifact, err := Compile("iteration.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	tsOutput := string(tsArtifact.Output)
	for _, expected := range []string{"for (let value of [1, 2, 3])", ".entries()) {", "const __trbItems1 = [1, 2, 3, 4, 5];", "let slice = __trbItems1.slice(", "function sum(values: Array<number>): number", "return sum(((bounds: [number, number, boolean])"} {
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
	if !ok || len(iteration.Bindings) != 1 || iteration.Bindings[0].Type.Kind != types.Int {
		t.Fatalf("iterator item type was not retained in IR: %#v", method.Body[1])
	}
	rangeIteration := method.Body[3].(*ir.Iterate)
	if sourceRange, ok := rangeIteration.Source.(*ir.Range); !ok || !sourceRange.Exclusive {
		t.Fatalf("exclusive range was not retained in IR: %#v", rangeIteration.Source)
	}
}

func TestPortableHashIterationLowersAcrossBackends(t *testing.T) {
	source := []byte(`def hash_total(labels: Hash<Integer, String>): Integer
	mut total := 0
	labels.each do |key, value|
		if key == 2
			next
		end
		total += key + value.size()
	end
	labels.each { |_, value| total += value.size() }
	return total
end
`)

	goArtifact, err := Compile("hash_iteration.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "hash_iteration.go", goArtifact.Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, goArtifact.Output)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", err, goArtifact.Output)
	}
	goOutput := string(goArtifact.Output)
	for _, expected := range []string{"for key, value := range maps.Clone(labels)", "for _, value := range maps.Clone(labels)"} {
		if !strings.Contains(goOutput, expected) {
			t.Fatalf("missing %q in generated Go:\n%s", expected, goOutput)
		}
	}

	rubyArtifact, err := Compile("hash_iteration.trb", source, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	rubyOutput := string(rubyArtifact.Output)
	for _, expected := range []string{"labels.to_a.each do |key, value|", "labels.to_a.each do |_, value|"} {
		if !strings.Contains(rubyOutput, expected) {
			t.Fatalf("missing %q in generated Ruby:\n%s", expected, rubyOutput)
		}
	}

	typescriptArtifact, err := Compile("hash_iteration.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	typescriptOutput := string(typescriptArtifact.Output)
	for _, expected := range []string{"Object.entries(labels)", "let key = Number(__trbKey1);", "void value;"} {
		if !strings.Contains(typescriptOutput, expected) {
			t.Fatalf("missing %q in generated TypeScript:\n%s", expected, typescriptOutput)
		}
	}

	method := goArtifact.IR.Statements[0].(*ir.Method)
	iteration, ok := method.Body[1].(*ir.Iterate)
	if !ok || len(iteration.Bindings) != 2 || iteration.Bindings[0].Type.Kind != types.Int || iteration.Bindings[1].Type.Kind != types.String {
		t.Fatalf("Hash key/value binding types were not retained in IR: %#v", method.Body[1])
	}
}

func TestPortableHashIterationRejectsUnspecifiedForms(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "missing value binding",
			source: "def bad(labels: Hash<Integer, String>)\n\tlabels.each do |key|\n\t\tputs(key)\n\tend\n\treturn\nend\n",
			want:   "each block expects 2 parameter(s), got 1",
		},
		{
			name:   "indexed Hash iteration",
			source: "def bad(labels: Hash<Integer, String>)\n\tlabels.each.with_index do |key, value|\n\t\tputs(key)\n\t\tputs(value)\n\tend\n\treturn\nend\n",
			want:   "Hash#each.with_index is not supported in v0.1",
		},
		{
			name:   "Hash transform",
			source: "def bad(labels: Hash<Integer, String>): Array<String>\n\treturn labels.map do |value|\n\t\tvalue\n\tend\nend\n",
			want:   "Hash iteration supports only each in v0.1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_hash_iteration.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
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

func TestPortableBackendsReportNativeFallbackAsUnsupportedSyntax(t *testing.T) {
	for _, mode := range []string{"go", "typescript"} {
		_, err := Compile("bad.trb", []byte("class User\n  belongs_to :account\nend\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "unsupported statement syntax in portable TypeRB") {
			t.Fatalf("%s: expected portable statement-syntax diagnostic, got %v", mode, err)
		}

		_, err = Compile("bad.trb", []byte("value := `native command`\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "unsupported expression syntax in portable TypeRB") {
			t.Fatalf("%s: expected portable expression-syntax diagnostic, got %v", mode, err)
		}
	}
}

func TestRubyNativeFallbackStillRequiresExplicitImport(t *testing.T) {
	_, err := Compile("bad.trb", []byte("class User\n  belongs_to :account\nend\n"), "ruby")
	if err == nil || !strings.Contains(err.Error(), "Ruby-native syntax requires import") {
		t.Fatalf("expected explicit Ruby-native import diagnostic, got %v", err)
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

func TestImplementedClassesAreAssignableToInterfaceValues(t *testing.T) {
	source := []byte(`interface Named
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

def display(value: Named): String
	return value.name()
end

def build(): Named
	return Person.new("Ada")
end

def people(): Array<Named>
	return [Person.new("Ada"), Person.new("Grace")]
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("interfaces.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected interface values: %v", mode, err)
		}
		if len(artifact.Output) == 0 {
			t.Fatalf("%s emitted no output", mode)
		}
		if mode == "typescript" && strings.Contains(string(artifact.Output), "this.name.bind(this)") {
			t.Fatalf("lexical parameter name was lowered as a method:\n%s", artifact.Output)
		}
	}
}

func TestGenericInterfacesAreSpecializedAcrossBackends(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result

enum LoadError
	Unavailable
end

interface Store<T>
	get(): T
	put(value: T): T
end

interface Loader<T, E>
	load(): Result<T, E>
end

class StringStore implements Store<String>, Loader<String, LoadError>
	@value: String

	def initialize(value: String)
		@value = value
		return
	end

	def get(): String
		return @value
	end

	def put(value: String): String
		@value = value
		return @value
	end

	def load(): Result<String, LoadError>
		return Result<String, LoadError>::Ok(@value)
	end
end

def read(store: Store<String>): String
	return store.get()
end

def build(): Store<String>
	return StringStore.new("initial")
end

def load(loader: Loader<String, LoadError>): Result<String, LoadError>
	return loader.load()
end
`)
	wants := map[string][]string{
		"go":         {"type Store[T any] interface", "type Loader[T any, E any] interface", "var _ Store[string] = (*StringStore)(nil)", "var _ Loader[string, LoadError] = (*StringStore)(nil)"},
		"ruby":       {"class StringStore", "def read(store)"},
		"typescript": {"export interface Store<T>", "export interface Loader<T, E>", "export class StringStore implements Store<string>, Loader<string, LoadError>"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("generic_interface.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected generic interface values: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("%s output is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestGenericInterfaceImplementationChecksSpecializedSignatures(t *testing.T) {
	wrongMethod := []byte(`interface Store<T>
	get(): T
end

class InvalidStore implements Store<String>
	def get(): Integer
		return 1
	end
end
`)
	if _, err := Compile("wrong_generic_interface.trb", wrongMethod, "go"); err == nil || !strings.Contains(err.Error(), "does not match interface Store<String>") {
		t.Fatalf("expected specialized interface signature diagnostic, got %v", err)
	}

	wrongArity := []byte(`interface Store<T>
	get(): T
end

class InvalidStore implements Store
	def get(): String
		return "value"
	end
end
`)
	if _, err := Compile("wrong_generic_interface_arity.trb", wrongArity, "go"); err == nil || !strings.Contains(err.Error(), "Store expects 1 type argument(s), got 0") {
		t.Fatalf("expected generic interface arity diagnostic, got %v", err)
	}
}

func TestGoConstructorPreservesMultiwordClassNames(t *testing.T) {
	source := []byte(`class TraceMiddleware
end

def build(): TraceMiddleware
	return TraceMiddleware.new()
end
`)
	artifact, err := Compile("multiword_class.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	if !strings.Contains(output, "func NewTraceMiddleware() *TraceMiddleware") || !strings.Contains(output, "return NewTraceMiddleware()") {
		t.Fatalf("multiword class name was not preserved:\n%s", output)
	}
}

func TestInterfaceValuesRequireNominalImplementationAndInvariantArrays(t *testing.T) {
	structural := []byte(`interface Named
	name(): String
end

class Person
	def name(): String
		return "Ada"
	end
end

def display(value: Named): String
	return value.name()
end

def invalid(): String
	return display(Person.new())
end
`)
	if _, err := Compile("structural.trb", structural, "go"); err == nil || !strings.Contains(err.Error(), "expected Named") {
		t.Fatalf("expected nominal interface diagnostic, got %v", err)
	}

	invariant := []byte(`interface Named
	name(): String
end

class Person implements Named
	def name(): String
		return "Ada"
	end
end

def invalid(): Array<Named>
	people := [Person.new()]
	return people
end
`)
	if _, err := Compile("invariant.trb", invariant, "go"); err == nil || !strings.Contains(err.Error(), "expected Array<Named>") {
		t.Fatalf("expected invariant Array diagnostic, got %v", err)
	}
}

func TestPortableConditionsRequireNonNullableBoolean(t *testing.T) {
	valid := []byte(`def decide(flag: Boolean)
  if flag
    return
  elsif false
    return
  else
    while false
      return
    end
  end
  return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("conditions.trb", valid, mode); err != nil {
			t.Fatalf("%s rejected Boolean conditions: %v", mode, err)
		}
	}

	invalid := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "if integer",
			source: "def bad()\n  if 1\n    return\n  end\n  return\nend\n",
			want:   "if condition must be Boolean, got Integer",
		},
		{
			name:   "elsif string",
			source: "def bad()\n  if true\n    return\n  elsif \"yes\"\n    return\n  end\n  return\nend\n",
			want:   "elsif condition must be Boolean, got String",
		},
		{
			name:   "while array",
			source: "def bad()\n  while [true]\n    return\n  end\n  return\nend\n",
			want:   "while condition must be Boolean, got Array<Boolean>",
		},
		{
			name:   "nullable Boolean",
			source: "def bad(value: Boolean?)\n  if value\n    return\n  end\n  return\nend\n",
			want:   "if condition must be Boolean, got Boolean?",
		},
		{
			name:   "portable Any",
			source: "def bad(value: Any)\n  if value\n    return\n  end\n  return\nend\n",
			want:   "if condition must be Boolean, got Any",
		},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_condition.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestExplicitRubyNativeAnyRetainsTruthinessCompatibility(t *testing.T) {
	source := []byte(`import trb/platform/ruby/native

def dynamic(value: Any)
  if value
    return
  end
  return
end
`)
	if _, err := Compile("dynamic.trb", source, "ruby"); err != nil {
		t.Fatal(err)
	}
}

func TestPortableOperatorRulesAndBackendSemantics(t *testing.T) {
	source := []byte(`def calculate(): Boolean
  grouped: Integer := (1 + 2) * 3
  quotient: Integer := -5 / 2
  remainder: Integer := -5 % 2
  power: Integer := 2 ** 3
  float_power: Float := 2.0 ** 3.0
  ratio: Float := 8.0 / 2.0
  mixed_product: Float := 0.25 * 100
  widened: Float := grouped
  message: String := "type" + "rb"
  mut updated: Integer := 8
  updated /= 3
  mut accumulated: Float := 1
  accumulated += 2
  mut enabled: Boolean := true
  enabled &&= false
  words: Boolean := true and false
  return grouped == 9 && quotient == -2 && remainder == -1 && power == 8 && float_power == 8.0 && ratio >= 4.0 && mixed_product == 25.0 && widened == 9.0 && accumulated == 3.0 && 1 == 1.0 && 1 < 1.5 && message == "typerb" && updated == 2 && !enabled && !words
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("operators.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected valid operators: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	goOutput := string(artifacts["go"].Output)
	for _, want := range []string{`import "math"`, `(1 + 2) * 3`, `panic("negative Integer exponent")`, `math.Pow(2.0, 3.0)`, `0.25 * float64(100)`, `float64(grouped)`, `accumulated += float64(2)`, `float64(1) == 1.0`} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, goOutput)
		}
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "operators.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go did not type-check: %v\n%s", err, goOutput)
	}

	rubyOutput := string(artifacts["ruby"].Output)
	for _, want := range []string{".quo(2).truncate", ".remainder(2)", "updated = (updated).quo(3).truncate", "0.25 * (100).to_f", "widened = (grouped).to_f", "accumulated += (2).to_f", "words = true && false"} {
		if !strings.Contains(rubyOutput, want) {
			t.Fatalf("generated Ruby is missing %q:\n%s", want, rubyOutput)
		}
	}
	typescriptOutput := string(artifacts["typescript"].Output)
	for _, want := range []string{"Math.trunc((-5) / 2)", "updated = Math.trunc(updated / 3)", "0.25 * Number(100)", "Number(grouped)", "accumulated += Number(2)"} {
		if !strings.Contains(typescriptOutput, want) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", want, typescriptOutput)
		}
	}
}

func TestIntegerToFloatWideningIsExplicitInTypedIR(t *testing.T) {
	source := []byte(`def accept(value: Float): Float
	return value
end

def direct(value: Integer): Float
	return value
end

def through_call(value: Integer): Float
	result: Float := accept(value)
	return result + value
end
`)

	wants := map[string][]string{
		"go":         {"return float64(value)", "Accept(float64(value))", "result + float64(value)"},
		"ruby":       {"return (value).to_f", "accept((value).to_f)", "result + (value).to_f"},
		"typescript": {"return Number(value)", "accept(Number(value))", "result + Number(value)"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("widening.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected Integer-to-Float widening: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s widening is missing %q:\n%s", mode, want, output)
			}
		}

		var directReturn ir.Expression
		for _, statement := range artifact.IR.Statements {
			method, ok := statement.(*ir.Method)
			if !ok || method.Name != "direct" || len(method.Body) == 0 {
				continue
			}
			directReturn = method.Body[0].(*ir.Return).Value
		}
		conversion, ok := directReturn.(*ir.Conversion)
		if !ok || conversion.Kind != ir.IntegerToFloatConversion || conversion.ExprType().Kind != types.Float || conversion.Value.ExprType().Kind != types.Int {
			t.Fatalf("%s did not retain widening in typed IR: %#v", mode, directReturn)
		}
	}
}

func TestNonNullableValuesAreExplicitlyConvertedToNullableTypes(t *testing.T) {
	source := []byte(`def maybe_name(): String?
	return "Ada"
end

def maybe_ratio(): Float?
	return 1
end
`)

	wants := map[string][]string{
		"go": {
			`func(value string) *string { return &value }("Ada")`,
			`func(value float64) *float64 { return &value }(float64(1))`,
		},
		"ruby":       {`return "Ada"`, `return (1).to_f`},
		"typescript": {`return "Ada";`, `return Number(1);`},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("nullable_conversion.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected non-nullable to nullable conversion: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s nullable conversion is missing %q:\n%s", mode, want, output)
			}
		}

		for _, statement := range artifact.IR.Statements {
			method, ok := statement.(*ir.Method)
			if !ok || method.Name != "maybe_name" {
				continue
			}
			returned := method.Body[0].(*ir.Return).Value
			conversion, ok := returned.(*ir.Conversion)
			if !ok || conversion.Kind != ir.NonNullableToNullableConversion || !conversion.ExprType().Nullable || conversion.Value.ExprType().Nullable {
				t.Fatalf("%s did not retain nullable conversion in typed IR: %#v", mode, returned)
			}
		}
	}
}

func TestNullableBindingsNarrowAcrossPortableControlFlow(t *testing.T) {
	source := []byte(`def guarded(value: String?): String
	if value == nil
		return "missing"
	end
	return value + "!"
end

def branched(value: String?): Integer
	if nil != value
		return value.size()
	else
		return 0
	end
end

def short_circuit(value: String?): Boolean
	return value != nil and value.size() > 0
end

def inverse_short_circuit(value: String?): Boolean
	return value == nil or value.size() == 0
end

def elsif_branch(value: String?): String
	if value == nil
		return "missing"
	elsif value.size() == 0
		return "empty"
	else
		return value
	end
end

def consume(mut_value: String?)
	while mut_value != nil
		puts(mut_value.size())
		mut_value = nil
	end
	return
end
`)

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("nullable_narrowing.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected nullable narrowing: %v", mode, err)
		}
		var guardedReturn ir.Expression
		for _, statement := range artifact.IR.Statements {
			method, ok := statement.(*ir.Method)
			if !ok || method.Name != "guarded" {
				continue
			}
			guardedReturn = method.Body[1].(*ir.Return).Value
		}
		binary, ok := guardedReturn.(*ir.Binary)
		if !ok {
			t.Fatalf("%s guarded return is not a binary expression: %#v", mode, guardedReturn)
		}
		conversion, ok := binary.Left.(*ir.Conversion)
		if !ok || conversion.Kind != ir.NullableToNonNullableConversion || conversion.ExprType().Nullable || !conversion.Value.ExprType().Nullable {
			t.Fatalf("%s did not retain nullable unwrap in typed IR: %#v", mode, binary.Left)
		}
	}
}

func TestNullableNarrowingIsInvalidatedByAssignment(t *testing.T) {
	source := []byte(`def invalid(value: String?): Integer
	if value == nil
		return 0
	end
	value = nil
	return value.size()
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("nullable_assignment.trb", source, mode); err == nil || !strings.Contains(err.Error(), "type String? has no member size") {
			t.Fatalf("%s expected invalidated nullable narrowing diagnostic, got %v", mode, err)
		}
	}
}

func TestReadonlyDataFieldsNarrowAcrossPortableControlFlow(t *testing.T) {
	source := []byte(`record Profile
	nickname: String?
end

class Account
	readonly @email: String?

	def initialize(email: String?)
		@email = email
		return
	end
end

def profile_name(profile: Profile): String
	if profile.nickname == nil
		return "missing"
	end
	return profile.nickname
end

def email_size(account: Account): Integer
	if nil != account.email
		return account.email.size()
	else
		return 0
	end
end

def has_name(profile: Profile): Boolean
	return profile.nickname != nil and profile.nickname.size() > 0
end
`)

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("readonly_field_narrowing.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected readonly field narrowing: %v", mode, err)
		}
		var profileReturn ir.Expression
		for _, statement := range artifact.IR.Statements {
			method, ok := statement.(*ir.Method)
			if ok && method.Name == "profile_name" {
				profileReturn = method.Body[1].(*ir.Return).Value
			}
		}
		conversion, ok := profileReturn.(*ir.Conversion)
		if !ok || conversion.Kind != ir.NullableToNonNullableConversion || conversion.ExprType().Nullable || !conversion.Value.ExprType().Nullable {
			t.Fatalf("%s did not retain readonly field unwrap in typed IR: %#v", mode, profileReturn)
		}
		if _, ok := conversion.Value.(*ir.Member); !ok {
			t.Fatalf("%s nullable field unwrap did not retain its member expression: %#v", mode, conversion.Value)
		}
	}
}

func TestNullableFieldNarrowingRequiresStableReceiverAndReadonlyField(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "mutable class field",
			source: `class Account
	@email: String?

	def initialize(email: String?)
		@email = email
		return
	end
end

def invalid(account: Account): Integer
	if account.email != nil
		return account.email.size()
	end
	return 0
end
`,
		},
		{
			name: "reassigned receiver",
			source: `record Profile
	nickname: String?
end

def invalid(profile: Profile): Integer
	if profile.nickname == nil
		return 0
	end
	profile = Profile.new(nickname: nil)
	return profile.nickname.size()
end
`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("invalid_readonly_field_narrowing.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), "type String? has no member size") {
				t.Fatalf("%s %s: expected nullable member diagnostic, got %v", mode, test.name, err)
			}
		}
	}
}

func TestCollectionLiteralsInferCommonNumericTypeAcrossBackends(t *testing.T) {
	source := []byte(`def array_values(): Array<Float>
	return [1, 2.5]
end

def hash_values(): Hash<String, Float>
	return { integer: 1, float: 2.5 }
end
`)

	wants := map[string][]string{
		"go":         {"[]float64{float64(1), 2.5}", `map[string]float64{"integer": float64(1), "float": 2.5}`},
		"ruby":       {"[(1).to_f, 2.5]", `{"integer" => (1).to_f, "float" => 2.5}`},
		"typescript": {"[Number(1), 2.5]", `{["integer"]: Number(1), ["float"]: 2.5}`},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("common_collections.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected collection common-type inference: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s common-type inference is missing %q:\n%s", mode, want, output)
			}
		}

		for _, statement := range artifact.IR.Statements {
			method, ok := statement.(*ir.Method)
			if !ok || len(method.Body) == 0 {
				continue
			}
			returned := method.Body[0].(*ir.Return).Value
			if returned.ExprType().Kind == types.Array {
				array := returned.(*ir.Array)
				if _, ok := array.Elements[0].(*ir.Conversion); !ok {
					t.Fatalf("%s Array literal did not retain Integer-to-Float conversion in typed IR: %#v", mode, array.Elements[0])
				}
			}
			if returned.ExprType().Kind == types.Hash {
				hash := returned.(*ir.Hash)
				if _, ok := hash.Entries[0].Value.(*ir.Conversion); !ok {
					t.Fatalf("%s Hash literal did not retain Integer-to-Float conversion in typed IR: %#v", mode, hash.Entries[0].Value)
				}
			}
		}
	}
}

func TestUnionTypesInferAndNarrowAcrossBackends(t *testing.T) {
	source := []byte(`def describe(value: Integer | String): String
	case value
	when Integer(number)
		return number.to_s()
	when String(text)
		return text
	end
end

def values(): Array<Integer | String>
	return [1, "two"]
end

def fields(): Hash<String, Integer | String>
	return { count: 1, name: "Ada" }
end

def mixed(): Array<Float | String>
	return [1, 2.5, "two"]
end

def widen(value: Integer | String): Float | String
	return value
end
`)

	wants := map[string][]string{
		"go": {
			"func Describe(value any) string",
			"if __trbCase1Value1, ok := __trbCase1.(int); ok {",
			"number := __trbCase1Value1",
			`[]any{1, "two"}`,
			`map[string]any{"count": 1, "name": "Ada"}`,
			`[]any{float64(1), 2.5, "two"}`,
			"if integer, ok := value.(int); ok {",
			"return float64(integer)",
		},
		"ruby": {
			"when Integer",
			"number = __trb_case1",
			`[1, "two"]`,
			`{"count" => 1, "name" => "Ada"}`,
			`[(1).to_f, 2.5, "two"]`,
			"(->(value) { value.is_a?(Integer) ? value.to_f : value }).call(value)",
		},
		"typescript": {
			"value: number | string",
			`typeof __trbCase1 === "number"`,
			"const number = __trbCase1;",
			`Array<number | string>`,
			`Record<string, number | string>`,
			`[Number(1), 2.5, "two"]`,
			`((value: number | string): number | string => typeof value === "number" ? Number(value) : value)(value)`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("unions.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected portable union types: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s union support is missing %q:\n%s", mode, want, output)
			}
		}
		if mode == "go" {
			fileSet := token.NewFileSet()
			parsed, parseErr := parser.ParseFile(fileSet, "unions.go", artifact.Output, parser.AllErrors)
			if parseErr != nil {
				t.Fatalf("generated Go union output did not parse: %v\n%s", parseErr, output)
			}
			if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
				t.Fatalf("generated Go union output did not type-check: %v\n%s", typeErr, output)
			}
		}
		for _, statement := range artifact.IR.Statements {
			method, ok := statement.(*ir.Method)
			if !ok || method.Name != "values" {
				continue
			}
			returned := method.Body[0].(*ir.Return).Value
			if returned.ExprType().String() != "Array<Integer | String>" {
				t.Fatalf("%s typed IR lost inferred union: %s", mode, returned.ExprType())
			}
		}
	}
}

func TestInvalidUnionTypeUsageIsRejectedAcrossModes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "union does not narrow implicitly",
			source: "def bad(value: Integer | String): Integer\n\treturn value\nend\n",
			want:   "return type is Integer | String, expected Integer",
		},
		{
			name: "type case must be exhaustive",
			source: `def bad(value: Integer | String): String
	case value
	when Integer(number)
		return number.to_s()
	end
end
`,
			want: "case for Integer | String is not exhaustive; missing String",
		},
		{
			name: "pattern must be an alternative",
			source: `def bad(value: Integer | String): String
	case value
	when Boolean(flag)
		return flag.to_s()
	else
		return "other"
	end
end
`,
			want: "when type must be an alternative of Integer | String",
		},
	}
	for _, test := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			if _, err := Compile("invalid_union.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s %s: expected %q, got %v", mode, test.name, test.want, err)
			}
		}
	}
}

func TestInvalidPortableOperatorsAreRejectedAcrossModes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "logical integer",
			source: "def bad(): Boolean\n  return 1 && true\nend\n",
			want:   "operator && does not support Integer and Boolean",
		},
		{
			name:   "unary not integer",
			source: "def bad(): Boolean\n  return !1\nend\n",
			want:   "operator ! does not support Integer",
		},
		{
			name:   "string ordering",
			source: "def bad(): Boolean\n  return \"a\" < \"b\"\nend\n",
			want:   "operator < does not support String and String",
		},
		{
			name:   "array equality",
			source: "def bad(): Boolean\n  return [1] == [1]\nend\n",
			want:   "operator == does not support Array<Integer> and Array<Integer>",
		},
		{
			name:   "float remainder",
			source: "def bad(): Float\n  return 2.0 % 1.0\nend\n",
			want:   "operator % does not support Float and Float",
		},
		{
			name:   "native-only comparison",
			source: "def bad(): Any\n  return 1 <=> 2\nend\n",
			want:   "operator <=> is not part of portable TypeRB",
		},
		{
			name:   "invalid compound assignment",
			source: "def bad()\n  mut value := 1\n  value += \"x\"\n  return\nend\n",
			want:   "operator + does not support Integer and String",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_operator.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestPortableCollectionTransformationsAcrossModes(t *testing.T) {
	source := []byte(`def mapped(): Array<String>
	return [1, 2].map do |value|
		value.to_s()
	end
end

def indexed(): Array<Integer>
	return [10, 20].map.with_index do |value, index|
		value + index
	end
end

def selected(): Array<Integer>
	return (0..4).select do |value|
		value % 2 == 0
	end
end

def total(): Integer
	return [1, 2, 3].reduce(0) do |sum, value|
		sum + value
	end
end

def any_large?(): Boolean
	return [1, 2, 3].any?() do |value|
		value > 2
	end
end

def all_positive?(): Boolean
	return [1, 2, 3].all? do |value|
		value > 0
	end
end

def none_negative?(): Boolean
	return [1, 2, 3].none?() do |value|
		value < 0
	end
end

def first_even(): Integer?
	return [1, 2, 3].find do |value|
		value % 2 == 0
	end
end

def first_large_index(): Integer?
	return [1, 2, 3].find_index() do |value|
		value > 2
	end
end
`)
	wants := map[string][]string{
		"go": {
			`make([]string, 0, len(`,
			`append(__trbResult`,
			`if (value % 2) == 0`,
			`sum := __trbResult`,
			`if value > 2 {`,
			`if !(value > 0) {`,
			`if value < 0 {`,
			`return &value`,
			`return &__trbResult`,
		},
		"ruby": {
			`.map { |value| value.to_s }`,
			`.map.with_index { |value, index| value + index }`,
			`.select { |value| ((value).remainder(2)) == 0 }`,
			`.reduce(0) { |sum, value| sum + value }`,
			`.any? { |value| value > 2 }`,
			`.all? { |value| value > 0 }`,
			`.none? { |value| value < 0 }`,
			`.find { |value| ((value).remainder(2)) == 0 }`,
			`.find_index { |value| value > 2 }`,
		},
		"typescript": {
			`.map((value) => String(value))`,
			`.map((value, index) => value + index)`,
			`.select`,
			`.reduce((sum, value) => sum + value, 0)`,
			`.some((value) => value > 2)`,
			`.every((value) => value > 0)`,
			`!([1, 2, 3].some((value) => value < 0))`,
			`.find((value) => (value % 2) == 0) ?? null`,
			`.findIndex((value) => value > 2)`,
		},
	}
	// TypeScript calls the portable select operation through Array#filter.
	wants["typescript"][2] = `.filter((value) => (value % 2) == 0)`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("transforms.trb", source, Options{Mode: mode, Package: "transforms", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable collection transformations: %v", mode, err)
		}
		for _, want := range wants[mode] {
			if output := string(artifact.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s transformation is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableArraySortingAcrossModes(t *testing.T) {
	source := []byte(`import trb/std/arrays

record Item
	name: String
	rank: Integer
end

def sorted_numbers(): Array<Integer>
	return [3, 1, 2].sort()
end

def descending_numbers(): Array<Integer>
	return [3, 1, 2].sort_descending()
end

def package_sorted_numbers(): Array<Integer>
	return arrays.sort([3, 1, 2])
end

def sorted_items(items: Array<Item>): Array<Item>
	return items.sort_by do |item|
		item.rank
	end
end

def descending_items(items: Array<Item>): Array<Item>
	return items.sort_by_descending do |item|
		item.rank
	end
end
`)
	wants := map[string][]string{
		"go":         {"slices.SortStableFunc", "type __trbDecorated", "left.key > right.key"},
		"ruby":       {"each_with_index.map", ".sort { |left, right|", "right[1] <=> left[1]"},
		"typescript": {".map((value, index) => ({ value, index }))", "key: item", "left.key > right.key"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("sorting.trb", source, Options{Mode: mode, Package: "sorting", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable Array sorting: %v", mode, err)
		}
		for _, want := range wants[mode] {
			if output := string(artifact.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s sorting is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableArraySortingDiagnosticsAcrossModes(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "record Item\n\tname: String\nend\ndef bad(values: Array<Item>): Array<Item>\n\treturn values.sort()\nend\n", want: "portable natural order is not defined for Item, required by sort()"},
		{source: "def bad(values: Array<Integer?>): Array<Integer?>\n\treturn values.sort()\nend\n", want: "portable natural order is not defined for Integer?, required by sort()"},
		{source: "record Item\n\tname: String\nend\ndef bad(values: Array<Item>): Array<Item>\n\treturn values.sort_by do |value|\n\t\tvalue\n\tend\nend\n", want: "sort_by block result must have portable natural order, got Item"},
		{source: "def bad(values: Array<Integer>): Array<Integer>\n\treturn values.sort_by do |value, index|\n\t\tvalue + index\n\tend\nend\n", want: "sort_by block expects 1 parameter(s), got 2"},
		{source: "import { Result } from trb/std/result\nrecord AppError\nend\ndef key(value: Integer): Result<Integer, AppError>\n\treturn Result<Integer, AppError>::Ok(value)\nend\ndef bad(values: Array<Integer>): Result<Array<Integer>, AppError>\n\tvalues := values.sort_by do |value|\n\t\ttry key(value)\n\tend\n\treturn Result<Array<Integer>, AppError>::Ok(values)\nend\n", want: "try is not supported inside value-producing collection transformations"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad_sort.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s: expected %q sorting diagnostic, got %v", mode, test.want, err)
			}
		}
	}
}

func TestPortableCollectionTransformationDiagnosticsAcrossModes(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{
			source: "def bad(): Array<Integer>\n\treturn [1].select do |value|\n\t\tvalue\n\tend\nend\n",
			want:   "select block result must be Boolean, got Integer",
		},
		{
			source: "def bad(): Boolean\n\treturn [1].any? do |value|\n\t\tvalue\n\tend\nend\n",
			want:   "any? block result must be Boolean, got Integer",
		},
		{
			source: "def bad(): Integer?\n\treturn [1].find do |value|\n\t\tvalue\n\tend\nend\n",
			want:   "find block result must be Boolean, got Integer",
		},
		{
			source: "def bad(): Boolean\n\treturn [1].all?.with_index do |value, index|\n\t\tvalue > index\n\tend\nend\n",
			want:   "all?.with_index is not supported",
		},
		{
			source: "def bad(): Array<Integer>\n\treturn [1].map do |value|\n\t\tvalue\n\t\tvalue + 1\n\tend\nend\n",
			want:   "",
		},
		{
			source: "def bad(): Array<Integer>\n\treturn [1].map do |value|\n\t\tdoubled := value * 2\n\tend\nend\n",
			want:   "map block must end with a result expression",
		},
		{
			source: "def bad(): Integer\n\treturn [1].reduce(0) do |_, value|\n\t\tvalue.to_s()\n\tend\nend\n",
			want:   "reduce block result is String, expected Integer",
		},
		{
			source: "def bad(): Integer\n\treturn [1].reduce do |sum, value|\n\t\tsum + value\n\tend\nend\n",
			want:   "reduce expects exactly one positional initial value",
		},
		{
			source: "def bad(): Array<Integer>\n\treturn [1].map do |value, index|\n\t\tvalue + index\n\tend\nend\n",
			want:   "map block expects 1 parameter(s), got 2",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			_, err := Compile("bad.trb", []byte(test.source), mode)
			if test.want == "" && err != nil {
				t.Fatalf("%s: expected multi-statement collection transformation to compile, got %v", mode, err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("%s: expected %q collection-transformation diagnostic, got %v", mode, test.want, err)
			}
		}
	}
}

func TestNullableEqualityAndRubyNativeOperatorsHaveExplicitBoundaries(t *testing.T) {
	portable := []byte(`def missing(value: String?): Boolean
  return value == nil
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("nullable_equality.trb", portable, mode); err != nil {
			t.Fatalf("%s rejected nullable nil comparison: %v", mode, err)
		}
	}

	native := []byte(`import trb/platform/ruby/native

def compare(left: Any, right: Any): Any
  return left <=> right
end
`)
	artifact, err := Compile("native_operator.trb", native, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(artifact.Output), "left <=> right") {
		t.Fatalf("Ruby-native operator was not preserved:\n%s", artifact.Output)
	}
}

func TestModeAloneNeverRelaxesPortableLanguageRules(t *testing.T) {
	source := []byte(`def legacy_assignment()
  undeclared = 1
  return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("common_grammar.trb", source, mode); err == nil || !strings.Contains(err.Error(), "undeclared is not declared") {
			t.Fatalf("%s mode unexpectedly relaxed assignment rules: %v", mode, err)
		}
	}

	native := []byte(`import trb/platform/ruby/native

def legacy_assignment()
  undeclared = 1
  return
end
`)
	if _, err := Compile("native_assignment.trb", native, "ruby"); err != nil {
		t.Fatalf("explicit Ruby-native import did not enable its compatibility surface: %v", err)
	}
}

func TestBreakAndNextLowerAcrossPortableLoops(t *testing.T) {
	source := []byte(`def collect(): Integer
  mut total: Integer := 0
  mut index: Integer := 0
  while index < 6
    index += 1
    if index == 2
      next
    end
    if index == 5
      break
    end
    total += index
  end
  [1, 2, 3].each do |value|
    if value == 2
      next
    end
    total += value
  end
  [1, 2, 3].each { |_| next }
  return total
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("loop_control.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected portable loop control: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	goOutput := string(artifacts["go"].Output)
	if !strings.Contains(goOutput, "break") || strings.Count(goOutput, "continue") < 3 {
		t.Fatalf("generated Go is missing loop control:\n%s", goOutput)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "loop_control.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&gotypes.Config{}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go did not type-check: %v\n%s", err, goOutput)
	}

	rubyOutput := string(artifacts["ruby"].Output)
	if !strings.Contains(rubyOutput, "break") || strings.Count(rubyOutput, "next") < 3 {
		t.Fatalf("generated Ruby is missing loop control:\n%s", rubyOutput)
	}
	typescriptOutput := string(artifacts["typescript"].Output)
	if !strings.Contains(typescriptOutput, "break;") || strings.Count(typescriptOutput, "continue;") < 3 {
		t.Fatalf("generated TypeScript is missing loop control:\n%s", typescriptOutput)
	}
}

func TestBreakAndNextRequireAnEnclosingLoop(t *testing.T) {
	for _, keyword := range []string{"break", "next"} {
		source := []byte("def invalid()\n  " + keyword + "\n  return\nend\n")
		want := keyword + " is only valid inside while or an iteration block"
		for _, mode := range []string{"go", "ruby", "typescript"} {
			if _, err := Compile("invalid_loop_control.trb", source, mode); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s %s: expected %q diagnostic, got %v", mode, keyword, want, err)
			}
		}
	}
}

func TestDivergenceSyntaxRequiresAnExistingControlFlowOwner(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "return 1\n", want: "return is only valid inside a function or method"},
		{source: "def invalid(): Never\n\treturn\nend\n", want: "Never is an internal compiler type and cannot be written in source"},
	}
	for _, test := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			if _, err := Compile("invalid_divergence.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
			}
		}
	}
}

func TestCollectionTransformationRejectsEnclosingReturn(t *testing.T) {
	source := []byte(`def invalid(values: Array<Integer>): Array<Integer>
	return values.map do |value|
		if value == 0
			return []
		else
			value
		end
	end
end
`)
	want := "return is not supported inside value-producing collection transformations yet"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("invalid_transform_return.trb", source, mode); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: expected %q diagnostic, got %v", mode, want, err)
		}
	}
}

func TestBreakAndNextDoNotAcceptValues(t *testing.T) {
	for _, keyword := range []string{"break", "next"} {
		source := []byte("def invalid()\n  while true\n    " + keyword + " 1\n  end\n  return\nend\n")
		want := keyword + " does not take a value"
		for _, mode := range []string{"go", "ruby", "typescript"} {
			if _, err := Compile("valued_loop_control.trb", source, mode); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s %s: expected %q diagnostic, got %v", mode, keyword, want, err)
			}
		}
	}
}

func TestPortableHashTypesAndRequiredLookupLowerAcrossBackends(t *testing.T) {
	source := []byte(`def accept(values: Hash<String, Integer>): Integer
  return 0
end

def score(): Integer
  mut scores: Hash<String, Integer> := {"alice" => 1, bonus: 2}
  scores["alice"] = 3
  mut labels: Hash<Integer, String> := {}
  labels[1] = "one"
  ignored := accept({})
  mixed := {"number" => 1, "word" => "one"}
  puts(mixed["word"])
  mut widened: Hash<String, Any> := {"count" => 1}
  widened["name"] = "one"
  mut floats: Hash<String, Float> := {"one" => 1}
  floats["two"] = 2.0
  mut nested: Hash<String, Hash<String, Integer>> := {"empty" => {}}
  nested["empty"]["one"] = 1
  return scores["alice"] + scores["bonus"] + ignored
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("hash.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected portable Hash: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	goOutput := string(artifacts["go"].Output)
	for _, want := range []string{
		"map[string]int{",
		"map[int]string{}",
		`panic("Hash key is missing")`,
		`scores["alice"] = 3`,
		"Accept(map[string]int{})",
		"map[string]any{",
	} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, goOutput)
		}
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "hash.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, goOutput)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go did not type-check: %v\n%s", err, goOutput)
	}

	rubyOutput := string(artifacts["ruby"].Output)
	for _, want := range []string{`{"alice" => 1, "bonus" => 2}`, `scores.fetch("alice")`} {
		if !strings.Contains(rubyOutput, want) {
			t.Fatalf("generated Ruby is missing %q:\n%s", want, rubyOutput)
		}
	}
	if strings.Contains(rubyOutput, ":bonus") {
		t.Fatalf("portable symbol-shaped key changed semantics in Ruby:\n%s", rubyOutput)
	}

	typescriptOutput := string(artifacts["typescript"].Output)
	for _, want := range []string{"Record<string, number>", "Record<number, string>", "Object.prototype.hasOwnProperty.call(values, key)"} {
		if !strings.Contains(typescriptOutput, want) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", want, typescriptOutput)
		}
	}

	method := artifacts["go"].IR.Statements[1].(*ir.Method)
	variable := method.Body[0].(*ir.Variable)
	if variable.Type.Kind != types.Hash || len(variable.Type.Args) != 2 || variable.Type.Args[0].Kind != types.String || variable.Type.Args[1].Kind != types.Int {
		t.Fatalf("Hash key/value types were not retained in typed IR: %#v", variable.Type)
	}
}

func TestPortableHashTypeErrorsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"bare Hash", "def bad(value: Hash)\n  return\nend\n", "Hash expects two type arguments, got 0"},
		{"one argument", "def bad(value: Hash<String>)\n  return\nend\n", "Hash expects two type arguments, got 1"},
		{"unsupported key type", "def bad(value: Hash<Boolean, Integer>)\n  return\nend\n", "Hash key type must be String or Integer, got Boolean"},
		{"mixed key types", "def bad()\n  value := {\"one\" => 1, 2 => 2}\n  return\nend\n", "Hash literal key type is Integer, expected String"},
		{"unsupported literal key", "def bad()\n  value := {true => 1}\n  return\nend\n", "Hash key must be String or Integer, got Boolean"},
		{"wrong lookup key", "def bad(value: Hash<String, Integer>): Integer\n  return value[1]\nend\n", "Hash index has type Integer, expected String"},
		{"wrong assigned value", "def bad()\n  mut value: Hash<String, Integer> := {}\n  value[\"one\"] = \"one\"\n  return\nend\n", "cannot assign String to Integer"},
		{"immutable update", "def bad()\n  value := {\"one\" => 1}\n  value[\"one\"] = 2\n  return\nend\n", "value is immutable; declare it with mut to use assignment"},
		{"untyped empty lookup", "def bad(): Any\n  value := {}\n  return value[\"one\"]\nend\n", "cannot index an untyped Hash; add Hash<K, V> annotation"},
		{"compound entry assignment", "def bad()\n  mut value := {\"one\" => 1}\n  value[\"one\"] += 1\n  return\nend\n", "Hash entry compound assignment is not supported; read and assign an explicit value"},
		{"mutable invariant alias", "def bad()\n  mut integers := {\"one\" => 1}\n  mut values: Hash<String, Any> := integers\n  return\nend\n", "cannot assign Hash<String, Integer> to Hash<String, Any>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_hash.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestPortableEnumAndExhaustiveCaseAcrossModes(t *testing.T) {
	source := []byte(`enum TokenKind
	Identifier
	Integer
	String
	EOF
end

def describe(kind: TokenKind): String
	case kind
	when TokenKind::Identifier
		return "identifier"
	when TokenKind::Integer
		return "integer"
	when TokenKind::String
		return "string"
	when TokenKind::EOF
		return "eof"
	end
end

def same(left: TokenKind, right: TokenKind): Boolean
	return left == right
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("enum.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected portable enum: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	goOutput := string(artifacts["go"].Output)
	for _, want := range []string{"type TokenKind int", "TokenKindIdentifier TokenKind = iota", "__trbCase1 == TokenKindIdentifier"} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, goOutput)
		}
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "enum.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, goOutput)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go did not type-check: %v\n%s", err, goOutput)
	}

	rubyOutput := string(artifacts["ruby"].Output)
	for _, want := range []string{"TokenKind = Data.define(:name)", "TokenKind::Identifier = TokenKind.new(:Identifier)", "case kind"} {
		if !strings.Contains(rubyOutput, want) {
			t.Fatalf("generated Ruby is missing %q:\n%s", want, rubyOutput)
		}
	}

	typescriptOutput := string(artifacts["typescript"].Output)
	for _, want := range []string{"export type TokenKind = string &", "export const TokenKind = Object.freeze", `Identifier: "Identifier" as TokenKind`, "__trbCase1 === TokenKind.Identifier"} {
		if !strings.Contains(typescriptOutput, want) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", want, typescriptOutput)
		}
	}

	if _, ok := artifacts["go"].AST.Statements[0].(*ast.EnumStatement); !ok {
		t.Fatalf("enum was not retained in AST: %T", artifacts["go"].AST.Statements[0])
	}
	method := artifacts["go"].IR.Statements[1].(*ir.Method)
	if _, ok := method.Body[0].(*ir.Case); !ok {
		t.Fatalf("case was not retained in typed IR: %T", method.Body[0])
	}
}

func TestCaseExpressionAcrossModes(t *testing.T) {
	source := []byte(`enum Outcome
	Text(value: String)
	Count(value: Integer)
end

def render(outcome: Outcome): String
	result := case outcome
	when Outcome::Text(value)
		decorated := "text:" + value
		decorated
	when Outcome::Count(value)
		value.to_s()
	end
	return result
end

def direct(outcome: Outcome): String
	return case outcome
	when Outcome::Text(value)
		value
	when Outcome::Count(value)
		value.to_s()
	end
end

def assign(outcome: Outcome): String
	mut result := ""
	result = case outcome
	when Outcome::Text(value)
		value
	when Outcome::Count(value)
		value.to_s()
	end
	return result
end

def render_all(outcomes: Array<Outcome>): Array<String>
	return outcomes.map do |outcome|
		case outcome
		when Outcome::Text(value)
			value
		when Outcome::Count(value)
			value.to_s()
		end
	end
end

def main()
	puts(
		case Outcome::Text("ok")
		when Outcome::Text(value)
			value
		when Outcome::Count(value)
			value.to_s()
		end
	)
	return
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("case_expression.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected case expressions: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	for mode, wants := range map[string][]string{
		"go":         {"result := func() string {", "return decorated", "return func() string {"},
		"ruby":       {"result = begin", "return begin", "case __trb_case"},
		"typescript": {"const result: string = (()", "return decorated;", "return (()"},
	} {
		output := string(artifacts[mode].Output)
		for _, want := range wants {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s case expression is missing %q:\n%s", mode, want, output)
			}
		}
	}

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "case_expression.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, artifacts["go"].Output)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated case expression Go did not type-check: %v\n%s", err, artifacts["go"].Output)
	}

	method := artifacts["go"].IR.Statements[1].(*ir.Method)
	variable := method.Body[0].(*ir.Variable)
	caseExpression, ok := variable.Value.(*ir.Case)
	if !ok || caseExpression.ExprType().String() != "String" || caseExpression.Branches[0].Result == nil {
		t.Fatalf("case expression value was not retained in typed IR: %#v", variable.Value)
	}
	astMethod := artifacts["go"].AST.Statements[1].(*ast.MethodStatement)
	astVariable := astMethod.Body[0].(*ast.VariableStatement)
	if _, ok := astVariable.Value.(*ast.CaseStatement); !ok {
		t.Fatalf("case expression value was not retained in syntax AST: %T", astVariable.Value)
	}
}

func TestLiteralCaseAlternativesAcrossModes(t *testing.T) {
	source := []byte(`def label(value: String): String
	return case value
	when "receipts", "receipt_detail"
		"receipts"
	else
		"other"
	end
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("case_alternatives.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected literal case alternatives: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	for mode, want := range map[string]string{
		"go":         `__trbCase1 == "receipts" || __trbCase1 == "receipt_detail"`,
		"ruby":       `when "receipts", "receipt_detail"`,
		"typescript": `__trbCase1 === "receipts" || __trbCase1 === "receipt_detail"`,
	} {
		if output := string(artifacts[mode].Output); !strings.Contains(output, want) {
			t.Fatalf("generated %s literal case is missing %q:\n%s", mode, want, output)
		}
	}

	method := artifacts["go"].IR.Statements[0].(*ir.Method)
	caseExpression := method.Body[0].(*ir.Return).Value.(*ir.Case)
	if got := len(caseExpression.Branches[0].Alternatives); got != 1 {
		t.Fatalf("typed IR retained %d alternatives, want 1", got)
	}
	astMethod := artifacts["go"].AST.Statements[0].(*ast.MethodStatement)
	astCase := astMethod.Body[0].(*ast.ReturnStatement).Value.(*ast.CaseStatement)
	if got := len(astCase.Branches[0].Alternatives); got != 1 {
		t.Fatalf("syntax AST retained %d alternatives, want 1", got)
	}
}

func TestCaseAlternativesRejectPatternsAcrossModes(t *testing.T) {
	source := []byte("enum State\n\tOpen\n\tClosed\nend\ndef show(state: State)\n\tcase state\n\twhen State::Open, State::Closed\n\t\treturn\n\tend\nend\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("bad_case_alternatives.trb", source, mode); err == nil || !strings.Contains(err.Error(), "case alternatives are supported only for Integer or String literals") {
			t.Fatalf("%s: expected a portable case-alternative diagnostic, got %v", mode, err)
		}
	}
}

func TestCaseExpressionDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "incompatible branches",
			source: "enum State\n\tOpen\n\tClosed\nend\ndef value(state: State): String\n\treturn case state\n\twhen State::Open\n\t\t\"open\"\n\twhen State::Closed\n\t\t1\n\tend\nend\n",
			want:   "case expression branches have incompatible types String and Integer",
		},
		{
			name:   "missing branch value",
			source: "enum State\n\tOpen\nend\ndef value(state: State): String\n\treturn case state\n\twhen State::Open\n\t\t# no value\n\tend\nend\n",
			want:   "case expression branch must end with an expression",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_case_expression.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestCaseExpressionUsesSafeCommonBranchType(t *testing.T) {
	source := []byte(`enum Choice
	Whole
	Fraction
end

def number(choice: Choice): Float
	return case choice
	when Choice::Whole
		1
	when Choice::Fraction
		2.5
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("numeric_case_expression.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected compatible numeric branches: %v", mode, err)
		}
		method := artifact.IR.Statements[1].(*ir.Method)
		caseExpression := method.Body[0].(*ir.Return).Value.(*ir.Case)
		if caseExpression.ExprType().String() != "Float" || caseExpression.Branches[0].Result.ExprType().String() != "Float" {
			t.Fatalf("%s did not retain Float branch widening in IR: %#v", mode, caseExpression)
		}
	}
}

func TestDiscriminatedUnionNarrowingAcrossModes(t *testing.T) {
	source := []byte(`record CreatedResponse
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

def status_text(response: CreateResponse): String
	return response.status.to_s()
end

def main()
	created: CreateResponse := CreatedResponse.new(status: 201, body: "created")
	invalid: CreateResponse := InvalidResponse.new(status: 422, body: ["invalid"])
	puts(render(created))
	puts(render(invalid))
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("discriminated_union.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected discriminated union narrowing: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	for mode, wants := range map[string][]string{
		"go": {
			"Status int",
			"func(value any) int",
			"case CreatedResponse:",
			"response := response.(CreatedResponse)",
		},
		"ruby": {
			"CreatedResponse = Data.define(:status, :body)",
			"case response.status",
		},
		"typescript": {
			"status: 201",
			"status: 422",
			"const response = __trbNarrow",
			"as CreatedResponse",
		},
	} {
		output := string(artifacts[mode].Output)
		for _, want := range wants {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s discriminated union support is missing %q:\n%s", mode, want, output)
			}
		}
	}

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "discriminated_union.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, artifacts["go"].Output)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated discriminated union Go did not type-check: %v\n%s", err, artifacts["go"].Output)
	}

	method := artifacts["go"].IR.Statements[3].(*ir.Method)
	caseStatement := method.Body[0].(*ir.Case)
	if caseStatement.Value.ExprType().String() != "201 | 422" || len(caseStatement.Branches[0].Narrowings) != 1 {
		t.Fatalf("typed IR lost discriminant or branch narrowing: %#v", caseStatement)
	}
}

func TestReadonlyClassFieldsMayDiscriminateAUnionAcrossModes(t *testing.T) {
	source := []byte(`class Loaded
	readonly @kind: "loaded"
	readonly @value: String

	def initialize(kind: "loaded", value: String)
		@kind = kind
		@value = value
	end
end

class Missing
	readonly @kind: "missing"
	readonly @message: String

	def initialize(kind: "missing", message: String)
		@kind = kind
		@message = message
	end
end

type LoadResult = Loaded | Missing

def read(result: LoadResult): String
	case result.kind
	when "loaded"
		return result.value
	when "missing"
		return result.message
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("class_discriminated_union.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected readonly class discriminants: %v", mode, err)
		}
		output := string(artifact.Output)
		if mode == "go" && (!strings.Contains(output, "case *Loaded:") || !strings.Contains(output, "result := result.(*Loaded)")) {
			t.Fatalf("generated Go did not project and narrow class alternatives:\n%s", output)
		}
		if mode == "typescript" && (!strings.Contains(output, `__trb_kind!: "loaded"`) || !strings.Contains(output, "as Loaded")) {
			t.Fatalf("generated TypeScript lost class literal fields or narrowing:\n%s", output)
		}
	}
}

func TestUnionDataMemberUsesSafeCommonTypeAcrossModes(t *testing.T) {
	source := []byte(`record Count
	value: Integer
end
record Ratio
	value: Float
end
type NumberValue = Count | Ratio
def number(value: NumberValue): Float
	return value.value
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("union_common_member.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected a safely widened common union member: %v", mode, err)
		}
		if mode == "go" && !strings.Contains(string(artifact.Output), "return float64(value.Value)") {
			t.Fatalf("generated Go did not widen the Integer member to Float:\n%s", artifact.Output)
		}
	}
}

func TestIntegerLiteralsAtPortableRangeBoundariesCompileAcrossModes(t *testing.T) {
	source := []byte(`def maximum(): Integer
	return 9007199254740991
end

def minimum(): Integer
	return -9007199254740991
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("integer_boundaries.trb", source, mode); err != nil {
			t.Fatalf("%s rejected portable Integer boundaries: %v", mode, err)
		}
	}
}

func TestLiteralAndDiscriminatedUnionDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "literal field mismatch",
			source: "record Response\n\tstatus: 201\nend\ndef main()\n\tresponse := Response.new(status: 200)\n\tputs(response.status)\nend\n",
			want:   "record field status has type Integer, expected 201",
		},
		{
			name:   "non exhaustive literal union",
			source: "record Found\n\tstatus: 200\nend\nrecord Missing\n\tstatus: 404\nend\ntype Response = Found | Missing\ndef show(response: Response)\n\tcase response.status\n\twhen 200\n\t\treturn\n\tend\nend\n",
			want:   "case for 200 | 404 is not exhaustive; missing 404",
		},
		{
			name:   "wrong literal kind",
			source: "def show(status: Integer)\n\tcase status\n\twhen \"ok\"\n\t\treturn\n\telse\n\t\treturn\n\tend\nend\n",
			want:   "when value has type String, expected Integer",
		},
		{
			name:   "invalid literal modifier",
			source: "def status(): 201?\n\treturn 201\nend\n",
			want:   "literal type 201 cannot have type arguments, array, or nullable modifiers",
		},
		{
			name:   "positive Integer literal outside portable range",
			source: "def value(): Integer\n\treturn 9007199254740992\nend\n",
			want:   "Integer literal is outside the portable range -9007199254740991..9007199254740991",
		},
		{
			name:   "negative Integer literal outside portable range",
			source: "def value(): Integer\n\treturn -9007199254740992\nend\n",
			want:   "Integer literal is outside the portable range -9007199254740991..9007199254740991",
		},
		{
			name:   "Integer literal type outside portable range",
			source: "def value(): 9007199254740992\n\treturn 1\nend\n",
			want:   "Integer literal is outside the portable range -9007199254740991..9007199254740991",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_discriminated_union.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestIfExpressionAcrossModes(t *testing.T) {
	source := []byte(`def choose(primary: Boolean, secondary: Boolean): String
	result := if primary
		value := "primary"
		value
	elsif secondary
		"secondary"
	else
		"fallback"
	end
	return result
end

def direct(enabled: Boolean): String
	return if enabled
		"on"
	else
		"off"
	end
end

def assign(enabled: Boolean): String
	mut result := ""
	result = if enabled
		"on"
	else
		"off"
	end
	return result
end

def render_all(flags: Array<Boolean>): Array<String>
	return flags.map do |enabled|
		if enabled
			"on"
		else
			"off"
		end
	end
end

def main()
	puts(
		if true
			"on"
		else
			"off"
		end
	)
	return
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("if_expression.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected if expressions: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	for mode, wants := range map[string][]string{
		"go":         {"result := func() string {", "return value", "return func() string {"},
		"ruby":       {"result = begin", "return begin", "elsif secondary"},
		"typescript": {"const result: string = ((): string => {", "return value;", "return (()"},
	} {
		output := string(artifacts[mode].Output)
		for _, want := range wants {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s if expression is missing %q:\n%s", mode, want, output)
			}
		}
	}

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "if_expression.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, artifacts["go"].Output)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated if expression Go did not type-check: %v\n%s", err, artifacts["go"].Output)
	}

	method := artifacts["go"].IR.Statements[0].(*ir.Method)
	variable := method.Body[0].(*ir.Variable)
	ifExpression, ok := variable.Value.(*ir.If)
	if !ok || ifExpression.ExprType().String() != "String" || ifExpression.ThenResult == nil || ifExpression.ElseIf[0].Result == nil || ifExpression.ElseResult == nil {
		t.Fatalf("if expression value was not retained in typed IR: %#v", variable.Value)
	}
	astMethod := artifacts["go"].AST.Statements[0].(*ast.MethodStatement)
	astVariable := astMethod.Body[0].(*ast.VariableStatement)
	if _, ok := astVariable.Value.(*ast.IfStatement); !ok {
		t.Fatalf("if expression value was not retained in syntax AST: %T", astVariable.Value)
	}
}

func TestIfExpressionDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "missing else",
			source: "def value(enabled: Boolean): String\n\treturn if enabled\n\t\t\"on\"\n\tend\nend\n",
			want:   "if expression requires an else branch",
		},
		{
			name:   "incompatible branches",
			source: "def value(enabled: Boolean): String\n\treturn if enabled\n\t\t\"on\"\n\telse\n\t\t1\n\tend\nend\n",
			want:   "if expression branches have incompatible types String and Integer",
		},
		{
			name:   "missing branch value",
			source: "def value(enabled: Boolean): String\n\treturn if enabled\n\t\t# no value\n\telse\n\t\t\"off\"\n\tend\nend\n",
			want:   "if expression branch must end with an expression",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_if_expression.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestIfExpressionUsesSafeCommonBranchType(t *testing.T) {
	source := []byte(`def number(whole: Boolean): Float
	return if whole
		1
	else
		2.5
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("numeric_if_expression.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected compatible numeric branches: %v", mode, err)
		}
		method := artifact.IR.Statements[0].(*ir.Method)
		ifExpression := method.Body[0].(*ir.Return).Value.(*ir.If)
		if ifExpression.ExprType().String() != "Float" || ifExpression.ThenResult.ExprType().String() != "Float" {
			t.Fatalf("%s did not retain Float branch widening in IR: %#v", mode, ifExpression)
		}
	}
}

func TestDivergingControlFlowExpressionAcrossModes(t *testing.T) {
	source := []byte(`enum Outcome
	Found(value: String)
	Missing(message: String)
end

def describe(outcome: Outcome): String
	message := "result: " + case outcome
	when Outcome::Found(value)
		value
	when Outcome::Missing(reason)
		return "missing: " + reason
	end
	return message
end

def choose(enabled: Boolean): String
	message := if enabled
		"enabled"
	else
		return "disabled"
	end
	return message
end

def choose_directly(enabled: Boolean): String
	return if enabled
		return "left"
	else
		return "right"
	end
end

def choose_nested(primary: Boolean, secondary: Boolean): String
	message := if primary
		if secondary
			return "first"
		else
			return "second"
		end
	else
		"fallback"
	end
	return message
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("diverging_expression.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected diverging control-flow expressions: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	for mode, wants := range map[string][]string{
		"go":         {"var __trbValue", "return \"missing: \" + reason", "return \"disabled\""},
		"ruby":       {"message = \"result: \" + begin", "return \"missing: \" + reason", "return \"disabled\""},
		"typescript": {"let __trbValue", "return \"missing: \" + reason;", "return \"disabled\";"},
	} {
		output := string(artifacts[mode].Output)
		for _, want := range wants {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s diverging expression is missing %q:\n%s", mode, want, output)
			}
		}
	}

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "diverging_expression.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, artifacts["go"].Output)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated diverging expression Go did not type-check: %v\n%s", err, artifacts["go"].Output)
	}

	method := artifacts["go"].IR.Statements[1].(*ir.Method)
	variable := method.Body[0].(*ir.Variable)
	binary := variable.Value.(*ir.Binary)
	caseExpression := binary.Right.(*ir.Case)
	if !caseExpression.Branches[1].Diverges || caseExpression.Branches[1].Result != nil || caseExpression.ExprType().String() != "String" {
		t.Fatalf("typed IR did not retain the diverging case branch: %#v", caseExpression.Branches[1])
	}
	allDiverge := artifacts["go"].IR.Statements[3].(*ir.Method).Body[0].(*ir.Return).Value.(*ir.If)
	if allDiverge.ExprType().Kind != types.Never || !allDiverge.ThenDiverges || !allDiverge.ElseDiverges {
		t.Fatalf("all-diverging if expression was not typed as Never: %#v", allDiverge)
	}
}

func TestPayloadEnumAndPatternBindingAcrossModes(t *testing.T) {
	source := []byte(`enum Token
	Text(value: String)
	Pair(left: Integer, right: Integer)
	EOF
end

def render(token: Token): String
	case token
	when Token::Text(value)
		return value
	when Token::Pair(left, right)
		return "#{left}:#{right}"
	when Token::EOF
		return "eof"
	end
end

def main()
	puts(render(Token::Text("hello")))
	return
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("payload_enum.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected payload enum: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	goOutput := string(artifacts["go"].Output)
	for _, want := range []string{
		"type TokenTag int",
		"type Token struct",
		"func NewTokenText(value string) Token",
		"NewTokenText(\"hello\")",
		"__trbCase1.Kind == TokenTextTag",
		"value := __trbCase1.TextValue",
	} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, goOutput)
		}
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "payload_enum.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, goOutput)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated payload enum Go did not type-check: %v\n%s", err, goOutput)
	}

	rubyOutput := string(artifacts["ruby"].Output)
	for _, want := range []string{
		"module Token",
		"Text = Data.define(:value)",
		"EOF = Data.define().new",
		"Token::Text.new(\"hello\")",
		"value = __trb_case1.value",
	} {
		if !strings.Contains(rubyOutput, want) {
			t.Fatalf("generated Ruby is missing %q:\n%s", want, rubyOutput)
		}
	}

	typescriptOutput := string(artifacts["typescript"].Output)
	for _, want := range []string{
		`export type Token = { readonly kind: "Text"; readonly value: string }`,
		`Text: (value: string): Token => ({ kind: "Text", value })`,
		`Token.Text("hello")`,
		`__trbCase1.kind === "Text"`,
		`const value = __trbCase1.value;`,
	} {
		if !strings.Contains(typescriptOutput, want) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", want, typescriptOutput)
		}
	}

	enum := artifacts["go"].AST.Statements[0].(*ast.EnumStatement)
	member := enum.Body[0].(*ast.EnumMemberStatement)
	if len(member.Parameters) != 1 || member.Parameters[0].Name != "value" {
		t.Fatalf("payload declaration was not retained in AST: %#v", member.Parameters)
	}
	method := artifacts["go"].IR.Statements[1].(*ir.Method)
	caseStatement := method.Body[0].(*ir.Case)
	if len(caseStatement.Branches[0].Bindings) != 1 || caseStatement.Branches[0].Bindings[0].Type.String() != "String" {
		t.Fatalf("typed pattern binding was not retained in IR: %#v", caseStatement.Branches[0])
	}
}

func TestPayloadEnumDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"missing payload type", "enum Token\n\tText(value)\nend\n", "enum payload value requires a name and type"},
		{"constructor type", "enum Token\n\tText(value: String)\nend\ndef bad(): Token\n\treturn Token::Text(1)\nend\n", "enum payload argument 1 has type Integer, expected String"},
		{"constructor arity", "enum Token\n\tText(value: String)\nend\ndef bad(): Token\n\treturn Token::Text()\nend\n", "expects 1 payload argument(s), got 0"},
		{"payload member value", "enum Token\n\tText(value: String)\nend\ndef bad(): Token\n\treturn Token::Text\nend\n", "requires 1 payload argument(s)"},
		{"payloadless call", "enum Token\n\tEOF\nend\ndef bad(): Token\n\treturn Token::EOF()\nend\n", "has no payload and is not callable"},
		{"pattern arity", "enum Token\n\tText(value: String)\nend\ndef bad(token: Token): String\n\tcase token\n\twhen Token::Text\n\t\treturn \"bad\"\n\tend\nend\n", "expects 1 binding(s), got 0"},
		{"payload equality", "enum Token\n\tText(value: String)\nend\ndef bad(left: Token, right: Token): Boolean\n\treturn left == right\nend\n", "operator == does not support Token and Token"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_payload_enum.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestExplicitUserGenericsAndTypeSubstitutionAcrossModes(t *testing.T) {
	source := []byte(`enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end

def identity<T>(value: T): T
	return value
end

def render(result: Result<Integer, String>): String
	case result
	when Result::Ok(value)
		return identity<String>("#{value}")
	when Result::Err(error)
		return error
	end
end

def main()
	puts(render(Result<Integer, String>::Ok(42)))
	names := identity<Array<String>>(["Ada"])
	puts(names[0])
	return
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("generics.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected explicit user generics: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	goOutput := string(artifacts["go"].Output)
	for _, want := range []string{
		"type Result[T any, E any] struct",
		"func NewResultOk[T any, E any](value T) Result[T, E]",
		"func Identity[T any](value T) T",
		"NewResultOk[int, string](42)",
		"Identity[string]",
		`Identity[[]string]([]string{"Ada"})`,
		"value := __trbCase1.OkValue",
	} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, goOutput)
		}
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "generics.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated generic Go: %v\n%s", err, goOutput)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated generic Go did not type-check: %v\n%s", err, goOutput)
	}

	rubyOutput := string(artifacts["ruby"].Output)
	for _, want := range []string{"module Result", "Ok = Data.define(:value)", "def identity(value)", "Result::Ok.new(42)", "identity(\"#{value}\")"} {
		if !strings.Contains(rubyOutput, want) {
			t.Fatalf("generated Ruby is missing %q:\n%s", want, rubyOutput)
		}
	}

	typescriptOutput := string(artifacts["typescript"].Output)
	for _, want := range []string{
		`export type Result<T, E> = { readonly kind: "Ok"; readonly value: T }`,
		`Ok: <T, E>(value: T): Result<T, E>`,
		`export function identity<T>(value: T): T`,
		`Result.Ok<number, string>(42)`,
		`identity<string>`,
		`identity<Array<string>>(["Ada"])`,
	} {
		if !strings.Contains(typescriptOutput, want) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", want, typescriptOutput)
		}
	}

	method := artifacts["go"].IR.Statements[2].(*ir.Method)
	caseStatement := method.Body[0].(*ir.Case)
	if got := caseStatement.Branches[0].Bindings[0].Type.String(); got != "Integer" {
		t.Fatalf("generic pattern binding was not substituted: %s", got)
	}
}

func TestGenericClassesRecordsAndInstanceMethodsAcrossModes(t *testing.T) {
	source := []byte(`class Box<T>
	@value: T

	def initialize(value: T)
		@value = value
		return
	end

	def value(): T
		return @value
	end

	def pair<U>(other: U): Pair<T, U>
		return Pair<T, U>.new(left: @value, right: other)
	end
end

record Pair<T, U>
	left: T
	right: U
end

def main()
	box := Box<Integer>.new(7)
	pair := box.pair<String>("Ada")
	puts(box.value())
	puts(pair.left)
	puts(pair.right)
	return
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("generic_objects.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected generic objects: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	goOutput := string(artifacts["go"].Output)
	for _, want := range []string{
		"type Box[T any] struct",
		"func NewBox[T any](value T) *Box[T]",
		"func (self *Box[T]) Value() T",
		"func (self *Box[T]) Pair[U any](other U) Pair[T, U]",
		"type Pair[T any, U any] struct",
		"NewBox[int](7)",
		"box.Pair[string](\"Ada\")",
	} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, goOutput)
		}
	}
	if strings.Contains(goOutput, "BoxPair") {
		t.Fatalf("generated Go retained the pre-1.27 generic method helper:\n%s", goOutput)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "generic_objects.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated generic Go: %v\n%s", err, goOutput)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated generic Go did not type-check: %v\n%s", err, goOutput)
	}

	rubyOutput := string(artifacts["ruby"].Output)
	for _, want := range []string{"class Box", "Pair = Data.define(:left, :right)", "Box.new(7)", `box.pair("Ada")`} {
		if !strings.Contains(rubyOutput, want) {
			t.Fatalf("generated Ruby is missing %q:\n%s", want, rubyOutput)
		}
	}

	typescriptOutput := string(artifacts["typescript"].Output)
	for _, want := range []string{
		"export class Box<T>",
		"pair<U>(other: U): Pair<T, U>",
		"export interface Pair<T, U>",
		"new Box<number>(7)",
		`box.pair<string>("Ada")`,
	} {
		if !strings.Contains(typescriptOutput, want) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", want, typescriptOutput)
		}
	}
}

func TestGenericEnumMethodsRetainRepresentationHelper(t *testing.T) {
	source := []byte(`enum Box<T>
	Value(value: T)

	def convert<U>(value: U): U
		return value
	end
end

def main()
	box := Box<Integer>::Value(7)
	puts(box.convert<String>("Ada"))
	return
end
`)

	artifact, err := Compile("generic_enum_method.trb", source, "go")
	if err != nil {
		t.Fatalf("Go rejected a generic enum method: %v", err)
	}
	output := string(artifact.Output)
	for _, want := range []string{
		"func BoxConvert[T any, U any](self Box[T], value U) U",
		"BoxConvert[int, string](box, \"Ada\")",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, output)
		}
	}

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "generic_enum_method.go", artifact.Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated generic enum method: %v\n%s", err, output)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated generic enum method did not type-check: %v\n%s", err, output)
	}
}

func TestTransparentGenericEnumAliasesAcrossModes(t *testing.T) {
	source := []byte(`enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end

record DbError
	message: String
end

type DbResult<T> = Result<T, DbError>

def successful(): DbResult<Integer>
	return DbResult<Integer>::Ok(7)
end

def value_or_zero(result: DbResult<Integer>): Integer
	return case result
	when DbResult::Ok(value)
		value
	when DbResult::Err(_error)
		0
	end
end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("type_alias.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected transparent generic alias: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	goOutput := string(artifacts["go"].Output)
	for _, want := range []string{
		"type DbResult[T any] = Result[T, DbError]",
		"const DbResultOkTag = ResultOkTag",
		"func NewDbResultOk[T any](value T) DbResult[T]",
		"NewDbResultOk[int](7)",
		"__trbCase1.Kind == DbResultOkTag",
	} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, goOutput)
		}
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "type_alias.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated alias Go: %v\n%s", err, goOutput)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated alias Go did not type-check: %v\n%s", err, goOutput)
	}

	rubyOutput := string(artifacts["ruby"].Output)
	for _, want := range []string{"DbResult = Result", "DbResult::Ok.new(7)", "when DbResult::Ok"} {
		if !strings.Contains(rubyOutput, want) {
			t.Fatalf("generated Ruby is missing %q:\n%s", want, rubyOutput)
		}
	}

	typescriptOutput := string(artifacts["typescript"].Output)
	for _, want := range []string{
		"export type DbResult<T> = Result<T, DbError>;",
		"export const DbResult = Result;",
		"DbResult.Ok<number>(7)",
		`__trbCase1.kind === "Ok"`,
	} {
		if !strings.Contains(typescriptOutput, want) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", want, typescriptOutput)
		}
	}
}

func TestQualifiedGenericPackageFunctionsRemainFunctions(t *testing.T) {
	source := []byte(`import trb/std/json

def encode_message()
	_encoded := json.encode<String>("hello") catch |_error|
		return
	end
	return
end
`)

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("qualified_generic_function.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected a qualified generic package function: %v", mode, err)
		}
		if strings.Contains(string(artifact.Output), "AnyFail") {
			t.Fatalf("%s lowered a package function as an instance generic method:\n%s", mode, artifact.Output)
		}
	}
}

func TestInitialUserGenericDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"function needs explicit arguments", "def identity<T>(value: T): T\n\treturn value\nend\ndef bad(): String\n\treturn identity(\"x\")\nend\n", "generic function identity requires explicit type arguments"},
		{"substituted function argument", "def identity<T>(value: T): T\n\treturn value\nend\ndef bad(): Integer\n\treturn identity<Integer>(\"x\")\nend\n", "argument 1 to identity() has type String, expected Integer"},
		{"function type arity", "def identity<T>(value: T): T\n\treturn value\nend\ndef bad(): String\n\treturn identity<String, Integer>(\"x\")\nend\n", "identity expects 1 type argument(s), got 2"},
		{"enum type arity in annotation", "enum Result<T, E>\n\tOk(value: T)\n\tErr(error: E)\nend\ndef bad(value: Result<Integer>)\n\treturn\nend\n", "Result expects 2 type argument(s), got 1"},
		{"enum construction needs all type arguments", "enum Result<T, E>\n\tOk(value: T)\n\tErr(error: E)\nend\ndef bad(): Result<Integer, String>\n\treturn Result::Ok(1)\nend\n", "Result expects 2 type argument(s), got 0"},
		{"payloadless generic variant", "enum Option<T>\n\tSome(value: T)\n\tNone\nend\n", "payloadless members of generic enums are reserved"},
		{"generic method needs explicit arguments", "class Box\n\tdef value<T>(item: T): T\n\t\treturn item\n\tend\nend\ndef bad(box: Box): String\n\treturn box.value(\"x\")\nend\n", "generic method value requires explicit type arguments"},
		{"generic class type arity", "class Box<T>\n\t@value: T\nend\ndef bad(value: Box)\n\treturn\nend\n", "Box expects 1 type argument(s), got 0"},
		{"generic class method rejected", "class Box<T>\n\tdef self.value(): T\n\t\treturn nil\n\tend\nend\n", "class methods on a generic class cannot use the class type parameters"},
		{"duplicate type parameter", "def identity<T, T>(value: T): T\n\treturn value\nend\n", "type parameter T is duplicated"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_generics.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestSemicolonSeparatesPortableStatementsAcrossModes(t *testing.T) {
	source := []byte(`class Empty; end
enum State; Open; Closed; end
def label(state: State): String; case state; when State::Open; return "open"; when State::Closed; return "closed"; end; end
def main(); left := 1; right := 2; puts(label(State::Open)); puts(left + right); return; end
`)

	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("separator.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected portable statement separators: %v", mode, err)
		}
		artifacts[mode] = artifact
		if len(artifact.AST.Statements) != 4 {
			t.Fatalf("%s parsed %d top-level statements, want 4", mode, len(artifact.AST.Statements))
		}
		if _, ok := artifact.AST.Statements[0].(*ast.ClassStatement); !ok {
			t.Fatalf("%s did not retain class AST: %T", mode, artifact.AST.Statements[0])
		}
		if _, ok := artifact.AST.Statements[1].(*ast.EnumStatement); !ok {
			t.Fatalf("%s did not retain enum AST: %T", mode, artifact.AST.Statements[1])
		}
		if _, ok := artifact.IR.Statements[2].(*ir.Method); !ok {
			t.Fatalf("%s did not retain method IR: %T", mode, artifact.IR.Statements[2])
		}
	}

	goOutput := string(artifacts["go"].Output)
	for _, want := range []string{"type Empty struct", "type State int", "func Label(state State) string", "left := 1", "right := 2"} {
		if !strings.Contains(goOutput, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, goOutput)
		}
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "separator.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, goOutput)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go did not type-check: %v\n%s", err, goOutput)
	}

	for mode, wants := range map[string][]string{
		"ruby":       {"class Empty", "State = Data.define(:name)", "def label(state)"},
		"typescript": {"export class Empty", "export type State = string &", "export function label(state: State): string"},
	} {
		output := string(artifacts[mode].Output)
		for _, want := range wants {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestEnumCaseDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "non exhaustive",
			source: "enum State\n\tOpen\n\tClosed\nend\ndef show(value: State)\n\tcase value\n\twhen State::Open\n\t\treturn\n\tend\nend\n",
			want:   "case for State is not exhaustive; missing Closed",
		},
		{
			name:   "duplicate branch",
			source: "enum State\n\tOpen\nend\ndef show(value: State)\n\tcase value\n\twhen State::Open\n\t\treturn\n\twhen State::Open\n\t\treturn\n\tend\nend\n",
			want:   "enum member Open is handled more than once",
		},
		{
			name:   "unknown member",
			source: "enum State\n\tOpen\nend\ndef show(value: State)\n\tcase value\n\twhen State::Closed\n\t\treturn\n\tend\nend\n",
			want:   "enum State has no member Closed",
		},
		{
			name:   "empty enum",
			source: "enum State\nend\n",
			want:   "enum State must declare at least one member",
		},
		{
			name:   "duplicate member",
			source: "enum State\n\tOpen\n\tOpen\nend\n",
			want:   "enum member Open was already declared",
		},
		{
			name:   "different enum equality",
			source: "enum State\n\tOpen\nend\nenum Other\n\tOpen\nend\ndef same(left: State, right: Other): Boolean\n\treturn left == right\nend\n",
			want:   "operator == does not support State and Other",
		},
		{
			name:   "type name collision",
			source: "class State\nend\nenum State\n\tOpen\nend\n",
			want:   "type State is already declared as class",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("bad_enum.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
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

func TestPortableDefaultParametersMustBeTrailing(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "required after default",
			source: "def invalid(optional: String = \"value\", required: String): String\n\treturn required\nend\n",
			want:   "required positional parameter cannot follow a default parameter",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				if _, err := Compile("invalid_default.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("unexpected diagnostic: %v", err)
				}
			})
		}
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

func TestValueReturningFunctionsMustReturnOnEveryPath(t *testing.T) {
	invalid := []struct {
		name   string
		source string
	}{
		{
			name: "function falls through",
			source: `def hello(): String
	puts("hello!")
end
`,
		},
		{
			name: "partial conditional",
			source: `def label(ready: Boolean): String
	if ready
		return "ready"
	end
end
`,
		},
		{
			name: "loop return is not guaranteed",
			source: `def label(): String
	while true
		return "ready"
	end
end
`,
		},
		{
			name: "instance method falls through",
			source: `class Greeter
	def hello(): String
		puts("hello!")
	end
end
`,
		},
		{
			name: "class method falls through",
			source: `class Greeter
	def self.hello(): String
		puts("hello!")
	end
end
`,
		},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("missing_return.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), "must return String on every path") {
					t.Fatalf("%s: expected missing return diagnostic, got %v", mode, err)
				}
			}
		})
	}

	valid := []struct {
		name   string
		source string
	}{
		{
			name: "direct return",
			source: `def hello(): String
	return "hello!"
end
`,
		},
		{
			name: "complete conditional",
			source: `def label(ready: Boolean): String
	if ready
		return "ready"
	else
		return "waiting"
	end
end
`,
		},
		{
			name: "return after partial conditional",
			source: `def label(ready: Boolean): String
	if ready
		return "ready"
	end
	return "waiting"
end
`,
		},
		{
			name: "exhaustive enum case",
			source: `enum State
	Ready
	Waiting
end

def label(state: State): String
	case state
	when State::Ready
		return "ready"
	when State::Waiting
		return "waiting"
	end
end
`,
		},
		{
			name: "exhaustive union case",
			source: `def label(value: Integer | String): String
	case value
	when Integer(number)
		return number.to_s()
	when String(text)
		return text
	end
end
`,
		},
		{
			name: "no-value function",
			source: `def greet()
	puts("hello!")
end
`,
		},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("complete_return.trb", []byte(test.source), mode); err != nil {
					t.Fatalf("%s rejected complete return flow: %v", mode, err)
				}
			}
		})
	}
}

func TestValueReturnFlowKeepsRubyNativeEscapeExplicit(t *testing.T) {
	native := []byte(`import trb/platform/ruby/native

def framework_value(): Any
	framework_call()
end
`)
	if _, err := Compile("native_return.trb", native, "ruby"); err != nil {
		t.Fatalf("explicit Ruby-native terminal expression was rejected: %v", err)
	}

	portable := []byte(`import trb/platform/ruby/native

def echo(value: String): String
	value
end
`)
	if _, err := Compile("portable_return.trb", portable, "ruby"); err == nil || !strings.Contains(err.Error(), "echo() must return String on every path") {
		t.Fatalf("portable expression incorrectly used Ruby implicit return: %v", err)
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
