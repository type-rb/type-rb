package compiler

import (
	"strings"
	"testing"
)

func TestNilEqualityRunsAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`def main()
	puts(nil == nil)
	puts(nil != nil)
	value: String? := nil
	puts(value == nil)
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{source}, Options{
				Mode: mode, GoModule: "example.com/nil-equality", RubyLoader: "require_relative",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/nil-equality")), "true\nfalse\ntrue"; got != want {
				t.Fatalf("generated %s Nil equality output=%q, want %q", mode, got, want)
			}
		})
	}
}

func TestNilOnlyBindingInferenceIsRejectedAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("nil_binding.trb", []byte("value := nil\n"), mode)
		want := "cannot infer the type of value from nil alone; add an explicit nullable type annotation"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: expected %q, got %v", mode, want, err)
		}
	}
}

func TestNilAndVoidCannotBeWrittenAsValueTypes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "Nil annotation",
			source: "value: Nil := nil\n",
			want:   "Nil is an internal flow type and cannot be written in source; use an explicit nullable type annotation",
		},
		{
			name:   "Void annotation",
			source: "def consume(value: Void)\nend\n",
			want:   "Void may be used only as the return type of a function type",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("invalid_value_type.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestVoidExpressionsAreRejectedAtValueUseAcrossModes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "binding initializer",
			source: "def main()\n\tprinted := puts(\"hello\")\nend\n",
			want:   "Void expression does not produce a value and cannot initialize printed",
		},
		{
			name:   "assignment",
			source: "def main()\n\tmut value: String? := nil\n\tvalue = puts(\"hello\")\nend\n",
			want:   "Void expression does not produce a value and cannot be assigned",
		},
		{
			name:   "argument",
			source: "def consume(value: Any)\nend\ndef main()\n\tconsume(puts(\"hello\"))\nend\n",
			want:   "Void expression does not produce a value and cannot be used as an argument",
		},
		{
			name:   "return value",
			source: "def print_value()\n\treturn puts(\"hello\")\nend\n",
			want:   "Void expression does not produce a value and cannot be returned as a value",
		},
		{
			name:   "operator operand",
			source: "def invalid(): Boolean\n\treturn puts(\"hello\") == nil\nend\n",
			want:   "Void expression does not produce a value and cannot be used as an operand",
		},
		{
			name:   "collection element",
			source: "def main()\n\tvalues := [puts(\"hello\")]\nend\n",
			want:   "Void expression does not produce a value and cannot be stored in a collection",
		},
		{
			name:   "member receiver",
			source: "def main()\n\tputs(\"hello\").to_s()\nend\n",
			want:   "Void expression does not produce a value and cannot be used as a member receiver",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				if _, err := Compile("void_value.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestVoidFunctionCallRemainsAValidExpressionStatement(t *testing.T) {
	source := []byte(`def invoke(callback: () -> Void)
	callback()
end

def main()
	callback: () -> Void := fn()
		puts("hello")
	end
	invoke(callback)
	unit := Unit.new()
	puts(unit)
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("void_statement.trb", source, mode); err != nil {
			t.Fatalf("%s rejected standalone Void call or Unit value: %v", mode, err)
		}
	}
}
