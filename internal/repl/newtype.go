package repl

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/ir"
)

func (e *Evaluator) newtypeMethodCall(call *ir.Call, module string, sc *scope) (Value, error) {
	dispatch := call.NewtypeMethod.Dispatch
	definition, ok := e.definitions[symbolKey(dispatch.Owner.Module, dispatch.Owner.Name+"#"+dispatch.Name)].(*functionDefinition)
	if !ok {
		return Value{}, fmt.Errorf("newtype method %s.%s is unavailable", dispatch.Owner.Name, dispatch.Name)
	}
	receiver := Value{}
	if expression := call.NewtypeReceiver(); expression != nil {
		var err error
		receiver, err = e.expression(expression, module, sc)
		if err != nil {
			return Value{}, err
		}
		if _, safe := safeCallMember(call.Callee); safe && receiver.Data == nil {
			return Value{Type: call.ExprType()}, nil
		}
	}
	arguments := make([]evaluatedArgument, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		value, err := e.expression(argument.Value, module, sc)
		if err != nil {
			return Value{}, err
		}
		arguments = append(arguments, evaluatedArgument{Name: argument.Name, Value: value})
	}
	value, err := e.call(&callable{Function: definition, Receiver: receiver}, arguments)
	value.Type = call.ExprType()
	return value, err
}
