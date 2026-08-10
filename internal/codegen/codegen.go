package codegen

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/codegen/golang"
	"github.com/type-rb/type-rb/internal/codegen/ruby"
	"github.com/type-rb/type-rb/internal/codegen/typescript"
	"github.com/type-rb/type-rb/internal/ir"
)

func Generate(program *ir.Program) ([]byte, error) {
	if program.Mode == "go" || program.Mode == "typescript" {
		program = normalizeDivergingControlFlow(program)
	}
	switch program.Mode {
	case "ruby":
		return []byte(ruby.Generate(program)), nil
	case "typescript":
		return []byte(typescript.Generate(program)), nil
	case "go":
		return []byte(golang.Generate(program)), nil
	default:
		return nil, fmt.Errorf("unsupported mode %q (want ruby, typescript, or go)", program.Mode)
	}
}

// GenerateProject emits a set of already-lowered modules in project order.
// Keeping the project boundary here lets a backend perform whole-project
// analysis without leaking backend-specific concerns into parsing, checking,
// or the shared IR.
func GenerateProject(programs []*ir.Program) ([][]byte, error) {
	outputs := make([][]byte, len(programs))
	for index, program := range programs {
		output, err := Generate(program)
		if err != nil {
			return nil, err
		}
		outputs[index] = output
	}
	return outputs, nil
}

func Extension(mode string) string {
	switch mode {
	case "ruby":
		return ".rb"
	case "typescript":
		return ".ts"
	case "go":
		return ".go"
	default:
		return ""
	}
}
