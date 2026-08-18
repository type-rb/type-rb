package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestResultTryAndCatchLowerAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result

enum AppError
	NotFound
	Invalid
end

type AppResult<T> = Result<T, AppError>
type NestedAppResult<T> = AppResult<T>

def source(success: Boolean): NestedAppResult<Integer>
	if success
		return AppResult<Integer>::Ok(7)
	end
	return AppResult<Integer>::Err(AppError::NotFound)
end

def propagated(success: Boolean): AppResult<String>
	value := try source(success)
	return AppResult<String>::Ok("value=" + value.to_s())
end

def recovered(success: Boolean): Integer
	return source(success) catch |_error|
		41
	end
end

def returned(success: Boolean): String
	value := source(success) catch |_error|
		return "caught"
	end
	return value.to_s()
end
`),
	}

	wants := map[string][]string{
		"go": {
			"func Propagated(success bool)",
			"ResultErrTag",
			"NewResultErr[string, AppError]",
			"func Recovered(success bool) int",
			"return 41",
			`return "caught"`,
		},
		"ruby": {
			"def propagated(success)",
			"when Result::Err",
			"Result::Err.new",
			"def recovered(success)",
			"41",
			`return "caught"`,
		},
		"typescript": {
			"function propagated(success: boolean)",
			`kind === "Err"`,
			"Result.Err<string, AppError>",
			"function recovered(success: boolean): number",
			"41",
			`return "caught";`,
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{
				Mode: mode, GoModule: "example.com/result-flow", RubyLoader: "require_relative",
			})
			if err != nil {
				t.Fatalf("%s rejected Result try/catch: %v", mode, err)
			}
			var consumer *Artifact
			for _, artifact := range artifacts {
				if artifact.IR.ModulePath == source.ModulePath {
					consumer = artifact
					break
				}
			}
			if consumer == nil {
				t.Fatalf("%s did not produce the Result try/catch consumer", mode)
			}
			for _, want := range wants[mode] {
				if output := string(consumer.Output); !strings.Contains(output, want) {
					t.Fatalf("generated %s Result try/catch code is missing %q:\n%s", mode, want, output)
				}
			}

			methods := map[string]*ir.Method{}
			for _, statement := range consumer.IR.Statements {
				if method, ok := statement.(*ir.Method); ok {
					methods[method.Name] = method
				}
			}
			for _, name := range []string{"propagated", "recovered", "returned"} {
				if methods[name] == nil {
					t.Fatalf("%s IR is missing %s(): %#v", mode, name, consumer.IR.Statements)
				}
			}
			if variable, ok := methods["propagated"].Body[0].(*ir.Variable); !ok {
				t.Fatalf("%s propagated() first statement is %T, want *ir.Variable", mode, methods["propagated"].Body[0])
			} else if _, ok := variable.Value.(*ir.Case); !ok {
				t.Fatalf("%s prefix try lowered to %T, want exhaustive *ir.Case", mode, variable.Value)
			}
		})
	}
}

func TestResultTryExpandsImportedAliasChainsAcrossBackends(t *testing.T) {
	contract := SourceUnit{
		Filename:   "/project/contracts/result.trb",
		ModulePath: "contracts/result",
		Package:    "contracts",
		Source: []byte(`import { Result } from trb/std/result

record AppError
	message: String
end

type AppResult<T> = Result<T, AppError>
type NestedAppResult<T> = AppResult<T>

def source(): NestedAppResult<Integer>
	return AppResult<Integer>::Ok(7)
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { AppResult, NestedAppResult, source } from contracts/result

type ConsumerResult<T> = NestedAppResult<T>

def propagated(): ConsumerResult<String>
	value := try source()
	return AppResult<String>::Ok(value.to_s())
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject([]SourceUnit{contract, consumer}, Options{
				Mode: mode, GoModule: "example.com/imported-result-alias", RubyLoader: "require_relative",
			}); err != nil {
				t.Fatalf("%s rejected a transitive imported alias of standard Result: %v", mode, err)
			}
		})
	}
}

func TestResultTryAndCatchDiagnosticsAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "try non Result",
			source: `import { Result } from trb/std/result
enum AppError
	Stopped
end
def invalid(): Result<Integer, AppError>
	value := try 1
	return Result<Integer, AppError>::Ok(value)
end
`,
			want: "try requires the standard Result<T, E>, got Integer",
		},
		{
			name: "catch non Result",
			source: `import { Result } from trb/std/result
def invalid(): Integer
	return 1 catch |_error|
		0
	end
end
`,
			want: "catch requires the standard Result<T, E>, got Integer",
		},
		{
			name: "non Result return boundary",
			source: `import { Result } from trb/std/result
enum AppError
	Stopped
end
def source(): Result<Integer, AppError>
	return Result<Integer, AppError>::Ok(1)
end
def invalid(): Integer
	return try source()
end
`,
			want: "try requires the enclosing function to return Result<T, E>, got Integer",
		},
		{
			name: "incompatible propagated error",
			source: `import { Result } from trb/std/result
enum ReadError
	Stopped
end
enum WriteError
	Stopped
end
def source(): Result<Integer, ReadError>
	return Result<Integer, ReadError>::Err(ReadError::Stopped)
end
def invalid(): Result<Integer, WriteError>
	value := try source()
	return Result<Integer, WriteError>::Ok(value)
end
`,
			want: "try cannot propagate ReadError through Result error type WriteError",
		},
		{
			name: "catch fallback type mismatch",
			source: `import { Result } from trb/std/result
enum AppError
	Stopped
end
def source(): Result<Integer, AppError>
	return Result<Integer, AppError>::Err(AppError::Stopped)
end
def invalid(): Integer
	return source() catch |_error|
		"wrong"
	end
end
`,
			want: "catch handler has type String, expected Integer",
		},
		{
			name: "value producing transform",
			source: `import { Result } from trb/std/result
enum AppError
	Stopped
end
def source(): Result<Integer, AppError>
	return Result<Integer, AppError>::Ok(1)
end
def invalid(): Result<Array<Integer>, AppError>
	values := [1].map do |_value|
		try source()
	end
	return Result<Array<Integer>, AppError>::Ok(values)
end
`,
			want: "try is not supported inside value-producing collection transformations",
		},
		{
			name: "return hidden in catch inside value producing transform",
			source: `import { Result } from trb/std/result
enum AppError
	Stopped
end
def source(): Result<Integer, AppError>
	return Result<Integer, AppError>::Err(AppError::Stopped)
end
def invalid(): Array<Integer>
	return [1].map do |_value|
		source() catch |_error|
			return 0
		end
	end
end
`,
			want: "return is not supported inside value-producing collection transformations",
		},
		{
			name: "break hidden in catch inside value producing transform",
			source: `import { Result } from trb/std/result
def source(): Result<Integer, String>
	return Result<Integer, String>::Err("stopped")
end
def invalid(): Integer
	mut count := 0
	while count < 1
		_values := [1].map do |_value|
			source() catch |_error|
				break
			end
		end
		count += 1
	end
	return count
end
`,
			want: "break is not supported inside value-producing collection transformations",
		},
		{
			name: "next hidden in catch inside value producing transform",
			source: `import { Result } from trb/std/result
def source(): Result<Integer, String>
	return Result<Integer, String>::Err("stopped")
end
def invalid(): Integer
	mut count := 0
	while count < 1
		_values := [1].map do |_value|
			source() catch |_error|
				next
			end
		end
		count += 1
	end
	return count
end
`,
			want: "next is not supported inside value-producing collection transformations",
		},
		{
			name: "nullable Result operand",
			source: `import { Result } from trb/std/result
enum AppError
	Stopped
end
def source(): Result<Integer, AppError>?
	return nil
end
def invalid(): Result<Integer, AppError>
	value := try source()
	return Result<Integer, AppError>::Ok(value)
end
`,
			want: "try requires the standard Result<T, E>, got Result<Integer, AppError>?",
		},
		{
			name: "nullable Result catch operand",
			source: `import { Result } from trb/std/result
enum AppError
	Stopped
end
def source(): Result<Integer, AppError>?
	return nil
end
def invalid(): Integer
	return source() catch |_error|
		0
	end
end
`,
			want: "catch requires the standard Result<T, E>, got Result<Integer, AppError>?",
		},
		{
			name: "nested non Result function boundary",
			source: `import { Result } from trb/std/result
enum AppError
	Stopped
end
def source(): Result<Integer, AppError>
	return Result<Integer, AppError>::Ok(1)
end
def invalid(): Result<Integer, AppError>
	callback := fn(): Integer
		return try source()
	end
	return Result<Integer, AppError>::Ok(callback())
end
`,
			want: "try requires the enclosing function to return Result<T, E>, got Integer",
		},
		{
			name: "unused catch binding",
			source: `import { Result } from trb/std/result
record AppError
	message: String
end
def source(): Result<Integer, AppError>
	return Result<Integer, AppError>::Err(AppError.new(message: "stopped"))
end
def invalid(): Integer
	return source() catch |error|
		0
	end
end
`,
			want: "catch binding error is not used",
		},
		{
			name: "catch binding scope",
			source: `import { Result } from trb/std/result
record AppError
	message: String
end
def source(): Result<String, AppError>
	return Result<String, AppError>::Err(AppError.new(message: "stopped"))
end
def invalid(): String
	value := source() catch |error|
		error.message
	end
	error = AppError.new(message: "outside")
	return value
end
`,
			want: "error is not declared",
		},
		{
			name: "local Result lookalike",
			source: `enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end
def source(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end
def invalid(): Result<Integer, String>
	value := try source()
	return Result<Integer, String>::Ok(value)
end
`,
			want: "try requires the standard Result<T, E>, got Result<Integer, String>",
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				_, err := CompileProject([]SourceUnit{{
					Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(test.source),
				}}, Options{Mode: mode, GoModule: "example.com/result-diagnostics", RubyLoader: "require_relative"})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("expected %q diagnostic, got %v", test.want, err)
				}
			})
		}
	}
}

func TestResultCatchContextualizesEmptyCollectionFallbacksAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result

def array_source(): Result<Array<Integer>, String>
	return Result<Array<Integer>, String>::Err("array")
end

def hash_source(): Result<Hash<String, Integer>, String>
	return Result<Hash<String, Integer>, String>::Err("hash")
end

def recovered_array(): Array<Integer>
	return array_source() catch |_error|
		[]
	end
end

def recovered_hash(): Hash<String, Integer>
	return hash_source() catch |_error|
		{}
	end
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject([]SourceUnit{source}, Options{
				Mode: mode, GoModule: "example.com/result-catch-collections", RubyLoader: "require_relative",
			}); err != nil {
				t.Fatalf("%s rejected contextually typed catch collection fallback: %v", mode, err)
			}
		})
	}
}

func TestValueProducingTransformsAllowNestedIterationControlAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename: "/project/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`def nested_controls(): Array<Integer>
	return [1].map do |_outer|
		[2].each do |_inner|
			break
		end
		[3].each do |_inner|
			next
		end
		1
	end
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject([]SourceUnit{source}, Options{
				Mode: mode, GoModule: "example.com/nested-transform-control", RubyLoader: "require_relative",
			}); err != nil {
				t.Fatalf("%s rejected nested iteration-owned break/next: %v", mode, err)
			}
		})
	}
}

func TestResultTryRejectsImportedResultLookalikeAcrossBackends(t *testing.T) {
	lookalike := SourceUnit{
		Filename:   "/project/contracts/result.trb",
		ModulePath: "contracts/result",
		Package:    "contracts",
		Source: []byte(`enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from contracts/result

def source(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end

def invalid(): Result<Integer, String>
	value := try source()
	return Result<Integer, String>::Ok(value)
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{lookalike, consumer}, Options{
				Mode: mode, GoModule: "example.com/result-lookalike", RubyLoader: "require_relative",
			})
			if err == nil || !strings.Contains(err.Error(), "try requires the standard Result<T, E>, got Result<Integer, String>") {
				t.Fatalf("%s accepted an imported Result lookalike: %v", mode, err)
			}
		})
	}
}

func TestResultControlKeywordsAreReservedAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "try function",
			source: `def try(): Integer
	return 1
end
`,
			want: "try is a reserved keyword and cannot be used as a function or method name",
		},
		{
			name: "catch method",
			source: `class Worker
	def catch(): Integer
		return 1
	end
end
`,
			want: "catch is a reserved keyword and cannot be used as a function or method name",
		},
		{
			name: "catch variable",
			source: `def invalid(): Integer
	catch := 1
	return catch
end
`,
			want: "catch is a reserved keyword and cannot be used as a variable name",
		},
		{
			name: "try parameter",
			source: `def invalid(try: Integer): Integer
	return try
end
`,
			want: "try is a reserved keyword and cannot be used as a parameter name",
		},
		{
			name: "catch block parameter",
			source: `def invalid()
	[1].each do |catch|
		puts(catch)
	end
end
`,
			want: "catch is a reserved keyword and cannot be used as a block parameter",
		},
		{
			name: "catch pattern binding",
			source: `enum Value
	Present(value: Integer)
end
def invalid(value: Value): Integer
	case value
	when Value::Present(catch)
		return catch
	end
end
`,
			want: "catch is a reserved keyword and cannot be used as a pattern binding",
		},
		{
			name: "catch record field",
			source: `record Invalid
	catch: Integer
end
`,
			want: "catch is a reserved keyword and cannot be used as a record field",
		},
		{
			name: "compiler prefix variable",
			source: `def invalid(): Integer
	__trbCatchValue1 := 1
	return __trbCatchValue1
end
`,
			want: "__trbCatchValue1 uses the compiler-reserved __trb prefix and cannot be used as a variable name",
		},
		{
			name: "compiler prefix catch binding",
			source: `import { Result } from trb/std/result
def source(): Result<Integer, String>
	return Result<Integer, String>::Err("stopped")
end
def invalid(): Integer
	return source() catch |__trbValue1|
		0
	end
end
`,
			want: "__trbValue1 uses the compiler-reserved __trb prefix and cannot be used as a catch binding",
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				_, err := CompileProject([]SourceUnit{{
					Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(test.source),
				}}, Options{Mode: mode, GoModule: "example.com/result-keywords", RubyLoader: "require_relative"})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("expected %q diagnostic, got %v", test.want, err)
				}
			})
		}
	}
}

func TestResultControlKeywordPrefixesRemainAvailableAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename: "/project/main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`def try_fetch(): Integer
	return 1
end

def catch?(): Boolean
	return true
end

def valid(): Integer
	retry_count := try_fetch()
	if catch?()
		return retry_count
	end
	return 0
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject([]SourceUnit{source}, Options{
				Mode: mode, GoModule: "example.com/result-keyword-prefixes", RubyLoader: "require_relative",
			}); err != nil {
				t.Fatalf("%s rejected identifiers that only contain a Result keyword: %v", mode, err)
			}
		})
	}
}

func TestResultControlKeywordsAreReservedInNestedJSX(t *testing.T) {
	_, err := CompileProject([]SourceUnit{{
		Filename: "/project/view.trb", ModulePath: "view", Package: "view",
		Source: []byte(`import { ReactNode } from trb/platform/typescript/react

def view(): ReactNode
	return <div><span catch="stopped" /></div>
end
`),
	}}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	want := "catch is a reserved keyword and cannot be used as a JSX attribute"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("expected %q diagnostic, got %v", want, err)
	}
}
