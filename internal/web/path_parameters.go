package web

import (
	"fmt"
	"strconv"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func validatePathParameterCalls(route Route, program *ast.Program, resolved resolver.Result) []Issue {
	if program == nil {
		return nil
	}
	contextBinding, imported := resolved.Symbols["Context"]
	if !imported || !officialWebBinding(contextBinding) {
		return nil
	}
	declared := make(map[string]bool, len(route.PathParameters))
	for _, name := range route.PathParameters {
		declared[name] = true
	}
	var issues []Issue
	for _, statement := range program.Statements {
		method, ok := statement.(*ast.MethodStatement)
		if !ok {
			continue
		}
		contextNames := map[string]bool{}
		for _, parameter := range method.Parameters {
			if parameter.Type.Name == "Context" {
				contextNames[parameter.Name] = true
			}
		}
		walkStatements(method.Body, func(call *ast.CallExpression) {
			if recordName, label, span, ok := officialTypedPathParameterCall(call, contextNames, route.ModulePath, resolved); ok {
				fields, found := pathParameterRecordFields(recordName, route.ModulePath, resolved)
				if !found {
					return
				}
				for _, field := range fields {
					if !declared[field.Name] {
						issues = append(issues, Issue{
							Filename: route.Filename,
							Message:  fmt.Sprintf("%s field %q is not declared by route %s", label, field.Name, route.Path),
							Span:     span,
						})
					}
				}
				fieldNames := map[string]bool{}
				for _, field := range fields {
					fieldNames[field.Name] = true
				}
				for _, name := range route.PathParameters {
					if !fieldNames[name] {
						issues = append(issues, Issue{
							Filename: route.Filename,
							Message:  fmt.Sprintf("%s is missing route parameter %q", label, name),
							Span:     span,
						})
					}
				}
				return
			}
			if !officialPathParameterCall(call, contextNames) || len(call.Arguments) != 1 {
				return
			}
			argument := call.Arguments[0].Value
			literal, ok := argument.(*ast.Literal)
			if !ok || literal.Kind != ast.StringLiteral {
				issues = append(issues, Issue{
					Filename: route.Filename,
					Message:  "Context#path_value() name must be a string literal in a route file",
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
				Message:  fmt.Sprintf("Context#path_value() references undeclared route parameter %q", name),
				Span:     argument.Span(),
			})
		})
	}
	return issues
}

func officialTypedPathParameterCall(call *ast.CallExpression, contextNames map[string]bool, modulePath string, resolved resolver.Result) (string, string, token.Span, bool) {
	generic, ok := call.Callee.(*ast.GenericExpression)
	if !ok || len(generic.Arguments) != 1 {
		return "", "", token.Span{}, false
	}
	member, ok := generic.Receiver.(*ast.MemberExpression)
	if !ok || (member.Name != "params" && member.Name != "bind") {
		return "", "", token.Span{}, false
	}
	receiver, ok := member.Receiver.(*ast.Identifier)
	if !ok || !contextNames[receiver.Name] {
		return "", "", token.Span{}, false
	}
	argument := generic.Arguments[0]
	if member.Name == "params" {
		return argument.Name, fmt.Sprintf("Context#params<%s>()", argument.Name), argument.Span(), true
	}
	fields, found := pathParameterRecordFields(argument.Name, modulePath, resolved)
	if !found {
		return "", "", token.Span{}, false
	}
	for _, field := range fields {
		if field.Name == "params" && field.Type.Kind == types.Named && !field.Type.Nullable && len(field.Type.Args) == 0 {
			return field.Type.Name, fmt.Sprintf("Context#bind<%s>() params", argument.Name), argument.Span(), true
		}
	}
	return "", "", token.Span{}, false
}

func pathParameterRecordFields(name, modulePath string, resolved resolver.Result) ([]resolver.RecordField, bool) {
	if binding := resolved.Symbols[name]; binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
		return binding.Export.Fields, true
	}
	if resolved.Catalog == nil {
		return nil, false
	}
	module := resolved.Catalog.Modules[modulePath]
	if module == nil {
		return nil, false
	}
	exported, ok := module.Exports[name]
	if !ok || exported.Kind != resolver.RecordExport {
		return nil, false
	}
	return exported.Fields, true
}

func officialPathParameterCall(call *ast.CallExpression, contextNames map[string]bool) bool {
	callee, ok := call.Callee.(*ast.MemberExpression)
	if !ok || callee.Name != "path_value" {
		return false
	}
	receiver, ok := callee.Receiver.(*ast.Identifier)
	return ok && contextNames[receiver.Name]
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
		walkExpression(node.Default, visit)
		for _, attribute := range node.Attributes {
			for _, argument := range attribute.Arguments {
				walkExpression(argument.Value, visit)
			}
		}
	case *ast.EnumStatement:
		walkStatements(node.Body, visit)
	case *ast.EnumMemberStatement:
		walkExpression(node.RawValue, visit)
		for _, parameter := range node.Parameters {
			walkExpression(parameter.Default, visit)
		}
		for _, attribute := range node.Attributes {
			for _, argument := range attribute.Arguments {
				walkExpression(argument.Value, visit)
			}
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
	case *ast.JSXElement:
		walkExpression(node.Component, visit)
		for _, attribute := range node.Attributes {
			walkExpression(attribute.Value, visit)
		}
		for _, child := range node.Children {
			switch child := child.(type) {
			case *ast.JSXElement:
				walkExpression(child, visit)
			case *ast.JSXExpression:
				walkExpression(child.Value, visit)
			}
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
	case *ast.AttemptExpression:
		walkExpression(node.Value, visit)
		walkStatements(node.Body, visit)
	case *ast.TryExpression:
		walkExpression(node.Value, visit)
	case *ast.CatchExpression:
		walkExpression(node.Value, visit)
		walkStatements(node.Body, visit)
	case *ast.LambdaExpression:
		for _, parameter := range node.Parameters {
			walkExpression(parameter.Default, visit)
		}
		walkStatements(node.Body, visit)
	case *ast.IterationExpression:
		walkExpression(node.Source, visit)
		walkExpression(node.SliceSize, visit)
		walkExpression(node.Initial, visit)
		walkExpression(node.Limit, visit)
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
		for _, alternative := range branch.Alternatives {
			walkExpression(alternative, visit)
		}
		walkStatements(branch.Body, visit)
	}
	walkStatements(node.Else, visit)
}
