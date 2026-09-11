package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) rangeExpr(span *ir.Range) string {
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	start := "__trbRangeStart" + suffix
	end := "__trbRangeEnd" + suffix
	exclusive := "0"
	if span.Exclusive {
		exclusive = "1"
	}
	// Go does not order a variable read before a later call in a composite
	// literal. Retain each endpoint before evaluating the next expression.
	return "func() [3]int { var " + start + " int = " + g.expr(span.Start) +
		"; var " + end + " int = " + g.expr(span.End) +
		"; return [3]int{" + start + ", " + end + ", " + exclusive + "} }()"
}
