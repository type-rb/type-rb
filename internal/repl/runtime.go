package repl

import (
	"errors"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// runtimeInvocation is the typed boundary between the IR evaluator and a host
// capability provider. Ordinary TypeRB package bodies do not use this path.
type runtimeInvocation struct {
	Name       string
	Arguments  []evaluatedArgument
	Type       types.Type
	Codec      *ir.CodecSchema
	Call       *ir.Call
	MemberName string
}

type runtimeProvider interface {
	Name() string
	Handles(intrinsic string) bool
	Configure(programs []*ir.Program) error
	Call(evaluator *Evaluator, invocation runtimeInvocation) (Value, error)
	Close() error
}

type runtimeProviderFactory func() runtimeProvider

var runtimeProviderFactories []runtimeProviderFactory

func registerRuntimeProvider(factory runtimeProviderFactory) {
	runtimeProviderFactories = append(runtimeProviderFactories, factory)
}

func newRuntimeProviders() []runtimeProvider {
	providers := make([]runtimeProvider, 0, len(runtimeProviderFactories))
	for _, factory := range runtimeProviderFactories {
		providers = append(providers, factory())
	}
	return providers
}

func (e *Evaluator) configureRuntimeProviders(programs []*ir.Program) error {
	for _, provider := range e.runtimeProviders {
		if err := provider.Configure(programs); err != nil {
			return err
		}
	}
	return nil
}

func (e *Evaluator) runtimeCall(invocation runtimeInvocation) (Value, bool, error) {
	for _, provider := range e.runtimeProviders {
		if provider.Handles(invocation.Name) {
			value, err := provider.Call(e, invocation)
			return value, true, err
		}
	}
	return Value{}, false, nil
}

func (e *Evaluator) runtimeProvider(name string) runtimeProvider {
	for _, provider := range e.runtimeProviders {
		if provider.Name() == name {
			return provider
		}
	}
	return nil
}

func (e *Evaluator) Close() error {
	var result error
	for index := len(e.runtimeProviders) - 1; index >= 0; index-- {
		result = errors.Join(result, e.runtimeProviders[index].Close())
	}
	return result
}
