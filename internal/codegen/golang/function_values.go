package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) namedFunctionValue(identifier *ir.Identifier, target, module, name string) string {
	parameters, returned, callable := types.FunctionSignature(identifier.ExprType())
	if !callable || g.execution == nil || !g.execution.Method(module, "", name) {
		return target
	}
	declarations := make([]string, len(parameters))
	arguments := []string{g.executionScopeArgument()}
	for index, parameter := range parameters {
		binding := "__trb_function_arg_" + strconv.Itoa(index)
		declarations[index] = binding + " " + g.goType(parameter)
		arguments = append(arguments, binding)
	}
	body := target + "(" + strings.Join(arguments, ", ") + ")"
	if returned.Kind != types.Void {
		body = "return " + body
	}
	return "func(" + strings.Join(declarations, ", ") + ")" + g.goReturn(returned) + " { " + body + " }"
}
