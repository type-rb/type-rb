package typescript

import (
	"fmt"
	"strings"

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
	for method, suspends := range plan.ParameterDefaults {
		if suspends && isReactComponent(method) {
			return nil, fmt.Errorf("TypeScript React component parameter defaults cannot use an operation that may suspend")
		}
	}
	if err := validateSuspendingDeclarationInitializers(programs, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func validateSuspendingDeclarationInitializers(programs []*ir.Program, plan *SuspensionPlan) error {
	for _, program := range programs {
		if err := suspendingDeclarationInitializerError(program.Statements, plan, ""); err != nil {
			return err
		}
	}
	return nil
}

func suspendingDeclarationInitializerError(statements []ir.Statement, plan *SuspensionPlan, namespace string) error {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Module:
			owner := qualifiedInitializerOwner(namespace, node.Name)
			if err := suspendingModuleInitializerError(node.Body, plan, owner); err != nil {
				return err
			}
		case *ir.Class:
			if err := suspendingClassInitializerError(node, plan, qualifiedInitializerOwner(namespace, node.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func suspendingModuleInitializerError(statements []ir.Statement, plan *SuspensionPlan, owner string) error {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Variable:
			if expressionSuspends(plan, node.Value) {
				return fmt.Errorf("TypeScript module constant %s::%s cannot use an operation that may suspend", owner, node.Name)
			}
		case *ir.Module:
			if err := suspendingModuleInitializerError(node.Body, plan, qualifiedInitializerOwner(owner, node.Name)); err != nil {
				return err
			}
		case *ir.Class:
			if err := suspendingClassInitializerError(node, plan, qualifiedInitializerOwner(owner, node.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func suspendingClassInitializerError(class *ir.Class, plan *SuspensionPlan, owner string) error {
	for _, statement := range class.Body {
		switch node := statement.(type) {
		case *ir.Field:
			if expressionSuspends(plan, node.Value) {
				return fmt.Errorf("TypeScript class field %s#%s cannot use an operation that may suspend", owner, strings.TrimPrefix(node.Name, "@"))
			}
		case *ir.Variable:
			if expressionSuspends(plan, node.Value) {
				return fmt.Errorf("TypeScript class constant %s::%s cannot use an operation that may suspend", owner, node.Name)
			}
		case *ir.Method:
			if node.Name == "initialize" && plan.Methods[node] {
				return fmt.Errorf("TypeScript class initializer %s#initialize cannot use an operation that may suspend", owner)
			}
		case *ir.Module:
			if err := suspendingModuleInitializerError(node.Body, plan, qualifiedInitializerOwner(owner, node.Name)); err != nil {
				return err
			}
		case *ir.Class:
			if err := suspendingClassInitializerError(node, plan, qualifiedInitializerOwner(owner, node.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func expressionSuspends(plan *SuspensionPlan, expression ir.Expression) bool {
	return expression != nil && plan != nil && plan.Expressions[expression]
}

func qualifiedInitializerOwner(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "::" + name
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
