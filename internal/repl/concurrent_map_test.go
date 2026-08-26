package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestEvaluateCompiledConcurrentMapAcrossModes(t *testing.T) {
	source := compiler.SourceUnit{
		Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main",
		Source: []byte(`result := [1, 2].concurrent_map(limit: 2) do |outer|
	[1, 2].concurrent_map(limit: 2) do |inner|
		outer * 10 + inner
	end
end
result
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{source}, compiler.Options{
				Mode: mode, GoModule: "example.com/concurrent-repl", RubyLoader: "require_relative", InteractiveModule: source.ModulePath,
			})
			if err != nil {
				t.Fatalf("%s rejected executable concurrent_map: %v", mode, err)
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
				t.Fatalf("%s could not load the concurrent_map project: %v", mode, err)
			}
			evaluator.LoadDefinitions(session)
			result, err := evaluator.Evaluate(session.Statements, source.ModulePath)
			if err != nil {
				t.Fatalf("%s concurrent_map evaluation failed: %v", mode, err)
			}
			if got, want := Inspect(result.Value), `[[11, 12], [21, 22]]`; !result.Display || got != want {
				t.Fatalf("%s concurrent_map evaluation=%s display=%t, want %s", mode, got, result.Display, want)
			}
		})
	}
}
