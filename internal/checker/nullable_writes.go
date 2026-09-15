package checker

import (
	"maps"

	"github.com/type-rb/type-rb/internal/ast"
)

// A repeated region starts with facts valid on every backedge. Forget facts
// for replaced bindings before checking the condition/body, then let fresh
// guards establish new ones. The same conservative facts hold after the loop.
func invalidateRepeatedNullableFacts(sc *scope, body []ast.Statement, parameters []string, condition ast.Expression) {
	hidden := map[string]bool{}
	for _, name := range parameters {
		hidden[name] = true
	}
	writes := map[string]bool{}
	collectNullableWrites(condition, hidden, writes)
	collectNullableStatementWrites(body, hidden, writes)
	invalidateNullableWrites(sc, writes)
}

// A conditional write cannot keep the parent's earlier value or field facts.
// Check each branch with its entry facts first, then forget replaced bindings
// at the join without exporting a branch-specific assigned type.
func invalidateConditionalNullableFacts(sc *scope, expression ast.Expression) {
	writes := map[string]bool{}
	collectNullableWrites(expression, map[string]bool{}, writes)
	invalidateNullableWrites(sc, writes)
}

func invalidateNullableWrites(sc *scope, writes map[string]bool) {
	for name := range writes {
		value, ok := sc.lookup(name)
		if !ok {
			continue
		}
		if value.declared.Nullable {
			sc.setAssignmentType(name, value.declared, value.declared)
		}
		sc.resetNullableMembers(name, value.span.Start.Offset)
	}
}

// This is a lexical write inventory, not expression checking. Walk evaluated
// subexpressions, including expression-valued branches, without entering a
// function declaration. Block/pattern/local bindings hide outer names.
func collectNullableStatementWrites(body []ast.Statement, inherited map[string]bool, writes map[string]bool) {
	hidden := maps.Clone(inherited)
	for _, statement := range body {
		switch node := statement.(type) {
		case *ast.VariableStatement:
			collectNullableWrites(node.Value, hidden, writes)
			hidden[node.Name] = true
		case *ast.AssignmentStatement:
			collectNullableWrites(node.Target, hidden, writes)
			collectNullableWrites(node.Value, hidden, writes)
			if target, ok := node.Target.(*ast.Identifier); ok && !hidden[target.Name] {
				writes[target.Name] = true
			}
		case *ast.ReturnStatement:
			collectNullableWrites(node.Value, hidden, writes)
		case *ast.ExpressionStatement:
			collectNullableWrites(node.Expression, hidden, writes)
		case *ast.WhileStatement:
			collectNullableWrites(node.Condition, hidden, writes)
			collectNullableStatementWrites(node.Body, hidden, writes)
		case *ast.NativeBlock:
			collectNullableStatementWrites(node.Body, hidden, writes)
		case ast.Expression:
			collectNullableWrites(node, hidden, writes)
		}
	}
}

func collectNullableBlockWrites(block *ast.BlockExpression, inherited map[string]bool, writes map[string]bool) {
	if block == nil {
		return
	}
	hidden := maps.Clone(inherited)
	for _, name := range block.Parameters {
		hidden[name] = true
	}
	collectNullableStatementWrites(block.Body, hidden, writes)
}

func collectNullableWrites(expression ast.Expression, hidden map[string]bool, writes map[string]bool) {
	switch node := expression.(type) {
	case *ast.IfStatement:
		collectNullableWrites(node.Condition, hidden, writes)
		collectNullableStatementWrites(node.Then, hidden, writes)
		for _, branch := range node.ElseIf {
			collectNullableWrites(branch.Condition, hidden, writes)
			collectNullableStatementWrites(branch.Body, hidden, writes)
		}
		collectNullableStatementWrites(node.Else, hidden, writes)
	case *ast.CaseStatement:
		collectNullableWrites(node.Value, hidden, writes)
		collectNullableStatementWrites(node.Leading, hidden, writes)
		for _, branch := range node.Branches {
			collectNullableWrites(branch.Value, hidden, writes)
			for _, alternative := range branch.Alternatives {
				collectNullableWrites(alternative, hidden, writes)
			}
			branchHidden := maps.Clone(hidden)
			for _, binding := range branch.Bindings {
				branchHidden[binding.Name] = true
			}
			collectNullableStatementWrites(branch.Body, branchHidden, writes)
		}
		collectNullableStatementWrites(node.Else, hidden, writes)
	case *ast.IterationExpression:
		collectNullableWrites(node.Source, hidden, writes)
		collectNullableWrites(node.Initial, hidden, writes)
		collectNullableWrites(node.SliceSize, hidden, writes)
		collectNullableWrites(node.Limit, hidden, writes)
		collectNullableBlockWrites(node.Block, hidden, writes)
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			collectNullableWrites(part.Expression, hidden, writes)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			collectNullableWrites(element, hidden, writes)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			collectNullableWrites(entry.Key, hidden, writes)
			collectNullableWrites(entry.Value, hidden, writes)
		}
	case *ast.UnaryExpression:
		collectNullableWrites(node.Operand, hidden, writes)
	case *ast.BinaryExpression:
		collectNullableWrites(node.Left, hidden, writes)
		collectNullableWrites(node.Right, hidden, writes)
	case *ast.RangeExpression:
		collectNullableWrites(node.Start, hidden, writes)
		collectNullableWrites(node.End, hidden, writes)
	case *ast.CallExpression:
		collectNullableWrites(node.Callee, hidden, writes)
		for _, argument := range node.Arguments {
			collectNullableWrites(argument.Value, hidden, writes)
		}
		collectNullableBlockWrites(node.Block, hidden, writes)
	case *ast.GenericExpression:
		collectNullableWrites(node.Receiver, hidden, writes)
	case *ast.MemberExpression:
		collectNullableWrites(node.Receiver, hidden, writes)
	case *ast.IndexExpression:
		collectNullableWrites(node.Receiver, hidden, writes)
		collectNullableWrites(node.Index, hidden, writes)
	case *ast.TryExpression:
		collectNullableWrites(node.Value, hidden, writes)
	case *ast.CatchExpression:
		collectNullableWrites(node.Value, hidden, writes)
		branchHidden := maps.Clone(hidden)
		branchHidden[node.Binding.Name] = true
		collectNullableStatementWrites(node.Body, branchHidden, writes)
	case *ast.AttemptExpression:
		collectNullableWrites(node.Value, hidden, writes)
		collectNullableStatementWrites(node.Body, hidden, writes)
	case *ast.JSXElement:
		collectNullableWrites(node.Component, hidden, writes)
		for _, attribute := range node.Attributes {
			collectNullableWrites(attribute.Value, hidden, writes)
		}
		for _, child := range node.Children {
			switch child := child.(type) {
			case *ast.JSXElement:
				collectNullableWrites(child, hidden, writes)
			case *ast.JSXExpression:
				collectNullableWrites(child.Value, hidden, writes)
			}
		}
	}
}
