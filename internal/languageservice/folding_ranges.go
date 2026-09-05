package languageservice

import (
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/token"
)

// FoldingRange identifies one structural source region without exposing an
// editor protocol. The LSP adapter translates byte offsets to UTF-16 positions.
type FoldingRange struct {
	Range OffsetRange
}

// FoldingRanges returns structural regions from the lossless syntax tree. It
// remains available while a document has parse or type errors.
func FoldingRanges(source string) []FoldingRange {
	program, _ := parser.Parse([]byte(source))
	collector := foldingCollector{seen: map[OffsetRange]bool{}}
	collector.statements(program.Statements)
	sort.SliceStable(collector.ranges, func(left, right int) bool {
		leftRange := collector.ranges[left].Range
		rightRange := collector.ranges[right].Range
		if leftRange.Start != rightRange.Start {
			return leftRange.Start < rightRange.Start
		}
		return leftRange.End > rightRange.End
	})
	return collector.ranges
}

type foldingCollector struct {
	ranges []FoldingRange
	seen   map[OffsetRange]bool
}

func (c *foldingCollector) add(node ast.Node) {
	if node == nil {
		return
	}
	span := node.Span()
	if span.End.Line <= span.Start.Line {
		return
	}
	range_ := offsetRangeFromSpan(span)
	if c.seen[range_] {
		return
	}
	c.seen[range_] = true
	c.ranges = append(c.ranges, FoldingRange{Range: range_})
}

func offsetRangeFromSpan(span token.Span) OffsetRange {
	return OffsetRange{Start: span.Start.Offset, End: span.End.Offset}
}

func (c *foldingCollector) statements(statements []ast.Statement) {
	for _, statement := range statements {
		c.statement(statement)
	}
}

func (c *foldingCollector) statement(statement ast.Statement) {
	switch node := statement.(type) {
	case *ast.ClassStatement:
		c.add(node)
		c.expression(node.Superclass)
		c.statements(node.Body)
	case *ast.RecordStatement:
		c.add(node)
		c.statements(node.Body)
	case *ast.EnumStatement:
		c.add(node)
		c.statements(node.Body)
	case *ast.NewtypeStatement:
		if node.HasBody {
			c.add(node)
			c.statements(node.Body)
		}
	case *ast.ModuleStatement:
		c.add(node)
		c.statements(node.Body)
	case *ast.InterfaceStatement:
		c.add(node)
		for _, method := range node.Methods {
			c.statement(method)
		}
	case *ast.MethodStatement:
		c.add(node)
		for _, parameter := range node.Parameters {
			c.expression(parameter.Default)
		}
		c.statements(node.Body)
	case *ast.IfStatement:
		c.ifExpression(node)
	case *ast.CaseStatement:
		c.caseExpression(node)
	case *ast.WhileStatement:
		c.add(node)
		c.expression(node.Condition)
		c.statements(node.Body)
	case *ast.NativeBlock:
		c.add(node)
		c.statements(node.Body)
	case *ast.RecordFieldStatement:
		for _, attribute := range node.Attributes {
			for _, argument := range attribute.Arguments {
				c.expression(argument.Value)
			}
		}
	case *ast.EnumMemberStatement:
		c.expression(node.RawValue)
	case *ast.FieldStatement:
		c.expression(node.Value)
	case *ast.VariableStatement:
		c.expression(node.Value)
	case *ast.AssignmentStatement:
		c.expression(node.Target)
		c.expression(node.Value)
	case *ast.ReturnStatement:
		c.expression(node.Value)
	case *ast.ExpressionStatement:
		c.expression(node.Expression)
	}
}

func (c *foldingCollector) expression(expression ast.Expression) {
	switch node := expression.(type) {
	case nil, *ast.Identifier, *ast.Literal, *ast.SymbolLiteral, *ast.NativeExpression:
		return
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			c.expression(part.Expression)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			c.expression(element)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			c.expression(entry.Key)
			c.expression(entry.Value)
		}
	case *ast.JSXElement:
		c.add(node)
		c.expression(node.Component)
		for _, attribute := range node.Attributes {
			c.expression(attribute.Value)
		}
		for _, child := range node.Children {
			switch child := child.(type) {
			case *ast.JSXElement:
				c.expression(child)
			case *ast.JSXExpression:
				c.expression(child.Value)
			}
		}
	case *ast.UnaryExpression:
		c.expression(node.Operand)
	case *ast.BinaryExpression:
		c.expression(node.Left)
		c.expression(node.Right)
	case *ast.RangeExpression:
		c.expression(node.Start)
		c.expression(node.End)
	case *ast.IfStatement:
		c.ifExpression(node)
	case *ast.CaseStatement:
		c.caseExpression(node)
	case *ast.TryExpression:
		c.expression(node.Value)
	case *ast.CatchExpression:
		c.add(node)
		c.expression(node.Value)
		c.statements(node.Body)
	case *ast.LambdaExpression:
		c.add(node)
		for _, parameter := range node.Parameters {
			c.expression(parameter.Default)
		}
		c.statements(node.Body)
	case *ast.CallExpression:
		c.expression(node.Callee)
		for _, argument := range node.Arguments {
			c.expression(argument.Value)
		}
		if node.Block != nil {
			c.expression(node.Block)
		}
	case *ast.GenericExpression:
		c.expression(node.Receiver)
	case *ast.MemberExpression:
		c.expression(node.Receiver)
	case *ast.IndexExpression:
		c.expression(node.Receiver)
		c.expression(node.Index)
	case *ast.BlockExpression:
		c.add(node)
		c.statements(node.Body)
	case *ast.IterationExpression:
		c.expression(node.Source)
		c.expression(node.SliceSize)
		c.expression(node.Initial)
		c.expression(node.Limit)
		if node.Block != nil {
			c.expression(node.Block)
		}
	}
}

func (c *foldingCollector) ifExpression(node *ast.IfStatement) {
	c.add(node)
	c.expression(node.Condition)
	c.statements(node.Then)
	for _, branch := range node.ElseIf {
		c.expression(branch.Condition)
		c.statements(branch.Body)
	}
	c.statements(node.Else)
}

func (c *foldingCollector) caseExpression(node *ast.CaseStatement) {
	c.add(node)
	c.expression(node.Value)
	c.statements(node.Leading)
	for _, branch := range node.Branches {
		c.expression(branch.Value)
		for _, alternative := range branch.Alternatives {
			c.expression(alternative)
		}
		c.statements(branch.Body)
	}
	c.statements(node.Else)
}
