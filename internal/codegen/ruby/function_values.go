package ruby

import "strconv"

func (g *generator) namedFunctionValue(target, module, name string) string {
	value := "method(" + strconv.Quote(target) + ")"
	if g.execution == nil || !g.execution.Method(module, "", name) {
		return value
	}
	return "->(*__trb_function_args) { " + value + ".call(" + g.executionScopeArgument() + ", *__trb_function_args) }"
}
