package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestEvaluateCompiledResultTryAndCatchAcrossModes(t *testing.T) {
	source := compiler.SourceUnit{
		Filename:   "/project/.trb-repl.trb",
		ModulePath: "__trb_repl__",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result

enum AppError
	NotFound
end

type AppResult<T> = Result<T, AppError>

def source(success: Boolean): AppResult<Integer>
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

def render(result: AppResult<String>): String
	case result
	when AppResult::Ok(value)
		return "ok:" + value
	when AppResult::Err(_error)
		return "err"
	end
end

[
	render(propagated(true)),
	render(propagated(false)),
	recovered(true).to_s(),
	recovered(false).to_s(),
	returned(true),
	returned(false),
]
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{source}, compiler.Options{
				Mode: mode, GoModule: "example.com/result-repl", RubyLoader: "require_relative", InteractiveModule: source.ModulePath,
			})
			if err != nil {
				t.Fatalf("%s rejected executable Result try/catch source: %v", mode, err)
			}

			programs := make([]*ir.Program, 0, len(artifacts))
			var session *ir.Program
			for _, artifact := range artifacts {
				programs = append(programs, artifact.IR)
				if artifact.IR.ModulePath == source.ModulePath {
					session = artifact.IR
				}
			}
			if session == nil {
				t.Fatalf("%s compilation did not produce the interactive session", mode)
			}

			evaluator := NewEvaluator(&bytes.Buffer{}, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			if err := evaluator.LoadProject(programs, source.ModulePath); err != nil {
				t.Fatalf("%s could not load the Result try/catch project: %v", mode, err)
			}
			evaluator.LoadDefinitions(session)
			result, err := evaluator.Evaluate(session.Statements, source.ModulePath)
			if err != nil {
				t.Fatalf("%s Result try/catch evaluation failed: %v", mode, err)
			}
			if got, want := Inspect(result.Value), `["ok:value=7", "err", "7", "41", "7", "caught"]`; !result.Display || got != want {
				t.Fatalf("%s Result try/catch evaluation=%s display=%t, want %s", mode, got, result.Display, want)
			}
		})
	}
}
