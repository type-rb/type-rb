package compiler

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func TestResultTryWidensScalarFailureIntoOuterUnionAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Result } from trb/std/result

def integer_failure(): Result<Integer, Integer>
	return Result<Integer, Integer>::Err(7)
end

def widened_failure(): Result<String, Float | String>
	value := try integer_failure()
	return Result<String, Float | String>::Ok(value.to_s())
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{
				Mode: mode, GoModule: "example.com/result-error-union-widening", RubyLoader: "require_relative",
			})
			if err != nil {
				t.Fatalf("%s rejected scalar Result failure widening into a union: %v", mode, err)
			}

			var widened *ir.Method
			for _, artifact := range artifacts {
				if artifact.IR.ModulePath != source.ModulePath {
					continue
				}
				for _, statement := range artifact.IR.Statements {
					method, ok := statement.(*ir.Method)
					if ok && method.Name == "widened_failure" {
						widened = method
					}
				}
			}
			if widened == nil {
				t.Fatalf("%s IR is missing widened_failure()", mode)
			}

			variable, ok := widened.Body[0].(*ir.Variable)
			if !ok {
				t.Fatalf("%s widened_failure() first statement is %T, want *ir.Variable", mode, widened.Body[0])
			}
			propagation, ok := variable.Value.(*ir.Case)
			if !ok || len(propagation.Branches) != 2 || len(propagation.Branches[1].Body) != 1 {
				t.Fatalf("%s try propagation has unexpected IR: %#v", mode, variable.Value)
			}
			returned, ok := propagation.Branches[1].Body[0].(*ir.Return)
			if !ok {
				t.Fatalf("%s Err branch statement is %T, want *ir.Return", mode, propagation.Branches[1].Body[0])
			}
			failure, ok := returned.Value.(*ir.EnumConstruct)
			if !ok || len(failure.Arguments) != 1 {
				t.Fatalf("%s Err branch return is unexpected: %#v", mode, returned.Value)
			}
			conversion, ok := failure.Arguments[0].(*ir.Conversion)
			if !ok || conversion.Kind != ir.IntegerToFloatConversion {
				t.Fatalf("%s propagated error is %T %#v, want Integer-to-Float conversion", mode, failure.Arguments[0], failure.Arguments[0])
			}
			if conversion.ExprType().Kind != types.Float || conversion.Value.ExprType().Kind != types.Int {
				t.Fatalf("%s propagated error conversion is %s from %s, want Float from Integer", mode, conversion.ExprType(), conversion.Value.ExprType())
			}
		})
	}
}
