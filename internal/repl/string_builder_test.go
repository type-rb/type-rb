package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
)

func TestInteractiveAliasedStringBuilderAcrossModes(t *testing.T) {
	source := []byte(`import trb/std/string_builder as Text
mut builder := Text.new()
builder.append("A")
saved := builder.to_s()
builder.append_codepoint(128512)
[saved, builder.to_s()]
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			unit := compiler.SourceUnit{Filename: "session.trb", ModulePath: "session", Package: "main", Source: source}
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{unit}, compiler.Options{Mode: mode, InteractiveModule: unit.ModulePath})
			if err != nil {
				t.Fatal(err)
			}
			program := artifacts[0].IR
			evaluator := NewEvaluator(&bytes.Buffer{}, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			evaluator.LoadDefinitions(program)
			result, err := evaluator.Evaluate(program.Statements, unit.ModulePath)
			if err != nil || !result.Display || Inspect(result.Value) != `["A", "A😀"]` {
				t.Fatalf("aliased builder evaluation: %#v, %v", result, err)
			}
		})
	}
}
