package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestFailsAndAttemptLowerAcrossBackends(t *testing.T) {
	source := []byte(`record AppError
	message: String
end

def read_number(): Integer fails AppError
	return 7
end

def forward_number(): Integer fails AppError
	return read_number()
end

def main()
	direct := attempt read_number()
	grouped := attempt do
		read_number()
	end
	puts(direct)
	puts(grouped)
end
`)

	expected := map[string][]string{
		"go": {
			"func ReadNumber() __trb_result.Result[int, AppError]",
			"return __trb_result.NewResultOk[int, AppError](7)",
			"func ForwardNumber() __trb_result.Result[int, AppError]",
			"__trb_result.ResultOkTag",
			"func() __trb_result.Result[int, AppError]",
		},
		"ruby": {
			"Result::Ok.new(7)",
			"when Result::Ok",
			"-> do",
			"end.call",
		},
		"typescript": {
			"function read_number(): Result<number, AppError>",
			"Result.Ok<number, AppError>(7)",
			`kind === "Ok"`,
			"((): Result<number, AppError> => {",
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("main.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifact.Output)
			for _, want := range expected[mode] {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s effect code is missing %q:\n%s", mode, want, output)
				}
			}
		})
	}
}

func TestGoDiscardsUnusedFallibleCallValueAfterPropagation(t *testing.T) {
	source := []byte(`record AppError
end

def read_number(): Integer fails AppError
	return 7
end

def run(): Integer fails AppError
	read_number()
	return 0
end
`)
	artifact, err := Compile("main.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	if output := string(artifact.Output); !strings.Contains(output, "_ = __trbValue") {
		t.Fatalf("generated Go does not explicitly discard the propagated call value:\n%s", output)
	}
}

func TestAttemptNormalizesVoidEffectsToUnitAcrossBackends(t *testing.T) {
	source := []byte(`record AppError
end

def perform() fails AppError
	return
end

def main()
	result := attempt perform()
	puts(result)
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("main.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected a Void effect attempt: %v", mode, err)
		}
		output := string(artifact.Output)
		if mode == "typescript" {
			if strings.Contains(output, "let __trbValue1: void") || !strings.Contains(output, "let __trbValue1: Unit") {
				t.Fatalf("TypeScript did not normalize the propagated success value to Unit:\n%s", output)
			}
		}
	}
}

func TestFailsDiagnosticsRequireExplicitHandling(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "missing declaration",
			source: `record AppError
end
def source(): Integer fails AppError
	return 1
end
def caller(): Integer
	return source()
end
`,
			want: "add fails AppError or handle it with attempt",
		},
		{
			name: "incompatible declaration",
			source: `record ReadError
end
record WriteError
end
def source(): Integer fails ReadError
	return 1
end
def caller(): Integer fails WriteError
	return source()
end
`,
			want: "declares only fails WriteError",
		},
		{
			name: "pure attempt",
			source: `def value(): Integer
	return attempt 1
end
`,
			want: "attempt requires an expression or block that may fail",
		},
		{
			name: "main effect",
			source: `record AppError
end
def main() fails AppError
	return
end
`,
			want: "main() cannot declare fails",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile("main.trb", []byte(test.source), "go")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected diagnostic containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestInteractiveModuleLeavesOnlyTopLevelEffectsToItsHost(t *testing.T) {
	unit := SourceUnit{
		Filename:   "/project/.trb-repl.trb",
		ModulePath: "__trb_repl__",
		Package:    "main",
		Source: []byte(`record AppError
end
def read_number(): Integer fails AppError
	return 7
end
read_number()
`),
	}
	artifacts, err := CompileProject([]SourceUnit{unit}, Options{
		Mode: "go", GoModule: "example.com/effect-repl", InteractiveModule: "__trb_repl__",
	})
	if err != nil {
		t.Fatal(err)
	}
	var session *Artifact
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath == unit.ModulePath {
			session = artifact
			break
		}
	}
	if session == nil {
		t.Fatal("interactive artifact is missing")
	}
	last, ok := session.IR.Statements[len(session.IR.Statements)-1].(*ir.ExpressionStatement)
	if !ok {
		t.Fatalf("interactive call lowered to %T", session.IR.Statements[len(session.IR.Statements)-1])
	}
	if _, ok := last.Expression.(*ir.UnhandledEffect); !ok {
		t.Fatalf("top-level effect lowered to %T, want *ir.UnhandledEffect", last.Expression)
	}
	resultRuntime := false
	for _, statement := range session.IR.Statements {
		if imported, ok := statement.(*ir.Import); ok && imported.Path == "trb/std/result/index" && imported.Implicit && imported.RuntimeRequired {
			resultRuntime = true
		}
	}
	if !resultRuntime {
		t.Fatal("interactive fallible call did not load its Result runtime")
	}

	unit.Source = []byte(`record AppError
end
def read_number(): Integer fails AppError
	return 7
end
def invalid(): Integer
	return read_number()
end
`)
	_, err = CompileProject([]SourceUnit{unit}, Options{
		Mode: "go", GoModule: "example.com/effect-repl", InteractiveModule: "__trb_repl__",
	})
	if err == nil || !strings.Contains(err.Error(), "add fails AppError or handle it with attempt") {
		t.Fatalf("interactive named function omitted effect diagnostic: %v", err)
	}
}

func TestFailsSignaturesCrossProjectImports(t *testing.T) {
	service := SourceUnit{
		Filename:   "/project/services/numbers.trb",
		ModulePath: "services/numbers",
		Package:    "services",
		Source: []byte(`record AppError
end
def read_number(): Integer fails AppError
	return 7
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { AppError, read_number } from services/numbers
def forward_number(): Integer fails AppError
	return read_number()
end
def main()
	puts(attempt forward_number())
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{consumer, service}, Options{
				Mode: mode, GoModule: "example.com/effects", RubyLoader: "require_relative",
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
