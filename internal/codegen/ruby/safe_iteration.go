package ruby

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// A statement guard preserves return/break/next ownership in each blocks.
func (g *generator) safeIteration(node *ir.Iterate) {
	present := *node
	present.Safe = false
	if !node.Source.ExprType().Nullable {
		g.statement(&present)
		return
	}
	g.temporary++
	name := "__trb_safe_collection_" + strconv.Itoa(g.temporary)
	g.line(name+" = "+g.expr(node.Source), "")
	typ := node.Source.ExprType()
	typ.Nullable = false
	present.Source = &ir.Identifier{ExprBase: ir.NewExprBase(node.Source.SourceSpan(), typ), Name: name, Lexical: true, Generated: true}
	g.line("if !"+name+".nil?", "")
	g.indent++
	g.statement(&present)
	g.indent--
	g.line("end", "")
}

func (g *generator) safeTransform(node *ir.Transform) string {
	present := *node
	present.Safe = false
	present.Type = node.PresentType
	if !node.Source.ExprType().Nullable {
		return g.transform(&present)
	}
	g.temporary++
	name := "__trb_safe_collection_" + strconv.Itoa(g.temporary)
	child := g.expressionChild()
	typ := node.Source.ExprType()
	typ.Nullable = false
	present.Source = &ir.Identifier{ExprBase: ir.NewExprBase(node.Source.SourceSpan(), typ), Name: name, Lexical: true, Generated: true}
	child.line("begin", "")
	child.indent++
	child.line(name+" = "+child.expr(node.Source), "")
	child.line("if "+name+".nil?", "")
	child.indent++
	child.line("nil", "")
	child.indent--
	child.line("else", "")
	child.indent++
	child.line(child.transform(&present), "")
	child.indent--
	child.line("end", "")
	child.indent--
	child.line("end", "")
	g.mergeExpressionChild(child)
	return strings.TrimSpace(child.b.String())
}
