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
	inventory := newNullableWriteInventory()
	inventory.expression(condition, hidden)
	inventory.statements(body, hidden)
	// A callback created later in the body may already exist on the next
	// iteration. Calls after a fresh inner guard must see that possible writer.
	markNullableCaptures(sc, inventory.captures)
	invalidateNullableWrites(sc, inventory.effects(sc))
}

// A conditional write cannot keep the parent's earlier value or field facts.
// Check each branch with its entry facts first, then forget replaced bindings
// at the join without exporting a branch-specific assigned type.
func invalidateConditionalNullableFacts(sc *scope, expression ast.Expression) {
	inventory := newNullableWriteInventory()
	inventory.expression(expression, map[string]bool{})
	invalidateNullableWrites(sc, inventory.effects(sc))
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
// function declaration for immediate effects. Deferred lambda writes are tracked
// separately. Block/pattern/local bindings hide outer names.
func (w *nullableWriteInventory) statements(body []ast.Statement, inherited map[string]bool) {
	hidden := maps.Clone(inherited)
	for _, statement := range body {
		switch node := statement.(type) {
		case *ast.VariableStatement:
			w.expression(node.Value, hidden)
			hidden[node.Name] = true
		case *ast.AssignmentStatement:
			w.expression(node.Target, hidden)
			w.expression(node.Value, hidden)
			if target, ok := node.Target.(*ast.Identifier); ok && !hidden[target.Name] {
				w.writes[target.Name] = true
			}
		case *ast.ReturnStatement:
			w.expression(node.Value, hidden)
		case *ast.ExpressionStatement:
			w.expression(node.Expression, hidden)
		case *ast.WhileStatement:
			w.expression(node.Condition, hidden)
			w.statements(node.Body, hidden)
		case *ast.NativeBlock:
			w.statements(node.Body, hidden)
		case ast.Expression:
			w.expression(node, hidden)
		}
	}
}

func (w *nullableWriteInventory) block(block *ast.BlockExpression, inherited map[string]bool) {
	if block == nil {
		return
	}
	hidden := maps.Clone(inherited)
	for _, name := range block.Parameters {
		hidden[name] = true
	}
	w.statements(block.Body, hidden)
}

func (w *nullableWriteInventory) expression(expression ast.Expression, hidden map[string]bool) {
	switch node := expression.(type) {
	case *ast.LambdaExpression:
		// Constructing a closure does not execute its writes. Keep them separate
		// so repeated regions can account for callbacks created on a backedge.
		captures := lambdaNullableWrites(node)
		for name := range captures {
			if !hidden[name] {
				w.captures[name] = true
			}
		}
	case *ast.IfStatement:
		w.expression(node.Condition, hidden)
		w.statements(node.Then, hidden)
		for _, branch := range node.ElseIf {
			w.expression(branch.Condition, hidden)
			w.statements(branch.Body, hidden)
		}
		w.statements(node.Else, hidden)
	case *ast.CaseStatement:
		w.expression(node.Value, hidden)
		w.statements(node.Leading, hidden)
		for _, branch := range node.Branches {
			w.expression(branch.Value, hidden)
			for _, alternative := range branch.Alternatives {
				w.expression(alternative, hidden)
			}
			branchHidden := maps.Clone(hidden)
			for _, binding := range branch.Bindings {
				branchHidden[binding.Name] = true
			}
			w.statements(branch.Body, branchHidden)
		}
		w.statements(node.Else, hidden)
	case *ast.IterationExpression:
		w.expression(node.Source, hidden)
		w.expression(node.Initial, hidden)
		w.expression(node.SliceSize, hidden)
		w.expression(node.Limit, hidden)
		w.block(node.Block, hidden)
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			w.expression(part.Expression, hidden)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			w.expression(element, hidden)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			w.expression(entry.Key, hidden)
			w.expression(entry.Value, hidden)
		}
	case *ast.UnaryExpression:
		w.expression(node.Operand, hidden)
	case *ast.BinaryExpression:
		w.expression(node.Left, hidden)
		w.expression(node.Right, hidden)
	case *ast.RangeExpression:
		w.expression(node.Start, hidden)
		w.expression(node.End, hidden)
	case *ast.CallExpression:
		w.calls = true
		w.expression(node.Callee, hidden)
		for _, argument := range node.Arguments {
			w.expression(argument.Value, hidden)
		}
		w.block(node.Block, hidden)
	case *ast.GenericExpression:
		w.expression(node.Receiver, hidden)
	case *ast.MemberExpression:
		w.expression(node.Receiver, hidden)
	case *ast.IndexExpression:
		w.expression(node.Receiver, hidden)
		w.expression(node.Index, hidden)
	case *ast.TryExpression:
		w.expression(node.Value, hidden)
	case *ast.CatchExpression:
		w.expression(node.Value, hidden)
		branchHidden := maps.Clone(hidden)
		branchHidden[node.Binding.Name] = true
		w.statements(node.Body, branchHidden)
	case *ast.AttemptExpression:
		w.expression(node.Value, hidden)
		w.statements(node.Body, hidden)
	case *ast.JSXElement:
		w.expression(node.Component, hidden)
		for _, attribute := range node.Attributes {
			w.expression(attribute.Value, hidden)
		}
		for _, child := range node.Children {
			switch child := child.(type) {
			case *ast.JSXElement:
				w.expression(child, hidden)
			case *ast.JSXExpression:
				w.expression(child.Value, hidden)
			}
		}
	}
}
