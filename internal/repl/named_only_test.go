package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestEvaluateNamedOnlyEnumMethodAcrossModes(t *testing.T) {
	source := compiler.SourceUnit{
		Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main",
		Source: []byte(`enum State
	Ready

	def describe(prefix: String = "prefix", *, label: String, suffix: String = "!"): String
		return prefix + ":" + label + suffix
	end
end

state := State::Ready
state.describe(label: "selected")
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{source}, compiler.Options{
				Mode: mode, GoModule: "example.com/named-only-repl", RubyLoader: "require_relative", InteractiveModule: source.ModulePath,
			})
			if err != nil {
				t.Fatalf("%s rejected executable named-only enum source: %v", mode, err)
			}
			programs := make([]*ir.Program, 0, len(artifacts))
			var session *ir.Program
			for _, artifact := range artifacts {
				programs = append(programs, artifact.IR)
				if artifact.IR.ModulePath == source.ModulePath {
					session = artifact.IR
				}
			}
			if session == nil {
				t.Fatalf("%s compilation did not produce the interactive session", mode)
			}
			evaluator := NewEvaluator(&bytes.Buffer{}, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			if err := evaluator.LoadProject(programs, source.ModulePath); err != nil {
				t.Fatalf("%s could not load the named-only enum project: %v", mode, err)
			}
			evaluator.LoadDefinitions(session)
			result, err := evaluator.Evaluate(session.Statements, source.ModulePath)
			if err != nil {
				t.Fatalf("%s named-only enum evaluation failed: %v", mode, err)
			}
			if got, want := Inspect(result.Value), `"prefix:selected!"`; !result.Display || got != want {
				t.Fatalf("%s named-only enum evaluation=%s display=%t, want %s", mode, got, result.Display, want)
			}
		})
	}
}
