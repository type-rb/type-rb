package compiler

import (
	"strings"
	"testing"
)

func TestCompileFirstClassFunctionsAcrossBackends(t *testing.T) {
	source := []byte(`def apply(value: Integer, callable: (Integer) -> String): String
	return callable(value)
end

def sample(): String
	prefix := "value: "
	formatter := fn(value: Integer): String
		return prefix + value.to_s()
	end
	return apply(2, formatter)
end
`)
	wants := map[string][]string{
		"go":         {"callable func(int) string", "formatter := func(value int) string", "return callable(value)"},
		"ruby":       {"formatter = ->(value) do", "callable.call(value)"},
		"typescript": {"callable: (arg0: number) => string", "const formatter: (arg0: number) => string = (value: number): string =>", "return (await callable(value));"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("lambda.trb", source, mode)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		output := string(artifact.Output)
		for _, want := range wants[mode] {
			if !strings.Contains(output, want) {
				t.Fatalf("%s output is missing %q:\n%s", mode, want, output)
			}
		}
	}
}

func TestRejectInvalidFirstClassFunctionsAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "missing parameter type",
			source: `def sample()
	value := fn(input)
		puts(input)
		return
	end
	value(1)
	return
end
`,
			want: "fn parameter input requires a type",
		},
		{
			name: "missing return",
			source: `def sample()
	value := fn(input: Integer): String
		puts(input)
	end
	puts(value(1))
	return
end
`,
			want: "fn must return String on every path",
		},
		{
			name: "wrong argument type",
			source: `def sample()
	value := fn(input: Integer): Integer
		return input
	end
	puts(value("wrong"))
	return
end
`,
			want: "argument 1 to fn() has type String, expected Integer",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("invalid_lambda.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestFunctionTypesCrossProjectModuleBoundaries(t *testing.T) {
	contracts := SourceUnit{Filename: "callback.trb", ModulePath: "app/contracts/callback", Source: []byte(`record Callback
	apply: (Integer) -> String
end
`)}
	consumer := SourceUnit{Filename: "consumer.trb", ModulePath: "app/consumer", Source: []byte(`import { Callback } from app/contracts/callback

def consume(callback: Callback): String
	return callback.apply(3)
end
`)}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := CompileProject([]SourceUnit{contracts, consumer}, Options{Mode: mode, TypeScriptRuntime: "bun"}); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}
