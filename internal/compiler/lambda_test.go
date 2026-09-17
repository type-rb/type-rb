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

func TestGenericCallableFactoriesUseDeclarationArgumentsAcrossModes(t *testing.T) {
	declarations := `def keep<T>(value: T, *, label: String = "saved"): () -> T
 puts(label)
 return fn(): T
  return value
 end
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			source := declarations + `def sample(): String
number := keep<Integer>(7)
text := keep<String>("held", label: "explicit")
puts(number())
return text()
end
`
			if _, err := Compile("factory.trb", []byte(source), mode); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct{ body, want string }{
				{`value := keep<Integer>()`, "keep() is missing required argument 1"},
				{`value := keep<Integer>("bad")`, "expected Integer"},
				{`value := keep<Integer>(7, label: 1)`, "expected String"},
				{`value := keep<Integer>(7); value(1)`, "fn() expects"},
			} {
				_, err := Compile("invalid_factory.trb", []byte(declarations+"def sample()\n"+tc.body+"\nend\n"), mode)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("%s: want %q, got %v", tc.body, tc.want, err)
				}
			}
		})
	}
}

func TestImportedGenericCallableFactoriesAcrossModes(t *testing.T) {
	factory := SourceUnit{Filename: "factory.trb", ModulePath: "factory", Source: []byte(`def keep<T>(value: T): () -> T
return fn(): T
return value
end
end
`)}
	consumer := SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte(`import { keep as retained } from factory
def sample(): Integer
callback := retained<Integer>(7)
return callback()
end
`)}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject([]SourceUnit{factory, consumer}, Options{Mode: mode, SourceRoot: "/project", ProjectRoot: "/project"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
