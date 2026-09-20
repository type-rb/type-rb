package golang

import (
	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) arrayQueryIntrinsic(name string, call *ir.Call, arguments []string) string {
	arrayType := arrayReceiverType(call)
	receiver, target, prefix := g.arrayArgumentEvaluation(call, arguments, g.goType(arrayType.Args[0]))
	values := receiver + "Values"
	// Retain the Array identity before the argument, then read its current
	// storage after the argument has had the opportunity to mutate it.
	prefix += values + " := " + g.arrayValues(receiver) + "; "
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
