package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
)

// rangeIterate consumes bounds directly, preserving structured loop transfers.
// Only each_slice allocates, and only for the current requested batch.
func (g *generator) rangeIterate(iteration *ir.Iterate) {
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	bounds := "__trbRange" + suffix
	current := "__trbCurrent" + suffix
	index := "__trbIndex" + suffix
	size := "__trbSize" + suffix
	chunk := "__trbChunk" + suffix
	condition := current + " < " + bounds + "[1] || (" + current + " == " + bounds + "[1] && " + bounds + "[2] == 0)"
	g.line("{")
	g.indent++
	g.line(bounds + " := " + g.expr(iteration.Source))
	post := current + ", " + index + " = " + current + "+1, " + index + "+1"
	if iteration.Operation == "each_slice" {
		g.line(size + " := " + g.expr(iteration.SliceSize))
		g.line("if " + size + " <= 0 { panic(\"each_slice size must be greater than zero\") }")
		post = index + "++"
	}
	g.line("for " + current + ", " + index + " := " + bounds + "[0], 0; " + condition + "; " + post + " {")
	g.indent++
	value := current
	if iteration.Operation == "each_slice" {
		g.line(chunk + " := []int{}")
		g.line("for len(" + chunk + ") < " + size + " && (" + condition + ") {")
		g.indent++
		g.line(chunk + " = append(" + chunk + ", " + current + ")")
		g.line(current + "++")
		g.indent--
		g.line("}")
		value = g.arrayReference(chunk)
	}
	if name := iteration.Bindings[0].Name; name != "_" {
		binding := g.bindingIdentifier(name)
		g.line(binding + " := " + value)
		g.line("_ = " + binding)
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
