package codegen

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/codegen/golang"
	"github.com/type-rb/type-rb/internal/codegen/ruby"
	"github.com/type-rb/type-rb/internal/codegen/typescript"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/sourcemap"
)

type Generated struct {
	Output    []byte
	SourceMap sourcemap.Map
}

func Generate(program *ir.Program) (Generated, error) {
	if program.Mode == "go" || program.Mode == "typescript" {
		program = normalizeDivergingControlFlow(program)
	}
	switch program.Mode {
	case "ruby":
		generated := ruby.GenerateMapped(program)
		return Generated{Output: []byte(generated.Output), SourceMap: generated.Map}, nil
	case "typescript":
		generated, err := typescript.GenerateMapped(program)
		return Generated{Output: []byte(generated.Output), SourceMap: generated.Map}, err
	case "go":
		generated := golang.GenerateMapped(program)
		return Generated{Output: []byte(generated.Output), SourceMap: generated.Map}, nil
	default:
		return Generated{}, fmt.Errorf("unsupported mode %q (want ruby, typescript, or go)", program.Mode)
	}
}

// GenerateProject emits a set of already-lowered modules in project order.
// Keeping the project boundary here lets a backend perform whole-project
// analysis without leaking backend-specific concerns into parsing, checking,
// or the shared IR.
func GenerateProject(programs []*ir.Program) ([]Generated, error) {
	if len(programs) > 0 && programs[0].Mode == "typescript" {
		normalized := make([]*ir.Program, len(programs))
		for index, program := range programs {
			normalized[index] = normalizeDivergingControlFlow(program)
		}
		generated, err := typescript.GenerateProjectMapped(normalized)
		if err != nil {
			return nil, err
		}
		outputs := make([]Generated, len(generated))
		for index, output := range generated {
			outputs[index] = Generated{Output: []byte(output.Output), SourceMap: output.Map}
		}
		return outputs, nil
	}
	if len(programs) > 0 && programs[0].Mode == "go" {
		normalized := make([]*ir.Program, len(programs))
		for index, program := range programs {
			normalized[index] = normalizeDivergingControlFlow(program)
		}
		generated := golang.GenerateProjectMapped(normalized)
		outputs := make([]Generated, len(generated))
		for index, output := range generated {
			outputs[index] = Generated{Output: []byte(output.Output), SourceMap: output.Map}
		}
		return outputs, nil
	}
	if len(programs) > 0 && programs[0].Mode == "ruby" {
		generated := ruby.GenerateProjectMapped(programs)
		outputs := make([]Generated, len(generated))
		for index, output := range generated {
			outputs[index] = Generated{Output: []byte(output.Output), SourceMap: output.Map}
		}
		return outputs, nil
	}
	outputs := make([]Generated, len(programs))
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
