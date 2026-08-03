package codegen

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/codegen/golang"
	"github.com/type-rb/type-rb/internal/codegen/ruby"
	"github.com/type-rb/type-rb/internal/codegen/typescript"
	"github.com/type-rb/type-rb/internal/ir"
)

func Generate(program *ir.Program) ([]byte, error) {
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
