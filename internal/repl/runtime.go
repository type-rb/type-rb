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

// runtimeBlockInvocation lets a provider own resource acquisition and cleanup
// while the evaluator continues to execute ordinary typed IR inside the block.
type runtimeBlockInvocation struct {
	Name      string
	Arguments []evaluatedArgument
	Type      types.Type
	Block     *ir.StructuredBlock
	Evaluate  func(bindings []Value) (Value, error)
}

// runtimeIterationInvocation lets a host provider stream values while the
// evaluator retains TypeRB block and control-flow semantics.
type runtimeIterationInvocation struct {
	Name      string
	Source    Value
	BatchSize int64
	Type      types.Type
	Iteration *ir.Iterate
	Evaluate  func(Value) (flowResult, error)
}

type runtimeProvider interface {
	Name() string
	Handles(intrinsic string) bool
	Configure(programs []*ir.Program) error
	Call(evaluator *Evaluator, invocation runtimeInvocation) (Value, error)
	Close() error
}

type runtimeBlockProvider interface {
	Block(evaluator *Evaluator, invocation runtimeBlockInvocation) (Value, error)
}

type runtimeIterationProvider interface {
	Iterate(evaluator *Evaluator, invocation runtimeIterationInvocation) (Value, flowResult, error)
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

func (e *Evaluator) runtimeHandles(intrinsic string) bool {
	for _, provider := range e.runtimeProviders {
		if provider.Handles(intrinsic) {
			return true
		}
	}
	return false
}

func (e *Evaluator) runtimeBlock(invocation runtimeBlockInvocation) (Value, bool, error) {
	for _, provider := range e.runtimeProviders {
		if !provider.Handles(invocation.Name) {
			continue
		}
		blockProvider, ok := provider.(runtimeBlockProvider)
		if !ok {
			return Value{}, true, errors.New("runtime provider does not support structured blocks")
		}
		value, err := blockProvider.Block(e, invocation)
		return value, true, err
	}
	return Value{}, false, nil
}

func (e *Evaluator) runtimeIteration(invocation runtimeIterationInvocation) (Value, flowResult, bool, error) {
	for _, provider := range e.runtimeProviders {
		if !provider.Handles(invocation.Name) {
			continue
		}
		iterationProvider, ok := provider.(runtimeIterationProvider)
		if !ok {
			return Value{}, flowResult{}, true, errors.New("runtime provider does not support structured iterations")
		}
		value, flow, err := iterationProvider.Iterate(e, invocation)
		return value, flow, true, err
	}
	return Value{}, flowResult{}, false, nil
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
