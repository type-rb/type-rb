package compiler

import (
	"strings"
	"testing"
)

const resultMustUseDiagnostic = "Result value must be used; handle it with try, catch, or case, or explicitly return, pass, or store it"

func TestStandardResultMustUseRejectsBareValuesAndUnusedBindingsAcrossModes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "bare inferred standard Result",
			source: `def run()
	[1].try_fetch(9)
	return
end
`,
			want: resultMustUseDiagnostic,
		},
		{
			name: "bare Result",
			source: `import { Result } from trb/std/result

def operation(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end

def run()
	operation()
	return
end
`,
			want: resultMustUseDiagnostic,
		},
		{
			name: "bare transitive alias",
			source: `import { Result } from trb/std/result

alias InnerResult<T> = Result<T, String>
alias AppResult<T> = InnerResult<T>

def operation(): AppResult<Integer>
	return AppResult<Integer>::Ok(1)
end

def run()
	operation()
	return
end
`,
			want: resultMustUseDiagnostic,
		},
		{
			name: "unused named binding",
			source: `import { Result } from trb/std/result

def operation(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end

def run()
	result := operation()
	return
end
`,
			want: "Result binding result must be used",
		},
		{
			name: "unused underscore-prefixed binding",
			source: `import { Result } from trb/std/result

def operation(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end

def run()
	_result := operation()
	return
end
`,
			want: "Result binding _result must be used",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("result_must_use.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected diagnostic containing %q, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestStandardResultMustUseRecognizesImportedTransparentAlias(t *testing.T) {
	producer := SourceUnit{
		Filename:   "/project/services/operation.trb",
		ModulePath: "services/operation",
		Package:    "services",
		Source: []byte(`import { Result } from trb/std/result

alias AppResult<T> = Result<T, String>

def operation(): AppResult<Integer>
	return AppResult<Integer>::Ok(1)
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { operation } from services/operation

def run()
	operation()
	return
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := CompileProject([]SourceUnit{producer, consumer}, Options{Mode: mode})
		if err == nil || !strings.Contains(err.Error(), resultMustUseDiagnostic) {
			t.Fatalf("%s: expected imported Result alias diagnostic, got %v", mode, err)
		}
	}
}

func TestStandardResultMustUseAcceptsExplicitHandlingAndTransferAcrossModes(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result

alias AppResult<T> = Result<T, String>

def operation(): AppResult<Integer>
	return AppResult<Integer>::Ok(1)
end

def propagate(): AppResult<Integer>
	value := try operation()
	return AppResult<Integer>::Ok(value)
end

def recover(): Integer
	value := operation() catch |_error|
		0
	end
	return value
end

def inspect(): Integer
	case operation()
	when AppResult::Ok(value)
		return value
	when AppResult::Err(_error)
		return 0
	end
end

def forward(): AppResult<Integer>
	return operation()
end

def consume(_result: AppResult<Integer>)
	return
end

def pass()
	consume(operation())
	return
end

def store(): Integer
	results := [operation()]
	return results.size()
end
`)

	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("result_must_use_handled.trb", source, mode); err != nil {
			t.Fatalf("%s rejected explicitly handled or transferred Result: %v", mode, err)
		}
	}
}

func TestStandardResultMustUseDoesNotMatchUserDefinedResult(t *testing.T) {
	source := []byte(`enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end

def operation(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end

def run()
	operation()
	_result := operation()
	return
end
`)

	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("user_result.trb", source, mode); err != nil {
			t.Fatalf("%s treated a user-defined Result as compiler-owned: %v", mode, err)
		}
	}
}

func TestInteractiveTopLevelDisplaysResultButFunctionsRemainChecked(t *testing.T) {
	topLevel := []byte(`import { Result } from trb/std/result

def operation(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end

operation()
`)
	insideFunction := []byte(`import { Result } from trb/std/result

def operation(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end

def run()
	operation()
	return
end
`)

	for _, mode := range []string{"go", "ruby", "typescript"} {
		options := Options{Mode: mode, ModulePath: "__trb_repl__", InteractiveModule: "__trb_repl__"}
		if _, err := CompileWithOptions(".trb-repl.trb", topLevel, options); err != nil {
			t.Fatalf("%s rejected displayed interactive Result: %v", mode, err)
		}
		if _, err := CompileWithOptions(".trb-repl.trb", topLevel, Options{Mode: mode, ModulePath: "__trb_repl__"}); err == nil || !strings.Contains(err.Error(), resultMustUseDiagnostic) {
			t.Fatalf("%s accepted a non-interactive bare Result: %v", mode, err)
		}
		if _, err := CompileWithOptions(".trb-repl.trb", insideFunction, options); err == nil || !strings.Contains(err.Error(), resultMustUseDiagnostic) {
			t.Fatalf("%s exempted a Result inside an interactive function: %v", mode, err)
		}
	}
}
