package checker

import (
	"fmt"
	"maps"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/token"
)

// A nil state has no continuing path. Other states contain only fields assigned
// on every incoming path; a deferred body never establishes constructor facts.
type initializedFields map[string]bool

type classInitialization struct {
	checker  *Checker
	fields   []*ast.FieldStatement
	declared map[string]bool
	deferred bool
}

func (c *Checker) checkFieldInitialization(class *ast.ClassStatement) {
	if c.inferenceOnly {
		return
	}
	f := classInitialization{checker: c, declared: map[string]bool{}}
	var initialize *ast.MethodStatement
	for _, statement := range class.Body {
		switch node := statement.(type) {
		case *ast.FieldStatement:
			f.fields = append(f.fields, node)
			f.declared[node.Name] = true
		case *ast.MethodStatement:
			if node.Name == "initialize" && !node.Class {
				initialize = node
			}
		}
	}
	if len(f.fields) == 0 {
		return
	}
	state := initializedFields{}
	for _, field := range f.fields {
		if field.Value != nil {
			state = f.expression(field.Value, state)
			if state != nil {
				state[field.Name] = true
			}
		}
	}
	if initialize != nil {
		state = f.statements(initialize.Body, state)
	}
	f.complete(state, token.Span{})
}

func (f *classInitialization) complete(state initializedFields, span token.Span) {
	if state == nil || f.deferred {
		return
	}
	for _, field := range f.fields {
		if !state[field.Name] {
			origin := span
			if origin == (token.Span{}) {
				origin = field.Span()
			}
			f.checker.error(origin, fmt.Sprintf("field %s must be initialized before initialize() completes", field.Name))
		}
	}
}

func (f *classInitialization) receiver(state initializedFields, span token.Span) {
	for _, field := range f.fields {
		if !state[field.Name] {
			f.checker.error(span, "self cannot be used before all instance fields are initialized")
			return
		}
	}
}

func (f *classInitialization) read(name string, state initializedFields, span token.Span) {
	if f.deferred {
		// Even a field projection captures the receiver, not its current value.
		f.receiver(state, span)
	} else if f.declared[name] && !state[name] {
		f.checker.error(span, fmt.Sprintf("field %s is read before initialization", name))
	}
}

func (f *classInitialization) field(expression ast.Expression) string {
	switch node := expression.(type) {
	case *ast.Identifier:
		if f.declared[node.Name] {
			return node.Name
		}
	case *ast.MemberExpression:
		if receiver, ok := node.Receiver.(*ast.Identifier); ok && receiver.Name == "self" && f.checker.result.ClassFieldAccesses[node] {
			name := "@" + node.Name
			if f.declared[name] {
				return name
			}
		}
	}
	return ""
}

func mergeInitializedFields(left, right initializedFields) initializedFields {
	if left == nil {
		return maps.Clone(right)
	}
	if right == nil {
		return maps.Clone(left)
	}
	result := initializedFields{}
	for name := range left {
		if right[name] {
			result[name] = true
		}
	}
	return result
}

func (f *classInitialization) statements(body []ast.Statement, state initializedFields) initializedFields {
	for _, statement := range body {
		if state == nil {
			break
		}
		switch node := statement.(type) {
		case *ast.VariableStatement:
			state = f.expression(node.Value, state)
		case *ast.AssignmentStatement:
			field := f.field(node.Target)
			if field == "" || node.Operator != "=" {
				state = f.expression(node.Target, state)
			} else if f.deferred {
				f.receiver(state, node.Target.Span())
			}
			if node.Operator == "&&=" || node.Operator == "||=" {
				state = mergeInitializedFields(state, f.expression(node.Value, maps.Clone(state)))
			} else {
				state = f.expression(node.Value, state)
			}
			if field != "" && state != nil && !f.deferred {
				state[field] = true
			}
		case *ast.ReturnStatement:
			state = f.expression(node.Value, state)
			f.complete(state, node.Span())
			state = nil
		case *ast.BreakStatement, *ast.NextStatement:
			state = nil
		case *ast.ExpressionStatement:
			state = f.expression(node.Expression, state)
		case *ast.WhileStatement:
			state = f.expression(node.Condition, state)
			// The body may run zero times; inspect its reads and exits, but do
			// not use its assignments to establish facts after the loop.
			f.statements(node.Body, maps.Clone(state))
		case *ast.NativeBlock:
			state = f.statements(node.Body, state)
		case ast.Expression:
			state = f.expression(node, state)
		}
	}
	return state
}

func (f *classInitialization) deferredBody(body []ast.Statement, state initializedFields) {
	deferred := *f
	deferred.deferred = true
	deferred.statements(body, maps.Clone(state))
}

func (f *classInitialization) expression(expression ast.Expression, state initializedFields) initializedFields {
	if expression == nil || state == nil {
		return state
	}
	switch node := expression.(type) {
	case *ast.Identifier:
		if strings.HasPrefix(node.Name, "@") {
			f.read(node.Name, state, node.Span())
		} else if dispatch := f.checker.result.ExpressionDispatches[node]; node.Name == "self" || !dispatch.Class && dispatch.Owner.Kind == identity.Class {
			f.receiver(state, node.Span())
		}
	case *ast.MemberExpression:
		if field := f.field(node); field != "" {
			f.read(field, state, node.Span())
		} else {
			state = f.expression(node.Receiver, state)
		}
	case *ast.IfStatement:
		remaining := f.expression(node.Condition, state)
		joined := f.statements(node.Then, maps.Clone(remaining))
		for _, branch := range node.ElseIf {
			remaining = f.expression(branch.Condition, remaining)
			joined = mergeInitializedFields(joined, f.statements(branch.Body, maps.Clone(remaining)))
		}
		state = mergeInitializedFields(joined, f.statements(node.Else, remaining))
	case *ast.CaseStatement:
		remaining := f.expression(node.Value, state)
		remaining = f.statements(node.Leading, remaining)
		var joined initializedFields
		for _, branch := range node.Branches {
			remaining = f.expression(branch.Value, remaining)
			for _, alternative := range branch.Alternatives {
				remaining = f.expression(alternative, remaining)
			}
			joined = mergeInitializedFields(joined, f.statements(branch.Body, maps.Clone(remaining)))
		}
		if node.HasElse || !f.checker.caseCoversSelector(node) {
			joined = mergeInitializedFields(joined, f.statements(node.Else, remaining))
		}
		state = joined
	case *ast.LambdaExpression:
		deferred := *f
		deferred.deferred = true
		for _, parameter := range node.Parameters {
			deferred.expression(parameter.Default, maps.Clone(state))
		}
		deferred.statements(node.Body, maps.Clone(state))
	case *ast.IterationExpression:
		state = f.expression(node.Source, state)
		skipped := maps.Clone(state)
		state = f.expression(node.SliceSize, state)
		state = f.expression(node.Initial, state)
		state = f.expression(node.Limit, state)
		if node.Block != nil {
			if node.Operation == "concurrent_map" {
				f.deferredBody(node.Block.Body, state)
			} else {
				f.statements(node.Block.Body, maps.Clone(state))
			}
		}
		if node.Safe {
			state = mergeInitializedFields(skipped, state)
		}
	case *ast.CallExpression:
		state = f.expression(node.Callee, state)
		skipped := maps.Clone(state)
		for _, argument := range node.Arguments {
			state = f.expression(argument.Value, state)
		}
		if node.Block != nil {
			if boundary, known := f.checker.constructorBlockBoundaries[node]; known && !boundary {
				f.statements(node.Block.Body, maps.Clone(state))
			} else {
				f.deferredBody(node.Block.Body, state)
			}
		}
		if _, safe := f.checker.result.SafeNavigationCallTypes[node]; safe {
			state = mergeInitializedFields(skipped, state)
		}
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			state = f.expression(part.Expression, state)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			state = f.expression(element, state)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			state = f.expression(entry.Key, state)
			state = f.expression(entry.Value, state)
		}
	case *ast.UnaryExpression:
		state = f.expression(node.Operand, state)
	case *ast.BinaryExpression:
		state = f.expression(node.Left, state)
		if node.Operator == "&&" || node.Operator == "||" {
			state = mergeInitializedFields(state, f.expression(node.Right, maps.Clone(state)))
		} else {
			state = f.expression(node.Right, state)
		}
	case *ast.RangeExpression:
		state = f.expression(node.Start, state)
		state = f.expression(node.End, state)
	case *ast.GenericExpression:
		state = f.expression(node.Receiver, state)
	case *ast.IndexExpression:
		state = f.expression(node.Receiver, state)
		state = f.expression(node.Index, state)
	case *ast.TryExpression:
		state = f.expression(node.Value, state)
	case *ast.CatchExpression:
		state = f.expression(node.Value, state)
		state = mergeInitializedFields(state, f.statements(node.Body, maps.Clone(state)))
	case *ast.JSXElement:
		state = f.expression(node.Component, state)
		for _, attribute := range node.Attributes {
			state = f.expression(attribute.Value, state)
		}
		for _, child := range node.Children {
			switch child := child.(type) {
			case *ast.JSXElement:
				state = f.expression(child, state)
			case *ast.JSXExpression:
				state = f.expression(child.Value, state)
			}
		}
	}
	if !f.checker.expressionFallsThrough(expression) {
		return nil
	}
	return state
}
