package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
)

// Evaluate the receiver before its argument: Go does not order a plain variable
// read against calls in another argument. Retain the Array object, not its
// current backing slice, so argument-side mutation remains visible.
func (g *generator) arrayArgumentEvaluation(call *ir.Call, arguments []string, argumentType string) (receiver, argument, prefix string) {
	g.temporary++
	id := strconv.Itoa(g.temporary)
	receiver = "__trbArrayReceiver" + id
	argument = "__trbArrayArgument" + id
	prefix = "func() " + g.goType(call.ExprType()) + " { " + receiver + " := " + arguments[0] + "; var " + argument + " " + argumentType + " = " + arguments[1] + "; "
	return
}
