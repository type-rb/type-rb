//go:build js && wasm

package repl

import (
	"errors"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// Browser playgrounds can type-check trb/orm projects, but database drivers
// are intentionally unavailable in the WebAssembly evaluator.
type ormRuntime struct{}

type ormQueryValue struct {
	model struct{ QueryType string }
}

func (e *Evaluator) loadORMRuntime(_ []*ir.Program) error { return nil }

func (e *Evaluator) Close() error { return nil }

func (e *Evaluator) ormIntrinsic(_ string, _ []evaluatedArgument, _ types.Type, _ *ir.Call) (Value, error) {
	return Value{}, errors.New("trb/orm database operations are not executable in the browser playground")
}

func (e *Evaluator) ormColumn(_ Value, _ string) (Value, error) {
	return Value{}, errors.New("trb/orm database values are not executable in the browser playground")
}
