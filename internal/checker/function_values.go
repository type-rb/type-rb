package checker

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

// A function value has the same required positional signature as an authored
// fn. Other declaration call forms need an explicit fn adapter.
func (c *Checker) namedFunctionValueType(span token.Span, name string, signature methodSignature, generic bool) types.Type {
	if generic || signature.variadic {
		c.error(span, fmt.Sprintf("function %s requires an explicit fn wrapper to be used as a value", name))
		return invalidType()
	}
	parameters := make([]types.Type, len(signature.parameters))
	for index, parameter := range signature.parameters {
		if parameter.Kind != callsignature.Positional || parameter.Presence != callsignature.Required {
			c.error(span, fmt.Sprintf("function %s requires an explicit fn wrapper to be used as a value", name))
			return invalidType()
		}
		parameters[index] = parameter.Type
	}
	return types.FunctionOf(parameters, signature.returnType)
}
