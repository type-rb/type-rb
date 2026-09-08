package lower

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// arrayAssignment fixes the logical target before any RHS control flow. All
// execution paths consume these same typed statements, including the REPL.
func (l *lowerer) arrayAssignment(node *ast.AssignmentStatement, index *ast.IndexExpression) []ir.Statement {
	span := node.Span()
	capture := func(value ir.Expression) (*ir.Variable, *ir.Identifier) {
		l.temporary++
		name := "__trbAssignment" + strconv.Itoa(l.temporary)
		id := &ir.Identifier{ExprBase: ir.NewExprBase(span, value.ExprType()), Name: name, Lexical: true, Generated: true}
		return &ir.Variable{Base: ir.Base{Span: span}, Name: name, Type: value.ExprType(), Value: value, Mutable: true, Generated: true}, id
	}
	arrayDecl, array := capture(l.expression(index.Receiver))
	indexDecl, requested := capture(l.expression(index.Index))
	positionDecl, position := capture(&ir.Index{ExprBase: ir.NewExprBase(span, types.FromName("Integer")), Receiver: array, Index: requested, PositionOnly: true})
	prefix := []ir.Statement{arrayDecl, indexDecl, positionDecl}
	target := &ir.Index{ExprBase: ir.NewExprBase(index.Span(), l.checked.Expressions[index]), Receiver: array, Index: position}
	var old *ir.Identifier
	if node.Operator != "=" {
		declaration, identifier := capture(target)
		prefix = append(prefix, declaration)
		old = identifier
	}
	// Structured blocks retain their existing enclosing control-flow owner, but
	// deliver their value to a temporary rather than evaluating the target late.
	rhsType := l.checked.Expressions[node.Value]
	if node.Operator == "=" {
		rhsType = target.ExprType()
	}
	l.temporary++
	rhs := &ir.Identifier{ExprBase: ir.NewExprBase(span, rhsType), Name: "__trbAssignment" + strconv.Itoa(l.temporary), Lexical: true, Generated: true}
	var body []ir.Statement
	if block, ok := l.structuredBlock(node.Value); ok {
		body = append(body, &ir.Temporary{Base: ir.Base{Span: span}, Name: rhs.Name, Type: rhsType})
		block.Result = &ir.StructuredBlockResult{Target: rhs, Type: rhsType}
		body = append(body, block)
	} else if iteration, ok := l.structuredIteration(node.Value); ok {
		body = append(body, &ir.Temporary{Base: ir.Base{Span: span}, Name: rhs.Name, Type: rhsType})
		iteration.Result = &ir.IterationResult{Target: rhs, Type: rhsType}
		body = append(body, iteration)
	} else {
		valuePrefix, value, ok := l.structuredResultValue(node.Value)
		if !ok {
			value = l.expression(node.Value)
		}
		body = append(body, valuePrefix...)
		body = append(body, &ir.Variable{Base: ir.Base{Span: span}, Name: rhs.Name, Type: rhsType, Value: value, Generated: true})
	}
	var result ir.Expression = rhs
	lazy := node.Operator == "&&=" || node.Operator == "||="
	if node.Operator != "=" && !lazy {
		declaration, value := capture(&ir.Binary{ExprBase: target.ExprBase, Operator: strings.TrimSuffix(node.Operator, "="), Left: old, Right: rhs})
		body = append(body, declaration)
		result = value
	}
	store := &ir.Assignment{Base: ir.Base{Span: span, TrailingComment: node.TrailingComment}, Target: target, Operator: "=", Value: result}
	body = append(body, store)
	if !lazy {
		return append(prefix, body...)
	}
	// Preserve the assignment's displayed value even when the RHS is skipped.
	body = append(body, &ir.Assignment{Base: ir.Base{Span: span}, Target: old, Operator: "=", Value: rhs})
	var condition ir.Expression = old
	if node.Operator == "||=" {
		condition = &ir.Unary{ExprBase: ir.NewExprBase(span, types.FromName("Boolean")), Operator: "!", Operand: old}
	}
	prefix = append(prefix, &ir.If{ExprBase: ir.NewExprBase(span, types.FromName("Void")), Condition: condition, Then: body})
	return append(prefix, &ir.ExpressionStatement{Base: ir.Base{Span: span}, Expression: old})
}
