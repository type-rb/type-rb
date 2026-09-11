package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) arrayIterate(iteration *ir.Iterate) {
	if iteration.Operation == "each_slice" {
		converted := *iteration
		target := iteration.Source.ExprType()
		target.Kind, target.Name = types.Iterable, "Iterable"
		converted.Source = &ir.Conversion{
			ExprBase: ir.NewExprBase(iteration.Source.SourceSpan(), target),
			Kind:     ir.ToIterableConversion,
			Value:    iteration.Source,
		}
		g.iterableIterate(&converted)
		return
	}
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	items, index := "__trbArray"+suffix, "__trbIndex"+suffix
	g.line("{")
	g.indent++
	g.line(items + " := " + g.expr(iteration.Source))
	g.line("for " + index + " := 0; " + index + " < len(*" + items + "); " + index + "++ {")
	g.indent++
	if name := iteration.Bindings[0].Name; name != "_" {
		item := g.bindingIdentifier(name)
		g.line(item + " := (*" + items + ")[" + index + "]")
		g.line("_ = " + item)
	}
	if iteration.WithIndex && iteration.Bindings[1].Name != "_" {
		binding := g.bindingIdentifier(iteration.Bindings[1].Name)
		g.line(binding + " := " + index)
		g.line("_ = " + binding)
	}
	g.statements(iteration.Body)
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
}
