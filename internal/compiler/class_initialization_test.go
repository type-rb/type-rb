package compiler

import (
	"strings"
	"testing"
)

func TestConstructorFieldInitializationPaths(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"both branches", "if flag\n@value = 1\nelse\n@value = 2\nend", ""},
		{"branch missing", "if flag\n@value = 1\nend", "must be initialized"},
		{"early return", "if flag\nreturn\nend\n@value = 1", "must be initialized"},
		{"initialized return", "if flag\n@value = 1\nreturn\nend\n@value = 2", ""},
		{"unreachable assignment", "puts(flag)\nreturn\n@value = 1", "must be initialized"},
		{"zero-trip while", "while flag\n@value = 1\nbreak\nend", "must be initialized"},
		{"zero-trip each", "puts(flag)\n[1].each { |value| @value = value }", "must be initialized"},
		{"iteration early return", "[1].each { |value|\nputs(value)\nreturn if flag\n}\n@value = 1", "must be initialized"},
		{"break before read", "while flag\nbreak\nputs(@value)\nend\n@value = 1", ""},
		{"next before assignment", "while flag\nnext\n@value = 1\nend", "must be initialized"},
		{"read before write", "puts(flag)\nputs(@value)\n@value = 1", "read before initialization"},
		{"self read before write", "puts(flag)\nputs(self.value)\n@value = 1", "read before initialization"},
		{"self assignment remains readonly", "puts(flag)\nself.value = 1", "self is immutable"},
		{"compound first write", "puts(flag)\n@value += 1", "read before initialization"},
		{"rhs read", "puts(flag)\n@value = @value + 1", "read before initialization"},
		{"implicit call", "puts(flag)\nputs(read())\n@value = 1", "self cannot be used before"},
		{"explicit call", "puts(flag)\nputs(self.read())\n@value = 1", "self cannot be used before"},
		{"receiver alias", "puts(flag)\nalias_value := self\n@value = 1\nputs(alias_value.value)", "self cannot be used before"},
		{"deferred writer", "puts(flag)\nwrite := fn()\n@value = 1\nend\nwrite()", "self cannot be used before"},
		{"deferred reader", "puts(flag)\nread_value := fn(): Integer\nreturn @value\nend\n@value = 1\nputs(read_value())", "self cannot be used before"},
		{"deferred unrelated return", "puts(flag)\nread_value := fn(): Integer\nreturn 3\nend\n@value = read_value()", ""},
		{"expression branch transfer", "@value = if flag\nreturn\nelse\n2\nend", "must be initialized"},
		{"nested expression writes", "puts(if flag\n@value = 1\ntrue\nelse\n@value = 2\nfalse\nend)", ""},
		{"short circuit writes", "puts(flag && if flag\n@value = 1\ntrue\nelse\n@value = 2\nfalse\nend)", "must be initialized"},
		{"safe call argument writes", "text: String? := nil\ntext&.replace_all(if flag\n@value = 1\n\"a\"\nelse\n@value = 2\n\"b\"\nend, \"c\")", "must be initialized"},
		{"safe iteration argument writes", "values: Array<Integer>? := nil\nvalues&.each_slice(if flag\n@value = 1\n1\nelse\n@value = 2\n2\nend) { |items| puts(items.size()) }", "must be initialized"},
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				source := "class Value\n@value: Integer\ndef initialize(flag: Boolean)\n" + tc.body + "\nend\ndef read(): Integer\nreturn @value\nend\nend\ndef main()\nputs(Value.new(false).value)\nend\n"
				_, err := Compile("initialization.trb", []byte(source), mode)
				if tc.want == "" {
					if err != nil {
						t.Fatalf("valid constructor rejected: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v, want %q", err, tc.want)
				}
			})
		}
	}
}

func TestConstructorFieldInitializationDefaultsAndEnumPaths(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"default satisfies field", "class Value\n@value: Integer := 3\nend", ""},
		{"ordered defaults", "class Value\n@first: Integer := 2\n@value: Integer := @first + 1\nend", ""},
		{"forward default", "class Value\n@value: Integer := @later\n@later: Integer := 2\nend", "read before initialization"},
		{"self default", "class Value\n@value: Integer := self.value\nend", "read before initialization"},
		{"partial receiver call", "class Value\n@first: Integer := 2\n@value: Integer\ndef initialize()\n@value = read()\nend\ndef read(): Integer\nreturn @first\nend\nend", "self cannot be used before"},
		{"read initialized field", "class Value\n@first: Integer := 2\n@value: Integer\ndef initialize()\n@value = @first + self.first\nend\nend", ""},
		{"all fields defaulted receiver", "class Value\n@value: Integer := 2\ndef initialize()\nputs(read())\nend\ndef read(): Integer\nreturn @value\nend\nend", ""},
		{"exhaustive enum", "enum Choice\nFirst\nSecond\nend\nclass Value\n@value: Integer\ndef initialize(choice: Choice)\ncase choice\nwhen Choice::First\n@value = 1\nwhen Choice::Second\n@value = 2\nend\nend\nend", ""},
		{"enum arm uninitialized", "enum Choice\nFirst\nSecond\nend\nclass Value\n@value: Integer\ndef initialize(choice: Choice)\ncase choice\nwhen Choice::First\n@value = 1\nwhen Choice::Second\nputs(2)\nend\nend\nend", "must be initialized"},
		{"case else initializes", "enum Choice\nFirst\nSecond\nend\nclass Value\n@value: Integer\ndef initialize(choice: Choice)\ncase choice\nwhen Choice::First\n@value = 1\nelse\n@value = 2\nend\nend\nend", ""},
		{"catch early return", "import { Result } from trb/std/result\nclass Value\n@value: Integer\ndef initialize(result: Result<Integer, String>)\n@value = result catch |_|\nreturn\nend\nend\nend", "must be initialized"},
		{"catch value", "import { Result } from trb/std/result\nclass Value\n@value: Integer\ndef initialize(result: Result<Integer, String>)\n@value = result catch |_|\n3\nend\nend\nend", ""},
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				_, err := Compile("defaults.trb", []byte(tc.source+"\ndef main()\nend\n"), mode)
				if tc.want == "" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v, want %q", err, tc.want)
				}
			})
		}
	}
}

func TestConstructorInitializationDiagnosticsKeepImportedOrigins(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := CompileProject([]SourceUnit{
			{Filename: "/project/models.trb", ModulePath: "models", Source: []byte("module Models\nclass Value<T>\n@value: T\ndef initialize(value: T, flag: Boolean)\nif flag\n@value = value\nend\nend\nend\nend\n")},
			{Filename: "/project/main.trb", ModulePath: "main", Source: []byte("import { Models as Library } from models\ndef main()\nLibrary::Value<String>.new(\"kept\", false)\nend\n")},
		}, Options{Mode: mode, SourceRoot: "/project", ProjectRoot: "/project"})
		if err == nil || !strings.Contains(err.Error(), "models.trb:3:1:") || !strings.Contains(err.Error(), "must be initialized") {
			t.Fatalf("%s: missing imported constructor origin: %v", mode, err)
		}
	}
}
