//go:build js && wasm

package repl

import (
	"errors"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// Browser playgrounds can type-check trb/orm projects, but database drivers
// are intentionally unavailable in the WebAssembly evaluator.
type ormQueryValue struct {
	model struct{ QueryType string }
}

type ormSubqueryValue struct{}

type ormWASMRuntimeProvider struct{}

func init() {
	registerRuntimeProvider(func() runtimeProvider { return &ormWASMRuntimeProvider{} })
}

func (*ormWASMRuntimeProvider) Name() string { return "trb/orm" }

func (*ormWASMRuntimeProvider) Handles(intrinsic string) bool {
	return strings.HasPrefix(intrinsic, "trb.orm.")
}

func (*ormWASMRuntimeProvider) Configure(_ []*ir.Program) error { return nil }

func (*ormWASMRuntimeProvider) Close() error { return nil }

func (*ormWASMRuntimeProvider) Call(_ *Evaluator, _ runtimeInvocation) (Value, error) {
	return Value{}, errors.New("trb/orm database operations are not executable in the browser playground")
}

func (*ormWASMRuntimeProvider) Block(_ *Evaluator, _ runtimeBlockInvocation) (Value, error) {
	return Value{}, errors.New("trb/orm database operations are not executable in the browser playground")
}
