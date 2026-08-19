package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestEvaluateResultTryWidensScalarFailureIntoOuterUnionAcrossModes(t *testing.T) {
	source := compiler.SourceUnit{
		Filename:   "/project/.trb-repl.trb",
		ModulePath: "__trb_repl__",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result

def integer_failure(): Result<Integer, Integer>
	return Result<Integer, Integer>::Err(7)
end

def widened_failure(): Result<String, Float | String>
	value := try integer_failure()
	return Result<String, Float | String>::Ok(value.to_s())
end

def widened_error_is_float?(): Boolean
	case widened_failure()
	when Result::Ok(_value)
		return false
	when Result::Err(error)
		case error
		when Float(value)
			return value == 7.0
		when String(_value)
			return false
		end
	end
end

widened_error_is_float?()
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{source}, compiler.Options{
				Mode: mode, GoModule: "example.com/result-error-union-widening-repl", RubyLoader: "require_relative", InteractiveModule: source.ModulePath,
			})
			if err != nil {
				t.Fatalf("%s rejected scalar Result failure widening: %v", mode, err)
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
				t.Fatalf("%s could not load the scalar Result failure widening project: %v", mode, err)
			}
			evaluator.LoadDefinitions(session)
			result, err := evaluator.Evaluate(session.Statements, source.ModulePath)
			if err != nil {
				t.Fatalf("%s scalar Result failure widening evaluation failed: %v", mode, err)
			}
			if got, want := Inspect(result.Value), "true"; !result.Display || got != want {
				t.Fatalf("%s scalar Result failure widening evaluation=%s display=%t, want %s", mode, got, result.Display, want)
			}
		})
	}
}
