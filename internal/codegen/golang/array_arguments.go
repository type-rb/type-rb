package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
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

func arrayReceiverType(call *ir.Call) types.Type {
	arrayType := call.Arguments[0].Value.ExprType()
	if member, ok := receiverMember(call.Callee); ok && member.Receiver.ExprType().Kind == types.Array {
		arrayType = member.Receiver.ExprType()
	}
	return arrayType
}
