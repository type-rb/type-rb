package compiler

import (
	"encoding/hex"
	goast "go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	gotypes "go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/ir"
)

const portableMain = `import trb/std/io

def main()
  message := "Hello, TypeRB".upcase()
  puts(1 + 2)
  IO.puts(message)
  return
end
`

func TestPortableStandardLibraryLowersAcrossBackends(t *testing.T) {
	goArtifact, err := CompileWithOptions("main.trb", []byte(portableMain), Options{Mode: "go", Package: "main"})
	if err != nil {
		t.Fatal(err)
	}
	goOutput := string(goArtifact.Output)
	for _, want := range []string{`import "fmt"`, `import "strings"`, `strings.ToUpper("Hello, TypeRB")`, `fmt.Println(trbIntegerAdd_`, `fmt.Println(message)`} {
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
	for _, want := range []string{`"Hello, TypeRB".toUpperCase()`, `console.log(__trbIntegerAdd(1, 2));`, `console.log(message);`, `main();`} {
		if !strings.Contains(tsOutput, want) {
			t.Fatalf("generated TypeScript does not contain %q:\n%s", want, tsOutput)
		}
	}

	rubyArtifact, err := CompileWithOptions("main.trb", []byte(portableMain), Options{Mode: "ruby", RubyLoader: "require_relative"})
	if err != nil {
		t.Fatal(err)
	}
	rubyOutput := string(rubyArtifact.Output)
	for _, want := range []string{`"Hello, TypeRB".upcase`, `$stdout.puts(__trb_integer_add(1, 2))`, `$stdout.puts(message)`, `main()`} {
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
	if resolved == nil || resolved.Package != "trb/internal/strings" || resolved.Intrinsic != "trb.std.strings.uppercase" || !resolved.ReceiverMethod {
		t.Fatalf("standard call was not retained as a resolved IR reference: %#v", resolved)
	}
}

func TestPortableReceiverMethodsShareStandardContractsAcrossBackends(t *testing.T) {
	source := []byte(`def receiver_text(): String
	return 123.to_s()
end

def float_text(): String
	return 0.25.to_s()
end

def integer_as_float(): Float
	return 2.to_f()
end

def truncated_float(): Integer
	return (-2.75).to_i()
end

def print_float()
	puts(0.25 * 100)
	return
end

def parsed(): Integer
	return "123".to_i()
end

def text_size(): Integer
	return "a😀".size()
end
`)
	wants := map[string][]string{
		"go":         {`strconv.Itoa(123)`, `strconv.FormatFloat(value, 'f', -1, 64)`, `float64(2)`, `math.Trunc(-2.75)`, `fmt.Println(func() string`, `regexp.MatchString`, `strconv.ParseInt`, `utf8.RuneCountInString("a😀")`},
		"ruby":       {`123.to_s`, `raw = value.to_s`, `(2).to_f`, `value.truncate`, `$stdout.puts(->(value)`, `Integer(input, 10)`, `"a😀".each_codepoint.count`},
		"typescript": {`String(123)`, `const raw = String(value)`, `Number(2)`, `Math.trunc(value)`, `console.log(((value: number): string`, `Number.isSafeInteger(__trbValue)`, `Array.from("a😀").length`},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("methods.trb", source, Options{Mode: mode, Package: "methods", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable receiver methods: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s receiver method is missing %q:\n%s", mode, want, output)
			}
		}

		var receiverReference *ir.Reference
		for _, statement := range artifact.IR.Statements {
			method, ok := statement.(*ir.Method)
			if !ok || len(method.Body) == 0 {
				continue
			}
			returned, ok := method.Body[0].(*ir.Return)
			if !ok || returned.Value == nil {
				continue
			}
			call, ok := returned.Value.(*ir.Call)
			if !ok {
				continue
			}
			if method.Name == "receiver_text" {
				receiverReference = call.Callee.(*ir.Member).Reference
			}
		}
		if receiverReference == nil || receiverReference.Intrinsic != "trb.std.numbers.to_string" ||
			receiverReference.Package != "trb/internal/numbers" || !receiverReference.ReceiverMethod {
			t.Fatalf("%s did not retain the internal receiver contract: %#v", mode, receiverReference)
		}
	}
}

func TestPortableScalarReceiverMethodsLowerAcrossBackends(t *testing.T) {
	source := []byte(`def integer_absolute(): Integer
	return (-4).abs()
end

def integer_bounds(): Integer
	return 5.min(3) + 5.max(7) + 12.clamp(0, 10)
end

def integer_predicates(value: Integer): Boolean
	return value.zero?() || value.positive?() || value.negative?() || value.even?() || value.odd?()
end

def float_absolute(): Float
	return (-0.25).abs()
end

def float_rounding(): Integer
	return (-2.75).floor() + (-2.75).ceil() + (-2.5).round() + 2.75.to_i()
end

def float_predicates(value: Float): Boolean
	return value.finite?() || value.infinite?() || value.nan?()
end

def boolean_text(): String
	return true.to_s()
end
`)
	wants := map[string][]string{
		"go": {
			`if value < 0`,
			`min(5, 3)`,
			`max(5, 7)`,
			`clamp minimum exceeds maximum`,
			`value == 0`,
			`value > 0`,
			`value < 0`,
			`value%2 == 0`,
			`value%2 != 0`,
			`math.Abs`,
			`math.Floor`,
			`math.Ceil`,
			`math.Round`,
			`math.Trunc`,
			`!math.IsNaN(value) && !math.IsInf(value, 0)`,
			`math.IsInf(value, 0)`,
			`math.IsNaN(value)`,
			`strconv.FormatBool`,
		},
		"ruby": {
			`.abs`,
			`[(5), (3)].min`,
			`[(5), (7)].max`,
			`value.clamp(minimum, maximum)`,
			`value.floor`,
			`value.ceil`,
			`value.round`,
			`value.truncate`,
			`.zero?`,
			`.positive?`,
			`.negative?`,
			`.even?`,
			`.odd?`,
			`.finite?`,
			`!((value).infinite?).nil?`,
			`.nan?`,
			`(true).to_s`,
		},
		"typescript": {
			`Math.abs`,
			`Math.min(5, 3)`,
			`Math.max(5, 7)`,
			`Math.min(Math.max(value, minimum), maximum)`,
			`Math.floor(value)`,
			`Math.ceil(value)`,
			`value < 0 ? -Math.round(-value) : Math.round(value)`,
			`Math.trunc(value)`,
			`value === 0`,
			`value > 0`,
			`value < 0`,
			`value % 2 === 0`,
			`value % 2 !== 0`,
			`Math.abs`,
			`Number.isFinite(value)`,
			`value === Infinity || value === -Infinity`,
			`Number.isNaN(value)`,
			`String(true)`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("scalar_methods.trb", source, Options{Mode: mode, Package: "scalar_methods", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable scalar receiver methods: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s scalar receiver method is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableMathPackageLowersAcrossBackends(t *testing.T) {
	source := []byte(`import trb/std/math

def calculate(): Float
	return Math.sqrt(9) + Math.exp(0.0) + Math.log(1.0) + Math.log2(8.0) + Math.log10(100.0)
end
`)
	wants := map[string][]string{
		"go":         {"math.Sqrt(float64(9))", "math.Exp(0.0)", "math.Log(1.0)", "math.Log2(8.0)", "math.Log10(100.0)"},
		"ruby":       {"Math.sqrt(value)", ".call((9).to_f)", "Math.exp(0.0)", "Math.log(value)", "Math.log2(value)", "Math.log10(value)"},
		"typescript": {"Math.sqrt(Number(9))", "Math.exp(0.0)", "Math.log(1.0)", "Math.log2(8.0)", "Math.log10(100.0)"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("math.trb", source, Options{Mode: mode, Package: "portable_math", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable math: %v", mode, err)
		}
		for _, want := range wants[mode] {
			if !strings.Contains(string(artifact.Output), want) {
				t.Fatalf("generated %s math is missing %q:\n%s", mode, want, artifact.Output)
			}
		}
	}

	invalid := []struct {
		source string
		want   string
	}{
		{source: "import trb/std/math\n\ndef bad(): Float\n\treturn Math.sqrt(\"nine\")\nend\n", want: "argument 1 to sqrt() has type String, expected Float"},
		{source: "def bad(): Integer\n\treturn 1.5.clamp(0, 1)\nend\n", want: "type Float has no member clamp"},
	}
	for _, test := range invalid {
		if _, err := Compile("bad_math.trb", []byte(test.source), "go"); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected math diagnostic %q, got %v", test.want, err)
		}
	}
}

func TestPortablePredicateAndBangNamesLowerAcrossBackends(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/capability.trb",
		ModulePath: "contracts/capability",
		Package:    "contracts",
		Source: []byte(`interface Capability
	ready?(): Boolean
	save!(): String
end
`),
	}
	helpers := SourceUnit{
		Filename:   "/project/helpers/functions.trb",
		ModulePath: "helpers/functions",
		Package:    "helpers",
		Source: []byte(`def imported_ready?(): Boolean
	return true
end

def imported_save!(): String
	return "imported"
end

def imported_label?(): String
	return "question"
end
`),
	}
	base := SourceUnit{
		Filename:   "/project/models/base.trb",
		ModulePath: "models/base",
		Package:    "models",
		Source: []byte(`import { Capability } from contracts/capability

class Base implements Capability
	def ready?(): Boolean
		return true
	end

	def save!(): String
		return "base"
	end

	def self.available?(): Boolean
		return true
	end
end

def base_available?(): Boolean
	return Base.available?()
end
`),
	}
	child := SourceUnit{
		Filename:   "/project/models/child.trb",
		ModulePath: "models/child",
		Package:    "models",
		Source: []byte(`import { Base, base_available? } from models/base

class Child < Base
	def ready?(): Boolean
		return true
	end

	def child_ready?(): Boolean
		return self.ready?()
	end

	def inherited_available?(): Boolean
		return base_available?()
	end
end
`),
	}
	main := SourceUnit{
		Filename:   "/project/app/main.trb",
		ModulePath: "app/main",
		Package:    "main",
		Source: []byte(`import { imported_ready?, imported_save!, imported_label? } from helpers/functions
import { base_available? } from models/base
import { Child } from models/child

def local_ready?(): Boolean
	return true
end

def local_save!(): String
	return "local"
end

def main()
	child := Child.new()
	puts(local_ready?())
	puts(local_save!())
	puts(imported_ready?())
	puts(imported_save!())
	puts(imported_label?())
	puts(base_available?())
	puts(child.ready?())
	puts(child.save!())
	puts(child.child_ready?())
	puts(child.inherited_available?())
	return
end
`),
	}

	encoded := func(name string) string {
		return hex.EncodeToString([]byte(name))
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{main, child, base, helpers, contract}, Options{Mode: mode, GoModule: "example.com/predicate-names", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable predicate/bang names: %v", mode, err)
		}
		outputs := map[string]string{}
		for _, artifact := range artifacts {
			outputs[artifact.IR.ModulePath] = string(artifact.Output)
		}
		wants := map[string]map[string][]string{
			"go": {
				"contracts/capability": {"TrbQuestion_" + encoded("ready?") + "() bool", "TrbBang_" + encoded("save!") + "() string"},
				"helpers/functions":    {"func TrbQuestion_" + encoded("imported_ready?") + "() bool", "func TrbBang_" + encoded("imported_save!") + "() string", "func TrbQuestion_" + encoded("imported_label?") + "() string"},
				"models/base":          {"var _ contracts.Capability = (*Base)(nil)", "func (self *Base) TrbQuestion_" + encoded("ready?") + "() bool", "func (self *Base) TrbBang_" + encoded("save!") + "() string", "func BaseTrbQuestion_" + encoded("available?") + "() bool"},
				"models/child":         {"func (self *Child) TrbQuestion_" + encoded("ready?") + "() bool", "func (self *Child) TrbQuestion_" + encoded("child_ready?") + "() bool", "self.TrbQuestion_" + encoded("ready?") + "()", "TrbQuestion_" + encoded("base_available?") + "()"},
				"app/main":             {"helpers.TrbQuestion_" + encoded("imported_ready?") + "()", "helpers.TrbQuestion_" + encoded("imported_label?") + "()", "models.TrbQuestion_" + encoded("base_available?") + "()", "child.TrbBang_" + encoded("save!") + "()"},
			},
			"ruby": {
				"contracts/capability": {"def ready?()", "def save!()"},
				"helpers/functions":    {"def imported_ready?()", "def imported_save!()", "def imported_label?()"},
				"models/base":          {"def ready?()", "def save!()", "def self.available?()"},
				"models/child":         {"def ready?()", "def child_ready?()", "self.ready?()", "base_available?()"},
				"app/main":             {"imported_ready?()", "imported_label?()", "base_available?()", "child.save!()"},
			},
			"typescript": {
				"contracts/capability": {"$trb$question$" + encoded("ready?") + "(): boolean", "$trb$bang$" + encoded("save!") + "(): string"},
				"helpers/functions":    {"function $trb$question$" + encoded("imported_ready?"), "function $trb$bang$" + encoded("imported_save!"), "function $trb$question$" + encoded("imported_label?")},
				"models/base":          {"$trb$question$" + encoded("ready?") + "(): boolean", "$trb$bang$" + encoded("save!") + "(): string", "static $trb$question$" + encoded("available?")},
				"models/child":         {"$trb$question$" + encoded("ready?") + "(): boolean", "$trb$question$" + encoded("child_ready?") + "(): boolean", "this.$trb$question$" + encoded("ready?") + "()", "$trb$question$" + encoded("base_available?") + "()"},
				"app/main":             {"$trb$question$" + encoded("imported_ready?") + "()", "$trb$question$" + encoded("imported_label?") + "()", "$trb$question$" + encoded("base_available?") + "()", "child.$trb$bang$" + encoded("save!") + "()"},
			},
		}[mode]
		for module, fragments := range wants {
			for _, fragment := range fragments {
				if !strings.Contains(outputs[module], fragment) {
					t.Fatalf("generated %s module %s is missing %q:\n%s", mode, module, fragment, outputs[module])
				}
			}
		}

		for _, artifact := range artifacts {
			if artifact.IR.ModulePath != "helpers/functions" {
				continue
			}
			if method, ok := artifact.IR.Statements[0].(*ir.Method); !ok || method.Name != "imported_ready?" {
				t.Fatalf("%s changed the typed-IR source name: %#v", mode, artifact.IR.Statements[0])
			}
		}
	}
}

func TestPortableReceiverMethodDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "arity", source: "def bad(): String\n\treturn 1.to_s(2)\nend\n", want: "to_s() expects 0..0 arguments, got 1"},
		{name: "unknown", source: "def bad(): Integer\n\treturn 1.size()\nend\n", want: "type Integer has no member size"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s %s: expected %q diagnostic, got %v", mode, test.name, test.want, err)
			}
		}
	}
}

func TestReceiverOnlyStandardPackagesCannotBeImported(t *testing.T) {
	for _, packagePath := range []string{
		"trb/std/booleans",
		"trb/std/bytes",
		"trb/std/numbers",
		"trb/std/ranges",
		"trb/std/strings",
	} {
		source := []byte("import " + packagePath + "\n")
		for _, mode := range []string{"go", "ruby", "typescript"} {
			_, err := Compile("removed_package.trb", source, mode)
			want := "unknown TypeRB package " + packagePath
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s %s: expected %q diagnostic, got %v", mode, packagePath, want, err)
			}
		}
	}
}

func TestReceiverContractsCannotBeImportedDirectly(t *testing.T) {
	for _, packagePath := range []string{
		"trb/internal/booleans",
		"trb/internal/bytes",
		"trb/internal/numbers",
		"trb/internal/ranges",
		"trb/internal/string_builder",
		"trb/internal/strings",
	} {
		_, err := Compile("internal_package.trb", []byte("import "+packagePath+"\n"), "go")
		want := "package " + packagePath + " is internal to the TypeRB standard library"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: expected %q diagnostic, got %v", packagePath, want, err)
		}
	}
}

func TestPortableStringTrimmingLowersAcrossBackends(t *testing.T) {
	source := []byte(`def receiver_strip(value: String): String
	return value.strip()
end

def receiver_lstrip(value: String): String
	return value.lstrip()
end

def receiver_rstrip(value: String): String
	return value.rstrip()
end
`)
	wants := map[string][]string{
		"go":         {"strings.TrimFunc(value, stdunicode.IsSpace)", "strings.TrimLeftFunc(value, stdunicode.IsSpace)", "strings.TrimRightFunc(value, stdunicode.IsSpace)"},
		"ruby":       {`\u{0009}-\u{000D}`, `\u{3000}`},
		"typescript": {`\u0009-\u000d`, `\u3000`},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("strings.trb", source, Options{Mode: mode, Package: "strings_example", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable String trimming: %v", mode, err)
		}
		for _, want := range wants[mode] {
			if !strings.Contains(string(artifact.Output), want) {
				t.Fatalf("generated %s String trimming is missing %q:\n%s", mode, want, artifact.Output)
			}
		}
	}
}

func TestPortableStringTrimmingDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "wrong receiver type",
			source: "def bad(): String\n\treturn 1.strip()\nend\n",
			want:   "type Integer has no member strip",
		},
		{
			name:   "receiver arity",
			source: "def bad(): String\n\treturn \"value\".lstrip(1)\nend\n",
			want:   "lstrip() expects 0..0 arguments, got 1",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s %s: expected %q diagnostic, got %v", mode, test.name, test.want, err)
			}
		}
	}
}

func TestPortableStringReplacementLowersAcrossBackends(t *testing.T) {
	source := []byte(`def receiver_replace(value: String, pattern: String, replacement: String): String
	return value.replace_all(pattern, replacement)
end
`)
	wants := map[string][]string{
		"go":         {"strings.ReplaceAll(value, pattern, replacement)"},
		"ruby":       {"value.gsub(pattern) { replacement }"},
		"typescript": {"value.split(pattern).join(replacement)"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("strings.trb", source, Options{Mode: mode, Package: "strings_example", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable String replacement: %v", mode, err)
		}
		for _, want := range wants[mode] {
			if !strings.Contains(string(artifact.Output), want) {
				t.Fatalf("generated %s String replacement is missing %q:\n%s", mode, want, artifact.Output)
			}
		}
	}
}

func TestPortableStringReplacementDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "receiver replacement type",
			source: "def bad(): String\n\treturn \"value\".replace_all(\"v\", 1)\nend\n",
			want:   "argument 2 to replace_all() has type Integer, expected String",
		},
		{
			name:   "receiver arity",
			source: "def bad(): String\n\treturn \"value\".replace_all(\"v\")\nend\n",
			want:   "replace_all() expects 2..2 arguments, got 1",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s %s: expected %q diagnostic, got %v", mode, test.name, test.want, err)
			}
		}
	}
}

func TestPortableBytesReceiverMethodsLowerAcrossBackends(t *testing.T) {
	source := []byte(`def joined(): Bytes
	return "A".to_bytes().concat("😀".to_bytes())
end

def byte_length(): Integer
	return joined().concat("!".to_bytes()).size()
end

def byte_at(): Integer
	return "A".to_bytes().at(0)
end

def decoded(): String
	return joined().to_s()
end

def valid(): Boolean
	return joined().valid_utf8?()
end
`)
	wants := map[string][]string{
		"go": {
			`append(append([]byte{}, []byte("A")...), []byte("😀")...)`,
			`len(append(append([]byte{}, Joined()...), []byte("!")...))`,
			`return int(value[index])`,
			`strings.ToValidUTF8(string(Joined()), "�")`,
			`utf8.Valid(Joined())`,
		},
		"ruby": {
			`("A").encode(Encoding::UTF_8).b + ("😀").encode(Encoding::UTF_8).b`,
			`.bytesize`,
			`.bytes.fetch(0)`,
			`.dup.force_encoding(Encoding::UTF_8).encode(Encoding::UTF_8, invalid: :replace, undef: :replace)`,
			`.dup.force_encoding(Encoding::UTF_8).valid_encoding?`,
		},
		"typescript": {
			`new TextEncoder().encode("A")`,
			`new Uint8Array(left.byteLength + right.byteLength)`,
			`.byteLength`,
			`return value[index]!;`,
			`new TextDecoder("utf-8").decode(joined())`,
			`new TextDecoder("utf-8", { fatal: true })`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("bytes.trb", source, Options{Mode: mode, Package: "bytesexample", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable Bytes: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s Bytes output is missing %q:\n%s", mode, want, output)
			}
		}
		if mode == "go" {
			fileSet := token.NewFileSet()
			parsed, parseErr := parser.ParseFile(fileSet, "bytes.go", artifact.Output, parser.AllErrors)
			if parseErr != nil {
				t.Fatalf("generated Go Bytes output does not parse: %v\n%s", parseErr, output)
			}
			if _, checkErr := (&gotypes.Config{Importer: importer.Default()}).Check("bytesexample", fileSet, []*goast.File{parsed}, nil); checkErr != nil {
				t.Fatalf("generated Go Bytes output does not type-check: %v\n%s", checkErr, output)
			}
		}
	}
}

func TestPortableBytesDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "def bad(): Integer\n\treturn \"A\".to_bytes().at(\"0\")\nend\n", want: "argument 1 to at() has type String, expected Integer"},
		{source: "def bad(): Bytes\n\treturn \"A\"\nend\n", want: "return type is String, expected Bytes"},
		{source: "def bad(): Integer\n\treturn \"A\".to_bytes().missing()\nend\n", want: "type Bytes has no member missing"},
		{source: "def bad(): Bytes\n\treturn Bytes.new([])\nend\n", want: "type Bytes has no member new"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s: expected %q Bytes diagnostic, got %v", mode, test.want, err)
			}
		}
	}
}

func TestPortableHexPackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result
import trb/std/encoding/hex

def encoded(value: Bytes): String
	return Hex.encode(value)
end

def decoded(value: String): Result<Bytes, Hex::DecodeError>
	return Hex.decode(value)
end
`),
	}
	wants := map[string][]string{
		"go": {
			`stdhex.EncodeToString(value)`,
			`stdhex.DecodeString(input)`,
			`HexDecodeError{Kind: HexDecodeErrorKindInvalidcharacter`,
			`HexDecodeErrorKindOddlength`,
		},
		"ruby": {
			`.unpack1("H*")`,
			`[input].pack("H*").b`,
			`Hex::DecodeError.new(kind: Hex::DecodeErrorKind::InvalidCharacter`,
			`Hex::DecodeErrorKind::OddLength`,
		},
		"typescript": {
			`value.toString(16).padStart(2, "0")`,
			`new Uint8Array(__trbCharacters.length / 2)`,
			`kind: HexDecodeErrorKind.InvalidCharacter`,
			`kind: HexDecodeErrorKind.OddLength`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/portable-hex", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable hex: %v", mode, err)
		}
		var consumer, resultRuntime, hexRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/result/index":
				resultRuntime = artifact
			case "trb/std/encoding/hex/index":
				hexRuntime = artifact
			}
		}
		if consumer == nil || resultRuntime == nil || hexRuntime == nil {
			t.Fatalf("%s did not compile the hex consumer and runtimes: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(hexRuntime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s hex output is missing %q:\n%s", mode, want, output)
			}
		}
		errorWants := map[string][]string{
			"go":         {"type HexDecodeErrorKind int", "type HexDecodeError struct"},
			"ruby":       {"module Hex", "DecodeErrorKind = Data.define(:name)", "DecodeError = Data.define(:kind, :input, :index, :message)"},
			"typescript": {"export type HexDecodeErrorKind", "export interface HexDecodeError"},
		}[mode]
		for _, want := range errorWants {
			if output := string(hexRuntime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s error runtime is missing %q:\n%s", mode, want, output)
			}
		}
		if mode == "typescript" && strings.Contains(string(hexRuntime.Output), `from "./index.ts"`) {
			t.Fatalf("generated TypeScript hex runtime imports itself:\n%s", hexRuntime.Output)
		}
	}
}

func TestPortableHexDiagnosticsAreModeIndependent(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("bad.trb", []byte("import { Hex } from trb/std/encoding/hex\ndef bad(): String\n\treturn Hex.encode(\"41\")\nend\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "argument 1 to encode() has type String, expected Bytes") {
			t.Fatalf("%s: expected portable hex argument diagnostic, got %v", mode, err)
		}
	}
}

func TestCompilerOwnedIntrinsicOnlyNamespaceImportOmitsUnusedRuntimeImport(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/encoding/hex

def encoded(value: Bytes): String
	return Hex.encode(value)
end
`),
	}
	wants := map[string]string{
		"go":         `stdhex.EncodeToString(value)`,
		"ruby":       `.unpack1("H*")`,
		"typescript": `value.toString(16).padStart(2, "0")`,
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/namespace-intrinsic", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected an intrinsic-only namespace import: %v", mode, err)
		}
		consumer := findArtifactByModule(artifacts, "main")
		if consumer == nil {
			t.Fatalf("%s omitted the namespace-import consumer: %#v", mode, artifacts)
		}
		output := string(consumer.Output)
		if !strings.Contains(output, wants[mode]) {
			t.Fatalf("generated %s intrinsic-only consumer is missing %q:\n%s", mode, wants[mode], output)
		}
		if strings.Contains(output, "trb/std/encoding/hex/index") || mode == "go" && strings.Contains(output, "example.com/namespace-intrinsic/trb/std/encoding/hex") {
			t.Fatalf("generated %s intrinsic-only consumer retained the compiler-owned runtime import:\n%s", mode, output)
		}
	}
}

func TestCompilerOwnedNamespaceImportsRetainRequiredRuntimeAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/encoding/hex
import trb/std/filesystem
import { Result } from trb/std/result

def decoded(value: String): Result<Bytes, Hex::DecodeError>
	return Hex.decode(value)
end

def loaded(path: String): Result<String, FileSystem::Error>
	return FileSystem.read_text(path)
end
`),
	}
	wants := map[string][]string{
		"go": {
			`import "example.com/namespace-runtime/trb/std/encoding/hex"`,
			`import "example.com/namespace-runtime/trb/std/filesystem"`,
			`hex.HexDecodeError`,
			`filesystem.ReadText(path)`,
		},
		"ruby": {
			`require_relative "./trb/std/encoding/hex/index"`,
			`require_relative "./trb/std/filesystem/index"`,
			`def decoded(value)`,
			`read_text(path)`,
		},
		"typescript": {
			`import * as __trb_hex from "./trb/std/encoding/hex/index.ts";`,
			`import * as __trb_filesystem from "./trb/std/filesystem/index.ts";`,
			`__trb_hex.HexDecodeError`,
			`__trb_hex.HexDecodeErrorKind.InvalidCharacter`,
			`__trb_filesystem.read_text(path)`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/namespace-runtime", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected runtime namespace imports: %v", mode, err)
		}
		consumer := findArtifactByModule(artifacts, "main")
		if consumer == nil {
			t.Fatalf("%s omitted the runtime namespace consumer: %#v", mode, artifacts)
		}
		output := string(consumer.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s runtime namespace consumer is missing %q:\n%s", mode, want, output)
			}
		}
		for _, modulePath := range []string{"trb/std/encoding/hex/index", "trb/std/filesystem/index", "trb/std/result/index"} {
			count := 0
			for _, artifact := range artifacts {
				if artifact.IR.ModulePath == modulePath {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("%s emitted %s %d times, expected once", mode, modulePath, count)
			}
		}
	}
}

func TestRubyNamespaceImportedFunctionThatCollidesWithKernelStaysCallable(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/digest

def digest(value: Bytes): Bytes
	return Digest.sha512(value)
end
`),
	}}, Options{Mode: "ruby", RubyLoader: "require_relative"})
	if err != nil {
		t.Fatal(err)
	}
	consumer := findArtifactByModule(artifacts, "main")
	if consumer == nil {
		t.Fatalf("Ruby namespace-import consumer is missing: %#v", artifacts)
	}
	output := string(consumer.Output)
	if !strings.Contains(output, "return sha512(value)") || strings.Contains(output, "hash.sha512(value)") {
		t.Fatalf("generated Ruby did not resolve the namespace function to its loaded top-level definition:\n%s", output)
	}
}

func TestRubyPrivateTopLevelFunctionCallsInsideCatchUseModuleScopedNames(t *testing.T) {
	const modulePath = "helpers"
	artifacts, err := CompileProject([]SourceUnit{{
		Filename:   "/project/helpers.trb",
		ModulePath: modulePath,
		Package:    "helpers",
		Source: []byte(`import { Result } from trb/std/result

def required(value: String?): Result<String, String>
	text := _required(value) catch |error|
		return Result<String, String>::Err(error)
	end
	return Result<String, String>::Ok(text)
end

def _required(value: String?): Result<String, String>
	return Result<String, String>::Err("missing") if value == nil
	return Result<String, String>::Ok(value)
end
`),
	}}, Options{Mode: "ruby", RubyLoader: "require_relative"})
	if err != nil {
		t.Fatal(err)
	}
	consumer := findArtifactByModule(artifacts, modulePath)
	if consumer == nil {
		t.Fatalf("Ruby private-function consumer is missing: %#v", artifacts)
	}
	output := string(consumer.Output)
	target := "__trb_private_" + naming.PrivateSuffix(modulePath+"\x00_required")
	if strings.Count(output, target) != 2 || strings.Contains(output, "_required(value)") {
		t.Fatalf("generated Ruby did not use the module-scoped private target for both definition and catch call:\n%s", output)
	}
}

func findArtifactByModule(artifacts []*Artifact, modulePath string) *Artifact {
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath == modulePath {
			return artifact
		}
	}
	return nil
}

func TestTypeScriptImportsProjectTypesIntroducedByInferredLocals(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename:   "/project/contracts/types.trb",
			ModulePath: "contracts/types",
			Package:    "contracts",
			Source: []byte(`record Envelope
	label: String
end
`),
		},
		{
			Filename:   "/project/service/client.trb",
			ModulePath: "service/client",
			Package:    "service",
			Source: []byte(`import { Envelope } from contracts/types

def load(): Envelope
	return Envelope.new(label: "loaded")
end
`),
		},
		{
			Filename:   "/project/main.trb",
			ModulePath: "main",
			Package:    "main",
			Source: []byte(`import { load } from service/client

def consume()
	envelope := load()
	puts(envelope)
	return
end
`),
		},
	}, Options{Mode: "typescript"})
	if err != nil {
		t.Fatal(err)
	}
	consumer := findArtifactByModule(artifacts, "main")
	if consumer == nil {
		t.Fatal("consumer artifact is missing")
	}
	if output := string(consumer.Output); !strings.Contains(output, `import type { Envelope } from "./contracts/types.ts";`) {
		t.Fatalf("generated TypeScript is missing inferred local type imports:\n%s", output)
	}
	found := false
	for _, statement := range consumer.IR.Statements {
		imported, ok := statement.(*ir.Import)
		if ok && imported.Implicit && imported.Path == "contracts/types" && slices.Contains(imported.GeneratedTypeSymbols, "Envelope") {
			found = true
		}
	}
	if !found {
		t.Fatalf("lowered consumer is missing hidden generated type imports: %#v", consumer.IR.Statements)
	}
}

func TestGoImportsProjectTypesUsedByInferredCollectionLiterals(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename:   "/project/contracts/types.trb",
			ModulePath: "contracts/types",
			Package:    "contracts",
			Source: []byte(`record Item
	label: String
end

record Response
	items: Array<Item>
end
`),
		},
		{
			Filename:   "/project/service/response.trb",
			ModulePath: "service/response",
			Package:    "service",
			Source: []byte(`import { Item, Response } from contracts/types

def load(): Response
	return Response.new(items: [Item.new(label: "loaded")])
end

def assert_items(_actual: Array<Item>, _expected: Array<Item>)
	return
end

def assert_catalog(_actual: Hash<String, Item>)
	return
end
`),
		},
		{
			Filename:   "/project/main.trb",
			ModulePath: "main",
			Package:    "main",
			Source: []byte(`import { assert_catalog, assert_items, load } from service/response

def check()
	response := load()
	assert_items(response.items, [])
	assert_catalog({})
	return
end
`),
		},
	}, Options{Mode: "go", GoModule: "example.com/inferred-collections"})
	if err != nil {
		t.Fatal(err)
	}
	consumer := findArtifactByModule(artifacts, "main")
	if consumer == nil {
		t.Fatal("consumer artifact is missing")
	}
	output := string(consumer.Output)
	if !strings.Contains(output, `[]contracts.Item{}`) {
		t.Fatalf("generated Go is missing the transitive type import for an inferred empty array:\n%s", output)
	}
	if !strings.Contains(output, `map[string]contracts.Item{}`) {
		t.Fatalf("generated Go is missing the transitive type import for an inferred empty hash:\n%s", output)
	}
	var generated *ir.Import
	for _, statement := range consumer.IR.Statements {
		imported, ok := statement.(*ir.Import)
		if ok && imported.Implicit && imported.Path == "contracts/types" {
			generated = imported
			break
		}
	}
	if generated == nil || !slices.Contains(generated.GeneratedTypeSymbols, "Item") {
		t.Fatalf("lowered consumer is missing the transitive generated type import: %#v", generated)
	}
}

func TestPortableBase64PackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result
import trb/std/encoding/base64

def encoded(value: Bytes): String
	return Base64.encode(value) + Base64.url_encode(value)
end

def decoded(value: String): Result<Bytes, Base64::DecodeError>
	return Base64.decode(value)
end

def url_decoded(value: String): Result<Bytes, Base64::DecodeError>
	return Base64.url_decode(value)
end
`),
	}
	wants := map[string][]string{
		"go": {
			`stdbase64.StdEncoding.EncodeToString(value)`,
			`stdbase64.RawURLEncoding.EncodeToString(value)`,
			`stdbase64.StdEncoding.Strict().DecodeString(input)`,
			`stdbase64.RawURLEncoding.Strict().DecodeString(input)`,
			`Base64DecodeErrorKindInvalidlength`,
			`Base64DecodeErrorKindNoncanonical`,
		},
		"ruby": {
			`[value].pack("m0")`,
			`.tr("+/", "-_").delete("=")`,
			`input.unpack1("m0").b`,
			`Base64::DecodeErrorKind::InvalidLength`,
			`Base64::DecodeErrorKind::NonCanonical`,
		},
		"typescript": {
			`return btoa(binary)`,
			`const binary = atob(__trbInput)`,
			`const padded = __trbInput.replace`,
			`kind: Base64DecodeErrorKind.InvalidLength`,
			`kind: Base64DecodeErrorKind.NonCanonical`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/portable-base64", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable base64: %v", mode, err)
		}
		var consumer, resultRuntime, base64Runtime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/result/index":
				resultRuntime = artifact
			case "trb/std/encoding/base64/index":
				base64Runtime = artifact
			}
		}
		if consumer == nil || resultRuntime == nil || base64Runtime == nil {
			t.Fatalf("%s did not compile the base64 consumer and runtimes: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(base64Runtime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s base64 output is missing %q:\n%s", mode, want, output)
			}
		}
		errorWants := map[string][]string{
			"go":         {"type Base64DecodeErrorKind int", "type Base64DecodeError struct"},
			"ruby":       {"module Base64", "DecodeErrorKind = Data.define(:name)", "DecodeError = Data.define(:kind, :input, :index, :message)"},
			"typescript": {"export type Base64DecodeErrorKind", "export interface Base64DecodeError"},
		}[mode]
		for _, want := range errorWants {
			if output := string(base64Runtime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s base64 error runtime is missing %q:\n%s", mode, want, output)
			}
		}
		if mode == "typescript" && strings.Contains(string(base64Runtime.Output), `from "./index.ts"`) {
			t.Fatalf("generated TypeScript base64 runtime imports itself:\n%s", base64Runtime.Output)
		}
	}
}

func TestPortableBase64DiagnosticsAreModeIndependent(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("bad.trb", []byte("import { Base64 } from trb/std/encoding/base64\ndef bad(): String\n\treturn Base64.encode(\"QQ==\")\nend\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "argument 1 to encode() has type String, expected Bytes") {
			t.Fatalf("%s: expected portable base64 argument diagnostic, got %v", mode, err)
		}
	}
}

func TestPortableHashPackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/digest

def digest_md5(value: Bytes): Bytes
	return Digest.md5(value)
end

def digest_sha1(value: Bytes): Bytes
	return Digest.sha1(value)
end

def digest256(value: Bytes): Bytes
	return Digest.sha256(value)
end

def digest512(value: Bytes): Bytes
	return Digest.sha512(value)
end
`),
	}
	wants := map[string][]string{
		"go": {
			`stdmd5.Sum(value)`,
			`stdsha1.Sum(value)`,
			`stdsha256.Sum256(value)`,
			`stdsha512.Sum512(value)`,
		},
		"ruby": {
			`Digest::MD5.digest(value).b`,
			`Digest::SHA1.digest(value).b`,
			`Digest::SHA256.digest(value).b`,
			`Digest::SHA512.digest(value).b`,
		},
		"typescript": {
			`const shifts = [`,
			`let h4 = 0xc3d2e1f0;`,
			`const constants = new Uint32Array`,
			`let h0 = 0x6a09e667;`,
			`const mask = 0xffffffffffffffffn;`,
			`let h0 = 0x6a09e667f3bcc908n;`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/portable-hash", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable hash: %v", mode, err)
		}
		var consumer, hashRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/digest/index":
				hashRuntime = artifact
			}
		}
		if consumer == nil || hashRuntime == nil {
			t.Fatalf("%s did not compile the hash consumer and runtime: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(hashRuntime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s hash output is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableHashDiagnosticsAreModeIndependent(t *testing.T) {
	for _, name := range []string{"md5", "sha1", "sha256", "sha512"} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			source := "import trb/std/digest\ndef bad(): Bytes\n\treturn Digest." + name + "(\"abc\")\nend\n"
			_, err := Compile("bad.trb", []byte(source), mode)
			want := "argument 1 to " + name + "() has type String, expected Bytes"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s %s: expected portable hash argument diagnostic, got %v", mode, name, err)
			}
		}
	}
}

func TestPortableHMACPackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { HMAC } from trb/std/hmac

def digest256(key: Bytes, message: Bytes): Bytes
	return HMAC.sha256(key, message)
end

def digest512(key: Bytes, message: Bytes): Bytes
	return HMAC.sha512(key, message)
end

def matches(left: Bytes, right: Bytes): Boolean
	return HMAC.equal(left, right)
end
`),
	}
	wants := map[string][]string{
		"go": {
			`stdhmac.New(stdsha256.New, key)`,
			`stdhmac.New(stdsha512.New, key)`,
			`stdhmac.Equal(left, right)`,
		},
		"ruby": {
			`OpenSSL::HMAC.digest("SHA256", key, message).b`,
			`OpenSSL::HMAC.digest("SHA512", key, message).b`,
			`difference |= left_byte ^ right_byte`,
		},
		"typescript": {
			`const material = key.byteLength > 64 ? hash(key) : key;`,
			`const material = key.byteLength > 128 ? hash(key) : key;`,
			`difference |= left[index]! ^ right[index]!`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/portable-hmac", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable hmac: %v", mode, err)
		}
		var consumer, hmacRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/hmac/index":
				hmacRuntime = artifact
			}
		}
		if consumer == nil || hmacRuntime == nil {
			t.Fatalf("%s did not compile the hmac consumer and runtime: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(hmacRuntime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s hmac output is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableHMACDiagnosticsAreModeIndependent(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("bad.trb", []byte("import { HMAC } from trb/std/hmac\ndef bad(): Bytes\n\treturn HMAC.sha256(\"key\", \"message\".to_bytes())\nend\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "argument 1 to sha256() has type String, expected Bytes") {
			t.Fatalf("%s: expected portable hmac argument diagnostic, got %v", mode, err)
		}
	}
}

func TestPortableSecureComparePackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { SecureCompare } from trb/std/secure_compare

def matches(left: Bytes, right: Bytes): Boolean
	return SecureCompare.equal(left, right)
end
`),
	}
	wants := map[string]string{
		"go":         `stdhmac.Equal(left, right)`,
		"ruby":       `difference |= left_byte ^ right_byte`,
		"typescript": `difference |= left[index]! ^ right[index]!`,
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/portable-secure-compare", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable secure comparison: %v", mode, err)
		}
		var consumer *Artifact
		for _, artifact := range artifacts {
			if artifact.IR.ModulePath == "main" {
				consumer = artifact
			}
		}
		if consumer == nil || !strings.Contains(string(consumer.Output), wants[mode]) {
			t.Fatalf("generated %s secure comparison is missing %q: %#v", mode, wants[mode], consumer)
		}
	}
}

func TestPortableSecureCompareDiagnosticsAreModeIndependent(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("bad.trb", []byte("import { SecureCompare } from trb/std/secure_compare\ndef bad(): Boolean\n\treturn SecureCompare.equal(\"left\", \"right\".to_bytes())\nend\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "argument 1 to equal() has type String, expected Bytes") {
			t.Fatalf("%s: expected portable secure comparison diagnostic, got %v", mode, err)
		}
	}
}

func TestPortableRandomPackagesLowerAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/random
import trb/std/secure_random

def fraction(): Float
	return Random.float()
end

def index(upper: Integer): Integer
	return Random.integer(upper)
end

def token(length: Integer): Bytes
	return SecureRandom.bytes(length)
end
`),
	}
	wants := map[string][]string{
		"go": {
			`stdrand.Float64()`,
			`stdrand.IntN(upper)`,
			`stdcryptorand.Read(value)`,
		},
		"ruby": {
			`Random.rand()`,
			`Random.rand(upper)`,
			`Random.urandom(length).b`,
		},
		"typescript": {
			`Math.random()`,
			`Math.floor(Math.random() * upper)`,
			`globalThis.crypto.getRandomValues(new Uint8Array(length))`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/portable-random", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable random: %v", mode, err)
		}
		var output string
		for _, artifact := range artifacts {
			if artifact.IR.ModulePath == "main" {
				output = string(artifact.Output)
			}
		}
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s random output is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableRandomDiagnosticsAreModeIndependent(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("bad.trb", []byte("import { Random } from trb/std/random\ndef bad(): Integer\n\treturn Random.integer(1.5)\nend\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "argument 1 to integer() has type Float, expected Integer") {
			t.Fatalf("%s: expected portable random argument diagnostic, got %v", mode, err)
		}
		_, err = Compile("bad.trb", []byte("import { SecureRandom } from trb/std/secure_random\ndef bad(): Bytes\n\treturn SecureRandom.bytes(1.5)\nend\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "argument 1 to bytes() has type Float, expected Integer") {
			t.Fatalf("%s: expected portable secure random argument diagnostic, got %v", mode, err)
		}
	}
}

func TestPortableStringBuilderLowersAcrossBackends(t *testing.T) {
	source := []byte(`import trb/std/string_builder

def render(): String
	mut builder := StringBuilder.new()
	builder.append("A")
	builder.append_codepoint(128512)
	builder.append_codepoint(33)
	return builder.to_s()
end

def measured(): Integer
	mut builder := StringBuilder.from_string("A😀")
	return builder.size()
end

def blank(): Boolean
	builder := StringBuilder.new()
	return builder.empty?()
end

def reset(): String
	mut builder := StringBuilder.from_string("old")
	builder.clear()
	builder.append("new")
	return builder.to_s()
end
`)
	wants := map[string][]string{
		"go": {
			`builder := &strings.Builder{}`,
			`builder.WriteString("A")`,
			`builder.WriteRune(rune(value))`,
			`builder.String()`,
			`utf8.RuneCountInString(builder.String())`,
			`builder.Len() == 0`,
			`builder.Reset()`,
		},
		"ruby": {
			`builder = String.new(encoding: Encoding::UTF_8)`,
			`builder << "A"`,
			`builder << (128512).chr(Encoding::UTF_8)`,
			`builder.dup`,
			`builder.each_codepoint.count`,
			`builder.empty?`,
			`builder.clear`,
		},
		"typescript": {
			`let builder: Array<string> = [];`,
			`builder.push("A")`,
			`builder.push(String.fromCodePoint(value))`,
			`builder.join("")`,
			`Array.from(builder.join("")).length`,
			`builder.length === 0`,
			`builder.splice(0)`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("builder.trb", source, Options{Mode: mode, Package: "builderexample", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable StringBuilder: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s StringBuilder output is missing %q:\n%s", mode, want, output)
			}
		}
		if mode == "go" {
			fileSet := token.NewFileSet()
			parsed, parseErr := parser.ParseFile(fileSet, "builder.go", artifact.Output, parser.AllErrors)
			if parseErr != nil {
				t.Fatalf("generated Go StringBuilder output does not parse: %v\n%s", parseErr, output)
			}
			if _, checkErr := (&gotypes.Config{Importer: importer.Default()}).Check("builderexample", fileSet, []*goast.File{parsed}, nil); checkErr != nil {
				t.Fatalf("generated Go StringBuilder output does not type-check: %v\n%s", checkErr, output)
			}
		}
	}
}

func TestPortableStringBuilderMutabilityAndTypesAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "receiver requires mut",
			source: "import trb/std/string_builder\ndef bad()\n\tbuilder := StringBuilder.new()\n\tbuilder.append(\"x\")\n\treturn\nend\n",
			want:   "builder is immutable; declare it with mut to use append()",
		},
		{
			name:   "clear receiver requires mut",
			source: "import trb/std/string_builder\ndef bad()\n\tbuilder := StringBuilder.new()\n\tbuilder.clear()\n\treturn\nend\n",
			want:   "builder is immutable; declare it with mut to use clear()",
		},
		{
			name:   "append type",
			source: "import trb/std/string_builder\ndef bad()\n\tmut builder := StringBuilder.new()\n\tbuilder.append(1)\n\treturn\nend\n",
			want:   "argument 1 to append() has type Integer, expected String",
		},
		{
			name:   "instance operation is not static",
			source: "import trb/std/string_builder\ndef bad()\n\tmut builder := StringBuilder.new()\n\tStringBuilder.clear(builder)\n\treturn\nend\n",
			want:   "type StringBuilder has no member clear",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s %s: expected %q StringBuilder diagnostic, got %v", mode, test.name, test.want, err)
			}
		}
	}
}

func TestPortableArrayAndHashOperationsLowerAcrossBackends(t *testing.T) {
	source := []byte(`enum QueryState
	Ready
	Done
end

def array_value(): Integer
	values := [1, 2, 3]
	return values.dup()[1] + values.first() + values.last()
end

def array_state(): Boolean
	values := [1]
	return values.size() == 1 && !values.empty?()
end

def grow()
	mut values := [1]
	values.push(2)
	values.push(3)
	return
end

def edge_values(): Integer
	mut values := [2, 3]
	first := values.shift()
	values.unshift(1)
	values.unshift(0)
	reversed := values.reverse()
	return first + reversed.first() + values.reverse().last()
end

def query_values(): Integer
	values := [1, 2, 1]
	if values.include?(2) && values.include?(1)
		return values.count(1) + values.count(2)
	end
	return 0
end

def unique_values(): Array<Integer>
	values := [3, 1, 3, 2, 1]
	return values.uniq().concat([2, 2, 4].uniq())
end

def concatenated_values(): Array<Integer>
	return [1, 2].concat([3, 4])
end

def enum_query(state: QueryState): Boolean
	return [QueryState::Ready, QueryState::Done].include?(state)
end

def float_query(): Boolean
	values := [1.0, 2.0]
	return values.include?(1)
end

def hash_value(): String
	labels: Hash<Integer, String> := {1 => "one", 2 => "two"}
	return labels.fetch(1) + labels.fetch(2)
end

def hash_key(): Integer
	labels: Hash<Integer, String> := {1 => "one"}
	return labels.keys().first()
end

def copied_hash_value(): String
	labels: Hash<Integer, String> := {1 => "one"}
	return labels.dup().values().first()
end

def merged_hash_value(): Integer
	values: Hash<String, Integer> := {"one" => 1, "two" => 2}
	merged := values.merge({"two" => 20, "three" => 3})
	return merged.fetch("two") + values.merge({"four" => 4}).fetch("four") + values.fetch("two")
end

def updated_hash_value(): String
	mut labels: Hash<Integer, String> := {1 => "one", 2 => "two"}
	labels.update({2 => "TWO", 3 => "three"})
	labels.update({4 => "four"})
	first := labels.delete(1)
	second := labels.delete(2)
	return first + second + labels.fetch(3) + labels.fetch(4)
end

def hash_state(): Boolean
	labels: Hash<Integer, String> := {1 => "one"}
	return labels.size() == 1 && !labels.empty?() && labels.key?(1)
end

def string_state(): Boolean
	return "TypeRB".start_with?("Type") && "TypeRB".end_with?("RB")
end

def string_parts(): String
	mut parts := "root/leaf/".split("/")
	tail := parts.pop()
	return parts.join("|") + ":" + tail
end
`)
	wants := map[string][]string{
		"go": {
			`slices.Clone(trbArrayValues_`,
			`trbArrayIndex_`,
			`*values = append(*values, value)`,
			`*values = items[1:]`,
			`copy(items[1:], items[:len(items)-1])`,
			`slices.Reverse(values)`,
			`slices.Contains(trbArrayValues_`,
			`float64(1)`,
			`target := 1`,
			`!slices.Contains(result, value)`,
			`append(slices.Clone(trbArrayValues_`,
			`maps.Keys(labels)`,
			`maps.Values(maps.Clone(labels))`,
			`values := maps.Clone(values)`,
			`maps.Copy(values, map[string]int`,
			`maps.Copy(labels, map[int]string`,
			`delete(values, key)`,
			`panic("Hash key is missing")`,
			`strings.HasPrefix("TypeRB", "Type")`,
			`strings.Split(value, separator)`,
			`strings.Join(trbArrayValues_`,
		},
		"ruby": {
			`values.dup`,
			`raise IndexError, "Array index is out of bounds"`,
			`values << 2`,
			`values.shift`,
			`values.unshift(1)`,
			`values.reverse`,
			`values.include?(2)`,
			`values.include?((1).to_f)`,
			`values.count(1)`,
			`result << value unless result.any? { |known| known == value }`,
			`[1, 2] + [3, 4]`,
			`labels.keys`,
			`labels.dup.values`,
			`values.merge({`,
			`labels.update({`,
			`values.delete(key)`,
			`labels.fetch(1)`,
			`"TypeRB".start_with?("Type")`,
			`value.split(separator, -1)`,
			`parts.join("|")`,
		},
		"typescript": {
			`[...values]`,
			`throw new RangeError("Array index is out of bounds")`,
			`values.push(2)`,
			`values.shift()`,
			`values.unshift(1)`,
			`[...values].reverse()`,
			`values.indexOf(2) >= 0`,
			`values.indexOf(Number(1)) >= 0`,
			`if (value === target)`,
			`if (result.indexOf(value) < 0)`,
			`[...[1, 2], ...[3, 4]]`,
			`Object.keys(labels).map(Number)`,
			`Object.values(({ ...labels }))`,
			`({ ...values, ...{`,
			`Object.assign(labels, {`,
			`Reflect.deleteProperty(__trbValues, __trbKey)`,
			`throw new Error("Hash key is missing")`,
			`"TypeRB".startsWith("Type")`,
			`return value.split(separator);`,
			`parts.join("|")`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := CompileWithOptions("collections.trb", source, Options{Mode: mode, Package: "collectionsexample", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable Array/Hash operations: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("generated %s Array/Hash output is missing %q:\n%s", mode, want, output)
			}
		}
		if mode == "go" {
			fileSet := token.NewFileSet()
			parsed, parseErr := parser.ParseFile(fileSet, "collections.go", artifact.Output, parser.AllErrors)
			if parseErr != nil {
				t.Fatalf("generated Go Array/Hash output does not parse: %v\n%s", parseErr, output)
			}
			if _, checkErr := (&gotypes.Config{Importer: importer.Default()}).Check("collectionsexample", fileSet, []*goast.File{parsed}, nil); checkErr != nil {
				t.Fatalf("generated Go Array/Hash output does not type-check: %v\n%s", checkErr, output)
			}
		}
	}
}

func TestGoArrayIndexRuntimesArePrivateToGeneratedModules(t *testing.T) {
	root := t.TempDir()
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename:   filepath.Join(root, "domain", "first.trb"),
			ModulePath: "domain/first",
			Package:    "domain",
			Source:     []byte("def first(values: Array<Integer>): Integer\n\treturn values[0]\nend\n"),
		},
		{
			Filename:   filepath.Join(root, "domain", "second.trb"),
			ModulePath: "domain/second",
			Package:    "domain",
			Source:     []byte("def second(values: Array<Integer>): Integer\n\tmut copied := values.dup()\n\tcopied[0] = 2\n\treturn copied[0]\nend\n"),
		},
	}, Options{Mode: "go", GoModule: "example.com/private-runtime"})
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	files := make([]*goast.File, 0, len(artifacts))
	helperNames := map[string]bool{}
	for _, artifact := range artifacts {
		parsed, parseErr := parser.ParseFile(fileSet, artifact.Filename, artifact.Output, parser.AllErrors)
		if parseErr != nil {
			t.Fatalf("generated Go does not parse: %v\n%s", parseErr, artifact.Output)
		}
		files = append(files, parsed)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*goast.FuncDecl)
			if ok && strings.HasPrefix(function.Name.Name, "trbArrayIndex_") {
				helperNames[function.Name.Name] = true
			}
		}
	}
	if len(helperNames) != 2 {
		t.Fatalf("array index helpers must be module-private, got %#v", helperNames)
	}
	if _, checkErr := (&gotypes.Config{Importer: importer.Default()}).Check("example.com/private-runtime/domain", fileSet, files, nil); checkErr != nil {
		t.Fatalf("generated package does not type-check: %v", checkErr)
	}
}

func TestGoReceiverIntrinsicsSurviveImportsWithinOnePackage(t *testing.T) {
	root := t.TempDir()
	artifacts, err := CompileProject([]SourceUnit{
		{
			Filename:   filepath.Join(root, "domain", "result.trb"),
			ModulePath: "domain/result",
			Package:    "domain",
			Source:     []byte("record Parsed\n\tvalues: Array<String>\nend\n"),
		},
		{
			Filename:   filepath.Join(root, "domain", "parser.trb"),
			ModulePath: "domain/parser",
			Package:    "domain",
			Source:     []byte("import { Parsed } from domain/result\n\ndef parse_values(): Parsed\n\treturn Parsed.new(values: [\"one\", \"two\"])\nend\n"),
		},
		{
			Filename:   filepath.Join(root, "domain", "consumer.trb"),
			ModulePath: "domain/consumer",
			Package:    "domain",
			Source:     []byte("import { parse_values } from domain/parser\n\ndef value_count(): Integer\n\tparsed := parse_values()\n\treturn parsed.values.size()\nend\n"),
		},
	}, Options{Mode: "go", GoModule: "example.com/package-receiver"})
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	files := make([]*goast.File, 0, len(artifacts))
	foundLength := false
	consumerOutput := ""
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath == "domain/consumer" {
			consumerOutput = string(artifact.Output)
			foundLength = strings.Contains(consumerOutput, "len(trbArrayValues_")
		}
		parsed, parseErr := parser.ParseFile(fileSet, artifact.Filename, artifact.Output, parser.AllErrors)
		if parseErr != nil {
			t.Fatalf("generated Go does not parse: %v\n%s", parseErr, artifact.Output)
		}
		files = append(files, parsed)
	}
	if !foundLength {
		t.Fatalf("same-package import lost the Array#size intrinsic:\n%s", consumerOutput)
	}
	if _, checkErr := (&gotypes.Config{Importer: importer.Default()}).Check("example.com/package-receiver/domain", fileSet, files, nil); checkErr != nil {
		t.Fatalf("generated package does not type-check: %v", checkErr)
	}
}

func TestGoStringWhitespaceImportDoesNotConflictWithUnicodePackage(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{{
		Filename:   "text.trb",
		ModulePath: "text",
		Package:    "text",
		Source: []byte(`import trb/std/unicode

def blank?(value: String): Boolean
	return value.strip().empty?() || Unicode.whitespace(32)
end
`),
	}}, Options{Mode: "go", GoModule: "example.com/unicode-import"})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath == "text" {
			output = string(artifact.Output)
			break
		}
	}
	if output == "" {
		t.Fatal("consumer artifact is missing")
	}
	for _, want := range []string{`stdunicode "unicode"`, `stdunicode.IsSpace`, `"example.com/unicode-import/trb/std/unicode"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated Go is missing %q:\n%s", want, output)
		}
	}
}

func TestPortableArrayAndHashDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "uniq requires portable equality",
			source: "record Item\n\tname: String\nend\ndef bad(values: Array<Item>): Array<Item>\n\treturn values.uniq()\nend\n",
			want:   "portable equality is not defined for Item, required by uniq()",
		},
		{
			name:   "concat keeps element type exact",
			source: "def bad(values: Array<Integer>): Array<Integer>\n\treturn values.concat([\"two\"])\nend\n",
			want:   "argument 1 to concat() has type Array<String>, expected Array<Integer>",
		},
		{
			name:   "receiver push type",
			source: "def bad()\n\tmut values := [1]\n\tvalues.push(\"two\")\n\treturn\nend\n",
			want:   "argument 1 to push() has type String, expected Integer",
		},
		{
			name:   "arrays package is not public",
			source: "import trb/std/arrays\n",
			want:   "unknown TypeRB package trb/std/arrays",
		},
		{
			name:   "receiver push requires mut",
			source: "def bad()\n\tvalues := [1]\n\tvalues.push(2)\n\treturn\nend\n",
			want:   "values is immutable; declare it with mut to use push()",
		},
		{
			name:   "hash receiver key type",
			source: "def bad(): String\n\tlabels: Hash<Integer, String> := {1 => \"one\"}\n\treturn labels.fetch(\"1\")\nend\n",
			want:   "argument 1 to fetch() has type String, expected Integer",
		},
		{
			name:   "hashes package is not public",
			source: "import trb/std/hashes\n",
			want:   "unknown TypeRB package trb/std/hashes",
		},
		{
			name:   "hash delete requires mut",
			source: "def bad(): String\n\tlabels: Hash<Integer, String> := {1 => \"one\"}\n\treturn labels.delete(1)\nend\n",
			want:   "labels is immutable; declare it with mut to use delete()",
		},
		{
			name:   "hash merge keeps value type exact",
			source: "def bad(): Hash<String, Float>\n\tvalues: Hash<String, Float> := {\"one\" => 1.0}\n\tother: Hash<String, Integer> := {\"two\" => 2}\n\treturn values.merge(other)\nend\n",
			want:   "argument 1 to merge() has type Hash<String, Integer>, expected Hash<String, Float>",
		},
		{
			name:   "hash update key type",
			source: "def bad()\n\tmut values: Hash<String, Integer> := {\"one\" => 1}\n\tvalues.update({2 => 2})\n\treturn\nend\n",
			want:   "argument 1 to update() has type Hash<Integer, Integer>, expected Hash<String, Integer>",
		},
		{
			name:   "unresolved hash value type",
			source: "def bad(): Array<String>\n\treturn {}.values()\nend\n",
			want:   "cannot infer V for values()",
		},
		{
			name:   "join receiver element type",
			source: "def bad(): String\n\treturn [1, 2].join(\",\")\nend\n",
			want:   "type Array<Integer> has no member join",
		},
		{
			name:   "pop requires mut",
			source: "def bad(): Integer\n\tvalues := [1]\n\treturn values.pop()\nend\n",
			want:   "values is immutable; declare it with mut to use pop()",
		},
		{
			name:   "shift requires mut",
			source: "def bad(): Integer\n\tvalues := [1]\n\treturn values.shift()\nend\n",
			want:   "values is immutable; declare it with mut to use shift()",
		},
		{
			name:   "unshift value type",
			source: "def bad()\n\tmut values := [1]\n\tvalues.unshift(\"zero\")\n\treturn\nend\n",
			want:   "argument 1 to unshift() has type String, expected Integer",
		},
		{
			name:   "include value type",
			source: "def bad(): Boolean\n\treturn [1].include?(\"one\")\nend\n",
			want:   "argument 1 to include?() has type String, expected Integer",
		},
		{
			name:   "receiver equality requirement",
			source: "def bad(): Boolean\n\treturn [[1]].include?([1])\nend\n",
			want:   "portable equality is not defined for Array<Integer>, required by include?()",
		},
		{
			name:   "payload enum equality requirement",
			source: "enum Box\n\tValue(value: Integer)\nend\ndef bad(value: Box): Boolean\n\treturn [value].include?(value)\nend\n",
			want:   "portable equality is not defined for Box, required by include?()",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s %s: expected %q Array/Hash diagnostic, got %v", mode, test.name, test.want, err)
			}
		}
	}
}

func TestCompilerOwnedPortablePathPackageLowersAcrossBackends(t *testing.T) {
	consumerSource := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/path

def normalized(): String
	return Path.clean("a/./b/../c")
end

def joined(): String
	return Path.join("/srv/app", "../data")
end

def inspected(): Boolean
	return Path.absolute("/srv/app") && Path.base("/srv/app/main.trb") == "main.trb" && Path.directory("/srv/app/main.trb") == "/srv/app"
end

def parts(): Array<String>
	return Path.components("/srv/app/main.trb")
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{consumerSource}, Options{Mode: mode, GoModule: "example.com/path-app", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected compiler-owned path package: %v", mode, err)
		}
		var consumer, runtime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/path/index":
				runtime = artifact
			}
		}
		if consumer == nil || runtime == nil {
			t.Fatalf("%s did not emit consumer and path runtime: %#v", mode, artifacts)
		}
		consumerWants := map[string][]string{
			"go": {
				`import "example.com/path-app/trb/std/path"`,
				`path.Clean("a/./b/../c")`,
				`path.Components("/srv/app/main.trb")`,
			},
			"ruby": {
				`require_relative "./trb/std/path/index"`,
				`Path.clean("a/./b/../c")`,
				`Path.components("/srv/app/main.trb")`,
			},
			"typescript": {
				`import * as __trb_path from "./trb/std/path/index.ts";`,
				`__trb_path.clean("a/./b/../c")`,
				`__trb_path.components("/srv/app/main.trb")`,
			},
		}[mode]
		for _, want := range consumerWants {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s path consumer is missing %q:\n%s", mode, want, output)
			}
		}
		runtimeWants := map[string][]string{
			"go":         {`func PathClean(value string) string`, `func Components(value string) *[]string`, `*values = append(*values, value)`},
			"ruby":       {`class Path`, `def self.clean(value)`, `def components(value)`},
			"typescript": {`export class Path`, `static clean(value: string): string`, `export function components(value: string): Array<string>`},
		}[mode]
		for _, want := range runtimeWants {
			if output := string(runtime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s path runtime is missing %q:\n%s", mode, want, output)
			}
		}
		if mode == "go" {
			fileSet := token.NewFileSet()
			parsed, parseErr := parser.ParseFile(fileSet, "path.go", runtime.Output, parser.AllErrors)
			if parseErr != nil {
				t.Fatalf("generated Go path runtime does not parse: %v\n%s", parseErr, runtime.Output)
			}
			if _, checkErr := (&gotypes.Config{Importer: importer.Default()}).Check("path", fileSet, []*goast.File{parsed}, nil); checkErr != nil {
				t.Fatalf("generated Go path runtime does not type-check: %v\n%s", checkErr, runtime.Output)
			}
		}
	}
}

func TestPortablePathDiagnosticsAreModeIndependent(t *testing.T) {
	source := []byte("import trb/std/path\ndef bad(): String\n\treturn Path.clean(1)\nend\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("bad.trb", source, mode); err == nil || !strings.Contains(err.Error(), "argument 1 to clean() has type Integer, expected String") {
			t.Fatalf("%s did not reject invalid path argument: %v", mode, err)
		}
	}
}

func TestPortableURLComponentsLowerAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result
import trb/std/url

def encoded(value: String): String
	return URL.encode_component(value)
end

def decoded(value: String): Result<String, URL::DecodeError>
	return URL.decode_component(value)
end
`),
	}
	wants := map[string][]string{
		"go": {
			`const hexadecimal = "0123456789ABCDEF"`,
			`utf8.Valid(value)`,
			`URLDecodeErrorKindInvalidescape`,
			`URLDecodeErrorKindInvalidutf8`,
		},
		"ruby": {
			`format("%%%02X", byte)`,
			`bytes.pack("C*").force_encoding(Encoding::UTF_8)`,
			`URL::DecodeErrorKind::InvalidEscape`,
			`URL::DecodeErrorKind::InvalidUtf8`,
		},
		"typescript": {
			`new TextEncoder().encode(value)`,
			`new TextDecoder("utf-8", { fatal: true })`,
			`URLDecodeErrorKind.InvalidEscape`,
			`URLDecodeErrorKind.InvalidUtf8`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/portable-url", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable URL components: %v", mode, err)
		}
		var consumer, resultRuntime, urlRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/result/index":
				resultRuntime = artifact
			case "trb/std/url/index":
				urlRuntime = artifact
			}
		}
		if consumer == nil || resultRuntime == nil || urlRuntime == nil {
			t.Fatalf("%s did not compile the URL consumer and runtimes: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(urlRuntime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s URL output is missing %q:\n%s", mode, want, output)
			}
		}
		errorWants := map[string][]string{
			"go":         {"type URLDecodeErrorKind int", "type URLDecodeError struct"},
			"ruby":       {"module URL", "DecodeErrorKind = Data.define(:name)", "DecodeError = Data.define(:kind, :input, :message)"},
			"typescript": {"export type URLDecodeErrorKind", "export interface URLDecodeError"},
		}[mode]
		for _, want := range errorWants {
			if output := string(urlRuntime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s URL error runtime is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableURLComponentDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "import trb/std/url\ndef bad(): String\n\treturn URL.encode_component(1)\nend\n", want: "argument 1 to encode_component() has type Integer, expected String"},
		{source: "import trb/std/url\ndef bad()\n\tURL.decode_component(1)\n\treturn\nend\n", want: "argument 1 to decode_component() has type Integer, expected String"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s: expected %q URL component diagnostic, got %v", mode, test.want, err)
			}
		}
	}
}

func TestPortableURLQueryCompilesFromSharedSourceAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result
import trb/std/url

def parsed(value: String): Result<Array<URL::QueryParameter>, URL::DecodeError>
	return URL.parse_query(value)
end

def built(parameters: Array<URL::QueryParameter>): String
	return URL.build_query(parameters)
end
`),
	}
	wants := map[string][]string{
		"go": {
			`type URLQueryParameter struct`,
			`func parseQueryParameter(value string)`,
			`func ParseQuery(value string)`,
			`func BuildQuery(parameters *[]URLQueryParameter) string`,
			`DecodeComponent(strings.Join`,
			`encoded := EncodeComponent(value)`,
		},
		"ruby": {
			`QueryParameter = Data.define(:name, :value)`,
			`def __trb_private_`,
			`def parse_query(value)`,
			`def build_query(parameters)`,
			`case (__trb_case1 = decode_component(`,
			`invalid percent escape in URL query component`,
			`encoded = encode_component(value)`,
		},
		"typescript": {
			`export interface URLQueryParameter`,
			`function _parse_query_parameter(value: string)`,
			`export function parse_query(value: string)`,
			`export function build_query(parameters: Array<URLQueryParameter>): string`,
			`const __trbCase1 = decode_component(`,
			`invalid percent escape in URL query component`,
			`const encoded: string = encode_component(value)`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/portable-url-query", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable URL query operations: %v", mode, err)
		}
		urlRuntime := findArtifactByModule(artifacts, "trb/std/url/index")
		if urlRuntime == nil {
			t.Fatalf("%s did not emit the URL runtime: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(urlRuntime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s URL query output is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableURLQueryDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "import trb/std/url\ndef bad()\n\tURL.parse_query(1)\n\treturn\nend\n", want: "argument 1 to parse_query() has type Integer, expected String"},
		{source: "import trb/std/url\ndef bad(): String\n\treturn URL.build_query([1])\nend\n", want: "argument 1 to build_query() has type Array<Integer>, expected Array<URL::QueryParameter>"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s: expected %q URL query diagnostic, got %v", mode, test.want, err)
			}
		}
	}
}

func TestCompilerOwnedUnicodePackageLowersSameTablesAcrossBackends(t *testing.T) {
	consumerSource := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/unicode

def classified(): Boolean
	return Unicode.letter(65) && Unicode.letter(12354) && Unicode.digit(1632) && Unicode.uppercase(65) && Unicode.lowercase(97) && Unicode.whitespace(12288)
end

def identifiers(): Boolean
	return Unicode.identifier_start(64) && Unicode.identifier_start(12354) && Unicode.identifier_part(1632)
end

def scalar(): String
	return Unicode.from_codepoint(128512)
end

def string_methods(): Boolean
	return "A😀".codepoints().size() == 2 && "".empty?() && "TypeRB".include?("RB") && "ada".upcase() == "ADA"
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{consumerSource}, Options{Mode: mode, GoModule: "example.com/unicode-app", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected compiler-owned Unicode package: %v", mode, err)
		}
		var consumer, runtime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/unicode/index":
				runtime = artifact
			}
		}
		if consumer == nil || runtime == nil {
			t.Fatalf("%s did not emit consumer and Unicode runtime: %#v", mode, artifacts)
		}
		consumerWants := map[string][]string{
			"go": {
				`import "example.com/unicode-app/trb/std/unicode"`,
				`unicode.UnicodeLetter(65)`,
				`unicode.UnicodeFromCodepoint(128512)`,
				`func(value string) []int`,
			},
			"ruby": {
				`require_relative "./trb/std/unicode/index"`,
				`Unicode.letter(65)`,
				`Unicode.from_codepoint(128512)`,
				`"A😀".codepoints`,
			},
			"typescript": {
				`import * as __trb_unicode from "./trb/std/unicode/index.ts";`,
				`__trb_unicode.Unicode.letter(65)`,
				`__trb_unicode.Unicode.from_codepoint(128512)`,
				`Array.from("A😀", (value): number => value.codePointAt(0)!)`,
			},
		}[mode]
		for _, want := range consumerWants {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s Unicode consumer is missing %q:\n%s", mode, want, output)
			}
		}
		runtimeWants := map[string][]string{
			"go":         {`var UnicodeDataVersion string = "17.0.0"`, `func UnicodeLetter(value int) bool`, `func inRanges(value int, ranges *[]*[]int) bool`},
			"ruby":       {`class Unicode`, `UNICODE_DATA_VERSION = "17.0.0"`, `def self.letter(value)`, `def __trb_private_`},
			"typescript": {`export class Unicode`, `export const UNICODE_DATA_VERSION: string = "17.0.0";`, `static letter(value: number): boolean`, `export function _in_ranges(value: number, ranges: Array<Array<number>>): boolean`},
		}[mode]
		for _, want := range runtimeWants {
			if output := string(runtime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s Unicode runtime is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestUnicodePackageDiagnosticsAreModeIndependent(t *testing.T) {
	wrongType := []byte("import trb/std/unicode\ndef bad(): Boolean\n\treturn Unicode.letter(\"A\")\nend\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("bad.trb", wrongType, mode); err == nil || !strings.Contains(err.Error(), "argument 1 to letter() has type String, expected Integer") {
			t.Fatalf("%s: expected Unicode argument diagnostic, got %v", mode, err)
		}
	}
}

func TestCompilerOwnedUnicodeSupportsNamedImportsAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/named.trb",
		ModulePath: "named",
		Package:    "named",
		Source: []byte(`import { Unicode } from trb/std/unicode

def accepted(): Boolean
	return Unicode.letter(12354)
end

def character(): String
	return Unicode.from_codepoint(128512)
end
`),
	}
	wants := map[string][]string{
		"go":         {`import "example.com/unicode-named/trb/std/unicode"`, `return unicode.UnicodeLetter(12354)`, `return unicode.UnicodeFromCodepoint(128512)`},
		"ruby":       {`require_relative "./trb/std/unicode/index"`, `return Unicode.letter(12354)`, `return Unicode.from_codepoint(128512)`},
		"typescript": {`import * as __trb_unicode from "./trb/std/unicode/index.ts";`, `return __trb_unicode.Unicode.letter(12354);`, `return __trb_unicode.Unicode.from_codepoint(128512);`},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/unicode-named", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected named Unicode imports: %v", mode, err)
		}
		for _, artifact := range artifacts {
			if artifact.IR.ModulePath != "named" {
				continue
			}
			for _, want := range wants[mode] {
				if output := string(artifact.Output); !strings.Contains(output, want) {
					t.Fatalf("generated %s named Unicode consumer is missing %q:\n%s", mode, want, output)
				}
			}
		}
	}
}

func TestPortableResultPackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result

def successful(): Result<Integer, String>
	return Result<Integer, String>::Ok(7)
end

def unwrap(value: Result<Integer, String>): Integer
	case value
	when Result::Ok(number)
		return number
	when Result::Err(_)
		return 0
	end
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/result-app", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected the standard Result package: %v", mode, err)
		}
		var consumer, runtime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/result/index":
				runtime = artifact
			}
		}
		if consumer == nil || runtime == nil {
			t.Fatalf("%s did not compile both the consumer and Result runtime: %#v", mode, artifacts)
		}
		consumerWants := map[string][]string{
			"go":         {`__trb_result "example.com/result-app/trb/std/result"`, "__trb_result.Result[int, string]", "__trb_result.NewResultOk[int, string](7)", "__trbCase1.Kind == __trb_result.ResultOkTag"},
			"ruby":       {`require_relative "./trb/std/result/index"`, "Result::Ok.new(7)", "when Result::Ok"},
			"typescript": {`import { Result } from "./trb/std/result/index.ts";`, "Result<number, string>", "Result.Ok<number, string>(7)", `__trbCase1.kind === "Ok"`},
		}[mode]
		for _, want := range consumerWants {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s Result consumer is missing %q:\n%s", mode, want, output)
			}
		}
		runtimeWants := map[string][]string{
			"go":         {"type Result[T any, E any] struct", "func NewResultOk[T any, E any](value T) Result[T, E]"},
			"ruby":       {"module Result", "Ok = Data.define(:value)", "Err = Data.define(:error)"},
			"typescript": {"export type Result<T, E> =", "export const Result = Object.freeze({", "Ok: <T, E>(value: T): Result<T, E>"},
		}[mode]
		for _, want := range runtimeWants {
			if output := string(runtime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s Result runtime is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableUnitValueLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Unit } from trb/std/unit

def completed(): Unit
	return Unit.new()
end
`),
	}
	wants := map[string]string{
		"go":         "return unit.Unit{}",
		"ruby":       "return Unit.new",
		"typescript": "return ({} satisfies Unit);",
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/unit-app", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected Unit: %v", mode, err)
		}
		var consumer, runtime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/unit/index":
				runtime = artifact
			}
		}
		if consumer == nil || runtime == nil {
			t.Fatalf("%s did not compile the Unit consumer and runtime: %#v", mode, artifacts)
		}
		if output := string(consumer.Output); !strings.Contains(output, wants[mode]) {
			t.Fatalf("generated %s Unit consumer is missing %q:\n%s", mode, wants[mode], output)
		}
	}
}

func TestPortableResultPackageDiagnostics(t *testing.T) {
	wrongArity := []byte(`import { Result } from trb/std/result

def bad(value: Result<Integer>)
	return
end
`)
	if _, err := Compile("bad.trb", wrongArity, "go"); err == nil || !strings.Contains(err.Error(), "Result expects 2 type argument(s), got 1") {
		t.Fatalf("expected standard Result type-arity diagnostic, got %v", err)
	}

	wrongPayload := []byte(`import { Result } from trb/std/result

def bad(): Result<Integer, String>
	return Result<Integer, String>::Ok("not an integer")
end
`)
	if _, err := Compile("bad.trb", wrongPayload, "typescript"); err == nil || !strings.Contains(err.Error(), "argument 1 to Result::Ok() has type String, expected Integer") {
		t.Fatalf("expected standard Result payload diagnostic, got %v", err)
	}
}

func TestSafePortableConversionAndLookupLowerAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result
import { IndexLookupError, KeyLookupError, NumberParseError } from trb/std/errors

def parsed(value: String): Result<Integer, NumberParseError>
	return value.try_to_i()
end

def parsed_float(value: String): Result<Float, NumberParseError>
	return value.try_to_f()
end

def float_value(value: String): Float
	return value.to_f()
end

def string_value(value: String, index: Integer): Result<String, IndexLookupError>
	return value.try_fetch(index)
end

def characters(value: String): Array<String>
	return value.chars()
end

def reversed(value: String): String
	return value.reverse()
end

def array_value(values: Array<Integer>, index: Integer): Result<Integer, IndexLookupError>
	return values.try_fetch(index)
end

def hash_value(values: Hash<String, Integer>, key: String): Result<Integer, KeyLookupError>
	return values.try_fetch(key)
end
`),
	}

	wants := map[string][]string{
		"go": {
			`regexp.MatchString`,
			`panic("invalid Float")`,
			`__trb_errors.NumberParseError{Kind: __trb_errors.NumberParseErrorKindInvalidformat, Input: input, Message: "invalid Integer"}`,
			`__trb_errors.NumberParseError{Kind: __trb_errors.NumberParseErrorKindInvalidformat, Input: input, Message: "invalid Float"}`,
			`__trb_errors.IndexLookupError{Index: requested, Size: len(values), Message: "Array index is out of bounds"}`,
			`__trb_errors.IndexLookupError{Index: requested, Size: len(value), Message: "String index is out of bounds"}`,
			`slices.Reverse(characters)`,
			`__trb_errors.KeyLookupError{Key: key, Message: "Hash key is missing"}`,
		},
		"ruby": {
			`raise ArgumentError, "invalid Float"`,
			`NumberParseError.new(kind: NumberParseErrorKind::InvalidFormat, input: input, message: "invalid Integer")`,
			`NumberParseError.new(kind: NumberParseErrorKind::InvalidFormat, input: input, message: "invalid Float")`,
			`IndexLookupError.new(index: requested, size: values.length, message: "Array index is out of bounds")`,
			`IndexLookupError.new(index: requested, size: characters.length, message: "String index is out of bounds")`,
			`.each_char.to_a.reverse.join`,
			`KeyLookupError.new(key: key, message: "Hash key is missing")`,
		},
		"typescript": {
			`throw new Error("invalid Float")`,
			`{ kind: NumberParseErrorKind.InvalidFormat, input: __trbInput, message: "invalid Integer" } satisfies NumberParseError`,
			`{ kind: NumberParseErrorKind.InvalidFormat, input: __trbInput, message: "invalid Float" } satisfies NumberParseError`,
			`{ index: __trbRequested, size: __trbValues.length, message: "Array index is out of bounds" } satisfies IndexLookupError`,
			`{ index: __trbRequested, size: __trbValue.length, message: "String index is out of bounds" } satisfies IndexLookupError`,
			`.reverse().join("")`,
			`{ key: __trbKey, message: "Hash key is missing" } satisfies KeyLookupError`,
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/safe-values", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected safe portable operations: %v", mode, err)
		}
		var consumer, resultRuntime, errorRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/result/index":
				resultRuntime = artifact
			case "trb/std/errors/index":
				errorRuntime = artifact
			}
		}
		if consumer == nil || resultRuntime == nil || errorRuntime == nil {
			t.Fatalf("%s did not compile the consumer and structured error runtimes: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s safe operation is missing %q:\n%s", mode, want, output)
			}
		}
		errorWants := map[string][]string{
			"go":         {"type NumberParseErrorKind int", "type NumberParseError struct", "type IndexLookupError struct", "type KeyLookupError struct"},
			"ruby":       {"NumberParseError = Data.define(:kind, :input, :message)", "IndexLookupError = Data.define(:index, :size, :message)", "KeyLookupError = Data.define(:key, :message)"},
			"typescript": {"export type NumberParseErrorKind", "export interface NumberParseError", "export interface IndexLookupError", "export interface KeyLookupError"},
		}[mode]
		for _, want := range errorWants {
			if output := string(errorRuntime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s error runtime is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableSliceAndStringSearchLowerAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result
import { SliceRangeError } from trb/std/errors

def values_slice(values: Array<Integer>, bounds: Range<Integer>): Array<Integer>
	return values.slice(bounds)
end

def safe_values_slice(values: Array<Integer>, bounds: Range<Integer>): Result<Array<Integer>, SliceRangeError>
	return values.try_slice(bounds)
end

def text_slice(value: String, bounds: Range<Integer>): String
	return value.slice(bounds)
end

def safe_text_slice(value: String, bounds: Range<Integer>): Result<String, SliceRangeError>
	return value.try_slice(bounds)
end

def first(value: String, substring: String): Integer?
	return value.index(substring)
end

def last(value: String, substring: String): Integer?
	return value.rindex(substring)
end

def character(value: String, index: Integer): String
	return value[index]
end
`),
	}
	wants := map[string][]string{
		"go": {
			`bounds [3]int`,
			`SliceRangeError{Start: start, Finish: end`,
			`characters, needle := []rune(value), []rune(substring)`,
			`characters := []rune(value)`,
		},
		"ruby": {
			`range.exclude_end?`,
			`SliceRangeError.new(start: start, finish: finish`,
			`needle = substring.each_char.to_a`,
			`characters = value.each_char.to_a`,
		},
		"typescript": {
			`bounds: [number, number, boolean]`,
			`{ start: __trbStart, finish: __trbEnd, exclusive: __trbExclusive`,
			`const needle = Array.from(substring)`,
			`const characters = Array.from(value)`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/slices", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected portable slicing: %v", mode, err)
		}
		var consumer, errorsRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/errors/index":
				errorsRuntime = artifact
			}
		}
		if consumer == nil || errorsRuntime == nil {
			t.Fatalf("%s did not compile slice consumer and error runtime", mode)
		}
		for _, want := range wants[mode] {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s slicing is missing %q:\n%s", mode, want, output)
			}
		}
		if output := string(errorsRuntime.Output); !strings.Contains(output, "SliceRangeError") {
			t.Fatalf("generated %s errors runtime lacks SliceRangeError:\n%s", mode, output)
		}
	}
}

func TestPortableSliceAPIsRejectNonCanonicalFormsAcrossBackends(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		returnType string
		want       string
	}{
		{name: "range index", expression: `"abc"[0...1]`, returnType: "String", want: "String index must be Integer"},
		{name: "integer slice", expression: `"abc".slice(0)`, returnType: "String", want: "expected Range<Integer>"},
		{name: "removed string fetch", expression: `"abc".fetch(0)`, returnType: "String", want: "type String has no member fetch"},
		{name: "removed array fetch", expression: `[1].fetch(0)`, returnType: "Integer", want: "type Array<Integer> has no member fetch"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				source := SourceUnit{
					Filename:   "/project/main.trb",
					ModulePath: "main",
					Package:    "main",
					Source:     []byte("def value(): " + test.returnType + "\n\treturn " + test.expression + "\nend\n"),
				}
				_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/slice-diagnostics", RubyLoader: "require_relative"})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("expected %q diagnostic, got %v", test.want, err)
				}
			})
		}
	}
}

func TestStructuredSafeOperationsRejectStringErrorAnnotationsAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "number parse", source: `"nope".try_to_i()`, want: "Result<Integer, NumberParseError>"},
		{name: "float parse", source: `"nope".try_to_f()`, want: "Result<Float, NumberParseError>"},
		{name: "array lookup", source: `[1].try_fetch(9)`, want: "Result<Integer, IndexLookupError>"},
		{name: "hash lookup", source: `{"name" => "Ada"}.try_fetch("missing")`, want: "Result<String, KeyLookupError>"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			source := SourceUnit{
				Filename:   "/project/main.trb",
				ModulePath: "main",
				Package:    "main",
				Source:     []byte("import { Result } from trb/std/result\n\ndef value(): Result<Integer, String>\n\treturn " + test.source + "\nend\n"),
			}
			if test.name == "hash lookup" {
				source.Source = []byte("import { Result } from trb/std/result\n\ndef value(): Result<String, String>\n\treturn " + test.source + "\nend\n")
			}
			_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/structured-error-diagnostic", RubyLoader: "require_relative"})
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "expected Result") {
				t.Fatalf("%s %s: expected structured error result diagnostic, got %v", mode, test.name, err)
			}
		}
	}
}

func TestInferredResultOperationsLoadTheirRuntimeAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`def inspect_results()
	puts("not-an-integer".try_to_i())
	puts("not-a-float".try_to_f())
	puts("A".try_fetch(9))
	puts([1, 2].try_fetch(9))
	puts({"name" => "Ada"}.try_fetch("missing"))
	return
end
`),
	}

	wants := map[string][]string{
		"go": {
			`__trb_result "example.com/inferred-results/trb/std/result"`,
			`__trb_errors "example.com/inferred-results/trb/std/errors"`,
			`__trb_errors.NumberParseError{Kind: __trb_errors.NumberParseErrorKindInvalidformat, Input: input, Message: "invalid Integer"}`,
			`__trb_errors.NumberParseError{Kind: __trb_errors.NumberParseErrorKindInvalidformat, Input: input, Message: "invalid Float"}`,
			`__trb_errors.IndexLookupError{Index: requested, Size: len(value), Message: "String index is out of bounds"}`,
			`__trb_errors.IndexLookupError{Index: requested, Size: len(values), Message: "Array index is out of bounds"}`,
			`__trb_errors.KeyLookupError{Key: key, Message: "Hash key is missing"}`,
		},
		"ruby": {
			`require_relative "./trb/std/result/index"`,
			`require_relative "./trb/std/errors/index"`,
			`NumberParseError.new(kind: NumberParseErrorKind::InvalidFormat, input: input, message: "invalid Integer")`,
			`NumberParseError.new(kind: NumberParseErrorKind::InvalidFormat, input: input, message: "invalid Float")`,
			`IndexLookupError.new(index: requested, size: characters.length, message: "String index is out of bounds")`,
			`IndexLookupError.new(index: requested, size: values.length, message: "Array index is out of bounds")`,
			`KeyLookupError.new(key: key, message: "Hash key is missing")`,
		},
		"typescript": {
			`import { Result } from "./trb/std/result/index.ts";`,
			`import { NumberParseErrorKind } from "./trb/std/errors/index.ts";`,
			`{ kind: NumberParseErrorKind.InvalidFormat, input: __trbInput, message: "invalid Integer" } satisfies NumberParseError`,
			`{ kind: NumberParseErrorKind.InvalidFormat, input: __trbInput, message: "invalid Float" } satisfies NumberParseError`,
			`{ index: __trbRequested, size: __trbValue.length, message: "String index is out of bounds" } satisfies IndexLookupError`,
			`{ index: __trbRequested, size: __trbValues.length, message: "Array index is out of bounds" } satisfies IndexLookupError`,
			`{ key: __trbKey, message: "Hash key is missing" } satisfies KeyLookupError`,
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/inferred-results", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected inferred Result operations: %v", mode, err)
		}
		var consumer, resultRuntime, errorRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/result/index":
				resultRuntime = artifact
			case "trb/std/errors/index":
				errorRuntime = artifact
			}
		}
		if consumer == nil || resultRuntime == nil || errorRuntime == nil {
			t.Fatalf("%s did not compile the consumer and inferred structured error runtimes: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s inferred Result operation is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestInferredStructuredErrorFieldsAreAvailableWithoutErrorImports(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result

def inspect()
	parsed := "nope".try_to_i()
	case parsed
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.input + ":" + error.message)
	end

	indexed := [1].try_fetch(9)
	case indexed
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.index + error.size)
	end

	keyed := {"name" => "Ada"}.try_fetch("missing")
	case keyed
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		case error.key
		when Integer(key)
			puts(key)
		when String(key)
			puts(key)
		end
	end
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/inferred-structured-fields", RubyLoader: "require_relative"}); err != nil {
			t.Fatalf("%s rejected inferred structured error fields: %v", mode, err)
		}
	}
}

func TestTypeScriptEmitsRootRuntimeOnceForNestedTypes(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/filesystem
import { Result } from trb/std/result

def load(path: String): Result<String, FileSystem::Error>
	return FileSystem.read_text(path)
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript"})
	if err != nil {
		t.Fatalf("compile root and support type imports: %v", err)
	}
	consumer := findArtifactByModule(artifacts, "main")
	if consumer == nil {
		t.Fatal("missing consumer artifact")
	}
	const runtimeImport = `import * as __trb_filesystem from "./trb/std/filesystem/index.ts";`
	if count := strings.Count(string(consumer.Output), runtimeImport); count != 1 {
		t.Fatalf("filesystem runtime import count = %d, want 1:\n%s", count, consumer.Output)
	}
}

func TestStructuredErrorTypeNamesStillRequireExplicitImports(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source:     []byte("def inspect(error: NumberParseError)\n\tputs(error.message)\n\treturn\nend\n"),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/structured-error-import", RubyLoader: "require_relative"})
		if err == nil || !strings.Contains(err.Error(), "type NumberParseError is not declared or imported") {
			t.Fatalf("%s: expected explicit structured error import diagnostic, got %v", mode, err)
		}
	}
}

func TestPortableFilesystemPackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/filesystem
import { Result } from trb/std/result

def load(path: String): Result<String, FileSystem::Error>
	return FileSystem.read_text(path)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/filesystem-app", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected the standard filesystem package: %v", mode, err)
		}
		var consumer, filesystemRuntime, resultRuntime, unitRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/filesystem/index":
				filesystemRuntime = artifact
			case "trb/std/result/index":
				resultRuntime = artifact
			case "trb/std/unit/index":
				unitRuntime = artifact
			}
		}
		if consumer == nil || filesystemRuntime == nil || resultRuntime == nil || unitRuntime == nil {
			t.Fatalf("%s did not compile the consumer, filesystem runtime, and transitive Result/Unit runtimes: %#v", mode, artifacts)
		}
		consumerWants := map[string][]string{
			"go":         {`"example.com/filesystem-app/trb/std/filesystem"`, "filesystem.ReadText(path)"},
			"ruby":       {`require_relative "./trb/std/filesystem/index"`, "read_text(path)"},
			"typescript": {`import * as __trb_filesystem from "./trb/std/filesystem/index.ts";`, "__trb_filesystem.FileSystemError", "__trb_filesystem.read_text(path)"},
		}[mode]
		for _, want := range consumerWants {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s filesystem consumer is missing %q:\n%s", mode, want, output)
			}
		}
		runtimeWants := map[string][]string{
			"go":         {"type FileSystemError struct", "os.ReadFile(path)", "__trb_result.NewResultErr[string, FileSystemError]", "__trb_result.NewResultOk[unit.Unit, FileSystemError]", "slices.Sort(names)"},
			"ruby":       {"module FileSystem", "Error = Data.define(:operation, :path, :message)", "File.binread(path)", "Result::Err.new", "Result::Ok.new(Unit.new)", "Dir.children(path).sort"},
			"typescript": {"export interface FileSystemError", `getBuiltinModule?.("fs")`, "Result.Err<string, FileSystemError>", "Result.Ok<Unit, FileSystemError>", "{} satisfies Unit", "fs.readdirSync(__trbPath)"},
		}[mode]
		for _, want := range runtimeWants {
			if output := string(filesystemRuntime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s filesystem runtime is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableFilesystemPackageDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		modulePath string
		want       string
	}{
		{
			name:   "read path",
			source: "import { FileSystem } from trb/std/filesystem\nvalue := FileSystem.read_text(1)\n",
			want:   "argument 1 to read_text() has type Integer, expected String",
		},
		{
			name:   "write bytes value",
			source: "import { FileSystem } from trb/std/filesystem\nvalue := FileSystem.write_bytes(\"output.bin\", \"not bytes\")\n",
			want:   "argument 2 to write_bytes() has type String, expected Bytes",
		},
		{
			name:       "internal import",
			source:     "import trb/internal/filesystem as native_fs\nvalue := native_fs.exists(\"input.txt\")\n",
			modulePath: "trb/std/user_source_cannot_spoof_internal_access",
			want:       "package trb/internal/filesystem is internal to the TypeRB standard library",
		},
	}
	for _, test := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				modulePath := test.modulePath
				if modulePath == "" {
					modulePath = "main"
				}
				_, err := CompileProject([]SourceUnit{{
					Filename:   "/project/main.trb",
					ModulePath: modulePath,
					Package:    "main",
					Source:     []byte(test.source),
				}}, Options{Mode: mode, GoModule: "example.com/filesystem-diagnostics", RubyLoader: "require_relative"})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("expected %s diagnostic %q, got %v", mode, test.want, err)
				}
			})
		}
	}
}

func TestPortableProcessPackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/process
import { Result } from trb/std/result

def execute(command: String, args: Array<String>): Result<Process::Output, Process::Error>
	return Process.run(command, args)
end
`),
	}
	wants := map[string][]string{
		"go": {
			`exec.Command(commandName, commandArguments...)`,
			`ProcessOutput{Status: status`,
			`os.LookupEnv(`,
		},
		"ruby": {
			`Open3.capture3(command, *arguments)`,
			`Process::Output.new(status: status.exitstatus || -1`,
			`ENV[name]`,
		},
		"typescript": {
			`childProcess.spawnSync(__trbCommand, __trbArguments)`,
			`} satisfies ProcessOutput`,
			`?.env?.[name] ?? null`,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/process-app", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected the standard process package: %v", mode, err)
		}
		var consumer, runtime, resultRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/process/index":
				runtime = artifact
			case "trb/std/result/index":
				resultRuntime = artifact
			}
		}
		if consumer == nil || runtime == nil || resultRuntime == nil {
			t.Fatalf("%s did not compile the process consumer and runtimes: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(runtime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s process runtime is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableProcessPackageDiagnosticsAcrossBackends(t *testing.T) {
	tests := []struct {
		source     string
		modulePath string
		want       string
	}{
		{
			source: "import { Process } from trb/std/process\nvalue := Process.run(\"tool\", [1])\n",
			want:   "argument 2 to run() has type Array<Integer>, expected Array<String>",
		},
		{
			source:     "import trb/internal/process\nvalue := Process.argv()\n",
			modulePath: "trb/std/user_source_cannot_import_internal_process",
			want:       "package trb/internal/process is internal to the TypeRB standard library",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			modulePath := test.modulePath
			if modulePath == "" {
				modulePath = "main"
			}
			_, err := CompileProject([]SourceUnit{{
				Filename:   "/project/main.trb",
				ModulePath: modulePath,
				Package:    "main",
				Source:     []byte(test.source),
			}}, Options{Mode: mode, GoModule: "example.com/process-diagnostics", RubyLoader: "require_relative"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s: expected process diagnostic %q, got %v", mode, test.want, err)
			}
		}
	}
}

func TestPortableJSONPackagesCompileAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/json
import trb/std/jsonc
import trb/std/result

def strict(source: String): Result<JSON::Value, JSON::Error>
	return JSON.parse(source)
end

def comments(source: String): Result<JSON::Value, JSON::Error>
	return JSONC.parse(source)
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/json-app", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected the JSON packages: %v", mode, err)
		}
		modules := map[string]bool{}
		var consumerOutput, jsonRuntimeOutput string
		for _, artifact := range artifacts {
			modules[artifact.IR.ModulePath] = true
			if artifact.IR.ModulePath == "main" {
				consumerOutput = string(artifact.Output)
			}
			if artifact.IR.ModulePath == "trb/std/json/index" {
				jsonRuntimeOutput = string(artifact.Output)
			}
		}
		for _, module := range []string{"main", "trb/std/json/index", "trb/std/jsonc/index", "trb/std/result/index"} {
			if !modules[module] {
				t.Fatalf("%s omitted compiler-owned module %s: %#v", mode, module, artifacts)
			}
		}
		if mode == "typescript" {
			for _, want := range []string{"__trb_json.parse(source)", "__trb_jsonc.parse(source)"} {
				if !strings.Contains(consumerOutput, want) {
					t.Fatalf("TypeScript JSON root call is missing %q:\n%s", want, consumerOutput)
				}
			}
			for _, want := range []string{"__trb_result.Result.Ok<JSONValue, JSONError>", "__trb_result.Result.Err<JSONValue, JSONError>"} {
				if !strings.Contains(jsonRuntimeOutput, want) {
					t.Fatalf("TypeScript JSON runtime is missing resolved Result binding %q:\n%s", want, jsonRuntimeOutput)
				}
			}
			if strings.Contains(jsonRuntimeOutput, "return Result.") {
				t.Fatalf("TypeScript JSON runtime bypasses the resolved Result binding:\n%s", jsonRuntimeOutput)
			}
		}
	}
}

func TestPortableJSONPackagesReportDiagnosticsAcrossBackends(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		modulePath string
		want       string
	}{
		{
			name:   "parse source",
			source: "import { JSON } from trb/std/json\nvalue := JSON.parse(1)\n",
			want:   "argument 1 to parse() has type Integer, expected String",
		},
		{
			name:   "stringify value",
			source: "import { JSON } from trb/std/json\nvalue := JSON.stringify(\"not JSON\")\n",
			want:   "argument 1 to stringify() has type String, expected JSON::Value",
		},
		{
			name:       "internal import",
			source:     "import trb/internal/json as native_json\nvalue := native_json.parse(\"null\")\n",
			modulePath: "trb/std/user_source_cannot_spoof_internal_json_access",
			want:       "package trb/internal/json is internal to the TypeRB standard library",
		},
	}
	for _, test := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				modulePath := test.modulePath
				if modulePath == "" {
					modulePath = "main"
				}
				_, err := CompileProject([]SourceUnit{{
					Filename:   "/project/main.trb",
					ModulePath: modulePath,
					Package:    "main",
					Source:     []byte(test.source),
				}}, Options{Mode: mode, GoModule: "example.com/json-diagnostics", RubyLoader: "require_relative"})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("expected %s diagnostic %q, got %v", mode, test.want, err)
				}
			})
		}
	}
}

func TestTypedJSONRecordCodecsLowerAcrossBackends(t *testing.T) {
	contracts := SourceUnit{
		Filename:   "/project/contracts/user.trb",
		ModulePath: "contracts/user",
		Package:    "user",
		Source: []byte(`alias UserId = Integer

record Address
	city: String
end

enum UserStatus
	Active = "ACTIVE"
	Disabled = "DISABLED"
end

record User
	id: UserId @json("user_id")
	name: String
	nickname: String?
	scores: Array<Float>
	metadata: Hash<String, Integer>
	address: Address
	status: UserStatus
end
`),
	}
	main := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { User } from contracts/user
import trb/std/json
import { Result } from trb/std/result

def decode_user(source: String): Result<User, JSON::Error>
	return JSON.decode<User>(source)
end

def encode_user(user: User): Result<String, JSON::Error>
	return JSON.encode(user)
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{contracts, main}, Options{Mode: mode, GoModule: "example.com/json-codec", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected typed JSON codecs: %v", mode, err)
		}
		var output string
		for _, artifact := range artifacts {
			if artifact.IR.ModulePath == "main" {
				output = string(artifact.Output)
			}
		}
		for _, want := range map[string][]string{
			"go":         {"func DecodeUser", "JSONErrorKindDecode", "Id: field0", "func(value int)"},
			"ruby":       {"JSON::ErrorKind::Decode", `User.new(id: field0`},
			"typescript": {"import { UserStatus }", "__trb_json.JSONErrorKind.Decode", "case \"ACTIVE\": return UserStatus.Active", "return { id: field0"},
		}[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("%s typed JSON codec output does not contain %q:\n%s", mode, want, output)
			}
		}
		if mode == "go" && strings.Contains(output, "func(value UserId)") {
			t.Fatalf("Go JSON codec leaked an unqualified transparent alias across packages:\n%s", output)
		}
	}
}

func TestTypedJSONInfersTransitiveProjectRecordContractsAcrossBackends(t *testing.T) {
	contracts := SourceUnit{
		Filename:   "/project/contracts/payload.trb",
		ModulePath: "contracts/payload",
		Package:    "contracts",
		Source: []byte(`newtype PayloadId = Integer

enum Status
	Ready = "ready"
end

record Metadata
	source: String
end

record Payload
	id: PayloadId
	status: Status
	metadata: Metadata
end
`),
	}
	mapper := SourceUnit{
		Filename:   "/project/app/mapper.trb",
		ModulePath: "app/mapper",
		Package:    "mapper",
		Source: []byte(`import { Metadata, Payload, PayloadId, Status } from contracts/payload

def payload(): Payload
	return Payload.new(
		id: PayloadId.new(7),
		status: Status::Ready,
		metadata: Metadata.new(source: "fixture"),
	)
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/routes/index.trb",
		ModulePath: "routes/index",
		Package:    "routes",
		Source: []byte(`import { payload } from app/mapper
import { Response } from trb/web

def get(): Response
	return Response.json(payload())
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{contracts, mapper, consumer}, Options{
				Mode: mode, GoModule: "example.com/transitive-json", RubyLoader: "require_relative", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatalf("%s rejected a transitive JSON record contract: %v", mode, err)
			}
			artifact := artifactForModule(artifacts, "routes/index")
			if artifact == nil {
				t.Fatal("consumer artifact was not generated")
			}
			var generated *ir.Import
			for _, statement := range artifact.IR.Statements {
				imported, ok := statement.(*ir.Import)
				if ok && imported.Path == "contracts/payload" {
					generated = imported
					break
				}
			}
			if generated == nil || len(generated.Symbols) != 0 {
				t.Fatalf("%s did not retain a generated-only contracts import: %#v", mode, generated)
			}
			for _, name := range []string{"Metadata", "Payload", "Status"} {
				if !slices.Contains(generated.GeneratedTypeSymbols, name) {
					t.Fatalf("%s generated contracts import is missing %s: %#v", mode, name, generated.GeneratedTypeSymbols)
				}
			}
		})
	}
}

func TestImportedTransparentAliasesUseUnderlyingReceiverMethods(t *testing.T) {
	contracts := SourceUnit{
		Filename:   "/project/contracts/ids.trb",
		ModulePath: "contracts/ids",
		Package:    "ids",
		Source: []byte(`import { Result } from trb/std/result

alias UserId = Integer
alias MemberId = Integer
alias EmailAddress = String

record MemberRef
	member_id: MemberId?
end

alias MemberResult = Result<MemberRef, String>
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { EmailAddress, MemberId, MemberRef, MemberResult } from contracts/ids

record QueryResult<T>
	data: T
end

def member_id_text(id: MemberId): String
	return id.to_s()
end

def email_label(email: EmailAddress): String
	return "Email: " + email
end

def optional_member_id_text(ref: MemberRef): String
	if ref.member_id == nil
		return ""
	end
	return ref.member_id.to_s()
end


def nested_optional_member_id_text(query: QueryResult<MemberRef>): String
	if query.data.member_id == nil
		return ""
	end
	return query.data.member_id.to_s()
end

def alias_collection_operations(id: MemberId): Boolean
	mut ids: Array<MemberId> := []
	ids.push(id)
	return ids.include?(id)
end

def optional_member_error(): MemberResult?
	return MemberResult::Err("missing")
end

def optional_member_error_from_helper(): MemberResult?
	return _member_error()
end

def _member_error(): MemberResult
	return MemberResult::Err("missing")
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{contracts, consumer}, Options{Mode: mode, GoModule: "example.com/alias-methods", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected a receiver method on an imported transparent alias: %v", mode, err)
		}
		var output string
		for _, artifact := range artifacts {
			if artifact.IR.ModulePath == "main" {
				output = string(artifact.Output)
				break
			}
		}
		want := map[string]string{
			"go":         "strconv.Itoa(id)",
			"ruby":       "id.to_s",
			"typescript": "String(id)",
		}[mode]
		if !strings.Contains(output, want) {
			t.Fatalf("%s did not lower the alias receiver through Integer.to_s():\n%s", mode, output)
		}
		if !strings.Contains(output, `"Email: " + email`) {
			t.Fatalf("%s did not use the imported String alias in concatenation:\n%s", mode, output)
		}
		narrowedWant := map[string]string{
			"go":         "strconv.Itoa(*(ref.MemberId))",
			"ruby":       "ref.member_id.to_s",
			"typescript": "String(ref.member_id)",
		}[mode]
		if !strings.Contains(output, narrowedWant) {
			t.Fatalf("%s did not lower the narrowed nullable alias receiver through Integer.to_s():\n%s", mode, output)
		}
		if mode == "go" {
			if !strings.Contains(output, "func(value contracts.MemberResult) *contracts.MemberResult") || strings.Contains(output, "func(value Result[") {
				t.Fatalf("Go nullable conversion did not preserve the imported transparent alias:\n%s", output)
			}
		}
		nestedWant := map[string]string{
			"go":         "strconv.Itoa(*(query.Data.MemberId))",
			"ruby":       "query.data.member_id.to_s",
			"typescript": "String(query.data.member_id)",
		}[mode]
		if !strings.Contains(output, nestedWant) {
			t.Fatalf("%s did not lower the nested narrowed alias receiver through Integer.to_s():\n%s", mode, output)
		}
	}
}

func TestTypedJSONRecordCodecsReportDiagnosticsAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "decode requires a type",
			source: "import { JSON } from trb/std/json\nvalue := JSON.decode(\"{}\")\n",
			want:   "cannot infer T for decode()",
		},
		{
			name:   "class is not a record codec",
			source: "import { JSON } from trb/std/json\nclass User; end\nvalue := JSON.decode<User>(\"{}\")\n",
			want:   "JSON codec type User must be a record or JSON-compatible built-in type",
		},
		{
			name:   "unsupported record field",
			source: "import { JSON } from trb/std/json\nrecord Document; payload: Bytes; end\nvalue := JSON.decode<Document>(\"{}\")\n",
			want:   "JSON codec type Bytes is not supported",
		},
		{
			name:   "recursive record",
			source: "import { JSON } from trb/std/json\nrecord Node; child: Node?; end\nvalue := JSON.decode<Node>(\"{}\")\n",
			want:   "recursive JSON codec record Node is not supported yet",
		},
	}
	for _, test := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				_, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(test.source)}}, Options{Mode: mode, GoModule: "example.com/json-codec-diagnostics", RubyLoader: "require_relative"})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("expected %s diagnostic %q, got %v", mode, test.want, err)
				}
			})
		}
	}
}

func TestPutsPreludeLowersWithoutImportAcrossBackends(t *testing.T) {
	source := []byte("def main()\n  puts(1 + 2)\n  return\nend\n")
	tests := []struct {
		mode string
		want string
	}{
		{mode: "go", want: "fmt.Println(trbIntegerAdd_"},
		{mode: "typescript", want: "console.log(__trbIntegerAdd(1, 2));"},
		{mode: "ruby", want: "$stdout.puts(__trb_integer_add(1, 2))"},
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

func TestGoPutsDereferencesNullableValues(t *testing.T) {
	source := []byte("def print_name(name: String?)\n\tputs(name)\n\treturn\nend\n")
	artifact, err := CompileWithOptions("main.trb", source, Options{Mode: "go", Package: "main"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, want := range []string{"fmt.Println(func(value *string) any", "if value == nil", "return *value", "}(name))"} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated Go does not print the nullable String value; missing %q:\n%s", want, output)
		}
	}
}

func TestPortableStringLengthUsesUnicodeCodePoints(t *testing.T) {
	source := []byte("def count(): Integer\n  return \"😀a\".size()\nend\n")
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
	source := []byte("import trb/platform/go/context\n\ndef main()\n  Context.background()\n  return\nend\n")
	if _, err := Compile("main.trb", source, "typescript"); err == nil || !strings.Contains(err.Error(), "does not support mode typescript") {
		t.Fatalf("expected platform mode diagnostic, got %v", err)
	}
	artifact, err := Compile("main.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	if output := string(artifact.Output); !strings.Contains(output, `import trbcontext "context"`) || !strings.Contains(output, `trbcontext.Background()`) {
		t.Fatalf("Go platform binding was not lowered:\n%s", output)
	}
}

func TestStandardPackageSignaturesAndReservedPathsAreChecked(t *testing.T) {
	anyType := []byte("import trb/std/io\n\ndef main()\n  IO.puts(1)\n  IO.puts([1, 2])\n  return\nend\n")
	if _, err := Compile("main.trb", anyType, "go"); err != nil {
		t.Fatalf("puts should accept any TypeRB value: %v", err)
	}
	wrongArity := []byte("import trb/std/io\n\ndef main()\n  IO.puts()\n  return\nend\n")
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

func TestPortableArrayOutputUsesOneLineAcrossBackends(t *testing.T) {
	source := []byte("def main()\n\tputs([\"compiler\", \"web\"])\n\treturn\nend\n")
	wants := map[string][]string{
		"go":         {"strconv.Quote(item)", `strings.Join(parts, ", ")`},
		"ruby":       {`$stdout.puts((["compiler", "web"]).inspect)`},
		"typescript": {`.map((item) => JSON.stringify(item)).join(", ")`},
	}
	for mode, expected := range wants {
		artifact, err := Compile("array_output.trb", source, mode)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, fragment := range expected {
			if !strings.Contains(output, fragment) {
				t.Fatalf("%s output is missing %q:\n%s", mode, fragment, output)
			}
		}
	}
}

func TestRubyNativeSyntaxRequiresExplicitPlatformImport(t *testing.T) {
	source := []byte("class Post < ApplicationRecord\n  belongs_to :author\nend\n")
	if _, err := Compile("post.trb", source, "ruby"); err == nil || !strings.Contains(err.Error(), "requires activate trb/platform/ruby") {
		t.Fatalf("expected explicit Ruby platform import diagnostic, got %v", err)
	}
	withImport := append([]byte("activate trb/platform/ruby/rails\n\n"), source...)
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

func TestGoProjectKeepsTypeAndFunctionExportsDistinct(t *testing.T) {
	view := SourceUnit{
		Filename:   "/project/models/view.trb",
		ModulePath: "models/view",
		Package:    "models",
		Source: []byte(`record TodoResponse
	message: String
end

def todo_response(): TodoResponse
	return TodoResponse.new(message: "ok")
end
`),
	}
	main := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { todo_response } from models/view

def main()
	puts(todo_response().message)
	return
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{main, view}, Options{Mode: "go", GoModule: "example.com/project"})
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{}
	for _, artifact := range artifacts {
		outputs[artifact.AST.ModulePath] = string(artifact.Output)
	}
	modelOutput := outputs["models/view"]
	mainOutput := outputs["main"]
	if !strings.Contains(modelOutput, "type TodoResponse struct") || !strings.Contains(modelOutput, "func TrbFunction_") {
		t.Fatalf("generated model did not disambiguate the type and function:\n%s", modelOutput)
	}
	if !strings.Contains(mainOutput, "models.TrbFunction_") {
		t.Fatalf("generated importer did not use the disambiguated function name:\n%s", mainOutput)
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
	aliased.Source = []byte("import { State as WorkflowState } from contracts/state\n\ndef label(): String\n\tvalue := WorkflowState::Open\n\tcase value\n\twhen WorkflowState::Open\n\t\treturn \"open\"\n\twhen WorkflowState::Closed\n\t\treturn \"closed\"\n\tend\nend\n")
	aliasedArtifacts, err := CompileProject([]SourceUnit{aliased, contract}, Options{Mode: "go", GoModule: "example.com/project"})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range aliasedArtifacts {
		if artifact.Filename == aliased.Filename && !strings.Contains(string(artifact.Output), "contracts.StateOpen") {
			t.Fatalf("aliased enum member was not qualified correctly:\n%s", artifact.Output)
		}
	}
}

func TestProjectCompilerExportsDiscriminatedUnionContracts(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/responses.trb",
		ModulePath: "contracts/responses",
		Package:    "contracts",
		Source: []byte(`record CreatedResponse
	status: 201
	body: String
end

record InvalidResponse
	status: 422
	body: Array<String>
end

alias CreateResponse = CreatedResponse | InvalidResponse
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { CreateResponse, CreatedResponse, InvalidResponse } from contracts/responses

def render(response: CreateResponse): String
	case response.status
	when 201
		return response.body
	when 422
		return response.body[0]
	end
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{consumer, contract}, Options{Mode: mode, GoModule: "example.com/project"})
		if err != nil {
			t.Fatalf("%s rejected imported discriminated union contract: %v", mode, err)
		}
		for _, artifact := range artifacts {
			if artifact.Filename != consumer.Filename {
				continue
			}
			output := string(artifact.Output)
			wants := map[string][]string{
				"go":         {"case contracts.CreatedResponse:", "response := response.(contracts.CreatedResponse)"},
				"ruby":       {"case response.status", "return response.body"},
				"typescript": {"import type { CreateResponse, CreatedResponse, InvalidResponse }", "as CreatedResponse"},
			}[mode]
			for _, want := range wants {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s imported contract consumer is missing %q:\n%s", mode, want, output)
				}
			}
		}
	}
}

func TestProjectCompilerExportsPayloadEnumSignatures(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/token.trb",
		ModulePath: "contracts/token",
		Package:    "contracts",
		Source: []byte(`enum Token
	Text(value: String)
	Number(value: Integer)
	EOF
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import contracts/token

def render(value: Token): String
	case value
	when Token::Text(text)
		return text
	when Token::Number(number)
		return number.to_s()
	when Token::EOF
		return "eof"
	end
end

def sample(): String
	return render(Token::Text("hello"))
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{consumer, contract}, Options{Mode: mode, GoModule: "example.com/project"})
		if err != nil {
			t.Fatalf("%s rejected imported payload enum: %v", mode, err)
		}
		for _, artifact := range artifacts {
			if artifact.Filename != consumer.Filename {
				continue
			}
			output := string(artifact.Output)
			wants := map[string][]string{
				"go":         {"contracts.NewTokenText(\"hello\")", "__trbCase1.Kind == contracts.TokenTextTag", "text := __trbCase1.TextValue"},
				"ruby":       {"Token::Text.new(\"hello\")", "text = __trb_case1.value"},
				"typescript": {"Token.Text(\"hello\")", `__trbCase1.kind === "Text"`, "const text = __trbCase1.value;"},
			}[mode]
			for _, want := range wants {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s payload enum consumer is missing %q:\n%s", mode, want, output)
				}
			}
		}
	}
}

func TestProjectCompilerExportsGenericSignatures(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/result.trb",
		ModulePath: "contracts/result",
		Package:    "contracts",
		Source: []byte(`enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end

def identity<T>(value: T): T
	return value
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result, identity } from contracts/result

def sample(): Result<Integer, String>
	value := identity<Integer>(1)
	return Result<Integer, String>::Ok(value)
end

def unwrap(result: Result<Integer, String>): Integer
	case result
	when Result::Ok(value)
		return value
	when Result::Err(_)
		return 0
	end
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{consumer, contract}, Options{Mode: mode, GoModule: "example.com/project"})
		if err != nil {
			t.Fatalf("%s rejected imported generics: %v", mode, err)
		}
		for _, artifact := range artifacts {
			if artifact.Filename != consumer.Filename {
				continue
			}
			output := string(artifact.Output)
			wants := map[string][]string{
				"go":         {"func Sample() contracts.Result[int, string]", "contracts.Identity[int](1)", "contracts.NewResultOk[int, string](value)", "value := __trbCase1.OkValue"},
				"ruby":       {"value = identity(1)", "Result::Ok.new(value)", "value = __trb_case1.value"},
				"typescript": {"function sample(): Result<number, string>", "identity<number>(1)", "Result.Ok<number, string>(value)", "const value = __trbCase1.value;"},
			}[mode]
			for _, want := range wants {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s generic consumer is missing %q:\n%s", mode, want, output)
				}
			}
		}
	}
}

func TestProjectCompilerExportsGenericClassesRecordsAndMethods(t *testing.T) {
	models := SourceUnit{Filename: "models.trb", ModulePath: "models/index", Source: []byte(`class Box<T>
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
`)}
	main := SourceUnit{Filename: "main.trb", ModulePath: "app/main", Source: []byte(`import { Box, Pair } from models

def pair(): Pair<Integer, String>
	box := Box<Integer>.new(7)
	return box.pair<String>("Ada")
end
`)}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{models, main}, Options{Mode: mode, GoModule: "example.com/generic-imports"})
		if err != nil {
			t.Fatalf("%s rejected imported generic objects: %v", mode, err)
		}
		if len(artifacts) != 2 {
			t.Fatalf("%s generated %d artifacts, want 2", mode, len(artifacts))
		}
		if mode == "go" {
			outputs := map[string]string{}
			for _, artifact := range artifacts {
				outputs[artifact.Filename] = string(artifact.Output)
			}
			if !strings.Contains(outputs[models.Filename], "func (self *Box[T]) Pair[U any](other U) Pair[T, U]") {
				t.Fatalf("generated Go model does not declare a native generic method:\n%s", outputs[models.Filename])
			}
			if !strings.Contains(outputs[main.Filename], `box.Pair[string]("Ada")`) {
				t.Fatalf("generated Go consumer does not call the imported generic method directly:\n%s", outputs[main.Filename])
			}
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
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Source: []byte(`import { Named } from contracts/named
import { User } from models/user

class Admin < User
end

def build(): Named
	return Admin.new()
end

def display(value: Named): String
	return value.name()
end

def label(): String
	return display(User.new())
end
`),
	}
	consumerFunction := SourceUnit{
		Filename:   "/project/presentation.trb",
		ModulePath: "presentation",
		Source: []byte(`import { Named } from contracts/named

def imported_display(value: Named): String
	return value.name()
end
`),
	}
	consumer.Source = []byte(`import { Named } from contracts/named
import { User } from models/user
import { imported_display } from presentation

class Admin < User
end

def build(): Named
	return Admin.new()
end

def display(value: Named): String
	return value.name()
end

def label(): String
	return imported_display(User.new())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := CompileProject([]SourceUnit{contract, valid, consumerFunction, consumer}, Options{Mode: mode, GoModule: "example.com/interfaces", RubyLoader: "require_relative"}); err != nil {
			t.Fatalf("%s rejected imported interface values: %v", mode, err)
		}
	}

	invalid := valid
	invalid.Source = []byte("import contracts/named\n\nclass User implements Named\n  def name(): Integer\n    return 1\n  end\nend\n")
	if _, err := CompileProject([]SourceUnit{contract, invalid}, Options{Mode: "typescript"}); err == nil || !strings.Contains(err.Error(), "does not match interface Named") {
		t.Fatalf("expected imported interface signature diagnostic, got %v", err)
	}
}

func TestProjectCompilerAssignsImportedClassToTransitiveInterfaceParameter(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/named.trb",
		ModulePath: "contracts/named",
		Source:     []byte("interface Named\n  name(): String\nend\n"),
	}
	implementation := SourceUnit{
		Filename:   "/project/models/user.trb",
		ModulePath: "models/user",
		Source: []byte(`import { Named } from contracts/named

class User implements Named
	def name(): String
		return "Alice"
	end
end
`),
	}
	service := SourceUnit{
		Filename:   "/project/services/display.trb",
		ModulePath: "services/display",
		Source: []byte(`import { Named } from contracts/named

def display(value: Named): String
	return value.name()
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Source: []byte(`import { User } from models/user
import { display } from services/display

def label(): String
	return display(User.new())
end
`),
	}
	factory := SourceUnit{
		Filename:   "/project/services/users.trb",
		ModulePath: "services/users",
		Source: []byte(`import { User } from models/user

def build_user(): User
	return User.new()
end
`),
	}
	transitiveConsumer := SourceUnit{
		Filename:   "/project/transitive_main.trb",
		ModulePath: "transitive_main",
		Source: []byte(`import { display } from services/display
import { build_user } from services/users

def transitive_label(): String
	return display(build_user())
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := CompileProject([]SourceUnit{contract, implementation, service, consumer}, Options{Mode: mode, GoModule: "example.com/transitive-interface", RubyLoader: "require_relative"}); err != nil {
			t.Fatalf("%s rejected an imported class for a transitive interface parameter: %v", mode, err)
		}
		if _, err := CompileProject([]SourceUnit{contract, implementation, service, factory, transitiveConsumer}, Options{Mode: mode, GoModule: "example.com/transitive-interface", RubyLoader: "require_relative"}); err != nil {
			t.Fatalf("%s rejected a transitive class for a transitive interface parameter: %v", mode, err)
		}
	}
}

func TestProjectCompilerExpandsImportedAliasesInInterfaceSignatures(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/repository.trb",
		ModulePath: "contracts/repository",
		Source: []byte(`import { Result } from trb/std/result

record Entity
	id: Integer
end

enum RepositoryError
	NotFound
end

alias EntityResult = Result<Entity, RepositoryError>

interface Repository
	find(id: Integer): EntityResult
end
`),
	}
	implementation := SourceUnit{
		Filename:   "/project/repositories/memory.trb",
		ModulePath: "repositories/memory",
		Source: []byte(`import { Entity, EntityResult, Repository } from contracts/repository

class MemoryRepository implements Repository
	def find(id: Integer): EntityResult
		return EntityResult::Ok(Entity.new(id: id))
	end
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Source: []byte(`import { EntityResult } from contracts/repository
import { MemoryRepository } from repositories/memory

def read(): Integer
	case MemoryRepository.new().find(1)
	when EntityResult::Err(_error)
		return 0
	when EntityResult::Ok(entity)
		return entity.id
	end
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := CompileProject([]SourceUnit{contract, implementation, consumer}, Options{Mode: mode, GoModule: "example.com/alias-interface", RubyLoader: "require_relative"}); err != nil {
			t.Fatalf("%s rejected imported aliases in interface signatures: %v", mode, err)
		}
	}
}

func TestProjectCompilerSpecializesImportedGenericInterfaces(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/store.trb",
		ModulePath: "contracts/store",
		Source:     []byte("interface Store<T>\n\tget(): T\n\tput(value: T): T\nend\n"),
	}
	implementation := SourceUnit{
		Filename:   "/project/stores/user_store.trb",
		ModulePath: "stores/user_store",
		Source: []byte(`import { Store } from contracts/store

record User
	name: String
end

class UserStore implements Store<User>
	@value: User

	def initialize(value: User)
		@value = value
		return
	end

	def get(): User
		return @value
	end

	def put(value: User): User
		@value = value
		return @value
	end
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Source: []byte(`import { Store } from contracts/store
import { User, UserStore } from stores/user_store

def read(store: Store<User>): String
	return store.get().name
end

def build(): Store<User>
	return UserStore.new(User.new(name: "Ada"))
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := CompileProject([]SourceUnit{contract, implementation, consumer}, Options{Mode: mode, GoModule: "example.com/generic-interfaces", RubyLoader: "require_relative"}); err != nil {
			t.Fatalf("%s rejected imported generic interface values: %v", mode, err)
		}
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
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Source:     []byte("import models/probe\n\ndef identifier(): Integer\n\tprobe := Probe.new(1)\n\treturn probe.id\nend\n"),
	}
	for mode, expected := range map[string]string{
		"go":         "probe.TrbFieldId",
		"ruby":       "probe.__trb_field_id",
		"typescript": "probe.__trb_id",
	} {
		artifacts, err := CompileProject([]SourceUnit{model, consumer}, Options{Mode: mode, GoModule: "example.com/project"})
		if err != nil {
			t.Fatalf("%s rejected imported class field access: %v", mode, err)
		}
		var output string
		for _, artifact := range artifacts {
			if artifact.IR.ModulePath == "main" {
				output = string(artifact.Output)
			}
		}
		if !strings.Contains(output, expected) {
			t.Fatalf("%s imported class field access is missing %q:\n%s", mode, expected, output)
		}
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

func TestProjectCatalogAllowsDuplicateExportedTypesAcrossModules(t *testing.T) {
	a := SourceUnit{Filename: "/project/a.trb", ModulePath: "a", Source: []byte("class User\nend\n")}
	b := SourceUnit{Filename: "/project/b.trb", ModulePath: "b", Source: []byte("class User\nend\n")}
	if _, err := CompileProject([]SourceUnit{a, b}, Options{Mode: "typescript"}); err != nil {
		t.Fatalf("duplicate declarations in separate modules should remain importable by exact identity: %v", err)
	}
}
