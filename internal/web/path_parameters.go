package web

import (
	"fmt"
	"strconv"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/resolver"
)

func validatePathParameterCalls(route Route, program *ast.Program, resolved resolver.Result) []Issue {
	if program == nil {
		return nil
	}
	declared := make(map[string]bool, len(route.PathParameters))
	for _, name := range route.PathParameters {
		declared[name] = true
	}
	var issues []Issue
	walkStatements(program.Statements, func(call *ast.CallExpression) {
		if !officialPathParameterCall(call, resolved) || len(call.Arguments) != 2 {
			return
		}
		argument := call.Arguments[1].Value
		literal, ok := argument.(*ast.Literal)
		if !ok || literal.Kind != ast.StringLiteral {
			issues = append(issues, Issue{
				Filename: route.Filename,
				Message:  "path_param() name must be a string literal in a route file",
				Span:     argument.Span(),
			})
			return
		}
		name, err := strconv.Unquote(literal.Raw)
		if err != nil || declared[name] {
			return
		}
		issues = append(issues, Issue{
			Filename: route.Filename,
			Message:  fmt.Sprintf("path_param() references undeclared route parameter %q", name),
			Span:     argument.Span(),
		})
	})
	return issues
}

func officialPathParameterCall(call *ast.CallExpression, resolved resolver.Result) bool {
	switch callee := call.Callee.(type) {
	case *ast.Identifier:
		binding, ok := resolved.Symbols[callee.Name]
		return ok && callee.Name == "path_param" && officialWebBinding(binding)
	case *ast.MemberExpression:
		receiver, ok := callee.Receiver.(*ast.Identifier)
		if !ok || callee.Name != "path_param" {
			return false
		}
		binding, ok := resolved.Member(receiver.Name, callee.Name)
		return ok && officialWebBinding(binding)
	default:
		return false
	}
}

func officialWebBinding(binding resolver.Binding) bool {
	return binding.Import != nil && binding.Import.RuntimePath() == ModulePath
}

func walkStatements(statements []ast.Statement, visit func(*ast.CallExpression)) {
	for _, statement := range statements {
		walkStatement(statement, visit)
	}
}

func walkStatement(statement ast.Statement, visit func(*ast.CallExpression)) {
	switch node := statement.(type) {
	case *ast.ClassStatement:
		walkExpression(node.Superclass, visit)
		walkStatements(node.Body, visit)
	case *ast.RecordStatement:
		walkStatements(node.Body, visit)
	case *ast.RecordFieldStatement:
		for _, attribute := range node.Attributes {
			for _, argument := range attribute.Arguments {
				walkExpression(argument.Value, visit)
			}
		}
	case *ast.EnumStatement:
		walkStatements(node.Body, visit)
	case *ast.EnumMemberStatement:
		for _, parameter := range node.Parameters {
			walkExpression(parameter.Default, visit)
		}
	case *ast.ModuleStatement:
		walkStatements(node.Body, visit)
	case *ast.InterfaceStatement:
		for _, method := range node.Methods {
			walkStatement(method, visit)
		}
	case *ast.FieldStatement:
		walkExpression(node.Value, visit)
	case *ast.MethodStatement:
		for _, parameter := range node.Parameters {
			walkExpression(parameter.Default, visit)
		}
		walkStatements(node.Body, visit)
	case *ast.VariableStatement:
		walkExpression(node.Value, visit)
	case *ast.AssignmentStatement:
		walkExpression(node.Target, visit)
		walkExpression(node.Value, visit)
	case *ast.ReturnStatement:
		walkExpression(node.Value, visit)
	case *ast.ExpressionStatement:
		walkExpression(node.Expression, visit)
	case *ast.IfStatement:
		walkIf(node, visit)
	case *ast.CaseStatement:
		walkCase(node, visit)
	case *ast.WhileStatement:
		walkExpression(node.Condition, visit)
		walkStatements(node.Body, visit)
	case *ast.NativeBlock:
		walkStatements(node.Body, visit)
	}
}

func walkExpression(expression ast.Expression, visit func(*ast.CallExpression)) {
	if expression == nil {
		return
	}
	switch node := expression.(type) {
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			walkExpression(part.Expression, visit)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			walkExpression(element, visit)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			walkExpression(entry.Key, visit)
			walkExpression(entry.Value, visit)
		}
	case *ast.UnaryExpression:
		walkExpression(node.Operand, visit)
	case *ast.BinaryExpression:
		walkExpression(node.Left, visit)
		walkExpression(node.Right, visit)
	case *ast.RangeExpression:
		walkExpression(node.Start, visit)
		walkExpression(node.End, visit)
	case *ast.CallExpression:
		visit(node)
		walkExpression(node.Callee, visit)
		for _, argument := range node.Arguments {
			walkExpression(argument.Value, visit)
		}
		if node.Block != nil {
			walkStatements(node.Block.Body, visit)
		}
	case *ast.GenericExpression:
		walkExpression(node.Receiver, visit)
	case *ast.MemberExpression:
		walkExpression(node.Receiver, visit)
	case *ast.IndexExpression:
		walkExpression(node.Receiver, visit)
		walkExpression(node.Index, visit)
	case *ast.BlockExpression:
		walkStatements(node.Body, visit)
	case *ast.IterationExpression:
		walkExpression(node.Source, visit)
		walkExpression(node.SliceSize, visit)
		walkExpression(node.Initial, visit)
		if node.Block != nil {
			walkStatements(node.Block.Body, visit)
		}
	case *ast.IfStatement:
		walkIf(node, visit)
	case *ast.CaseStatement:
		walkCase(node, visit)
	}
}

func walkIf(node *ast.IfStatement, visit func(*ast.CallExpression)) {
	walkExpression(node.Condition, visit)
	walkStatements(node.Then, visit)
	for _, branch := range node.ElseIf {
		walkExpression(branch.Condition, visit)
		walkStatements(branch.Body, visit)
	}
	walkStatements(node.Else, visit)
}

func walkCase(node *ast.CaseStatement, visit func(*ast.CallExpression)) {
	walkExpression(node.Value, visit)
	walkStatements(node.Leading, visit)
	for _, branch := range node.Branches {
		walkExpression(branch.Value, visit)
		walkStatements(branch.Body, visit)
	}
	walkStatements(node.Else, visit)
}
