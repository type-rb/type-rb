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

func TestPortableReceiverMethodsShareStandardContractsAcrossBackends(t *testing.T) {
	source := []byte(`import trb/std/numbers

def receiver_text(): String
	return 123.to_s()
end

def package_text(): String
	return numbers.to_string(123)
end

def parsed(): Integer
	return "123".to_i()
end

def text_size(): Integer
	return "a😀".size()
end
`)
	wants := map[string][]string{
		"go":         {`strconv.Itoa(123)`, `regexp.MatchString`, `strconv.ParseInt`, `utf8.RuneCountInString("a😀")`},
		"ruby":       {`123.to_s`, `Integer(input, 10)`, `"a😀".each_codepoint.count`},
		"typescript": {`String(123)`, `Number.isSafeInteger(__trbValue)`, `Array.from("a😀").length`},
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

		var receiverReference, packageReference *ir.Reference
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
			switch method.Name {
			case "receiver_text":
				receiverReference = call.Callee.(*ir.Member).Reference
			case "package_text":
				packageReference = call.Callee.(*ir.Member).Reference
			}
		}
		if receiverReference == nil || packageReference == nil ||
			receiverReference.Intrinsic != packageReference.Intrinsic ||
			receiverReference.Package != packageReference.Package ||
			!receiverReference.ReceiverMethod || packageReference.ReceiverMethod {
			t.Fatalf("%s did not retain a shared package/receiver contract: receiver=%#v package=%#v", mode, receiverReference, packageReference)
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

func TestPortableBytesPackageAndReceiverMethodsLowerAcrossBackends(t *testing.T) {
	source := []byte(`import trb/std/bytes

def joined(): Bytes
	return bytes.concat(bytes.from_string("A"), "😀".to_bytes())
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
	return joined().valid_utf8()
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
		{source: "import trb/std/bytes\ndef bad(): Integer\n\treturn bytes.at(bytes.from_string(\"A\"), \"0\")\nend\n", want: "argument 2 to at() has type String, expected Integer"},
		{source: "def bad(): Bytes\n\treturn \"A\"\nend\n", want: "return type is String, expected Bytes"},
		{source: "def bad(): Integer\n\treturn \"A\".to_bytes().missing()\nend\n", want: "type Bytes has no member missing"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s: expected %q Bytes diagnostic, got %v", mode, test.want, err)
			}
		}
	}
}

func TestPortableStringBuilderLowersAcrossBackends(t *testing.T) {
	source := []byte(`import trb/std/string_builder

def render(): String
	mut builder := string_builder.new()
	builder.append("A")
	string_builder.append_codepoint(builder, 128512)
	builder.append_codepoint(33)
	return builder.to_s()
end

def measured(): Integer
	mut builder := string_builder.from_string("A😀")
	return builder.size()
end

def blank(): Boolean
	builder := string_builder.new()
	return builder.empty?()
end

def reset(): String
	mut builder := string_builder.from_string("old")
	builder.clear()
	builder.append("new")
	return string_builder.to_string(builder)
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
			source: "import trb/std/string_builder\ndef bad()\n\tbuilder := string_builder.new()\n\tbuilder.append(\"x\")\n\treturn\nend\n",
			want:   "builder is immutable; declare it with mut to use append()",
		},
		{
			name:   "package requires mut",
			source: "import trb/std/string_builder\ndef bad()\n\tbuilder := string_builder.new()\n\tstring_builder.clear(builder)\n\treturn\nend\n",
			want:   "builder is immutable; declare it with mut to use clear()",
		},
		{
			name:   "append type",
			source: "import trb/std/string_builder\ndef bad()\n\tmut builder := string_builder.new()\n\tbuilder.append(1)\n\treturn\nend\n",
			want:   "argument 1 to append() has type Integer, expected String",
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
	source := []byte(`import trb/std/arrays
import trb/std/hashes

def array_value(): Integer
	values := [1, 2, 3]
	return arrays.copy(values).fetch(1) + values.first() + values.last()
end

def array_state(): Boolean
	values := [1]
	return values.size() == 1 and not values.empty?()
end

def grow()
	mut values := [1]
	arrays.push(values, 2)
	values.push(3)
	return
end

def hash_value(): String
	labels: Hash<Integer, String> := {1 => "one", 2 => "two"}
	return labels.fetch(1) + hashes.fetch(labels, 2)
end

def hash_key(): Integer
	labels: Hash<Integer, String> := {1 => "one"}
	return labels.keys().first()
end

def copied_hash_value(): String
	labels: Hash<Integer, String> := {1 => "one"}
	return hashes.copy(labels).values().first()
end

def hash_state(): Boolean
	labels: Hash<Integer, String> := {1 => "one"}
	return labels.size() == 1 and not labels.empty?() and labels.key?(1)
end

def string_state(): Boolean
	return "TypeRB".start_with?("Type") and "TypeRB".end_with?("RB")
end

def string_parts(): String
	mut parts := "root/leaf/".split("/")
	tail := parts.pop()
	return parts.join("|") + ":" + tail
end
`)
	wants := map[string][]string{
		"go": {
			`slices.Clone(values)`,
			`panic("Array index is out of bounds")`,
			`values = append(values, 2)`,
			`maps.Keys(labels)`,
			`maps.Values(maps.Clone(labels))`,
			`panic("Hash key is missing")`,
			`strings.HasPrefix("TypeRB", "Type")`,
			`strings.Split(value, separator)`,
			`strings.Join(parts, "|")`,
		},
		"ruby": {
			`values.dup`,
			`raise IndexError, "Array index is out of bounds"`,
			`values << 2`,
			`labels.keys`,
			`labels.dup.values`,
			`labels.fetch(1)`,
			`"TypeRB".start_with?("Type")`,
			`value.split(separator, -1)`,
			`parts.join("|")`,
		},
		"typescript": {
			`[...values]`,
			`throw new Error("Array index is out of bounds")`,
			`values.push(2)`,
			`Object.keys(labels).map(Number)`,
			`Object.values(({ ...labels }))`,
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

func TestPortableArrayAndHashDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "receiver push type",
			source: "def bad()\n\tmut values := [1]\n\tvalues.push(\"two\")\n\treturn\nend\n",
			want:   "argument 1 to push() has type String, expected Integer",
		},
		{
			name:   "package push type",
			source: "import trb/std/arrays\ndef bad()\n\tmut values := [1]\n\tarrays.push(values, \"two\")\n\treturn\nend\n",
			want:   "argument 2 to push() has type String, expected Integer",
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
			name:   "hash package key type",
			source: "import trb/std/hashes\ndef bad(): String\n\tlabels: Hash<Integer, String> := {1 => \"one\"}\n\treturn hashes.fetch(labels, \"1\")\nend\n",
			want:   "argument 2 to fetch() has type String, expected Integer",
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
			name:   "join package element type",
			source: "import trb/std/arrays\ndef bad(): String\n\treturn arrays.join([1, 2], \",\")\nend\n",
			want:   "argument 1 to join() has type Array<Integer>, expected Array<String>",
		},
		{
			name:   "pop requires mut",
			source: "def bad(): Integer\n\tvalues := [1]\n\treturn values.pop()\nend\n",
			want:   "values is immutable; declare it with mut to use pop()",
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
	return path.clean("a/./b/../c")
end

def joined(): String
	return path.join("/srv/app", "../data")
end

def inspected(): Boolean
	return path.absolute("/srv/app") and path.base("/srv/app/main.trb") == "main.trb" and path.directory("/srv/app/main.trb") == "/srv/app"
end

def parts(): Array<String>
	return path.components("/srv/app/main.trb")
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
				`import * as path from "./trb/std/path/index.ts";`,
				`path.clean("a/./b/../c")`,
				`path.components("/srv/app/main.trb")`,
			},
		}[mode]
		for _, want := range consumerWants {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s path consumer is missing %q:\n%s", mode, want, output)
			}
		}
		runtimeWants := map[string][]string{
			"go":         {`func PathClean(value string) string`, `func Components(value string) []string`, `normalized = append(normalized, part)`},
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
	source := []byte("import trb/std/path\ndef bad(): String\n\treturn path.clean(1)\nend\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("bad.trb", source, mode); err == nil || !strings.Contains(err.Error(), "argument 1 to clean() has type Integer, expected String") {
			t.Fatalf("%s did not reject invalid path argument: %v", mode, err)
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
	return unicode.letter(65) and unicode.letter(12354) and unicode.digit(1632) and unicode.uppercase(65) and unicode.lowercase(97) and unicode.whitespace(12288)
end

def identifiers(): Boolean
	return unicode.identifier_start(64) and unicode.identifier_start(12354) and unicode.identifier_part(1632)
end

def scalar(): String
	return unicode.from_codepoint(128512)
end

def string_methods(): Boolean
	return "A😀".codepoints().size() == 2 and "".empty?() and "TypeRB".include?("RB") and "ada".upcase() == "ADA"
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
				`import * as unicode from "./trb/std/unicode/index.ts";`,
				`unicode.Unicode.letter(65)`,
				`unicode.Unicode.from_codepoint(128512)`,
				`Array.from("A😀", (value): number => value.codePointAt(0)!)`,
			},
		}[mode]
		for _, want := range consumerWants {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s Unicode consumer is missing %q:\n%s", mode, want, output)
			}
		}
		runtimeWants := map[string][]string{
			"go":         {`var UnicodeDataVersion string = "15.0.0"`, `func UnicodeLetter(value int) bool`, `func inRanges(value int, ranges [][]int) bool`},
			"ruby":       {`class Unicode`, `UNICODE_DATA_VERSION = "15.0.0"`, `def self.letter(value)`, `def _in_ranges(value, ranges)`},
			"typescript": {`export class Unicode`, `export const UNICODE_DATA_VERSION: string = "15.0.0";`, `static letter(value: number): boolean`, `export function _in_ranges(value: number, ranges: Array<Array<number>>): boolean`},
		}[mode]
		for _, want := range runtimeWants {
			if output := string(runtime.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s Unicode runtime is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestUnicodePackageDiagnosticsAreModeIndependent(t *testing.T) {
	wrongType := []byte("import trb/std/unicode\ndef bad(): Boolean\n\treturn unicode.letter(\"A\")\nend\n")
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
		Source: []byte(`import { letter, from_codepoint } from trb/std/unicode

def accepted(): Boolean
	return letter(12354)
end

def character(): String
	return from_codepoint(128512)
end
`),
	}
	wants := map[string][]string{
		"go":         {`import "example.com/unicode-named/trb/std/unicode"`, `return unicode.Letter(12354)`, `return unicode.FromCodepoint(128512)`},
		"ruby":       {`require_relative "./trb/std/unicode/index"`, `return letter(12354)`, `return from_codepoint(128512)`},
		"typescript": {`import { from_codepoint, letter } from "./trb/std/unicode/index.ts";`, `return letter(12354);`, `return from_codepoint(128512);`},
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
	when Result::Err(error)
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
	if _, err := Compile("bad.trb", wrongPayload, "typescript"); err == nil || !strings.Contains(err.Error(), "enum payload argument 1 has type String, expected Integer") {
		t.Fatalf("expected standard Result payload diagnostic, got %v", err)
	}
}

func TestSafePortableConversionAndLookupLowerAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result
import trb/std/arrays
import trb/std/hashes
import trb/std/numbers

def parsed(value: String): Result<Integer, String>
	return value.try_to_i()
end

def package_parsed(value: String): Result<Integer, String>
	return numbers.try_parse_integer(value)
end

def array_value(values: Array<Integer>, index: Integer): Result<Integer, String>
	return arrays.try_fetch(values, index)
end

def hash_value(values: Hash<String, Integer>, key: String): Result<Integer, String>
	return hashes.try_fetch(values, key)
end
`),
	}

	wants := map[string][]string{
		"go": {
			`regexp.MatchString`,
			`__trb_result.NewResultErr[int, string]("invalid Integer")`,
			`__trb_result.NewResultErr[int, string]("Array index is out of bounds")`,
			`__trb_result.NewResultErr[int, string]("Hash key is missing")`,
		},
		"ruby": {
			`Result::Err.new("invalid Integer")`,
			`Result::Err.new("Array index is out of bounds")`,
			`Result::Err.new("Hash key is missing")`,
		},
		"typescript": {
			`Result.Err<number, string>("invalid Integer")`,
			`Result.Err<number, string>("Array index is out of bounds")`,
			`Result.Err<number, string>("Hash key is missing")`,
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/safe-values", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected safe portable operations: %v", mode, err)
		}
		var consumer, resultRuntime *Artifact
		for _, artifact := range artifacts {
			switch artifact.IR.ModulePath {
			case "main":
				consumer = artifact
			case "trb/std/result/index":
				resultRuntime = artifact
			}
		}
		if consumer == nil || resultRuntime == nil {
			t.Fatalf("%s did not compile the consumer and Result runtime: %#v", mode, artifacts)
		}
		for _, want := range wants[mode] {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s safe operation is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestPortableFilesystemPackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { FileError, read_text } from trb/std/filesystem
import { Result } from trb/std/result

def load(path: String): Result<String, FileError>
	return read_text(path)
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
			"typescript": {`import { read_text } from "./trb/std/filesystem/index.ts";`, `import type { FileError } from "./trb/std/filesystem/index.ts";`, "read_text(path)"},
		}[mode]
		for _, want := range consumerWants {
			if output := string(consumer.Output); !strings.Contains(output, want) {
				t.Fatalf("generated %s filesystem consumer is missing %q:\n%s", mode, want, output)
			}
		}
		runtimeWants := map[string][]string{
			"go":         {"type FileError struct", "os.ReadFile(path)", "__trb_result.NewResultErr[string, FileError]", "__trb_result.NewResultOk[unit.Unit, FileError]", "slices.Sort(names)"},
			"ruby":       {"FileError = Data.define(:operation, :path, :message)", "File.binread(path)", "Result::Err.new", "Result::Ok.new(Unit.new)", "Dir.children(path).sort"},
			"typescript": {"export interface FileError", `getBuiltinModule?.("fs")`, "Result.Err<string, FileError>", "Result.Ok<Unit, FileError>", "{} satisfies Unit", "fs.readdirSync(__trbPath)"},
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
			source: "import { read_text } from trb/std/filesystem\nvalue := read_text(1)\n",
			want:   "argument 1 to read_text() has type Integer, expected String",
		},
		{
			name:   "write bytes value",
			source: "import { write_bytes } from trb/std/filesystem\nvalue := write_bytes(\"output.bin\", \"not bytes\")\n",
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

func TestPortableJSONPackagesCompileAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { JsonError, JsonValue, parse } from trb/std/json
import trb/std/jsonc
import { Result } from trb/std/result

def strict(source: String): Result<JsonValue, JsonError>
	return parse(source)
end

def comments(source: String): Result<JsonValue, JsonError>
	return jsonc.parse(source)
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/json-app", RubyLoader: "require_relative"})
		if err != nil {
			t.Fatalf("%s rejected the JSON packages: %v", mode, err)
		}
		modules := map[string]bool{}
		for _, artifact := range artifacts {
			modules[artifact.IR.ModulePath] = true
		}
		for _, module := range []string{"main", "trb/std/json/index", "trb/std/jsonc/index", "trb/std/result/index"} {
			if !modules[module] {
				t.Fatalf("%s omitted compiler-owned module %s: %#v", mode, module, artifacts)
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
			source: "import { parse } from trb/std/json\nvalue := parse(1)\n",
			want:   "argument 1 to parse() has type Integer, expected String",
		},
		{
			name:   "stringify value",
			source: "import { stringify } from trb/std/json\nvalue := stringify(\"not JSON\")\n",
			want:   "argument 1 to stringify() has type String, expected JsonValue",
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
		Source: []byte(`record Address
	city: String
end

record User
	id: Integer @json("user_id")
	name: String
	nickname: String?
	scores: Array<Float>
	metadata: Hash<String, Integer>
	address: Address
end
`),
	}
	main := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Address, User } from contracts/user
import { JsonError, decode, encode } from trb/std/json
import { Result } from trb/std/result

def decode_user(source: String): Result<User, JsonError>
	return decode<User>(source)
end

def encode_user(user: User): Result<String, JsonError>
	return encode(user)
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
			"go":         {"func DecodeUser", "JsonErrorKindDecode", "Id: field0"},
			"ruby":       {"JsonErrorKind::Decode", `User.new(id: field0`},
			"typescript": {"JsonErrorKind.Decode", "return { id: field0"},
		}[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("%s typed JSON codec output does not contain %q:\n%s", mode, want, output)
			}
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
			source: "import { decode } from trb/std/json\nvalue := decode(\"{}\")\n",
			want:   "cannot infer T for decode()",
		},
		{
			name:   "class is not a record codec",
			source: "import { decode } from trb/std/json\nclass User; end\nvalue := decode<User>(\"{}\")\n",
			want:   "JSON codec type User must be a record or JSON-compatible built-in type",
		},
		{
			name:   "unsupported record field",
			source: "import { decode } from trb/std/json\nrecord Document; payload: Bytes; end\nvalue := decode<Document>(\"{}\")\n",
			want:   "JSON codec type Bytes is not supported",
		},
		{
			name:   "recursive record",
			source: "import { decode } from trb/std/json\nrecord Node; child: Node?; end\nvalue := decode<Node>(\"{}\")\n",
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
		Source: []byte(`import contracts/result

def sample(): Result<Integer, String>
	value := identity<Integer>(1)
	return Result<Integer, String>::Ok(value)
end

def unwrap(result: Result<Integer, String>): Integer
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
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
