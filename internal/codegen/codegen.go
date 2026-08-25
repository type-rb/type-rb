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

// ValidateProject performs backend-owned project validation without emitting
// target source. Backends without generation-time validation return directly;
// TypeScript receives the same normalized lowered IR as GenerateProject.
func ValidateProject(programs []*ir.Program) error {
	if len(programs) == 0 {
		return nil
	}
	switch programs[0].Mode {
	case "go", "ruby":
		return nil
	case "typescript":
		return typescript.ValidateProject(normalizeProjectDivergingControlFlow(programs))
	default:
		return fmt.Errorf("unsupported mode %q (want ruby, typescript, or go)", programs[0].Mode)
	}
}

// GenerateProject emits a set of already-lowered modules in project order.
// Keeping the project boundary here lets a backend perform whole-project
// analysis without leaking backend-specific concerns into parsing, checking,
// or the shared IR.
func GenerateProject(programs []*ir.Program) ([]Generated, error) {
	if len(programs) > 0 && programs[0].Mode == "typescript" {
		normalized := normalizeProjectDivergingControlFlow(programs)
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
		normalized := normalizeProjectDivergingControlFlow(programs)
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

func normalizeProjectDivergingControlFlow(programs []*ir.Program) []*ir.Program {
	prepared := make([]*ir.Program, len(programs))
	for index, program := range programs {
		prepared[index] = normalizeDivergingControlFlow(program)
	}
	return prepared
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

// GeneratedSourceDirectory returns the backend directory that owns generated
// files for a TypeRB source directory.
func GeneratedSourceDirectory(mode, directory string) string {
	if mode == "go" {
		return golang.GeneratedSourceDirectory(directory)
	}
	return directory
}
