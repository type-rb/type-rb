package typescript

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) rangeIterate(iteration *ir.Iterate) {
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	bounds := "__trbRange" + suffix
	current := "__trbCurrent" + suffix
	index := "__trbIndex" + suffix
	size := "__trbSize" + suffix
	chunk := "__trbChunk" + suffix
	condition := current + " < " + bounds + "[1] || (" + current + " === " + bounds + "[1] && !" + bounds + "[2])"
	g.line("{")
	g.indent++
	g.line("const " + bounds + " = " + g.expr(iteration.Source) + ";")
	post := current + "++, " + index + "++"
	if iteration.Operation == "each_slice" {
		g.line("const " + size + " = " + g.expr(iteration.SliceSize) + ";")
		g.line("if (" + size + " <= 0) throw new Error(\"each_slice size must be greater than zero\");")
		post = index + "++"
	}
	g.line("for (let " + current + " = " + bounds + "[0], " + index + " = 0; " + condition + "; " + post + ") {")
	g.indent++
	value := current
	if iteration.Operation == "each_slice" {
		g.line("const " + chunk + ": Array<number> = [];")
		g.line("while (" + chunk + ".length < " + size + " && (" + condition + ")) {")
		g.indent++
		g.line(chunk + ".push(" + current + ");")
		g.line(current + "++;")
		g.indent--
		g.line("}")
		value = chunk
	}
	if name := iteration.Bindings[0].Name; name != "_" {
		g.line("let " + name + " = " + value + ";")
		g.line("void " + name + ";")
	}
	if iteration.WithIndex && iteration.Bindings[1].Name != "_" {
		name := iteration.Bindings[1].Name
		g.line("let " + name + " = " + index + ";")
		g.line("void " + name + ";")
	}
	g.statements(iteration.Body)
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
}
