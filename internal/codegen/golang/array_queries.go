package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) arrayQueryIntrinsic(name string, call *ir.Call, arguments []string) string {
	arrayType := call.Arguments[0].Value.ExprType()
	if member, ok := receiverMember(call.Callee); ok && member.Receiver.ExprType().Kind == types.Array {
		arrayType = member.Receiver.ExprType()
	}
	g.temporary++
	id := strconv.Itoa(g.temporary)
	receiver := "__trbArrayReceiver" + id
	target := "__trbArrayTarget" + id
	values := "__trbArrayValues" + id
	// Retain the Array identity before the argument, then read its current
	// storage after the argument has had the opportunity to mutate it.
	prefix := "func() " + g.goType(call.ExprType()) + " { " + receiver + " := " + arguments[0] + "; var " + target + " " + g.goType(arrayType.Args[0]) + " = " + arguments[1] + "; " + values + " := " + g.arrayValues(receiver) + "; "
	switch name {
	case "trb.std.arrays.contains":
		g.requireImport("slices", "")
		return prefix + "return slices.Contains(" + values + ", " + target + ") }()"
	case "trb.std.arrays.index":
		return prefix + "for index, value := range " + values + " { if value == " + target + " { result := index; return &result } }; return nil }()"
	default:
		return prefix + "count := 0; for _, value := range " + values + " { if value == " + target + " { count++ } }; return count }()"
	}
}
