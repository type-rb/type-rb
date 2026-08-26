package typescript

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/codegen/effectplan"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/runtimeoperation"
	"github.com/type-rb/type-rb/internal/types"
)

// SuspensionPlan is backend-owned analysis data. It deliberately does not
// make suspension part of TypeRB syntax, checking, or shared semantics.
type SuspensionPlan = effectplan.Plan

func AnalyzeSuspension(programs []*ir.Program) (*SuspensionPlan, error) {
	plan := effectplan.Analyze(programs, effectplan.Options{
		Intrinsic: isSuspendingIntrinsic,
		Runtime: func(binding *ir.RuntimeBinding) bool {
			return binding.MaySuspend
		},
		Conversion: func(kind ir.ConversionKind) bool {
			return kind == ir.PromiseRejectionToResultConversion
		},
		Transform: func(transform *ir.Transform) bool {
			return transform.Operation == "concurrent_map"
		},
		WebNext:         true,
		PassToFunctions: true,
	})
	standardResults := make(map[string]bool, len(programs))
	for _, program := range programs {
		standardResults[program.ModulePath] = standardResultAvailable(program)
	}
	for lambda, suspends := range plan.Lambdas {
		resultBoundary := standardResults[plan.LambdaModules[lambda]] && functionReturnsStandardResult(lambda.ExprType())
		if suspends && lambda.ReturnType.Kind != types.Void && !resultBoundary {
			return nil, fmt.Errorf("TypeScript function values that may suspend must omit their return type")
		}
	}
	return plan, nil
}

func functionReturnsStandardResult(function types.Type) bool {
	_, returned, ok := types.FunctionSignature(function)
	return ok && !returned.Nullable && returned.Name == "Result" && len(returned.Args) == 2
}

func standardResultAvailable(program *ir.Program) bool {
	if program == nil || standardResultModule(program.ModulePath) {
		return true
	}
	return !statementsShadowStandardResult(program.Statements)
}

func statementsShadowStandardResult(statements []ir.Statement) bool {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Import:
			if node.Namespace || standardResultModule(node.Path) {
				continue
			}
			for _, symbol := range node.Symbols {
				if symbol == "Result" {
					return true
				}
			}
		case *ir.Class:
			if node.Name == "Result" || statementsShadowStandardResult(node.Body) {
				return true
			}
		case *ir.Record:
			if node.Name == "Result" || statementsShadowStandardResult(node.Body) {
				return true
			}
		case *ir.Enum:
			if node.Name == "Result" || statementsShadowStandardResult(node.Body) {
				return true
			}
		case *ir.TypeAlias:
			if node.Name == "Result" {
				return true
			}
		case *ir.Newtype:
			if node.Name == "Result" {
				return true
			}
		case *ir.Interface:
			if node.Name == "Result" {
				return true
			}
		case *ir.Module:
			if statementsShadowStandardResult(node.Body) {
				return true
			}
		case *ir.NativeBlock:
			if statementsShadowStandardResult(node.Body) {
				return true
			}
		}
	}
	return false
}

func standardResultModule(module string) bool {
	return module == "trb/std/result" || module == "trb/std/result/index"
}

func isSuspendingIntrinsic(intrinsic string) bool {
	return runtimeoperation.Describe(intrinsic).MaySuspend
}
